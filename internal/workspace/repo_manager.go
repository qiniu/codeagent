package workspace

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"strconv"

	"github.com/qiniu/x/log"
)

// RepoManager 仓库管理器，负责管理单个仓库的 worktree
type RepoManager struct {
	repoPath  string
	repoURL   string
	worktrees map[string]*WorktreeInfo // key: "aiModel-prNumber" 或 "prNumber" (向后兼容)
	mutex     sync.RWMutex
}

// WorktreeInfo worktree 信息
// 例子:
// worktree /Users/jicarl/codeagent/qbox/codeagent
// HEAD 6446817fba0a257f73b311c93126041b63ab6f78
// branch refs/heads/main

// worktree /Users/jicarl/codeagent/qbox/codeagent/issue-11-1752143989
// HEAD 5c2df7724d26a27c154b90f519b6d4f4efdd1436
// branch refs/heads/codeagent/issue-11-1752143989
type WorktreeInfo struct {
	Worktree string
	Head     string
	Branch   string
}

// generateWorktreeKey 生成 worktree 的 key
func generateWorktreeKey(aiModel string, prNumber int) string {
	if aiModel != "" {
		return fmt.Sprintf("%s-%d", aiModel, prNumber)
	}
	return fmt.Sprintf("%d", prNumber)
}

// NewRepoManager 创建新的仓库管理器
func NewRepoManager(repoPath, repoURL string) *RepoManager {
	return &RepoManager{
		repoPath:  repoPath,
		repoURL:   repoURL,
		worktrees: make(map[string]*WorktreeInfo),
	}
}

// Initialize 初始化仓库（首次克隆）
func (r *RepoManager) Initialize() error {
	log.Infof("Starting repository initialization: %s", r.repoPath)

	// 创建仓库目录
	if err := os.MkdirAll(r.repoPath, 0755); err != nil {
		return fmt.Errorf("failed to create repo directory: %w", err)
	}

	// 克隆仓库（带超时）
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "clone", r.repoURL, ".")
	cmd.Dir = r.repoPath

	log.Infof("Executing git clone: %s", strings.Join(cmd.Args, " "))

	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("git clone timed out after 5 minutes: %w", err)
		}
		return fmt.Errorf("failed to clone repository: %w, output: %s", err, string(output))
	}

	// 配置 Git 安全目录
	cmd = exec.Command("git", "config", "--local", "--add", "safe.directory", r.repoPath)
	cmd.Dir = r.repoPath
	configOutput, err := cmd.CombinedOutput()
	if err != nil {
		log.Warnf("Failed to configure safe directory: %v\nCommand output: %s", err, string(configOutput))
	}

	// 配置 rebase 为默认拉取策略
	cmd = exec.Command("git", "config", "--local", "pull.rebase", "true")
	cmd.Dir = r.repoPath
	rebaseConfigOutput, err := cmd.CombinedOutput()
	if err != nil {
		log.Warnf("Failed to configure pull.rebase: %v\nCommand output: %s", err, string(rebaseConfigOutput))
	}

	log.Infof("Successfully initialized repository: %s", r.repoPath)
	return nil
}

// isInitialized 检查仓库是否已初始化
func (r *RepoManager) isInitialized() bool {
	gitDir := filepath.Join(r.repoPath, ".git")
	_, err := os.Stat(gitDir)
	return err == nil
}

// GetWorktree 获取指定 PR 的 worktree（向后兼容，默认无AI模型）
func (r *RepoManager) GetWorktree(prNumber int) *WorktreeInfo {
	return r.GetWorktreeWithAI(prNumber, "")
}

// GetWorktreeWithAI 获取指定 PR 和 AI 模型的 worktree
func (r *RepoManager) GetWorktreeWithAI(prNumber int, aiModel string) *WorktreeInfo {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	key := generateWorktreeKey(aiModel, prNumber)
	return r.worktrees[key]
}

// RemoveWorktree 移除指定 PR 的 worktree（向后兼容，默认无AI模型）
func (r *RepoManager) RemoveWorktree(prNumber int) error {
	return r.RemoveWorktreeWithAI(prNumber, "")
}

// RemoveWorktreeWithAI 移除指定 PR 和 AI 模型的工作空间
func (r *RepoManager) RemoveWorktreeWithAI(prNumber int, aiModel string) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	key := generateWorktreeKey(aiModel, prNumber)
	workspace := r.worktrees[key]
	if workspace == nil {
		log.Infof("Workspace for PR #%d with AI model %s not found in memory, skipping removal", prNumber, aiModel)
		return nil // 已经不存在
	}

	// 检查工作空间目录是否存在
	if _, err := os.Stat(workspace.Worktree); os.IsNotExist(err) {
		log.Infof("Workspace directory %s does not exist, removing from memory only", workspace.Worktree)
		// 目录不存在，只从内存中移除
		delete(r.worktrees, key)
		return nil
	}

	// 直接删除工作空间目录
	log.Infof("Removing workspace directory: %s", workspace.Worktree)
	err := os.RemoveAll(workspace.Worktree)
	if err != nil {
		log.Errorf("Failed to remove workspace directory: %v", err)
		// 即使删除失败，也从映射中移除，避免内存状态不一致
		log.Warnf("Removing workspace from memory despite removal failure")
	} else {
		log.Infof("Successfully removed workspace: %s", workspace.Worktree)
	}

	// 从映射中移除
	delete(r.worktrees, key)

	log.Infof("Removed workspace for PR #%d with AI model %s from memory", prNumber, aiModel)
	return nil
}

// ListWorktrees 列出所有工作空间
func (r *RepoManager) ListWorktrees() ([]*WorktreeInfo, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	var workspaces []*WorktreeInfo

	// 扫描与仓库同级的目录，寻找工作空间
	orgDir := filepath.Dir(r.repoPath)
	entries, err := os.ReadDir(orgDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read org directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		workspacePath := filepath.Join(orgDir, entry.Name())

		// 跳过主仓库目录
		if workspacePath == r.repoPath {
			continue
		}

		// 检查是否是一个有效的 Git 仓库
		gitDir := filepath.Join(workspacePath, ".git")
		if _, err := os.Stat(gitDir); os.IsNotExist(err) {
			continue
		}

		// 获取当前分支
		branchCmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
		branchCmd.Dir = workspacePath
		branchOutput, err := branchCmd.Output()
		if err != nil {
			log.Warnf("Failed to get branch for workspace %s: %v", workspacePath, err)
			continue
		}
		branch := strings.TrimSpace(string(branchOutput))

		// 获取当前 commit
		commitCmd := exec.Command("git", "rev-parse", "HEAD")
		commitCmd.Dir = workspacePath
		commitOutput, err := commitCmd.Output()
		if err != nil {
			log.Warnf("Failed to get commit for workspace %s: %v", workspacePath, err)
			continue
		}
		commit := strings.TrimSpace(string(commitOutput))

		workspace := &WorktreeInfo{
			Worktree: workspacePath,
			Head:     commit,
			Branch:   branch,
		}

		workspaces = append(workspaces, workspace)
		log.Infof("Found workspace: %s, branch: %s, commit: %s", workspacePath, branch, commit)
	}

	log.Infof("Found %d workspaces", len(workspaces))
	return workspaces, nil
}

// CreateWorktreeWithName 使用指定名称创建工作空间（通过 git clone）
func (r *RepoManager) CreateWorktreeWithName(worktreeName string, branch string, createNewBranch bool) (*WorktreeInfo, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	log.Infof("Creating workspace with name: %s, branch: %s, createNewBranch: %v", worktreeName, branch, createNewBranch)

	// 确保仓库已初始化
	if !r.isInitialized() {
		log.Infof("Repository not initialized, initializing: %s", r.repoPath)
		if err := r.Initialize(); err != nil {
			return nil, err
		}
	} else {
		// 仓库已存在，确保主仓库代码是最新的
		if err := r.updateMainRepository(); err != nil {
			log.Warnf("Failed to update main repository: %v", err)
			// 不因为更新失败而阻止workspace创建，但记录警告
		}
	}

	// 创建工作空间路径（与仓库目录同级）
	orgDir := filepath.Dir(r.repoPath)
	workspacePath := filepath.Join(orgDir, worktreeName)
	log.Infof("Workspace path: %s", workspacePath)

	// 检查目标路径是否已存在
	if _, err := os.Stat(workspacePath); err == nil {
		log.Infof("Workspace already exists at: %s, removing old one", workspacePath)
		if err := os.RemoveAll(workspacePath); err != nil {
			return nil, fmt.Errorf("failed to remove existing workspace: %w", err)
		}
	}

	// 使用 git clone 创建工作空间
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// 从本地仓库克隆到工作空间
	cloneCmd := exec.CommandContext(ctx, "git", "clone", r.repoPath, workspacePath)
	log.Infof("Executing clone command: %s", strings.Join(cloneCmd.Args, " "))
	cloneOutput, err := cloneCmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("git clone timed out after 5 minutes: %w", err)
		}
		return nil, fmt.Errorf("failed to clone repository: %w, output: %s", err, string(cloneOutput))
	}
	log.Infof("Clone output: %s", string(cloneOutput))

	// 配置 Git 安全目录
	cmd := exec.Command("git", "config", "--local", "--add", "safe.directory", workspacePath)
	cmd.Dir = workspacePath
	configOutput, err := cmd.CombinedOutput()
	if err != nil {
		log.Warnf("Failed to configure safe directory: %v\nCommand output: %s", err, string(configOutput))
	} else {
		log.Infof("Successfully configured safe directory: %s", workspacePath)
	}

	// 配置 rebase 为默认拉取策略
	cmd = exec.Command("git", "config", "--local", "pull.rebase", "true")
	cmd.Dir = workspacePath
	rebaseConfigOutput, err := cmd.CombinedOutput()
	if err != nil {
		log.Warnf("Failed to configure pull.rebase: %v\nCommand output: %s", err, string(rebaseConfigOutput))
	}

	// 处理分支切换
	if createNewBranch {
		// 创建并切换到新分支
		log.Infof("Creating and checking out new branch: %s", branch)

		// 首先检查默认分支
		defaultBranchCmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
		defaultBranchCmd.Dir = workspacePath
		defaultBranchOutput, err := defaultBranchCmd.Output()
		if err != nil {
			log.Errorf("Failed to get default branch, using 'main': %v", err)
			defaultBranchOutput = []byte("main")
		}
		defaultBranch := strings.TrimSpace(string(defaultBranchOutput))
		if defaultBranch == "" {
			defaultBranch = "main"
		}

		// 创建新分支
		branchCmd := exec.Command("git", "checkout", "-b", branch, defaultBranch)
		branchCmd.Dir = workspacePath
		branchOutput, err := branchCmd.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("failed to create new branch %s: %w, output: %s", branch, err, string(branchOutput))
		}
		log.Infof("Created new branch: %s", branch)
	} else {
		// 切换到现有分支
		log.Infof("Checking out existing branch: %s", branch)

		// 首先检查本地是否存在该分支
		localBranchCmd := exec.Command("git", "show-ref", "--verify", "--quiet", fmt.Sprintf("refs/heads/%s", branch))
		localBranchCmd.Dir = workspacePath
		localBranchExists := localBranchCmd.Run() == nil

		if localBranchExists {
			// 本地分支存在，直接切换
			checkoutCmd := exec.Command("git", "checkout", branch)
			checkoutCmd.Dir = workspacePath
			checkoutOutput, err := checkoutCmd.CombinedOutput()
			if err != nil {
				return nil, fmt.Errorf("failed to checkout local branch %s: %w, output: %s", branch, err, string(checkoutOutput))
			}
		} else {
			// 检查远程分支是否存在
			remoteBranchCmd := exec.Command("git", "ls-remote", "--heads", "origin", branch)
			remoteBranchCmd.Dir = workspacePath
			remoteBranchOutput, err := remoteBranchCmd.CombinedOutput()
			if err != nil {
				log.Errorf("Failed to check remote branch: %v, output: %s", err, string(remoteBranchOutput))
			}

			if strings.TrimSpace(string(remoteBranchOutput)) != "" {
				// 远程分支存在，创建本地跟踪分支
				trackCmd := exec.Command("git", "checkout", "-b", branch, fmt.Sprintf("origin/%s", branch))
				trackCmd.Dir = workspacePath
				trackOutput, err := trackCmd.CombinedOutput()
				if err != nil {
					return nil, fmt.Errorf("failed to checkout remote branch %s: %w, output: %s", branch, err, string(trackOutput))
				}
				log.Infof("Created local tracking branch for origin/%s", branch)
			} else {
				// 远程分支不存在，创建新分支
				log.Warnf("Remote branch origin/%s does not exist, creating new branch", branch)
				defaultBranchCmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
				defaultBranchCmd.Dir = workspacePath
				defaultBranchOutput, err := defaultBranchCmd.Output()
				if err != nil {
					log.Warnf("Failed to get default branch, using 'main': %v", err)
					defaultBranchOutput = []byte("main")
				}
				defaultBranch := strings.TrimSpace(string(defaultBranchOutput))
				if defaultBranch == "" {
					defaultBranch = "main"
				}

				newBranchCmd := exec.Command("git", "checkout", "-b", branch, defaultBranch)
				newBranchCmd.Dir = workspacePath
				newBranchOutput, err := newBranchCmd.CombinedOutput()
				if err != nil {
					return nil, fmt.Errorf("failed to create new branch %s from %s: %w, output: %s", branch, defaultBranch, err, string(newBranchOutput))
				}
				log.Infof("Created new branch %s from %s", branch, defaultBranch)
			}
		}
	}

	// 创建工作空间信息
	workspace := &WorktreeInfo{
		Worktree: workspacePath,
		Branch:   branch,
	}

	log.Infof("Successfully created workspace: %s on branch %s", workspacePath, branch)
	return workspace, nil
}

// RegisterWorktree 注册单个 worktree 到内存（向后兼容，默认无AI模型）
func (r *RepoManager) RegisterWorktree(prNumber int, worktree *WorktreeInfo) {
	r.RegisterWorktreeWithAI(prNumber, "", worktree)
}

// RegisterWorktreeWithAI 注册单个 worktree 到内存（支持AI模型）
func (r *RepoManager) RegisterWorktreeWithAI(prNumber int, aiModel string, worktree *WorktreeInfo) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	key := generateWorktreeKey(aiModel, prNumber)
	r.worktrees[key] = worktree
}

// GetRepoPath 获取仓库路径
func (r *RepoManager) GetRepoPath() string {
	return r.repoPath
}

// GetRepoURL 获取仓库 URL
func (r *RepoManager) GetRepoURL() string {
	return r.repoURL
}

// GetWorktreeCount 获取 worktree 数量
func (r *RepoManager) GetWorktreeCount() int {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return len(r.worktrees)
}

// updateMainRepository 更新主仓库代码到最新版本
func (r *RepoManager) updateMainRepository() error {
	log.Infof("Updating main repository: %s", r.repoPath)

	// 1. 获取远程最新引用
	cmd := exec.Command("git", "fetch", "--all", "--prune")
	cmd.Dir = r.repoPath
	fetchOutput, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to fetch latest changes: %w, output: %s", err, string(fetchOutput))
	}

	// 2. 获取当前分支
	cmd = exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = r.repoPath
	currentBranchOutput, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get current branch: %w", err)
	}
	currentBranch := strings.TrimSpace(string(currentBranchOutput))

	// 3. 检查是否有未提交的变更
	cmd = exec.Command("git", "status", "--porcelain")
	cmd.Dir = r.repoPath
	statusOutput, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to check status: %w", err)
	}

	hasChanges := strings.TrimSpace(string(statusOutput)) != ""
	if hasChanges {
		// 主仓库不应该有未提交的变更，这违反了最佳实践
		log.Warnf("Main repository has uncommitted changes, this violates best practices")
		log.Warnf("Uncommitted changes:\n%s", string(statusOutput))

		// 为了安全，暂存这些变更
		cmd = exec.Command("git", "stash", "push", "-m", "Auto-stash from updateMainRepository")
		cmd.Dir = r.repoPath
		stashOutput, err := cmd.CombinedOutput()
		if err != nil {
			log.Warnf("Failed to stash changes: %v, output: %s", err, string(stashOutput))
		} else {
			log.Infof("Stashed uncommitted changes in main repository")
		}
	}

	// 4. 使用 rebase 更新到最新版本
	remoteBranch := fmt.Sprintf("origin/%s", currentBranch)
	cmd = exec.Command("git", "rebase", remoteBranch)
	cmd.Dir = r.repoPath
	rebaseOutput, err := cmd.CombinedOutput()
	if err != nil {
		// rebase 失败，尝试 reset 到远程分支
		log.Warnf("Rebase failed, attempting hard reset: %v, output: %s", err, string(rebaseOutput))

		cmd = exec.Command("git", "reset", "--hard", remoteBranch)
		cmd.Dir = r.repoPath
		resetOutput, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to reset to remote branch: %w, output: %s", err, string(resetOutput))
		}
		log.Infof("Hard reset main repository to %s", remoteBranch)
	} else {
		log.Infof("Successfully rebased main repository to %s", remoteBranch)
	}

	// 5. 清理无用的引用
	cmd = exec.Command("git", "gc", "--auto")
	cmd.Dir = r.repoPath
	gcOutput, err := cmd.CombinedOutput()
	if err != nil {
		log.Warnf("Failed to run git gc: %v, output: %s", err, string(gcOutput))
	}

	log.Infof("Main repository updated successfully")
	return nil
}

// EnsureMainRepositoryUpToDate 确保主仓库是最新的（公开方法，可被外部调用）
func (r *RepoManager) EnsureMainRepositoryUpToDate() error {
	if !r.isInitialized() {
		return fmt.Errorf("repository not initialized")
	}
	return r.updateMainRepository()
}

// RestoreWorktrees 扫描磁盘上的工作空间并注册到内存
func (r *RepoManager) RestoreWorktrees() error {
	workspaces, err := r.ListWorktrees()
	if err != nil {
		return err
	}
	for _, ws := range workspaces {
		// 处理工作空间目录名，提取PR信息
		base := filepath.Base(ws.Worktree)

		// 检查是否包含 __pr__ (新格式) 或 -pr- (旧格式)
		if strings.Contains(base, "__pr__") {
			log.Infof("Parsing workspace directory name (new format): %s", base)

			// 新格式：{aiModel}__{repo}__pr__{prNumber}__{timestamp}
			parts := strings.Split(base, "__pr__")
			if len(parts) != 2 {
				log.Warnf("Invalid workspace name format (new): %s", base)
				continue
			}

			// 提取PR编号
			suffixParts := strings.Split(parts[1], "__")
			if len(suffixParts) < 1 {
				log.Warnf("Invalid workspace name format (no PR number): %s", base)
				continue
			}

			prNumber, err := strconv.Atoi(suffixParts[0])
			if err != nil {
				log.Warnf("Invalid PR number in workspace name: %s, error: %v", base, err)
				continue
			}

			// 提取AI模型
			prefixParts := strings.Split(parts[0], "__")
			var aiModel string
			if len(prefixParts) >= 2 {
				aiModel = prefixParts[0]
				if aiModel == "gemini" || aiModel == "claude" {
					r.RegisterWorktreeWithAI(prNumber, aiModel, ws)
					log.Infof("Restored workspace for PR #%d with AI model %s: %s", prNumber, aiModel, ws.Worktree)
				} else {
					// 向后兼容处理
					r.RegisterWorktree(prNumber, ws)
					log.Infof("Restored workspace for PR #%d (unknown AI model): %s", prNumber, ws.Worktree)
				}
			} else {
				r.RegisterWorktree(prNumber, ws)
				log.Infof("Restored workspace for PR #%d (no AI model): %s", prNumber, ws.Worktree)
			}
		} else if strings.Contains(base, "-pr-") {
			log.Infof("Parsing workspace directory name (old format): %s", base)

			// 旧格式：{aiModel}-{repo}-pr-{prNumber}-{timestamp}
			prIndex := strings.Index(base, "-pr-")
			if prIndex == -1 {
				log.Warnf("Invalid workspace name format (old, no -pr- found): %s", base)
				continue
			}

			// 提取后缀部分（PR编号和时间戳）
			suffix := base[prIndex+4:] // 跳过 "-pr-"
			suffixParts := strings.Split(suffix, "-")
			if len(suffixParts) < 1 {
				log.Warnf("Invalid workspace name format (old, insufficient suffix parts): %s", base)
				continue
			}

			// 解析PR编号
			prNumber, err := strconv.Atoi(suffixParts[0])
			if err != nil {
				log.Warnf("Invalid PR number in workspace name: %s, error: %v", base, err)
				continue
			}

			// 提取AI模型（从前缀的第一部分）
			prefix := base[:prIndex]
			prefixParts := strings.Split(prefix, "-")
			if len(prefixParts) >= 1 {
				aiModel := prefixParts[0]
				if aiModel == "gemini" || aiModel == "claude" {
					r.RegisterWorktreeWithAI(prNumber, aiModel, ws)
					log.Infof("Restored workspace for PR #%d with AI model %s: %s", prNumber, aiModel, ws.Worktree)
				} else {
					// 向后兼容处理
					r.RegisterWorktree(prNumber, ws)
					log.Infof("Restored workspace for PR #%d (unknown AI model): %s", prNumber, ws.Worktree)
				}
			} else {
				r.RegisterWorktree(prNumber, ws)
				log.Infof("Restored workspace for PR #%d (no AI model): %s", prNumber, ws.Worktree)
			}
		} else if strings.Contains(base, "__issue__") || strings.Contains(base, "issue-") {
			// Issue 工作空间，暂时跳过
			log.Infof("Found Issue workspace (not registering): %s", base)
		} else {
			log.Debugf("Skipping non-workspace directory: %s", base)
		}
	}
	return nil
}
