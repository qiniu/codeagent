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

	"github.com/qiniu/codeagent/internal/config"
	"github.com/qiniu/codeagent/pkg/models"

	"github.com/google/go-github/v58/github"
	"github.com/qiniu/x/log"
)

const (
	// BranchPrefix branch name prefix, used to identify branches created by codeagent
	BranchPrefix = "codeagent"
)

func key(orgRepo string, prNumber int) string {
	return fmt.Sprintf("%s/%d", orgRepo, prNumber)
}

func keyWithAI(orgRepo string, prNumber int, aiModel string) string {
	if aiModel == "" {
		return fmt.Sprintf("%s/%d", orgRepo, prNumber)
	}
	return fmt.Sprintf("%s/%s/%d", aiModel, orgRepo, prNumber)
}

// Directory format related common methods

// GenerateIssueDirName generates Issue directory name
func (m *Manager) GenerateIssueDirName(aiModel, repo string, issueNumber int, timestamp int64) string {
	return m.dirFormatter.generateIssueDirName(aiModel, repo, issueNumber, timestamp)
}

// GeneratePRDirName generates PR directory name
func (m *Manager) GeneratePRDirName(aiModel, repo string, prNumber int, timestamp int64) string {
	return m.dirFormatter.generatePRDirName(aiModel, repo, prNumber, timestamp)
}

// GenerateSessionDirName generates Session directory name
func (m *Manager) GenerateSessionDirName(aiModel, repo string, prNumber int, timestamp int64) string {
	return m.dirFormatter.generateSessionDirName(aiModel, repo, prNumber, timestamp)
}

// ParsePRDirName parses PR directory name
func (m *Manager) ParsePRDirName(dirName string) (*PRDirFormat, error) {
	return m.dirFormatter.parsePRDirName(dirName)
}

// ExtractSuffixFromPRDir extracts suffix from PR directory name
func (m *Manager) ExtractSuffixFromPRDir(aiModel, repo string, prNumber int, dirName string) string {
	return m.dirFormatter.extractSuffixFromPRDir(aiModel, repo, prNumber, dirName)
}

// ExtractSuffixFromIssueDir extracts suffix from Issue directory name
func (m *Manager) ExtractSuffixFromIssueDir(aiModel, repo string, issueNumber int, dirName string) string {
	return m.dirFormatter.extractSuffixFromIssueDir(aiModel, repo, issueNumber, dirName)
}

// createSessionPath creates Session directory path
func (m *Manager) createSessionPath(underPath, aiModel, repo string, prNumber int, suffix string) string {
	return m.dirFormatter.createSessionPath(underPath, aiModel, repo, prNumber, suffix)
}

// createSessionPathWithTimestamp creates Session directory path (using timestamp)
func (m *Manager) createSessionPathWithTimestamp(underPath, aiModel, repo string, prNumber int, timestamp int64) string {
	return m.dirFormatter.createSessionPathWithTimestamp(underPath, aiModel, repo, prNumber, timestamp)
}

// ExtractAIModelFromBranch extracts AI model information from branch name
// Branch format: codeagent/{aimodel}/{type}-{number}-{timestamp}
func (m *Manager) ExtractAIModelFromBranch(branchName string) string {
	// Check if it's a codeagent branch
	if !strings.HasPrefix(branchName, BranchPrefix+"/") {
		return ""
	}

	// Remove codeagent/ prefix
	branchWithoutPrefix := strings.TrimPrefix(branchName, BranchPrefix+"/")

	// Split to get aimodel part
	parts := strings.Split(branchWithoutPrefix, "/")
	if len(parts) >= 2 {
		aiModel := parts[0]
		// Validate if it's a valid AI model
		if aiModel == "claude" || aiModel == "gemini" {
			return aiModel
		}
	}

	return m.config.CodeProvider
}

type Manager struct {
	baseDir string

	// key: aimodel/org/repo/pr-number
	workspaces map[string]*models.Workspace
	mutex      sync.RWMutex
	config *config.Config

	// 目录格式管理器
	dirFormatter *dirFormatter
}

func NewManager(cfg *config.Config) *Manager {
	m := &Manager{
		baseDir:      cfg.Workspace.BaseDir,
		workspaces:   make(map[string]*models.Workspace),
		config:       cfg,
		dirFormatter: newDirFormatter(),
	}

	// 启动时恢复现有工作空间
	m.recoverExistingWorkspaces()

	return m
}

// GetBaseDir returns the base directory for workspaces
func (m *Manager) GetBaseDir() string {
	return m.baseDir
}

// cloneRepository creates a new git clone for a workspace
func (m *Manager) cloneRepository(repoURL, clonePath, branch string, createNewBranch bool) error {
	log.Infof("Cloning repository: %s to %s, branch: %s, createNewBranch: %v", repoURL, clonePath, branch, createNewBranch)

	// Create parent directory if needed
	if err := os.MkdirAll(filepath.Dir(clonePath), 0755); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	// Clone the repository with shallow depth for efficiency
	var cmd *exec.Cmd
	if createNewBranch {
		// Clone the default branch first, then create new branch
		cmd = exec.Command("git", "clone", "--depth", "1", repoURL, clonePath)
	} else {
		// Try to clone specific branch directly
		cmd = exec.Command("git", "clone", "--depth", "1", "--branch", branch, repoURL, clonePath)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		if !createNewBranch {
			// If direct branch clone failed, try cloning default branch first
			log.Warnf("Failed to clone specific branch %s directly, cloning default branch: %v", branch, err)
			cmd = exec.Command("git", "clone", "--depth", "1", repoURL, clonePath)
			output, err = cmd.CombinedOutput()
		}
		if err != nil {
			return fmt.Errorf("failed to clone repository: %w, output: %s", err, string(output))
		}
	}

	// Configure Git safe directory
	cmd = exec.Command("git", "config", "--local", "--add", "safe.directory", clonePath)
	cmd.Dir = clonePath
	if configOutput, err := cmd.CombinedOutput(); err != nil {
		log.Warnf("Failed to configure safe directory: %v\nCommand output: %s", err, string(configOutput))
	}

	// Configure rebase as default pull strategy
	cmd = exec.Command("git", "config", "--local", "pull.rebase", "true")
	cmd.Dir = clonePath
	if rebaseConfigOutput, err := cmd.CombinedOutput(); err != nil {
		log.Warnf("Failed to configure pull.rebase: %v\nCommand output: %s", err, string(rebaseConfigOutput))
	}

	if createNewBranch {
		// Create and switch to new branch
		cmd = exec.Command("git", "checkout", "-b", branch)
		cmd.Dir = clonePath
		if branchOutput, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to create new branch %s: %w, output: %s", branch, err, string(branchOutput))
		}
		log.Infof("Created new branch: %s", branch)
	} else if branch != "" {
		// Verify we're on the correct branch or switch to it
		cmd = exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
		cmd.Dir = clonePath
		currentBranchOutput, err := cmd.Output()
		if err != nil {
			log.Warnf("Failed to check current branch: %v", err)
		} else {
			currentBranch := strings.TrimSpace(string(currentBranchOutput))
			if currentBranch != branch {
				// Try to switch to the target branch
				cmd = exec.Command("git", "checkout", branch)
				cmd.Dir = clonePath
				if switchOutput, err := cmd.CombinedOutput(); err != nil {
					// Branch might not exist locally, try creating tracking branch
					cmd = exec.Command("git", "checkout", "-b", branch, fmt.Sprintf("origin/%s", branch))
					cmd.Dir = clonePath
					if trackOutput, err := cmd.CombinedOutput(); err != nil {
						log.Warnf("Failed to switch to branch %s: %v, output: %s", branch, err, string(trackOutput))
					} else {
						log.Infof("Created tracking branch: %s", branch)
					}
				} else {
					log.Infof("Switched to existing branch: %s", branch)
				}
			}
		}
	}

	log.Infof("Successfully cloned repository to: %s", clonePath)
	return nil
}

// cleanupClonedRepository removes a cloned repository directory
func (m *Manager) cleanupClonedRepository(clonePath string) error {
	if clonePath == "" {
		return nil
	}

	log.Infof("Cleaning up cloned repository: %s", clonePath)

	// Remove the entire directory
	if err := os.RemoveAll(clonePath); err != nil {
		return fmt.Errorf("failed to remove cloned repository directory: %w", err)
	}

	log.Infof("Successfully removed cloned repository: %s", clonePath)
	return nil
}

// cleanupWorkspace 清理单个工作空间，返回是否清理成功
func (m *Manager) CleanupWorkspace(ws *models.Workspace) bool {
	if ws == nil || ws.Path == "" {
		return false
	}

	// 清理内存中映射
	m.mutex.Lock()
	delete(m.workspaces, keyWithAI(fmt.Sprintf("%s/%s", ws.Org, ws.Repo), ws.PRNumber, ws.AIModel))
	m.mutex.Unlock()

	// 清理物理工作空间 - 使用新的克隆清理方法
	return m.cleanupWorkspaceWithClone(ws)
}

// cleanupWorkspaceWithClone 清理基于克隆的工作空间，返回是否清理成功
func (m *Manager) cleanupWorkspaceWithClone(ws *models.Workspace) bool {
	cloneRemoved := false
	sessionRemoved := false

	// 删除克隆的仓库目录
	if ws.Path != "" {
		if err := m.cleanupClonedRepository(ws.Path); err != nil {
			log.Errorf("Failed to remove cloned repository %s: %v", ws.Path, err)
		} else {
			cloneRemoved = true
			log.Infof("Successfully removed cloned repository: %s", ws.Path)
		}
	}

	// 删除 session 目录
	if ws.SessionPath != "" {
		if err := os.RemoveAll(ws.SessionPath); err != nil {
			log.Errorf("Failed to remove session directory %s: %v", ws.SessionPath, err)
		} else {
			sessionRemoved = true
			log.Infof("Successfully removed session directory: %s", ws.SessionPath)
		}
	}

	// 清理相关的Docker容器
	containerRemoved := m.cleanupRelatedContainers(ws)
	if !containerRemoved {
		log.Warnf("Failed to cleanup containers for workspace %s", ws.Path)
	}

	// 只有克隆和 session 都清理成功才返回 true
	return cloneRemoved && sessionRemoved
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

// recoverExistingWorkspaces 扫描目录名恢复现有克隆的工作空间
func (m *Manager) recoverExistingWorkspaces() {
	log.Infof("Starting to recover existing workspaces by scanning directory names from %s", m.baseDir)

	entries, err := os.ReadDir(m.baseDir)
	if err != nil && !os.IsNotExist(err) {
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

			// 检查是否是 PR 工作空间目录：{repo}__pr__{pr-number}__{timestamp} 或 {aimodel}__{repo}__pr__{pr-number}__{timestamp}
			if !strings.Contains(dirName, "__pr__") {
				continue
			}

			// 检查是否是有效的 git 仓库（克隆的工作空间应该包含完整的 .git 目录）
			if _, err := os.Stat(filepath.Join(dirPath, ".git")); os.IsNotExist(err) {
				log.Warnf("Directory %s does not contain .git, skipping", dirPath)
				continue
			}

			// 使用目录格式管理器解析目录名
			prFormat, err := m.ParsePRDirName(dirName)
			if err != nil {
				log.Errorf("Failed to parse PR directory name %s: %v, skipping", dirName, err)
				continue
			}

			aiModel := prFormat.AIModel
			repoName := prFormat.Repo
			prNumber := prFormat.PRNumber

			// 获取远程仓库 URL
			remoteURL, err := m.getRemoteURL(dirPath)
			if err != nil {
				log.Warnf("Failed to get remote URL for %s: %v", dirPath, err)
				continue
			}

			// 恢复 PR 工作空间
			if err := m.recoverPRWorkspaceFromClone(org, repoName, dirPath, remoteURL, prNumber, aiModel); err != nil {
				log.Errorf("Failed to recover PR workspace %s: %v", dirName, err)
			} else {
				recoveredCount++
			}
		}
	}

	log.Infof("Workspace recovery completed. Recovered %d workspaces", recoveredCount)
}

// recoverPRWorkspaceFromClone 恢复基于克隆的单个 PR 工作空间
func (m *Manager) recoverPRWorkspaceFromClone(org, repo, clonePath, remoteURL string, prNumber int, aiModel string) error {
	// 从克隆路径提取 PR 信息
	cloneDir := filepath.Base(clonePath)
	var timestamp string

	if aiModel != "" {
		// 有AI模型的情况: aimodel__repo__pr__number__timestamp
		timestamp = strings.TrimPrefix(cloneDir, aiModel+"__"+repo+"__pr__"+strconv.Itoa(prNumber)+"__")
	} else {
		// 没有AI模型的情况: repo__pr__number__timestamp
		timestamp = strings.TrimPrefix(cloneDir, repo+"__pr__"+strconv.Itoa(prNumber)+"__")
	}

	// 将 timestamp 字符串转换为时间
	var createdAt time.Time
	if timestampInt, err := strconv.ParseInt(timestamp, 10, 64); err == nil {
		createdAt = time.Unix(timestampInt, 0)
	} else {
		log.Errorf("Failed to parse timestamp %s, using current time: %v", timestamp, err)
		return fmt.Errorf("failed to parse timestamp %s", timestamp)
	}

	// 创建对应的 session 目录（与克隆目录同级）
	// session目录格式：{aiModel}-{repo}-session-{prNumber}-{timestamp}
	timestampInt, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		log.Warnf("Failed to parse timestamp %s, using current time: %v", timestamp, err)
		timestampInt = time.Now().Unix()
	}
	sessionPath := m.createSessionPathWithTimestamp(m.baseDir, aiModel, repo, prNumber, timestampInt)

	// 获取当前分支信息
	var currentBranch string
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = clonePath
	if branchOutput, err := cmd.Output(); err != nil {
		log.Warnf("Failed to get current branch for %s: %v", clonePath, err)
		currentBranch = ""
	} else {
		currentBranch = strings.TrimSpace(string(branchOutput))
	}

	// 恢复工作空间对象
	ws := &models.Workspace{
		Org:         org,
		Repo:        repo,
		AIModel:     aiModel,
		Path:        clonePath,
		PRNumber:    prNumber,
		SessionPath: sessionPath,
		Repository:  remoteURL,
		Branch:      currentBranch,
		CreatedAt:   createdAt,
	}

	// 注册到内存映射
	orgRepoPath := fmt.Sprintf("%s/%s", org, repo)
	prKey := keyWithAI(orgRepoPath, prNumber, aiModel)
	m.mutex.Lock()
	m.workspaces[prKey] = ws
	m.mutex.Unlock()

	log.Infof("Recovered PR workspace from clone: %v", ws)
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

	prKey := keyWithAI(fmt.Sprintf("%s/%s", ws.Org, ws.Repo), pr.GetNumber(), ws.AIModel)
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



func (m *Manager) CreateSessionPath(underPath, aiModel, repo string, prNumber int, suffix string) (string, error) {
	// session目录格式：{aiModel}-{repo}-session-{prNumber}-{timestamp}
	// 只保留时间戳部分，避免重复信息
	sessionPath := m.createSessionPath(underPath, aiModel, repo, prNumber, suffix)
	if err := os.MkdirAll(sessionPath, 0755); err != nil {
		log.Errorf("Failed to create session directory: %v", err)
		return "", err
	}
	return sessionPath, nil
}

// CreateWorkspaceFromIssueWithAI 从 Issue 创建工作空间，支持指定AI模型（使用git clone方式）
func (m *Manager) CreateWorkspaceFromIssueWithAI(issue *github.Issue, aiModel string) *models.Workspace {
	log.Infof("Creating workspace from Issue #%d with AI model: %s (using git clone)", issue.GetNumber(), aiModel)

	// 从 Issue 的 HTML URL 中提取仓库信息
	repoURL, org, repo, err := m.extractRepoURLFromIssueURL(issue.GetHTMLURL())
	if err != nil {
		log.Errorf("Failed to extract repository URL from Issue URL: %s, %v", issue.GetHTMLURL(), err)
		return nil
	}

	// 生成分支名，包含AI模型信息
	timestamp := time.Now().Unix()
	var branchName string
	if aiModel != "" {
		branchName = fmt.Sprintf("%s/%s/issue-%d-%d", BranchPrefix, aiModel, issue.GetNumber(), timestamp)
	} else {
		branchName = fmt.Sprintf("%s/issue-%d-%d", BranchPrefix, issue.GetNumber(), timestamp)
	}

	// 生成 Issue 工作空间目录名（与其他工作空间同级）
	issueDir := m.GenerateIssueDirName(aiModel, repo, issue.GetNumber(), timestamp)
	clonePath := filepath.Join(m.baseDir, org, issueDir)

	// 克隆仓库并创建新分支
	if err := m.cloneRepository(repoURL, clonePath, branchName, true); err != nil {
		log.Errorf("Failed to clone repository for Issue #%d: %v", issue.GetNumber(), err)
		return nil
	}

	// 创建工作空间对象
	ws := &models.Workspace{
		Org:     org,
		Repo:    repo,
		AIModel: aiModel,
		Path:    clonePath,
		// 本阶段没有 session 目录
		SessionPath: "",
		Repository:  repoURL,
		Branch:      branchName,
		CreatedAt:   time.Now(),
		Issue:       issue,
	}

	log.Infof("Successfully created workspace from Issue #%d using git clone: %s", issue.GetNumber(), clonePath)
	return ws
}

// MoveIssueToPR 将 Issue 工作空间重命名为 PR 工作空间（使用目录重命名）
func (m *Manager) MoveIssueToPR(ws *models.Workspace, prNumber int) error {
	// 构建新的命名: aimodel__repo__issue__number__timestamp -> aimodel__repo__pr__number__timestamp
	oldPrefix := fmt.Sprintf("%s__%s__issue__%d__", ws.AIModel, ws.Repo, ws.Issue.GetNumber())
	issueSuffix := strings.TrimPrefix(filepath.Base(ws.Path), oldPrefix)
	newCloneName := fmt.Sprintf("%s__%s__pr__%d__%s", ws.AIModel, ws.Repo, prNumber, issueSuffix)

	newClonePath := filepath.Join(filepath.Dir(ws.Path), newCloneName)
	log.Infof("Moving workspace from %s to %s", ws.Path, newClonePath)

	// 重命名目录（简单的文件系统操作）
	if err := os.Rename(ws.Path, newClonePath); err != nil {
		log.Errorf("Failed to rename workspace directory: %v", err)
		return fmt.Errorf("failed to rename workspace directory: %w", err)
	}

	log.Infof("Successfully moved workspace: %s -> %s", ws.Path, newClonePath)

	// 更新工作空间路径
	ws.Path = newClonePath
	ws.PRNumber = prNumber

	return nil
}

func (m *Manager) GetWorkspaceByPR(pr *github.PullRequest) *models.Workspace {
	return m.GetWorkspaceByPRAndAI(pr, "")
}

// GetAllWorkspacesByPR 获取PR的所有工作空间（所有AI模型）
func (m *Manager) GetAllWorkspacesByPR(pr *github.PullRequest) []*models.Workspace {
	orgRepoPath := fmt.Sprintf("%s/%s", pr.GetBase().GetRepo().GetOwner().GetLogin(), pr.GetBase().GetRepo().GetName())
	prNumber := pr.GetNumber()

	var workspaces []*models.Workspace

	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// 遍历所有工作空间，找到与该PR相关的
	for _, ws := range m.workspaces {
		// 检查是否是该PR的工作空间
		if ws.PRNumber == prNumber &&
			fmt.Sprintf("%s/%s", ws.Org, ws.Repo) == orgRepoPath {
			workspaces = append(workspaces, ws)
		}
	}

	return workspaces
}

func (m *Manager) GetWorkspaceByPRAndAI(pr *github.PullRequest, aiModel string) *models.Workspace {
	orgRepoPath := fmt.Sprintf("%s/%s", pr.GetBase().GetRepo().GetOwner().GetLogin(), pr.GetBase().GetRepo().GetName())
	prKey := keyWithAI(orgRepoPath, pr.GetNumber(), aiModel)
	m.mutex.RLock()
	if ws, exists := m.workspaces[prKey]; exists {
		m.mutex.RUnlock()
		log.Infof("Found existing workspace for PR #%d with AI model %s: %s", pr.GetNumber(), aiModel, ws.Path)
		return ws
	}
	m.mutex.RUnlock()
	return nil
}

// CreateWorkspaceFromPR 从 PR 创建工作空间（直接包含 PR 号）
func (m *Manager) CreateWorkspaceFromPR(pr *github.PullRequest) *models.Workspace {
	return m.CreateWorkspaceFromPRWithAI(pr, "")
}

// CreateWorkspaceFromPRWithAI 从 PR 创建工作空间，支持指定AI模型（使用git clone方式）
func (m *Manager) CreateWorkspaceFromPRWithAI(pr *github.PullRequest, aiModel string) *models.Workspace {
	log.Infof("Creating workspace from PR #%d with AI model: %s (using git clone)", pr.GetNumber(), aiModel)

	// 获取仓库 URL
	repoURL := pr.GetBase().GetRepo().GetCloneURL()
	if repoURL == "" {
		log.Errorf("Failed to get repository URL for PR #%d", pr.GetNumber())
		return nil
	}

	// 获取 PR 分支
	prBranch := pr.GetHead().GetRef()

	// 生成 PR 工作空间目录名，包含AI模型信息
	timestamp := time.Now().Unix()
	repo := pr.GetBase().GetRepo().GetName()
	prDir := m.GeneratePRDirName(aiModel, repo, pr.GetNumber(), timestamp)

	org := pr.GetBase().GetRepo().GetOwner().GetLogin()
	clonePath := filepath.Join(m.baseDir, org, prDir)

	// 克隆仓库到指定分支（不创建新分支，切换到现有分支）
	if err := m.cloneRepository(repoURL, clonePath, prBranch, false); err != nil {
		log.Errorf("Failed to clone repository for PR #%d: %v", pr.GetNumber(), err)
		return nil
	}

	// 创建 session 目录
	// 从PR目录名中提取suffix，支持新的目录格式：{aiModel}-{repo}-pr-{prNumber}-{timestamp}
	suffix := m.ExtractSuffixFromPRDir(aiModel, repo, pr.GetNumber(), prDir)
	sessionPath, err := m.CreateSessionPath(filepath.Join(m.baseDir, org), aiModel, repo, pr.GetNumber(), suffix)
	if err != nil {
		log.Errorf("Failed to create session directory: %v", err)
		return nil
	}

	// 创建工作空间对象
	ws := &models.Workspace{
		Org:         org,
		Repo:        repo,
		AIModel:     aiModel,
		PRNumber:    pr.GetNumber(),
		Path:        clonePath,
		SessionPath: sessionPath,
		Repository:  repoURL,
		Branch:      prBranch,
		PullRequest: pr,
		CreatedAt:   time.Now(),
	}

	// 注册到内存映射
	prKey := keyWithAI(fmt.Sprintf("%s/%s", org, repo), pr.GetNumber(), aiModel)
	m.mutex.Lock()
	m.workspaces[prKey] = ws
	m.mutex.Unlock()

	log.Infof("Created workspace from PR #%d using git clone: %s", pr.GetNumber(), ws.Path)
	return ws
}

// GetOrCreateWorkspaceForPR 获取或创建 PR 的工作空间
func (m *Manager) GetOrCreateWorkspaceForPR(pr *github.PullRequest) *models.Workspace {
	return m.GetOrCreateWorkspaceForPRWithAI(pr, "")
}

// GetOrCreateWorkspaceForPRWithAI 获取或创建 PR 的工作空间，支持指定AI模型
func (m *Manager) GetOrCreateWorkspaceForPRWithAI(pr *github.PullRequest, aiModel string) *models.Workspace {
	// 1. 先尝试从内存中获取对应AI模型的工作空间
	ws := m.GetWorkspaceByPRAndAI(pr, aiModel)
	if ws != nil {
		// 验证工作空间是否对应正确的 PR 分支
		if m.validateWorkspaceForPR(ws, pr) {
			return ws
		}
		// 如果验证失败，清理旧的工作空间
		log.Infof("Workspace validation failed for PR #%d with AI model %s, cleaning up old workspace", pr.GetNumber(), aiModel)
		m.CleanupWorkspace(ws)
	}

	// 2. 创建新的工作空间
	log.Infof("Creating new workspace for PR #%d with AI model: %s", pr.GetNumber(), aiModel)
	return m.CreateWorkspaceFromPRWithAI(pr, aiModel)
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
	// PR 目录格式:
	// - {aiModel}__{repo}__pr__{number}__{timestamp} (有AI模型)
	// - {repo}__pr__{number}__{timestamp} (无AI模型)
	if strings.Contains(prDir, "__pr__") {
		parts := strings.Split(prDir, "__pr__")
		if len(parts) >= 2 {
			numberParts := strings.Split(parts[1], "__")
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
	// Issue 目录格式:
	// - {aiModel}__{repo}__issue__{number}__{timestamp} (有AI模型)
	// - {repo}__issue__{number}__{timestamp} (无AI模型)
	if strings.Contains(issueDir, "__issue__") {
		parts := strings.Split(issueDir, "__issue__")
		if len(parts) >= 2 {
			numberParts := strings.Split(parts[1], "__")
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

// cleanupRelatedContainers 清理与工作空间相关的Docker容器
func (m *Manager) cleanupRelatedContainers(ws *models.Workspace) bool {
	if ws == nil {
		return false
	}

	// 基于工作空间的AI模型和PR信息构建容器名称
	var containerNames []string

	// 根据AI模型类型构建对应的容器名称
	switch ws.AIModel {
	case "claude":
		// 新的命名格式
		containerNames = append(containerNames, fmt.Sprintf("claude__%s__%s__%d", ws.Org, ws.Repo, ws.PRNumber))
		// 旧的命名格式（向后兼容）
		containerNames = append(containerNames, fmt.Sprintf("claude-%s-%s-%d", ws.Org, ws.Repo, ws.PRNumber))

		// 检查是否有interactive容器
		containerNames = append(containerNames, fmt.Sprintf("claude__interactive__%s__%s__%d", ws.Org, ws.Repo, ws.PRNumber))
		containerNames = append(containerNames, fmt.Sprintf("claude-interactive-%s-%s-%d", ws.Org, ws.Repo, ws.PRNumber))

	case "gemini":
		// 新的命名格式
		containerNames = append(containerNames, fmt.Sprintf("gemini__%s__%s__%d", ws.Org, ws.Repo, ws.PRNumber))
		// 旧的命名格式（向后兼容）
		containerNames = append(containerNames, fmt.Sprintf("gemini-%s-%s-%d", ws.Org, ws.Repo, ws.PRNumber))

	default:
		// 如果AI模型未知，尝试所有可能的模式
		containerNames = append(containerNames,
			fmt.Sprintf("claude__%s__%s__%d", ws.Org, ws.Repo, ws.PRNumber),
			fmt.Sprintf("gemini__%s__%s__%d", ws.Org, ws.Repo, ws.PRNumber),
			fmt.Sprintf("claude__interactive__%s__%s__%d", ws.Org, ws.Repo, ws.PRNumber),
			fmt.Sprintf("claude-%s-%s-%d", ws.Org, ws.Repo, ws.PRNumber),
			fmt.Sprintf("gemini-%s-%s-%d", ws.Org, ws.Repo, ws.PRNumber),
			fmt.Sprintf("claude-interactive-%s-%s-%d", ws.Org, ws.Repo, ws.PRNumber),
		)
	}

	removedCount := 0
	for _, containerName := range containerNames {
		if m.removeContainerIfExists(containerName) {
			removedCount++
			log.Infof("Successfully removed container: %s", containerName)
		}
	}

	return removedCount > 0 || len(containerNames) == 0
}

// removeContainerIfExists 如果容器存在则删除它
func (m *Manager) removeContainerIfExists(containerName string) bool {
	// 检查容器是否存在
	checkCmd := exec.Command("docker", "ps", "--filter", fmt.Sprintf("name=%s", containerName), "--format", "{{.Names}}")
	output, err := checkCmd.Output()
	if err != nil {
		log.Debugf("Failed to check container %s: %v", containerName, err)
		return false
	}

	containerStatus := strings.TrimSpace(string(output))
	if containerStatus == "" {
		// 容器不存在或未运行
		return false
	}

	// 强制删除容器
	removeCmd := exec.Command("docker", "rm", "-f", containerName)
	if err := removeCmd.Run(); err != nil {
		log.Errorf("Failed to remove container %s: %v", containerName, err)
		return false
	}

	return true
}
