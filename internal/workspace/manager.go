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

const (
	// BranchPrefix 分支名前缀，用于标识由 codeagent 创建的分支
	BranchPrefix = "codeagent"
)

// MappingManager 映射文件管理器
type MappingManager struct {
	mappingDir string
	mutex      sync.RWMutex
}

// NewMappingManager 创建映射管理器
func NewMappingManager(repoPath string) *MappingManager {
	mappingDir := filepath.Join(repoPath, ".git", "info", "exclude")
	return &MappingManager{
		mappingDir: mappingDir,
	}
}

// CreateMapping 创建 Issue 到 PR 的映射
func (m *MappingManager) CreateMapping(issueDir string, prNumber int) error {
	mappingFile := filepath.Join(m.mappingDir, issueDir+".txt")

	m.mutex.Lock()
	defer m.mutex.Unlock()

	// 确保目录存在
	if err := os.MkdirAll(m.mappingDir, 0755); err != nil {
		return err
	}

	// 写入 PR 号
	return os.WriteFile(mappingFile, []byte(strconv.Itoa(prNumber)), 0644)
}

// GetPRNumber 根据 Issue 目录获取 PR 号
func (m *MappingManager) GetPRNumber(issueDir string) (int, error) {
	mappingFile := filepath.Join(m.mappingDir, issueDir+".txt")

	m.mutex.RLock()
	defer m.mutex.RUnlock()

	data, err := os.ReadFile(mappingFile)
	if err != nil {
		return 0, err
	}

	return strconv.Atoi(strings.TrimSpace(string(data)))
}

// ListMappings 列出所有映射
func (m *MappingManager) ListMappings() (map[string]int, error) {
	mappings := make(map[string]int)

	m.mutex.RLock()
	defer m.mutex.RUnlock()

	entries, err := os.ReadDir(m.mappingDir)
	if err != nil {
		return mappings, nil // 目录不存在时返回空映射
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".txt") {
			issueDir := strings.TrimSuffix(entry.Name(), ".txt")
			prNumber, err := m.GetPRNumber(issueDir)
			if err == nil && prNumber > 0 {
				mappings[issueDir] = prNumber
			}
		}
	}

	return mappings, nil
}

// RemoveMapping 删除映射
func (m *MappingManager) RemoveMapping(issueDir string) error {
	mappingFile := filepath.Join(m.mappingDir, issueDir+".txt")

	m.mutex.Lock()
	defer m.mutex.Unlock()

	return os.Remove(mappingFile)
}

type Manager struct {
	baseDir      string
	workspaces   map[int]*models.Workspace
	issueMapping map[string]int // Issue目录 -> PR号
	repoManagers map[string]*RepoManager
	mutex        sync.RWMutex
	config       *config.Config
}

func NewManager(cfg *config.Config) *Manager {
	m := &Manager{
		baseDir:      cfg.Workspace.BaseDir,
		workspaces:   make(map[int]*models.Workspace),
		issueMapping: make(map[string]int),
		repoManagers: make(map[string]*RepoManager),
		config:       cfg,
	}

	// 启动定期清理协程
	go m.startCleanupRoutine()

	// 启动时恢复现有工作空间
	if cfg.Workspace.UseWorktree {
		m.recoverExistingWorkspacesWithMapping()
	} else {
		m.recoverExistingWorkspaces()
	}

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

	if m.config.Workspace.UseWorktree {
		m.cleanupWorkspaceWithWorktree(ws)
	} else {
		m.cleanupWorkspaceWithClone(ws)
	}
}

// cleanupWorkspaceWithWorktree 清理 worktree 工作空间
func (m *Manager) cleanupWorkspaceWithWorktree(ws *models.Workspace) {
	// 从工作空间路径提取 PR 号
	prNumber := m.extractPRNumberFromWorkspaceDir(filepath.Base(ws.Path))
	if prNumber == 0 {
		log.Warnf("Could not extract PR number from workspace path: %s", ws.Path)
		return
	}

	// 获取仓库管理器
	repoName := m.extractRepoName(ws.Repository)
	m.mutex.RLock()
	repoManager, exists := m.repoManagers[repoName]
	m.mutex.RUnlock()

	if !exists {
		log.Warnf("Repo manager not found for %s", repoName)
		return
	}

	// 移除 worktree
	if err := repoManager.RemoveWorktree(prNumber); err != nil {
		log.Errorf("Failed to remove worktree for PR #%d: %v", prNumber, err)
	}

	// 删除 session 目录
	if err := os.RemoveAll(ws.SessionPath); err != nil {
		log.Errorf("Failed to remove session directory %s: %v", ws.SessionPath, err)
	}
}

// cleanupWorkspaceWithClone 清理克隆工作空间（原有逻辑）
func (m *Manager) cleanupWorkspaceWithClone(ws *models.Workspace) {
	// 删除工作空间目录
	if err := os.RemoveAll(ws.Path); err != nil {
		log.Errorf("Failed to remove workspace directory %s: %v", ws.Path, err)
	}

	// 删除 session 目录
	if err := os.RemoveAll(ws.SessionPath); err != nil {
		log.Errorf("Failed to remove session directory %s: %v", ws.SessionPath, err)
	}
}

// Prepare 准备工作空间（从 Issue）
func (m *Manager) Prepare(issue *github.Issue) models.Workspace {
	log.Infof("Preparing workspace for Issue #%d", issue.GetNumber())

	// 从 Issue 的 HTML URL 中提取仓库信息
	// Issue URL 格式: https://github.com/owner/repo/issues/123
	repoURL := m.extractRepoURLFromIssueURL(issue.GetHTMLURL())
	if repoURL == "" {
		log.Errorf("Failed to extract repository URL from Issue URL: %s", issue.GetHTMLURL())
		return models.Workspace{
			Issue: issue,
		}
	}

	// 生成分支名
	branchName := fmt.Sprintf("%s/issue-%d-%d", BranchPrefix, issue.GetNumber(), time.Now().Unix())

	// 创建临时工作空间（使用 Issue 号作为临时 ID）
	ws := m.CreateWorkspace(issue.GetNumber(), repoURL, branchName, true)
	if ws == nil {
		log.Errorf("Failed to create workspace for Issue #%d", issue.GetNumber())
		return models.Workspace{
			Issue: issue,
		}
	}

	// 设置 Issue 相关字段
	ws.Issue = issue

	return *ws
}

// PrepareFromPR 从 PR 准备工作空间
func (m *Manager) PrepareFromPR(pr *github.PullRequest) models.Workspace {
	log.Infof("Preparing workspace for PR #%d", pr.GetNumber())

	// 获取仓库 URL
	repoURL := pr.GetBase().GetRepo().GetCloneURL()
	if repoURL == "" {
		log.Errorf("Failed to get repository URL for PR #%d", pr.GetNumber())
		return models.Workspace{}
	}

	// 获取 PR 分支
	prBranch := pr.GetHead().GetRef()

	// 使用 PR number 创建工作空间
	ws := m.CreateWorkspace(pr.GetNumber(), repoURL, prBranch, false)
	if ws == nil {
		return models.Workspace{}
	}

	// 设置 PR 相关字段
	ws.PullRequest = pr

	return *ws
}

// ConvertToFormalWorkspace 将临时工作空间转换为正式工作空间（保留所有工作）
func (m *Manager) ConvertToFormalWorkspace(tempWs *models.Workspace, pr *github.PullRequest) models.Workspace {
	log.Infof("Converting temporary workspace to formal workspace for PR #%d", pr.GetNumber())

	// 创建新的工作空间对象，保留临时工作空间的所有信息
	formalWs := *tempWs

	// 更新工作空间信息
	formalWs.PullRequest = pr
	formalWs.Issue = nil // 清除 Issue 信息，因为现在有 PR 了

	// 生成新的工作空间 ID（从 issue 格式转换为 pr 格式）
	timestamp := time.Now().Unix()
	formalWs.ID = fmt.Sprintf("pr-%d-%d", pr.GetNumber(), timestamp)

	// 重命名物理目录：从 issue-{issueNumber}-{timestamp} 转换为 pr-{prNumber}-{timestamp}
	if err := m.renameWorkspaceDirectory(tempWs.Path, formalWs.ID); err != nil {
		log.Errorf("Failed to rename workspace directory: %v", err)
		// 即使重命名失败，也继续使用原路径
	} else {
		// 更新路径为新的目录路径
		formalWs.Path = filepath.Join(filepath.Dir(tempWs.Path), formalWs.ID)
		log.Infof("Renamed workspace directory from %s to %s", tempWs.Path, formalWs.Path)
	}

	log.Infof("Converted workspace: ID=%s, Path=%s, Branch=%s", formalWs.ID, formalWs.Path, formalWs.Branch)
	return formalWs
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
	// Issue 事件本身不创建工作空间，需要先创建 PR
	log.Infof("Issue comment event for Issue #%d, but workspace should be created after PR is created", event.Issue.GetNumber())

	// 返回空工作空间，表示需要先创建 PR
	return models.Workspace{
		Issue: event.Issue,
	}
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
	ws := m.CreateWorkspace(pr.GetNumber(), repoURL, prBranch, false)
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

// CreateWorkspace 通用的工作空间创建方法
func (m *Manager) CreateWorkspace(entityNumber int, repoURL, branch string, createNewBranch bool) *models.Workspace {
	if m.config.Workspace.UseWorktree {
		return m.createWorkspaceWithWorktree(entityNumber, repoURL, branch, createNewBranch)
	} else {
		return m.createWorkspaceWithClone(entityNumber, repoURL, branch, createNewBranch)
	}
}

// createWorkspaceWithWorktree 使用 Git worktree 创建工作空间
func (m *Manager) createWorkspaceWithWorktree(entityNumber int, repoURL, branch string, createNewBranch bool) *models.Workspace {
	log.Infof("Creating workspace with worktree for entity #%d, branch: %s, createNewBranch: %v", entityNumber, branch, createNewBranch)

	// 获取或创建仓库管理器
	repoManager := m.getOrCreateRepoManager(repoURL)
	log.Infof("Using repo manager for: %s", repoManager.GetRepoPath())

	// 检查是否已有该 PR 的 worktree
	if worktree := repoManager.GetWorktree(entityNumber); worktree != nil {
		log.Infof("Found existing worktree for entity #%d: %s", entityNumber, worktree.Path)
		return m.createWorkspaceFromWorktree(worktree, repoURL)
	}

	log.Infof("No existing worktree found, creating new one for entity #%d", entityNumber)
	// 创建新的 worktree
	worktree, err := repoManager.CreateWorktree(entityNumber, branch, createNewBranch)
	if err != nil {
		log.Errorf("Failed to create worktree for PR #%d: %v", entityNumber, err)
		return nil
	}

	log.Infof("Successfully created worktree: %s", worktree.Path)

	// 创建 session 目录
	sessionPath := filepath.Join(repoManager.GetRepoPath(), fmt.Sprintf("session-%d", entityNumber))
	if err := os.MkdirAll(sessionPath, 0755); err != nil {
		log.Errorf("Failed to create session directory: %v", err)
		return nil
	}

	// 创建工作空间对象
	timestamp := time.Now().Unix()
	id := fmt.Sprintf("pr-%d-%d", entityNumber, timestamp)

	ws := &models.Workspace{
		ID:          id,
		Path:        worktree.Path,
		SessionPath: sessionPath,
		Repository:  repoURL,
		Branch:      worktree.Branch,
		CreatedAt:   worktree.CreatedAt,
	}

	log.Infof("Created workspace with worktree: ID=%s, Path=%s, Branch=%s", ws.ID, ws.Path, ws.Branch)
	return ws
}

// createWorkspaceWithClone 使用完整克隆创建工作空间（原有逻辑）
func (m *Manager) createWorkspaceWithClone(entityNumber int, repoURL, branch string, createNewBranch bool) *models.Workspace {
	// 从仓库 URL 提取仓库名
	repoName := m.extractRepoName(repoURL)

	// 生成工作空间 ID 和路径
	timestamp := time.Now().Unix()

	// 根据是否创建新分支来判断是 Issue 还是 PR
	var id, workspaceDir string
	if createNewBranch {
		// Issue 阶段：创建新分支，使用 issue 格式
		id = fmt.Sprintf("issue-%d-%d", entityNumber, timestamp)
		workspaceDir = fmt.Sprintf("issue-%d-%d", entityNumber, timestamp)
	} else {
		// PR 阶段：使用现有分支，使用 pr 格式
		id = fmt.Sprintf("pr-%d-%d", entityNumber, timestamp)
		workspaceDir = fmt.Sprintf("pr-%d-%d", entityNumber, timestamp)
	}

	// 创建基于仓库名的目录结构
	repoPath := filepath.Join(m.baseDir, repoName)
	path := filepath.Join(repoPath, workspaceDir)
	sessionPath := filepath.Join(repoPath, fmt.Sprintf("session-%d", entityNumber))

	// 创建目录
	if err := os.MkdirAll(path, 0755); err != nil {
		log.Errorf("Failed to create workspace directory: %v", err)
		return nil
	}

	if err := os.MkdirAll(sessionPath, 0755); err != nil {
		log.Errorf("Failed to create session directory: %v", err)
		return nil
	}
	log.Infof("git clone repoURL: %s, path: %s", repoURL, path)
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

// createWorkspaceFromWorktree 从现有 worktree 创建工作空间
func (m *Manager) createWorkspaceFromWorktree(worktree *WorktreeInfo, repoURL string) *models.Workspace {
	// 创建 session 目录
	sessionPath := filepath.Join(filepath.Dir(worktree.Path), fmt.Sprintf("session-%d", worktree.PRNumber))
	if err := os.MkdirAll(sessionPath, 0755); err != nil {
		log.Errorf("Failed to create session directory: %v", err)
		return nil
	}

	// 创建工作空间对象
	timestamp := time.Now().Unix()
	id := fmt.Sprintf("pr-%d-%d", worktree.PRNumber, timestamp)

	ws := &models.Workspace{
		ID:          id,
		Path:        worktree.Path,
		SessionPath: sessionPath,
		Repository:  repoURL,
		Branch:      worktree.Branch,
		CreatedAt:   worktree.CreatedAt,
	}

	return ws
}

// recoverExistingWorkspaces 启动时恢复所有现有的工作空间
func (m *Manager) recoverExistingWorkspaces() {
	log.Infof("Starting to recover existing workspaces from %s", m.baseDir)

	// 扫描基础目录，查找所有仓库目录
	entries, err := os.ReadDir(m.baseDir)
	if err != nil {
		log.Errorf("Failed to read base directory: %v", err)
		return
	}

	recoveredCount := 0
	for _, repoEntry := range entries {
		if !repoEntry.IsDir() {
			continue
		}

		repoPath := filepath.Join(m.baseDir, repoEntry.Name())

		// 扫描仓库目录下的工作空间
		workspaceEntries, err := os.ReadDir(repoPath)
		if err != nil {
			log.Warnf("Failed to read repo directory %s: %v", repoPath, err)
			continue
		}

		for _, workspaceEntry := range workspaceEntries {
			if !workspaceEntry.IsDir() {
				continue
			}

			workspacePath := filepath.Join(repoPath, workspaceEntry.Name())

			// 检查是否是 Git 仓库
			gitDir := filepath.Join(workspacePath, ".git")
			if _, err := os.Stat(gitDir); os.IsNotExist(err) {
				continue
			}

			// 从工作空间目录名提取 PR 号
			prNumber := m.extractPRNumberFromWorkspaceDir(workspaceEntry.Name())
			if prNumber == 0 {
				log.Warnf("Could not extract PR number from workspace directory: %s", workspaceEntry.Name())
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
			sessionPath := filepath.Join(repoPath, fmt.Sprintf("session-%d", prNumber))
			if err := os.MkdirAll(sessionPath, 0755); err != nil {
				log.Warnf("Failed to create session directory: %v", err)
			}

			// 创建临时 PR 对象用于恢复
			tempPR := &github.PullRequest{
				Number: github.Int(prNumber),
			}

			// 恢复工作空间
			ws := &models.Workspace{
				ID:          workspaceEntry.Name(),
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
	}

	log.Infof("Recovery completed. Recovered %d workspaces", recoveredCount)
}

// recoverExistingWorkspacesWithMapping 使用映射文件恢复现有工作空间
func (m *Manager) recoverExistingWorkspacesWithMapping() {
	log.Infof("Starting to recover existing workspaces with mapping from %s", m.baseDir)

	entries, err := os.ReadDir(m.baseDir)
	if err != nil {
		log.Errorf("Failed to read base directory: %v", err)
		return
	}

	recoveredCount := 0
	for _, repoEntry := range entries {
		if !repoEntry.IsDir() {
			continue
		}

		repoPath := filepath.Join(m.baseDir, repoEntry.Name())

		// 检查是否有 .git 目录
		gitDir := filepath.Join(repoPath, ".git")
		if _, err := os.Stat(gitDir); os.IsNotExist(err) {
			continue
		}

		// 创建仓库管理器
		remoteURL, err := m.getRemoteURL(repoPath)
		if err != nil {
			log.Warnf("Failed to get remote URL for %s: %v", repoPath, err)
			continue
		}

		repoManager := NewRepoManager(repoPath, remoteURL)
		m.repoManagers[repoEntry.Name()] = repoManager

		// 读取映射文件
		mappingManager := repoManager.GetMappingManager()
		mappings, err := mappingManager.ListMappings()
		if err != nil {
			log.Warnf("Failed to list mappings for %s: %v", repoPath, err)
			continue
		}

		// 恢复每个工作空间
		for issueDir, prNumber := range mappings {
			if err := m.recoverWorkspace(repoPath, issueDir, prNumber, remoteURL); err != nil {
				log.Errorf("Failed to recover workspace %s: %v", issueDir, err)
				continue
			}
			recoveredCount++
		}
	}

	log.Infof("Mapping recovery completed. Recovered %d workspaces", recoveredCount)
}

// recoverWorkspace 恢复单个工作空间
func (m *Manager) recoverWorkspace(repoPath, issueDir string, prNumber int, remoteURL string) error {
	// 1. 检查 worktree 是否存在
	worktreePath := filepath.Join(repoPath, issueDir)
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		return fmt.Errorf("worktree not found: %s", worktreePath)
	}

	// 2. 创建 session 目录
	sessionPath := filepath.Join(repoPath, fmt.Sprintf("session-%d", prNumber))
	if err := os.MkdirAll(sessionPath, 0755); err != nil {
		return err
	}

	// 3. 获取当前分支
	branch, err := m.getCurrentBranch(worktreePath)
	if err != nil {
		return err
	}

	// 4. 恢复工作空间对象
	ws := &models.Workspace{
		ID:          issueDir,
		Path:        worktreePath,
		SessionPath: sessionPath,
		Repository:  remoteURL,
		Branch:      branch,
		PullRequest: &github.PullRequest{Number: github.Int(prNumber)},
		CreatedAt:   time.Now(),
	}

	// 5. 注册到内存映射
	m.mutex.Lock()
	m.workspaces[prNumber] = ws
	m.issueMapping[issueDir] = prNumber
	m.mutex.Unlock()

	log.Infof("Recovered workspace: Issue=%s, PR=%d, Path=%s", issueDir, prNumber, worktreePath)
	return nil
}

// getRemoteURL 获取远程仓库 URL
func (m *Manager) getRemoteURL(repoPath string) (string, error) {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// getCurrentBranch 获取当前分支
func (m *Manager) getCurrentBranch(worktreePath string) (string, error) {
	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = worktreePath
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// getOrCreateRepoManager 获取或创建仓库管理器
func (m *Manager) getOrCreateRepoManager(repoURL string) *RepoManager {
	repoName := m.extractRepoName(repoURL)

	m.mutex.Lock()
	defer m.mutex.Unlock()

	// 检查是否已存在
	if repoManager, exists := m.repoManagers[repoName]; exists {
		return repoManager
	}

	// 创建新的仓库管理器
	repoPath := filepath.Join(m.baseDir, repoName)
	repoManager := NewRepoManager(repoPath, repoURL)
	m.repoManagers[repoName] = repoManager

	return repoManager
}

// extractPRNumberFromWorkspaceDir 从工作空间目录名中提取 PR 号或 Issue 号
func (m *Manager) extractPRNumberFromWorkspaceDir(workspaceDir string) int {
	// 工作空间目录格式:
	// - pr-{number}-{timestamp} 例如: pr-91-1752121132
	// - issue-{number}-{timestamp} 例如: issue-11-1752121132
	if strings.HasPrefix(workspaceDir, "pr-") || strings.HasPrefix(workspaceDir, "issue-") {
		parts := strings.Split(workspaceDir, "-")
		if len(parts) >= 2 {
			if number, err := strconv.Atoi(parts[1]); err == nil {
				return number
			}
		}
	}
	return 0
}

// extractRepoName 从仓库 URL 中提取仓库名
func (m *Manager) extractRepoName(repoURL string) string {
	// 移除 .git 后缀
	repoURL = strings.TrimSuffix(repoURL, ".git")

	// 分割 URL 获取最后一部分作为仓库名
	parts := strings.Split(repoURL, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}

	return "unknown"
}

// extractRepoURLFromIssueURL 从 Issue URL 中提取仓库 URL
func (m *Manager) extractRepoURLFromIssueURL(issueURL string) string {
	// Issue URL 格式: https://github.com/owner/repo/issues/123
	if strings.Contains(issueURL, "github.com") {
		parts := strings.Split(issueURL, "/")
		if len(parts) >= 4 {
			owner := parts[len(parts)-4]
			repo := parts[len(parts)-3]
			return fmt.Sprintf("https://github.com/%s/%s.git", owner, repo)
		}
	}
	return ""
}

// renameWorkspaceDirectory 重命名工作空间目录
func (m *Manager) renameWorkspaceDirectory(oldPath, newDirName string) error {
	// 获取父目录
	parentDir := filepath.Dir(oldPath)
	newPath := filepath.Join(parentDir, newDirName)

	log.Infof("Renaming workspace directory: %s -> %s", oldPath, newPath)

	// 检查新路径是否已存在
	if _, err := os.Stat(newPath); err == nil {
		return fmt.Errorf("target directory already exists: %s", newPath)
	}

	// 重命名目录
	if err := os.Rename(oldPath, newPath); err != nil {
		return fmt.Errorf("failed to rename directory: %w", err)
	}

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

// GetRepoManagerCount 获取仓库管理器数量
func (m *Manager) GetRepoManagerCount() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return len(m.repoManagers)
}

// GetWorktreeCount 获取总 worktree 数量
func (m *Manager) GetWorktreeCount() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	total := 0
	for _, repoManager := range m.repoManagers {
		total += repoManager.GetWorktreeCount()
	}
	return total
}

// CreateWorkspaceFromIssue 从 Issue 创建工作空间
func (m *Manager) CreateWorkspaceFromIssue(issue *github.Issue) *models.Workspace {
	log.Infof("Creating workspace from Issue #%d", issue.GetNumber())

	// 从 Issue 的 HTML URL 中提取仓库信息
	repoURL := m.extractRepoURLFromIssueURL(issue.GetHTMLURL())
	if repoURL == "" {
		log.Errorf("Failed to extract repository URL from Issue URL: %s", issue.GetHTMLURL())
		return nil
	}

	// 生成分支名
	timestamp := time.Now().Unix()
	branchName := fmt.Sprintf("%s/issue-%d-%d", BranchPrefix, issue.GetNumber(), timestamp)

	// 生成 Issue 工作空间目录名
	issueDir := fmt.Sprintf("issue-%d-%d", issue.GetNumber(), timestamp)

	// 获取或创建仓库管理器
	repoManager := m.getOrCreateRepoManager(repoURL)

	// 创建 worktree
	worktree, err := repoManager.CreateWorktreeWithName(issueDir, branchName, true)
	if err != nil {
		log.Errorf("Failed to create worktree for Issue #%d: %v", issue.GetNumber(), err)
		return nil
	}

	// 创建 session 目录
	sessionPath := filepath.Join(repoManager.GetRepoPath(), fmt.Sprintf("session-%d-%d", issue.GetNumber(), timestamp))
	if err := os.MkdirAll(sessionPath, 0755); err != nil {
		log.Errorf("Failed to create session directory: %v", err)
		return nil
	}

	// 创建工作空间对象
	ws := &models.Workspace{
		ID:          issueDir,
		Path:        worktree.Path,
		SessionPath: sessionPath,
		Repository:  repoURL,
		Branch:      worktree.Branch,
		CreatedAt:   worktree.CreatedAt,
		Issue:       issue,
	}

	// 注册到 Issue 映射（临时标记，等待 PR 创建后更新）
	m.mutex.Lock()
	m.issueMapping[issueDir] = 0
	m.mutex.Unlock()

	log.Infof("Created workspace from Issue #%d: %s", issue.GetNumber(), ws.Path)
	return ws
}

// UpdateIssueToPRMapping 更新 Issue 到 PR 的映射
func (m *Manager) UpdateIssueToPRMapping(ws *models.Workspace, prNumber int) error {
	log.Infof("Updating mapping: %s -> PR #%d", ws.ID, prNumber)

	// 1. 更新内存映射
	m.mutex.Lock()
	m.issueMapping[ws.ID] = prNumber
	m.mutex.Unlock()

	return m.repoManagers[ws.Repository].GetMappingManager().CreateMapping(ws.ID, prNumber)
}

// GetWorkspaceByPR 根据 PR 号获取工作空间
func (m *Manager) GetWorkspaceByPR(prNumber int) *models.Workspace {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return m.workspaces[prNumber]
}

// GetWorkspaceByIssue 根据 Issue 号获取工作空间
func (m *Manager) GetWorkspaceByIssue(issueNumber int) *models.Workspace {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// 查找对应的 Issue 目录
	for issueDir, prNumber := range m.issueMapping {
		if strings.HasPrefix(issueDir, fmt.Sprintf("issue-%d-", issueNumber)) {
			return m.workspaces[prNumber]
		}
	}

	return nil
}

// extractRepoNameFromIssueDir 从 Issue 目录名提取仓库名
func (m *Manager) extractRepoNameFromIssueDir(issueDir string) string {
	// 从工作空间路径中提取仓库名
	// 格式：basedir/repoName/issue-{number}-{timestamp}
	// 或者直接是 issue-{number}-{timestamp}
	if strings.Contains(issueDir, "/") {
		parts := strings.Split(issueDir, "/")
		if len(parts) >= 2 {
			return parts[len(parts)-2] // 倒数第二个部分是仓库名
		}
	}

	// 如果没有路径分隔符，说明只是目录名，需要从工作空间路径中提取
	// 这里我们需要从工作空间的实际路径中提取仓库名
	for repoName, repoManager := range m.repoManagers {
		if strings.Contains(repoManager.GetRepoPath(), issueDir) {
			return repoName
		}
	}

	return "unknown"
}
