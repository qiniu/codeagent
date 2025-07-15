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

func key(orgRepo string, prNumber int) string {
	return fmt.Sprintf("%s/%d", orgRepo, prNumber)
}

type Manager struct {
	baseDir string

	// key: org/repo/pr-number
	workspaces map[string]*models.Workspace
	// key: org/repo
	repoManagers map[string]*RepoManager
	mutex        sync.RWMutex
	config       *config.Config
}

func NewManager(cfg *config.Config) *Manager {
	m := &Manager{
		baseDir:      cfg.Workspace.BaseDir,
		workspaces:   make(map[string]*models.Workspace),
		repoManagers: make(map[string]*RepoManager),
		config:       cfg,
	}

	// 启动时恢复现有工作空间
	m.recoverExistingWorkspaces()

	return m
}

// cleanupWorkspace 清理单个工作空间，返回是否清理成功
func (m *Manager) CleanupWorkspace(ws *models.Workspace) bool {
	if ws == nil || ws.Path == "" {
		return false
	}

	// 清理内存中映射
	m.mutex.Lock()
	delete(m.workspaces, key(fmt.Sprintf("%s/%s", ws.Org, ws.Repo), ws.PRNumber))
	m.mutex.Unlock()

	// 清理物理工作空间
	return m.cleanupWorkspaceWithWorktree(ws)
}

// cleanupWorkspaceWithWorktree 清理 worktree 工作空间，返回是否清理成功
func (m *Manager) cleanupWorkspaceWithWorktree(ws *models.Workspace) bool {
	// 从工作空间路径提取编号
	worktreeDir := filepath.Base(ws.Path)
	var entityNumber int

	// 根据目录类型提取编号
	if strings.Contains(worktreeDir, "-pr-") {
		entityNumber = m.extractPRNumberFromPRDir(worktreeDir)
	} else if strings.Contains(worktreeDir, "-issue-") {
		entityNumber = m.extractIssueNumberFromIssueDir(worktreeDir)
	}

	if entityNumber == 0 {
		log.Warnf("Could not extract entity number from workspace path: %s", ws.Path)
		return false
	}

	// 获取仓库管理器（不持锁，避免死锁）
	orgRepoPath := fmt.Sprintf("%s/%s", ws.Org, ws.Repo)
	var repoManager *RepoManager

	m.mutex.RLock()
	if rm, exists := m.repoManagers[orgRepoPath]; exists {
		repoManager = rm
	}
	m.mutex.RUnlock()

	if repoManager == nil {
		log.Warnf("Repo manager not found for %s", orgRepoPath)
		// 即使没有 repoManager，也要尝试删除 session 目录
		if ws.SessionPath != "" {
			if err := os.RemoveAll(ws.SessionPath); err != nil {
				log.Errorf("Failed to remove session directory %s: %v", ws.SessionPath, err)
			} else {
				log.Infof("Removed session directory: %s", ws.SessionPath)
			}
		}
		return false
	}

	// 移除 worktree
	worktreeRemoved := false
	if err := repoManager.RemoveWorktree(entityNumber); err != nil {
		log.Errorf("Failed to remove worktree for entity #%d: %v", entityNumber, err)
	} else {
		worktreeRemoved = true
		log.Infof("Successfully removed worktree for entity #%d", entityNumber)
	}

	// 删除 session 目录
	sessionRemoved := false
	if ws.SessionPath != "" {
		if err := os.RemoveAll(ws.SessionPath); err != nil {
			log.Errorf("Failed to remove session directory %s: %v", ws.SessionPath, err)
		} else {
			sessionRemoved = true
			log.Infof("Successfully removed session directory: %s", ws.SessionPath)
		}
	}

	// 只有 worktree 和 session 都清理成功才返回 true
	return worktreeRemoved && sessionRemoved
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

// recoverExistingWorkspaces 扫描目录名恢复现有工作空间
func (m *Manager) recoverExistingWorkspaces() {
	log.Infof("Starting to recover existing workspaces by scanning directory names from %s", m.baseDir)

	entries, err := os.ReadDir(m.baseDir)
	if err != nil {
		log.Errorf("Failed to read base directory: %v", err)
		return
	}

	recoveredCount := 0
	for _, orgEntry := range entries {
		if !orgEntry.IsDir() {
			continue
		}

		// 组织
		org := orgEntry.Name()
		orgPath := filepath.Join(m.baseDir, org)

		// 读取组织下的所有目录
		orgEntries, err := os.ReadDir(orgPath)
		if err != nil {
			log.Warnf("Failed to read org directory %s: %v", orgPath, err)
			continue
		}

		// 扫描所有目录，找到符合格式的工作空间
		for _, entry := range orgEntries {
			if !entry.IsDir() {
				continue
			}

			dirName := entry.Name()
			dirPath := filepath.Join(orgPath, dirName)

			// 检查是否是 PR 工作空间目录：{repo}-pr-{pr-number}-{timestamp}
			if !strings.Contains(dirName, "-pr-") {
				continue
			}

			parts := strings.Split(dirName, "-pr-")
			if len(parts) < 2 {
				log.Warnf("Invalid PR workspace directory name: %s", dirName)
				continue
			}

			// 提取仓库名
			repoName := parts[0]

			// 提取 PR 号
			numberParts := strings.Split(parts[1], "-")
			if len(numberParts) < 2 {
				log.Warnf("Invalid PR workspace directory name: %s", dirName)
				continue
			}

			if prNumber, err := strconv.Atoi(numberParts[0]); err == nil && prNumber > 0 {
				// 找到对应的仓库目录
				repoPath := filepath.Join(orgPath, repoName)
				if _, err := os.Stat(filepath.Join(repoPath, ".git")); err == nil {
					// 获取远程仓库 URL
					remoteURL, err := m.getRemoteURL(repoPath)
					if err != nil {
						log.Warnf("Failed to get remote URL for %s: %v", repoPath, err)
						continue
					}

					// 创建仓库管理器
					orgRepoPath := fmt.Sprintf("%s/%s", org, repoName)
					m.mutex.Lock()
					if m.repoManagers[orgRepoPath] == nil {
						repoManager := NewRepoManager(repoPath, remoteURL)
						// 恢复 worktrees
						if err := repoManager.RestoreWorktrees(); err != nil {
							log.Warnf("Failed to restore worktrees for %s: %v", orgRepoPath, err)
						}
						m.repoManagers[orgRepoPath] = repoManager
						log.Infof("Created repo manager for %s", orgRepoPath)
					}
					m.mutex.Unlock()

					// 恢复 PR 工作空间
					if err := m.recoverPRWorkspace(org, repoName, dirPath, remoteURL, prNumber); err != nil {
						log.Errorf("Failed to recover PR workspace %s: %v", dirName, err)
					} else {
						recoveredCount++
					}
				}
			}

		}
	}

	log.Infof("Workspace recovery completed. Recovered %d workspaces", recoveredCount)
}

// recoverPRWorkspace 恢复单个 PR 工作空间
func (m *Manager) recoverPRWorkspace(org, repo, worktreePath, remoteURL string, prNumber int) error {
	// 从 worktree 路径提取 PR 信息
	worktreeDir := filepath.Base(worktreePath)
	timestamp := strings.TrimPrefix(worktreeDir, repo+"-pr-"+strconv.Itoa(prNumber)+"-")
	sessionSuffix := strings.TrimPrefix(worktreeDir, repo+"-pr-")

	// 将 timestamp 字符串转换为时间
	var createdAt time.Time
	if timestampInt, err := strconv.ParseInt(timestamp, 10, 64); err == nil {
		createdAt = time.Unix(timestampInt, 0)
	} else {
		log.Errorf("Failed to parse timestamp %s, using current time: %v", timestamp, err)
		return fmt.Errorf("failed to parse timestamp %s", timestamp)
	}

	// 创建对应的 session 目录（与 repo 同级）
	sessionPath := filepath.Join(m.baseDir, org, fmt.Sprintf("%s-session-%s", repo, sessionSuffix))

	// 恢复工作空间对象
	ws := &models.Workspace{
		Org:         org,
		Repo:        repo,
		Path:        worktreePath,
		PRNumber:    prNumber,
		SessionPath: sessionPath,
		Repository:  remoteURL,
		CreatedAt:   createdAt,
	}

	// 注册到内存映射
	orgRepoPath := fmt.Sprintf("%s/%s", org, repo)
	prKey := key(orgRepoPath, prNumber)
	m.mutex.Lock()
	m.workspaces[prKey] = ws
	m.mutex.Unlock()

	log.Infof("Recovered PR workspace: %v", ws)
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

// getOrCreateRepoManager 获取或创建仓库管理器
func (m *Manager) getOrCreateRepoManager(org, repo string) *RepoManager {
	orgRepo := fmt.Sprintf("%s/%s", org, repo)
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// 检查是否已存在
	if repoManager, exists := m.repoManagers[orgRepo]; exists {
		return repoManager
	}

	// 创建新的仓库管理器
	repoPath := filepath.Join(m.baseDir, orgRepo)
	repoManager := NewRepoManager(repoPath, fmt.Sprintf("https://github.com/%s/%s.git", org, repo))
	m.repoManagers[orgRepo] = repoManager

	return repoManager
}

// extractPRNumberFromWorkspaceDir 从工作空间目录名中提取 PR 号或 Issue 号
func (m *Manager) extractPRNumberFromWorkspaceDir(workspaceDir string) int {
	// 工作空间目录格式:
	// - {repo}-pr-{number}-{timestamp} 例如: codeagent-pr-91-1752121132
	// - {repo}-issue-{number}-{timestamp} 例如: codeagent-issue-11-1752121132
	if strings.Contains(workspaceDir, "-pr-") {
		return m.extractPRNumberFromPRDir(workspaceDir)
	} else if strings.Contains(workspaceDir, "-issue-") {
		// 对于 Issue 目录，提取 Issue 号
		parts := strings.Split(workspaceDir, "-issue-")
		if len(parts) >= 2 {
			numberParts := strings.Split(parts[1], "-")
			if len(numberParts) >= 1 {
				if number, err := strconv.Atoi(numberParts[0]); err == nil {
					return number
				}
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

// extractOrgRepoPath 从仓库 URL 中提取 org/repo 路径
func (m *Manager) extractOrgRepoPath(repoURL string) string {
	// 移除 .git 后缀
	repoURL = strings.TrimSuffix(repoURL, ".git")

	// 处理 SSH URL 格式: git@github.com:org/repo
	if strings.HasPrefix(repoURL, "git@") {
		// 分割 git@github.com:org/repo
		parts := strings.Split(repoURL, ":")
		if len(parts) == 2 {
			// 分割 org/repo
			repoParts := strings.Split(parts[1], "/")
			if len(repoParts) >= 2 {
				return fmt.Sprintf("%s/%s", repoParts[0], repoParts[1])
			}
		}
		return "unknown/unknown"
	}

	// 处理 HTTPS URL 格式: https://github.com/org/repo
	parts := strings.Split(repoURL, "/")
	if len(parts) >= 2 {
		// 从 https://github.com/org/repo 提取 org/repo
		return fmt.Sprintf("%s/%s", parts[len(parts)-2], parts[len(parts)-1])
	}

	return "unknown/unknown"
}

// extractRepoURLFromIssueURL 从 Issue URL 中提取仓库 URL
func (m *Manager) extractRepoURLFromIssueURL(issueURL string) (url, org, repo string, err error) {
	// Issue URL 格式: https://github.com/owner/repo/issues/123
	if strings.Contains(issueURL, "github.com") {
		parts := strings.Split(issueURL, "/")
		if len(parts) >= 4 {
			org = parts[len(parts)-4]
			repo = parts[len(parts)-3]
			url = fmt.Sprintf("https://github.com/%s/%s.git", org, repo)
			return
		}
	}
	return "", "", "", fmt.Errorf("failed to extract repository URL from Issue URL: %s", issueURL)
}

// RegisterWorkspace 注册工作空间
func (m *Manager) RegisterWorkspace(ws *models.Workspace, pr *github.PullRequest) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	prKey := key(fmt.Sprintf("%s/%s", ws.Org, ws.Repo), pr.GetNumber())
	if _, exists := m.workspaces[prKey]; exists {
		// 这里不应该报错，因为一个 PR 只能有一个工作空间
		log.Errorf("Workspace %s already registered", prKey)
		return
	}

	m.workspaces[prKey] = ws
	log.Infof("Registered workspace: %s, %s", prKey, ws.Path)
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

func (m *Manager) CreateSessionPath(underPath, repo string, prNumber int, suffix string) (string, error) {
	sessionPath := filepath.Join(underPath, fmt.Sprintf("%s-session-%d-%s", repo, prNumber, suffix))
	if err := os.MkdirAll(sessionPath, 0755); err != nil {
		log.Errorf("Failed to create session directory: %v", err)
		return "", err
	}
	return sessionPath, nil
}

// CreateWorkspaceFromIssue 从 Issue 创建工作空间
func (m *Manager) CreateWorkspaceFromIssue(issue *github.Issue) *models.Workspace {
	log.Infof("Creating workspace from Issue #%d", issue.GetNumber())

	// 从 Issue 的 HTML URL 中提取仓库信息
	repoURL, org, repo, err := m.extractRepoURLFromIssueURL(issue.GetHTMLURL())
	if err != nil {
		log.Errorf("Failed to extract repository URL from Issue URL: %s, %v", issue.GetHTMLURL(), err)
		return nil
	}

	// 生成分支名
	timestamp := time.Now().Unix()
	branchName := fmt.Sprintf("%s/issue-%d-%d", BranchPrefix, issue.GetNumber(), timestamp)

	// 生成 Issue 工作空间目录名（与 repo 同级）
	issueDir := fmt.Sprintf("%s-issue-%d-%d", repo, issue.GetNumber(), timestamp)

	// 获取或创建仓库管理器
	repoManager := m.getOrCreateRepoManager(org, repo)

	// 创建 worktree
	worktree, err := repoManager.CreateWorktreeWithName(issueDir, branchName, true)
	if err != nil {
		log.Errorf("Failed to create worktree for Issue #%d: %v", issue.GetNumber(), err)
		return nil
	}

	// 创建工作空间对象
	ws := &models.Workspace{
		Org:  org,
		Repo: repo,
		Path: worktree.Worktree,
		// 本阶段没有 session 目录
		SessionPath: "",
		Repository:  repoURL,
		Branch:      worktree.Branch,
		CreatedAt:   time.Now(),
		Issue:       issue,
	}

	log.Infof("Created workspace from Issue #%d: %s", issue.GetNumber(), ws.Path)
	return ws
}

// MoveIssueToPR 使用 git worktree move 将 Issue 工作空间移动到 PR 工作空间
func (m *Manager) MoveIssueToPR(ws *models.Workspace, prNumber int) error {
	issueSuffix := strings.TrimPrefix(filepath.Base(ws.Path), fmt.Sprintf("%s-issue-%d-", ws.Repo, ws.Issue.GetNumber()))
	newWorktreeName := fmt.Sprintf("%s-pr-%d-%s", ws.Repo, prNumber, issueSuffix)

	newWorktreePath := filepath.Join(filepath.Dir(ws.Path), newWorktreeName)
	log.Infof("try to move workspace from %s to %s", ws.Path, newWorktreePath)

	// 获取仓库管理器
	orgRepoPath := fmt.Sprintf("%s/%s", ws.Org, ws.Repo)
	repoManager := m.repoManagers[orgRepoPath]
	if repoManager == nil {
		return fmt.Errorf("repo manager not found for %s", orgRepoPath)
	}

	// 执行 git worktree move 命令
	cmd := exec.Command("git", "worktree", "move", ws.Path, newWorktreePath)
	cmd.Dir = repoManager.GetRepoPath() // 在 Git 仓库根目录下执行

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Errorf("Failed to move worktree: %v, output: %s", err, string(output))
		return fmt.Errorf("failed to move worktree: %w, output: %s", err, string(output))
	}

	log.Infof("Successfully moved worktree: %s -> %s", ws.Path, newWorktreeName)

	// 更新工作空间路径
	ws.Path = newWorktreePath

	// 移动之后，注册worktree到内存中
	worktree := &WorktreeInfo{
		Worktree: ws.Path,
		Branch:   ws.Branch,
	}
	repoManager.RegisterWorktree(prNumber, worktree)
	return nil
}

func (m *Manager) GetWorkspaceByPR(pr *github.PullRequest) *models.Workspace {
	orgRepoPath := fmt.Sprintf("%s/%s", pr.GetBase().GetRepo().GetOwner().GetLogin(), pr.GetBase().GetRepo().GetName())
	prKey := key(orgRepoPath, pr.GetNumber())
	m.mutex.RLock()
	if ws, exists := m.workspaces[prKey]; exists {
		m.mutex.RUnlock()
		log.Infof("Found existing workspace for PR #%d: %s", pr.GetNumber(), ws.Path)
		return ws
	}
	m.mutex.RUnlock()
	return nil
}

// CreateWorkspaceFromPR 从 PR 创建工作空间（直接包含 PR 号）
func (m *Manager) CreateWorkspaceFromPR(pr *github.PullRequest) *models.Workspace {
	log.Infof("Creating workspace from PR #%d", pr.GetNumber())

	// 获取仓库 URL
	repoURL := pr.GetBase().GetRepo().GetCloneURL()
	if repoURL == "" {
		log.Errorf("Failed to get repository URL for PR #%d", pr.GetNumber())
		return nil
	}

	// 获取 PR 分支
	prBranch := pr.GetHead().GetRef()

	// 生成 PR 工作空间目录名（与 repo 同级）
	timestamp := time.Now().Unix()
	repo := pr.GetBase().GetRepo().GetName()
	prDir := fmt.Sprintf("%s-pr-%d-%d", repo, pr.GetNumber(), timestamp)

	org := pr.GetBase().GetRepo().GetOwner().GetLogin()

	// 获取或创建仓库管理器
	repoManager := m.getOrCreateRepoManager(org, repo)

	// 创建 worktree（不创建新分支，切换到现有分支）
	worktree, err := repoManager.CreateWorktreeWithName(prDir, prBranch, false)
	if err != nil {
		log.Errorf("Failed to create worktree for PR #%d: %v", pr.GetNumber(), err)
		return nil
	}

	// 注册worktree 到内存中
	repoManager.RegisterWorktree(pr.GetNumber(), worktree)

	// 创建 session 目录
	suffix := strings.TrimPrefix(filepath.Base(worktree.Worktree), fmt.Sprintf("%s-pr-%d-", repo, pr.GetNumber()))
	sessionPath, err := m.CreateSessionPath(filepath.Dir(repoManager.GetRepoPath()), repo, pr.GetNumber(), suffix)
	if err != nil {
		log.Errorf("Failed to create session directory: %v", err)
		return nil
	}

	// 创建工作空间对象
	ws := &models.Workspace{
		Org:         org,
		Repo:        repo,
		Path:        worktree.Worktree,
		SessionPath: sessionPath,
		Repository:  repoURL,
		Branch:      worktree.Branch,
		PullRequest: pr,
		CreatedAt:   time.Now(),
	}

	// 注册到内存映射
	prKey := key(fmt.Sprintf("%s/%s", org, repo), pr.GetNumber())
	m.mutex.Lock()
	m.workspaces[prKey] = ws
	m.mutex.Unlock()

	log.Infof("Created workspace from PR #%d: %s", pr.GetNumber(), ws.Path)
	return ws
}

// GetOrCreateWorkspaceForPR 获取或创建 PR 的工作空间
func (m *Manager) GetOrCreateWorkspaceForPR(pr *github.PullRequest) *models.Workspace {
	// 1. 先尝试从内存中获取
	ws := m.GetWorkspaceByPR(pr)
	if ws != nil {
		// 验证工作空间是否对应正确的 PR 分支
		if m.validateWorkspaceForPR(ws, pr) {
			return ws
		}
		// 如果验证失败，清理旧的工作空间
		log.Infof("Workspace validation failed for PR #%d, cleaning up old workspace", pr.GetNumber())
		m.CleanupWorkspace(ws)
	}

	// 2. 创建新的工作空间
	log.Infof("Creating new workspace for PR #%d", pr.GetNumber())
	return m.CreateWorkspaceFromPR(pr)
}

// validateWorkspaceForPR 验证工作空间是否对应正确的 PR 分支
func (m *Manager) validateWorkspaceForPR(ws *models.Workspace, pr *github.PullRequest) bool {
	// 检查工作空间路径是否存在
	if _, err := os.Stat(ws.Path); os.IsNotExist(err) {
		log.Infof("Workspace path does not exist: %s", ws.Path)
		return false
	}

	// 检查工作空间是否在正确的分支上
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = ws.Path
	output, err := cmd.Output()
	if err != nil {
		log.Infof("Failed to get current branch for workspace: %v", err)
		return false
	}

	currentBranch := strings.TrimSpace(string(output))
	expectedBranch := pr.GetHead().GetRef()

	log.Infof("Workspace branch validation: current=%s, expected=%s", currentBranch, expectedBranch)

	// 检查是否在正确的分支上，或者是否在 detached HEAD 状态
	if currentBranch == expectedBranch {
		return true
	}

	// 如果是 detached HEAD，检查是否指向正确的 commit
	if currentBranch == "HEAD" {
		// 获取当前 commit
		cmd = exec.Command("git", "rev-parse", "HEAD")
		cmd.Dir = ws.Path
		output, err = cmd.Output()
		if err != nil {
			log.Infof("Failed to get current commit for workspace: %v", err)
			return false
		}
		currentCommit := strings.TrimSpace(string(output))

		// 获取期望分支的最新 commit
		cmd = exec.Command("git", "rev-parse", fmt.Sprintf("origin/%s", expectedBranch))
		cmd.Dir = ws.Path
		output, err = cmd.Output()
		if err != nil {
			log.Infof("Failed to get expected branch commit: %v", err)
			return false
		}
		expectedCommit := strings.TrimSpace(string(output))

		log.Infof("Commit validation: current=%s, expected=%s", currentCommit, expectedCommit)
		return currentCommit == expectedCommit
	}

	return false
}

// extractPRNumberFromPRDir 从 PR 目录名提取 PR 号
func (m *Manager) extractPRNumberFromPRDir(prDir string) int {
	// PR 目录格式: {repo}-pr-{number}-{timestamp}
	if strings.Contains(prDir, "-pr-") {
		parts := strings.Split(prDir, "-pr-")
		if len(parts) >= 2 {
			numberParts := strings.Split(parts[1], "-")
			if len(numberParts) >= 1 {
				if number, err := strconv.Atoi(numberParts[0]); err == nil {
					return number
				}
			}
		}
	}
	return 0
}

// extractIssueNumberFromIssueDir 从 Issue 目录名提取 Issue 号
func (m *Manager) extractIssueNumberFromIssueDir(issueDir string) int {
	// Issue 目录格式: {repo}-issue-{number}-{timestamp}
	if strings.Contains(issueDir, "-issue-") {
		parts := strings.Split(issueDir, "-issue-")
		if len(parts) >= 2 {
			numberParts := strings.Split(parts[1], "-")
			if len(numberParts) >= 1 {
				if number, err := strconv.Atoi(numberParts[0]); err == nil {
					return number
				}
			}
		}
	}
	return 0
}

func (m *Manager) GetExpiredWorkspaces() []*models.Workspace {
	expiredWorkspaces := []*models.Workspace{}
	now := time.Now()
	m.mutex.RLock()
	for _, ws := range m.workspaces {
		if now.Sub(ws.CreatedAt) > m.config.Workspace.CleanupAfter {
			expiredWorkspaces = append(expiredWorkspaces, ws)
		}
	}
	m.mutex.RUnlock()

	return expiredWorkspaces
}
