package workspace

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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

	// 启动时恢复现有工作空间
	m.recoverExistingWorkspaces()

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
	// 从 Issue 的 Repository 字段构建克隆 URL
	repo := issue.GetRepository()
	if repo == nil {
		log.Errorf("Repository not found in issue")
		return models.Workspace{}
	}

	repoURL := repo.GetCloneURL()
	if repoURL == "" {
		log.Errorf("Failed to get repository URL")
		return models.Workspace{}
	}

	// 生成分支名
	branch := fmt.Sprintf("codeagent/issue-%d-%d", issue.GetNumber(), time.Now().Unix())

	// 使用通用方法创建工作空间
	ws := m.createWorkspace(issue.GetNumber(), repoURL, branch, true)
	if ws == nil {
		return models.Workspace{}
	}

	// 设置 Issue 相关字段
	ws.Issue = issue

	return *ws
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

	// 生成分支名
	branch := fmt.Sprintf("codeagent/issue-%d-%d", event.Issue.GetNumber(), time.Now().Unix())

	// 使用通用方法创建工作空间
	ws := m.createWorkspace(event.Issue.GetNumber(), repoURL, branch, true)
	if ws == nil {
		return models.Workspace{}
	}

	// 设置 Issue 相关字段
	ws.Issue = event.Issue

	return *ws
}

// Getworkspace 获取或创建 PR 的工作空间
func (m *Manager) Getworkspace(pr *github.PullRequest) *models.Workspace {
	m.mutex.RLock()
	if ws, ok := m.workspaces[pr.GetNumber()]; ok {
		m.mutex.RUnlock()
		return ws
	}
	m.mutex.RUnlock()

	// 如果内存中没有找到，说明文件系统中也没有，需要创建新的工作空间
	log.Infof("No existing workspace found for PR #%d, creating new workspace", pr.GetNumber())
	return m.createNewWorkspaceForPR(pr)
}

// createNewWorkspaceForPR 为 PR 创建新的工作空间
func (m *Manager) createNewWorkspaceForPR(pr *github.PullRequest) *models.Workspace {
	log.Infof("Creating new workspace for PR #%d", pr.GetNumber())

	// 获取仓库 URL
	repoURL := pr.GetBase().GetRepo().GetCloneURL()
	if repoURL == "" {
		log.Errorf("Failed to get repository URL for PR #%d", pr.GetNumber())
		return nil
	}

	// 获取 PR 分支
	prBranch := pr.GetHead().GetRef()

	// 使用通用方法创建工作空间（不创建新分支，切换到现有分支）
	ws := m.createWorkspace(pr.GetNumber(), repoURL, prBranch, false)
	if ws == nil {
		return nil
	}

	// 设置 PR 相关字段
	ws.PullRequest = pr

	// 注册到内存映射
	m.mutex.Lock()
	m.workspaces[pr.GetNumber()] = ws
	m.mutex.Unlock()

	log.Infof("Successfully created new workspace for PR #%d: %s", pr.GetNumber(), ws.Path)
	return ws
}

// createWorkspace 通用的工作空间创建方法
func (m *Manager) createWorkspace(entityNumber int, repoURL, branch string, createNewBranch bool) *models.Workspace {
	// 生成工作空间 ID 和路径
	id := fmt.Sprintf("workspace-%d-%d", entityNumber, time.Now().UnixNano())
	session := fmt.Sprintf("session-%d", entityNumber)
	path := filepath.Join(m.baseDir, id)
	sessionPath := filepath.Join(m.baseDir, session)

	// 创建目录
	if err := os.MkdirAll(path, 0755); err != nil {
		log.Errorf("Failed to create workspace directory: %v", err)
		return nil
	}

	if err := os.MkdirAll(sessionPath, 0755); err != nil {
		log.Errorf("Failed to create session directory: %v", err)
		return nil
	}

	// 克隆仓库
	cmd := exec.Command("git", "clone", repoURL, path)
	cloneOutput, err := cmd.CombinedOutput()
	if err != nil {
		log.Errorf("Failed to clone repository: %v\nCommand output: %s", err, string(cloneOutput))
		os.RemoveAll(path)
		return nil
	}

	if createNewBranch {
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
			os.RemoveAll(path)
			return nil
		}
	} else {
		// 切换到现有分支
		cmd = exec.Command("git", "checkout", branch)
		cmd.Dir = path
		checkoutOutput, err := cmd.CombinedOutput()
		if err != nil {
			log.Errorf("Failed to checkout branch %s: %v\nCommand output: %s", branch, err, string(checkoutOutput))
			os.RemoveAll(path)
			return nil
		}
	}

	// 配置 Git 安全目录
	cmd = exec.Command("git", "config", "--local", "--add", "safe.directory", path)
	cmd.Dir = path
	configOutput, err := cmd.CombinedOutput()
	if err != nil {
		log.Warnf("Failed to configure safe directory: %v\nCommand output: %s", err, string(configOutput))
	}

	// 创建工作空间对象
	ws := &models.Workspace{
		ID:          id,
		Path:        path,
		SessionPath: sessionPath,
		Repository:  repoURL,
		Branch:      branch,
		CreatedAt:   time.Now(),
	}

	return ws
}

// recoverExistingWorkspaces 启动时恢复所有现有的工作空间
func (m *Manager) recoverExistingWorkspaces() {
	log.Infof("Starting to recover existing workspaces from %s", m.baseDir)

	// 扫描基础目录，查找所有工作空间
	entries, err := os.ReadDir(m.baseDir)
	if err != nil {
		log.Errorf("Failed to read base directory: %v", err)
		return
	}

	recoveredCount := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		workspacePath := filepath.Join(m.baseDir, entry.Name())

		// 检查是否是 Git 仓库
		gitDir := filepath.Join(workspacePath, ".git")
		if _, err := os.Stat(gitDir); os.IsNotExist(err) {
			continue
		}

		// 获取当前分支
		cmd := exec.Command("git", "branch", "--show-current")
		cmd.Dir = workspacePath
		branchOutput, err := cmd.Output()
		if err != nil {
			log.Warnf("Failed to get current branch for %s: %v", workspacePath, err)
			continue
		}

		currentBranch := strings.TrimSpace(string(branchOutput))

		// 尝试从分支名提取 PR 号
		prNumber := m.extractPRNumberFromBranch(currentBranch)
		if prNumber == 0 {
			continue
		}

		// 获取远程仓库 URL
		cmd = exec.Command("git", "remote", "get-url", "origin")
		cmd.Dir = workspacePath
		remoteOutput, err := cmd.Output()
		if err != nil {
			log.Warnf("Failed to get remote URL for %s: %v", workspacePath, err)
			continue
		}

		remoteURL := strings.TrimSpace(string(remoteOutput))

		// 创建 session 路径
		sessionPath := filepath.Join(m.baseDir, fmt.Sprintf("session-pr-%d", prNumber))
		if err := os.MkdirAll(sessionPath, 0755); err != nil {
			log.Warnf("Failed to create session directory: %v", err)
		}

		// 创建临时 PR 对象用于恢复
		tempPR := &github.PullRequest{
			Number: github.Int(prNumber),
		}

		// 恢复工作空间
		ws := &models.Workspace{
			ID:          entry.Name(),
			Path:        workspacePath,
			SessionPath: sessionPath,
			Repository:  remoteURL,
			Branch:      currentBranch,
			PullRequest: tempPR,
			CreatedAt:   time.Now(),
		}

		// 注册到内存映射
		m.mutex.Lock()
		m.workspaces[prNumber] = ws
		m.mutex.Unlock()

		recoveredCount++
		log.Infof("Recovered workspace for PR #%d: %s", prNumber, workspacePath)
	}

	log.Infof("Recovery completed. Recovered %d workspaces", recoveredCount)
}

// extractPRNumberFromBranch 从分支名中提取 PR 号
func (m *Manager) extractPRNumberFromBranch(branchName string) int {
	// 匹配 codeagent/issue-{number}-{timestamp} 格式
	if strings.HasPrefix(branchName, "codeagent/issue-") {
		parts := strings.Split(branchName, "-")
		if len(parts) >= 3 {
			if number, err := strconv.Atoi(parts[2]); err == nil {
				return number
			}
		}
	}

	// 匹配 codeagent/issue-{number}-{timestamp} 格式
	if strings.HasPrefix(branchName, "codeagent/issue-") {
		parts := strings.Split(branchName, "-")
		if len(parts) >= 3 {
			if number, err := strconv.Atoi(parts[2]); err == nil {
				return number
			}
		}
	}

	// 匹配包含 pr-{number} 的格式
	if strings.Contains(branchName, "pr-") {
		parts := strings.Split(branchName, "pr-")
		if len(parts) >= 2 {
			numberPart := strings.Split(parts[1], "-")[0]
			if number, err := strconv.Atoi(numberPart); err == nil {
				return number
			}
		}
	}

	return 0
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
