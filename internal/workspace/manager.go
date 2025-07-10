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
	baseDir      string
	workspaces   map[int]*models.Workspace
	repoManagers map[string]*RepoManager // 仓库管理器映射
	mutex        sync.RWMutex
	config       *config.Config
}

func NewManager(cfg *config.Config) *Manager {
	m := &Manager{
		baseDir:      cfg.Workspace.BaseDir,
		workspaces:   make(map[int]*models.Workspace),
		repoManagers: make(map[string]*RepoManager),
		config:       cfg,
	}

	// 启动定期清理协程
	go m.startCleanupRoutine()

	// 启动时恢复现有工作空间
	if cfg.Workspace.UseWorktree {
		m.recoverExistingWorkspacesWithWorktree()
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
	ws := m.CreateWorkspace(issue.GetNumber(), repoURL, branch, true)
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
	ws := m.CreateWorkspace(event.Issue.GetNumber(), repoURL, branch, true)
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

	// 生成工作空间 ID 和路径（统一使用 PR 格式）
	timestamp := time.Now().Unix()
	id := fmt.Sprintf("pr-%d-%d", entityNumber, timestamp)
	workspaceDir := fmt.Sprintf("pr-%d-%d", entityNumber, timestamp)

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

// recoverExistingWorkspacesWithWorktree 使用 worktree 恢复现有工作空间
func (m *Manager) recoverExistingWorkspacesWithWorktree() {
	log.Infof("Starting to recover existing workspaces with worktree from %s", m.baseDir)

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

		log.Infof("repoPath: %s", repoPath)

		// 检查是否有 .git 目录
		gitDir := filepath.Join(repoPath, ".git")
		if _, err := os.Stat(gitDir); os.IsNotExist(err) {
			continue
		}

		// 获取远程仓库 URL
		cmd := exec.Command("git", "remote", "get-url", "origin")
		cmd.Dir = repoPath
		remoteOutput, err := cmd.Output()
		if err != nil {
			log.Warnf("Failed to get remote URL for %s: %v", repoPath, err)
			continue
		}
		remoteURL := strings.TrimSpace(string(remoteOutput))

		log.Infof("remoteURL: %s", remoteURL)
		// 创建仓库管理器
		repoManager := NewRepoManager(repoPath, remoteURL)
		m.repoManagers[repoEntry.Name()] = repoManager

		// 获取所有 worktree
		worktrees, err := repoManager.ListWorktrees()
		if err != nil {
			log.Warnf("Failed to list worktrees for %s: %v", repoPath, err)
			continue
		}

		// 恢复每个 worktree
		for _, worktree := range worktrees {
			log.Infof("worktree: %s", worktree.Path)
			// 创建 session 路径
			sessionPath := filepath.Join(repoPath, fmt.Sprintf("session-%d", worktree.PRNumber))
			log.Infof("sessionPath: %s", sessionPath)
			if err := os.MkdirAll(sessionPath, 0755); err != nil {
				log.Warnf("Failed to create session directory: %v", err)
			}

			// 创建临时 PR 对象用于恢复
			tempPR := &github.PullRequest{
				Number: github.Int(worktree.PRNumber),
			}

			// 恢复工作空间
			ws := &models.Workspace{
				ID:          fmt.Sprintf("pr-%d-%d", worktree.PRNumber, worktree.CreatedAt.Unix()),
				Path:        worktree.Path,
				SessionPath: sessionPath,
				Repository:  remoteURL,
				Branch:      worktree.Branch,
				PullRequest: tempPR,
				CreatedAt:   worktree.CreatedAt,
			}

			// 注册到内存映射
			m.mutex.Lock()
			m.workspaces[worktree.PRNumber] = ws
			m.mutex.Unlock()

			recoveredCount++
			log.Infof("Recovered worktree workspace for PR #%d: %s", worktree.PRNumber, worktree.Path)
		}
	}

	log.Infof("Worktree recovery completed. Recovered %d workspaces", recoveredCount)
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

// extractPRNumberFromWorkspaceDir 从工作空间目录名中提取 PR 号
func (m *Manager) extractPRNumberFromWorkspaceDir(workspaceDir string) int {
	// 工作空间目录格式: pr-{number}-{timestamp}
	// 例如: pr-91-1752121132
	if strings.HasPrefix(workspaceDir, "pr-") {
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
