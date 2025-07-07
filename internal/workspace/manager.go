package workspace

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/qbox/codeagent/internal/config"
	"github.com/qbox/codeagent/pkg/models"

	"github.com/google/go-github/v58/github"
	"github.com/qiniu/x/log"
)

type Manager struct {
	baseDir    string
	workspaces map[int]*models.Workspace
	mutex      sync.RWMutex
	config     *config.Config
}

func NewManager(cfg *config.Config) *Manager {
	m := &Manager{
		baseDir:    cfg.Workspace.BaseDir,
		workspaces: make(map[int]*models.Workspace),
		config:     cfg,
	}

	// 启动定期清理协程
	go m.startCleanupRoutine()

	return m
}

// startCleanupRoutine 启动定期清理协程
func (m *Manager) startCleanupRoutine() {
	ticker := time.NewTicker(1 * time.Hour) // 每小时检查一次
	defer ticker.Stop()

	for range ticker.C {
		m.cleanupExpiredWorkspaces()
	}
}

// cleanupExpiredWorkspaces 清理过期的工作空间
func (m *Manager) cleanupExpiredWorkspaces() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	now := time.Now()
	expiredWorkspaces := []int{}

	for id, ws := range m.workspaces {
		if now.Sub(ws.CreatedAt) > m.config.Workspace.CleanupAfter {
			expiredWorkspaces = append(expiredWorkspaces, id)
		}
	}

	for _, id := range expiredWorkspaces {
		ws := m.workspaces[id]
		m.cleanupWorkspace(ws)
		delete(m.workspaces, id)
		log.Infof("Cleaned up expired workspace: %d", id)
	}
}

// cleanupWorkspace 清理单个工作空间
func (m *Manager) cleanupWorkspace(ws *models.Workspace) {
	if ws == nil || ws.Path == "" {
		return
	}

	// 删除工作空间目录
	if err := os.RemoveAll(ws.Path); err != nil {
		log.Errorf("Failed to remove workspace directory %s: %v", ws.Path, err)
	}
}

// Prepare 准备工作空间（从 Issue）
func (m *Manager) Prepare(issue *github.Issue) models.Workspace {
	id := fmt.Sprintf("issue-%d-%d", issue.GetNumber(), time.Now().UnixNano())
	path := filepath.Join(m.baseDir, id)

	if err := os.MkdirAll(path, 0o755); err != nil {
		log.Errorf("Failed to create workspace directory: %v", err)
		return models.Workspace{}
	}

	// 从 Issue 的 Repository 字段构建克隆 URL
	repo := issue.GetRepository()
	if repo == nil {
		log.Errorf("Repository not found in issue")
		return models.Workspace{}
	}

	// 构建 SSH 或 HTTPS URL
	repoURL := repo.GetCloneURL() // 这会返回 HTTPS URL
	if repoURL == "" {
		log.Errorf("Failed to get repository URL")
		return models.Workspace{}
	}

	branch := fmt.Sprintf("xgo-agent/issue-%d-%d", issue.GetNumber(), time.Now().Unix())

	// 克隆仓库
	cmd := exec.Command("git", "clone", repoURL, path)
	cloneOutput, err := cmd.CombinedOutput()
	if err != nil {
		log.Errorf("Failed to clone repository: %v\nCommand output: %s", err, string(cloneOutput))
		os.RemoveAll(path) // 清理失败的目录
		return models.Workspace{}
	}

	// 检查远程分支是否已存在
	cmd = exec.Command("git", "ls-remote", "--heads", "origin", branch)
	cmd.Dir = path
	lsOutput, err := cmd.CombinedOutput()
	if err != nil {
		log.Warnf("Failed to check remote branch existence: %v", err)
	} else if strings.TrimSpace(string(lsOutput)) != "" {
		log.Infof("Remote branch %s already exists, will handle during push", branch)
	}

	// 创建新分支
	cmd = exec.Command("git", "checkout", "-b", branch)
	cmd.Dir = path
	checkoutOutput, err := cmd.CombinedOutput()
	if err != nil {
		log.Errorf("Failed to create branch: %v\nCommand output: %s", err, string(checkoutOutput))
		os.RemoveAll(path) // 清理失败的目录
		return models.Workspace{}
	}

	ws := models.Workspace{
		ID:         id,
		Path:       path,
		Repository: repoURL,
		Branch:     branch,
		Issue:      issue,
		CreatedAt:  time.Now(),
	}

	return ws
}

// Cleanup 清理工作空间
func (m *Manager) Cleanup(workspace models.Workspace) {
	if workspace.PullRequest == nil {
		return
	}

	// 从注册表中移除
	m.mutex.Lock()
	delete(m.workspaces, workspace.PullRequest.GetNumber())
	m.mutex.Unlock()

	// 清理文件系统
	m.cleanupWorkspace(&workspace)
}

// PrepareFromEvent 从完整的 IssueCommentEvent 准备工作空间
func (m *Manager) PrepareFromEvent(event *github.IssueCommentEvent) models.Workspace {
	id := fmt.Sprintf("issue-%d-%d", event.Issue.GetNumber(), time.Now().UnixNano())
	session := fmt.Sprintf("session-%d-%d", event.Issue.GetNumber(), time.Now().UnixNano())
	path := filepath.Join(m.baseDir, id)
	sessionPath := filepath.Join(m.baseDir, session)

	if err := os.MkdirAll(path, 0755); err != nil {
		log.Errorf("Failed to create workspace directory: %v", err)
		return models.Workspace{}
	}

	if err := os.MkdirAll(sessionPath, 0755); err != nil {
		log.Errorf("Failed to create session directory: %v", err)
		return models.Workspace{}
	}

	// 从事件中获取仓库信息
	repo := event.GetRepo()
	if repo == nil {
		log.Errorf("Repository not found in event")
		return models.Workspace{}
	}

	// 构建克隆 URL
	repoURL := repo.GetCloneURL()
	if repoURL == "" {
		log.Errorf("Failed to get repository")
		return models.Workspace{}
	}

	branch := fmt.Sprintf("xgo-agent/issue-%d-%d", event.Issue.GetNumber(), time.Now().Unix())

	// 克隆仓库
	cmd := exec.Command("git", "clone", repoURL, path)
	cloneOutput, err := cmd.CombinedOutput()
	if err != nil {
		log.Errorf("Failed to clone repository: %v\nCommand output: %s", err, string(cloneOutput))
		os.RemoveAll(path) // 清理失败的目录
		return models.Workspace{}
	}

	// 检查远程分支是否已存在
	cmd = exec.Command("git", "ls-remote", "--heads", "origin", branch)
	cmd.Dir = path
	lsOutput, err := cmd.CombinedOutput()
	if err != nil {
		log.Warnf("Failed to check remote branch existence: %v", err)
	} else if strings.TrimSpace(string(lsOutput)) != "" {
		log.Infof("Remote branch %s already exists, will handle during push", branch)
	}

	// 创建新分支
	cmd = exec.Command("git", "checkout", "-b", branch)
	cmd.Dir = path
	checkoutOutput, err := cmd.CombinedOutput()
	if err != nil {
		log.Errorf("Failed to create branch: %v\nCommand output: %s", err, string(checkoutOutput))
		os.RemoveAll(path) // 清理失败的目录
		return models.Workspace{}
	}

	ws := models.Workspace{
		ID:          id,
		Path:        path,
		SessionPath: sessionPath,
		Repository:  repoURL,
		Branch:      branch,
		Issue:       event.Issue,
		CreatedAt:   time.Now(),
	}

	return ws
}

// PrepareFromPR 从 PullRequest 准备工作空间
func (m *Manager) Getworkspace(pr *github.PullRequest) *models.Workspace {
	m.mutex.RLock()
	if ws, ok := m.workspaces[pr.GetNumber()]; ok {
		m.mutex.RUnlock()
		return ws
	}
	m.mutex.RUnlock()
	return nil
}

// RegisterWorkspace 注册工作空间
func (m *Manager) RegisterWorkspace(ws *models.Workspace, pr *github.PullRequest) {
	if ws == nil {
		log.Errorf("Invalid workspace to register")
		return
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	ws.PullRequest = pr
	if _, exists := m.workspaces[ws.PullRequest.GetNumber()]; exists {
		log.Warnf("Workspace %s already registered", ws.ID)
		return
	}

	m.workspaces[ws.PullRequest.GetNumber()] = ws
	log.Infof("Registered workspace: %s, %s", ws.ID, ws.Path)
}

// GetWorkspaceCount 获取当前工作空间数量
func (m *Manager) GetWorkspaceCount() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return len(m.workspaces)
}
