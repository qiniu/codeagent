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

// RemoveWorktreeWithAI 移除指定 PR 和 AI 模型的 worktree
func (r *RepoManager) RemoveWorktreeWithAI(prNumber int, aiModel string) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	key := generateWorktreeKey(aiModel, prNumber)
	worktree := r.worktrees[key]
	if worktree == nil {
		log.Infof("Worktree for PR #%d with AI model %s not found in memory, skipping removal", prNumber, aiModel)
		return nil // 已经不存在
	}

	// 检查 worktree 目录是否存在
	if _, err := os.Stat(worktree.Worktree); os.IsNotExist(err) {
		log.Infof("Worktree directory %s does not exist, removing from memory only", worktree.Worktree)
		// 目录不存在，只从内存中移除
		delete(r.worktrees, key)
		return nil
	}

	// 删除 worktree
	cmd := exec.Command("git", "worktree", "remove", "--force", worktree.Worktree)
	cmd.Dir = r.repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Errorf("Failed to remove worktree: %v, output: %s", err, string(output))
		// 即使删除失败，也从映射中移除，避免内存状态不一致
		log.Warnf("Removing worktree from memory despite removal failure")
	} else {
		log.Infof("Successfully removed worktree: %s", worktree.Worktree)
	}

	// 删除相关的本地分支（如果存在）
	if worktree.Branch != "" {
		log.Infof("Attempting to delete local branch: %s", worktree.Branch)
		branchCmd := exec.Command("git", "branch", "-D", worktree.Branch)
		branchCmd.Dir = r.repoPath
		branchOutput, err := branchCmd.CombinedOutput()
		if err != nil {
			log.Warnf("Failed to delete local branch %s: %v, output: %s", worktree.Branch, err, string(branchOutput))
			// 分支删除失败不是致命错误，可能是分支不存在或正在使用
		} else {
			log.Infof("Successfully deleted local branch: %s", worktree.Branch)
		}
	}

	// 从映射中移除
	delete(r.worktrees, key)

	log.Infof("Removed worktree for PR #%d with AI model %s from memory", prNumber, aiModel)
	return nil
}

// ListWorktrees 列出所有 worktree
func (r *RepoManager) ListWorktrees() ([]*WorktreeInfo, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	// 获取 Git worktree 列表
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = r.repoPath
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list worktrees: %w", err)
	}

	return r.parseWorktreeList(string(output))
}

// parseWorktreeList 解析 worktree 列表输出
func (r *RepoManager) parseWorktreeList(output string) ([]*WorktreeInfo, error) {
	var worktrees []*WorktreeInfo
	lines := strings.Split(strings.TrimSpace(output), "\n")

	log.Infof("Parsing worktree list output: %s", output)

	// 过滤掉空行
	var filteredLines []string
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			filteredLines = append(filteredLines, line)
		}
	}

	for i := 0; i < len(filteredLines); i += 3 {
		if i+2 >= len(filteredLines) {
			break
		}

		// 解析 worktree 路径（第一行）
		pathLine := strings.TrimSpace(filteredLines[i])
		if !strings.HasPrefix(pathLine, "worktree ") {
			log.Warnf("Invalid worktree line: %s", pathLine)
			continue
		}
		path := strings.TrimPrefix(pathLine, "worktree ")

		// 跳过 HEAD 行（第二行）
		headLine := strings.TrimSpace(filteredLines[i+1])
		if !strings.HasPrefix(headLine, "HEAD ") {
			log.Warnf("Invalid HEAD line: %s", headLine)
			continue
		}
		head := strings.TrimPrefix(headLine, "HEAD ")

		// 解析分支信息（第三行）
		branchLine := strings.TrimSpace(filteredLines[i+2])
		var branch string
		if !strings.HasPrefix(branchLine, "branch ") {
			log.Warnf("Invalid branch line: %s", branchLine)
			continue
		}
		branch = strings.TrimPrefix(branchLine, "branch ")

		worktree := &WorktreeInfo{
			Worktree: path,
			Head:     head,
			Branch:   branch,
		}
		log.Infof("Found worktree: %s, head: %s, branch: %s", path, head, branch)
		worktrees = append(worktrees, worktree)
	}

	log.Infof("Parsed %d worktrees", len(worktrees))
	return worktrees, nil
}

// CreateWorktreeWithName 使用指定名称创建 worktree
func (r *RepoManager) CreateWorktreeWithName(worktreeName string, branch string, createNewBranch bool) (*WorktreeInfo, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	log.Infof("Creating worktree with name: %s, branch: %s, createNewBranch: %v", worktreeName, branch, createNewBranch)

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
			// 不因为更新失败而阻止worktree创建，但记录警告
		}
	}

	// 创建 worktree 路径（与仓库目录同级）
	orgDir := filepath.Dir(r.repoPath)
	worktreePath := filepath.Join(orgDir, worktreeName)
	log.Infof("Worktree path: %s", worktreePath)

	// 检查是否存在现有的 worktree 使用相同分支
	if err := r.handleExistingWorktree(branch, worktreePath); err != nil {
		if err.Error() == "worktree_exists_at_target_path" {
			// 工作树已存在于目标路径，直接返回现有的信息
			log.Infof("Reusing existing worktree at: %s", worktreePath)
			return &WorktreeInfo{
				Worktree: worktreePath,
				Branch:   branch,
			}, nil
		}
		log.Errorf("Failed to handle existing worktree: %v", err)
		return nil, err
	}

	// 创建 worktree
	var cmd *exec.Cmd
	if createNewBranch {
		// 创建新分支的 worktree
		// 首先检查默认分支是什么
		log.Infof("Checking default branch for new branch creation")
		defaultBranchCmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
		defaultBranchCmd.Dir = r.repoPath
		defaultBranchOutput, err := defaultBranchCmd.Output()
		if err != nil {
			log.Errorf("Failed to get default branch, using 'main': %v", err)
			defaultBranchOutput = []byte("main")
		}
		defaultBranch := strings.TrimSpace(string(defaultBranchOutput))
		if defaultBranch == "" {
			defaultBranch = "main"
		}

		log.Infof("Creating new branch worktree: git worktree add -b %s %s %s", branch, worktreePath, defaultBranch)
		cmd = exec.Command("git", "worktree", "add", "-b", branch, worktreePath, defaultBranch)
	} else {
		// 创建现有分支的 worktree
		// 首先检查本地分支是否已经存在
		log.Infof("Checking if local branch exists: %s", branch)
		localBranchCmd := exec.Command("git", "show-ref", "--verify", "--quiet", fmt.Sprintf("refs/heads/%s", branch))
		localBranchCmd.Dir = r.repoPath
		localBranchExists := localBranchCmd.Run() == nil

		if localBranchExists {
			log.Infof("Local branch %s already exists, creating worktree without -b flag", branch)
			// 本地分支已存在，直接创建 worktree 而不使用 -b 标志
			cmd = exec.Command("git", "worktree", "add", worktreePath, branch)
		} else {
			// 本地分支不存在，检查远程分支是否存在
			log.Infof("Local branch does not exist, checking if remote branch exists: origin/%s", branch)
			checkCmd := exec.Command("git", "ls-remote", "--heads", "origin", branch)
			checkCmd.Dir = r.repoPath
			checkOutput, err := checkCmd.CombinedOutput()
			if err != nil {
				log.Errorf("Failed to check remote branch: %v, output: %s", err, string(checkOutput))
			} else if strings.TrimSpace(string(checkOutput)) == "" {
				log.Errorf("Remote branch origin/%s does not exist, will create new branch", branch)
				// 如果远程分支不存在，创建新分支
				defaultBranchCmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
				defaultBranchCmd.Dir = r.repoPath
				defaultBranchOutput, err := defaultBranchCmd.Output()
				if err != nil {
					log.Warnf("Failed to get default branch, using 'main': %v", err)
					defaultBranchOutput = []byte("main")
				}
				defaultBranch := strings.TrimSpace(string(defaultBranchOutput))
				if defaultBranch == "" {
					defaultBranch = "main"
				}
				cmd = exec.Command("git", "worktree", "add", "-b", branch, worktreePath, defaultBranch)
			} else {
				log.Infof("Remote branch exists, creating worktree: git worktree add -b %s %s origin/%s", branch, worktreePath, branch)
				cmd = exec.Command("git", "worktree", "add", "-b", branch, worktreePath, fmt.Sprintf("origin/%s", branch))
			}
		}
	}

	if cmd == nil {
		// 如果还没有设置命令，使用默认的创建新分支方式
		log.Infof("Using default new branch creation: git worktree add -b %s %s main", branch, worktreePath)
		cmd = exec.Command("git", "worktree", "add", "-b", branch, worktreePath, "main")
	}

	cmd.Dir = r.repoPath

	log.Infof("Executing command: %s", strings.Join(cmd.Args, " "))
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Errorf("Failed to create worktree: %v, output: %s", err, string(output))
		return nil, fmt.Errorf("failed to create worktree: %w, output: %s", err, string(output))
	}

	// 配置 Git 安全目录
	cmd = exec.Command("git", "config", "--local", "--add", "safe.directory", worktreePath)
	cmd.Dir = worktreePath // 在 worktree 目录下配置安全目录
	configOutput, err := cmd.CombinedOutput()
	if err != nil {
		log.Warnf("Failed to configure safe directory: %v\nCommand output: %s", err, string(configOutput))
	} else {
		log.Infof("Successfully configured safe directory: %s", worktreePath)
	}

	log.Infof("Worktree creation output: %s", string(output))

	// 创建 worktree 信息
	worktree := &WorktreeInfo{
		Worktree: worktreePath,
		Branch:   branch,
	}

	log.Infof("Successfully created worktree: %s", worktreePath)
	return worktree, nil
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
	log.Infof("Fetched latest changes for main repository")

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

// handleExistingWorktree 处理已存在的 worktree 冲突
func (r *RepoManager) handleExistingWorktree(branch string, targetPath string) error {
	log.Infof("Checking for existing worktrees using branch: %s", branch)

	// 获取当前所有 worktree
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = r.repoPath
	output, err := cmd.Output()
	if err != nil {
		log.Warnf("Failed to list worktrees: %v", err)
		return nil // 不阻止创建，继续执行
	}

	worktrees, err := r.parseWorktreeList(string(output))
	if err != nil {
		log.Warnf("Failed to parse worktree list: %v", err)
		return nil // 不阻止创建，继续执行
	}

	// 检查是否有 worktree 正在使用相同的分支
	for _, wt := range worktrees {
		if strings.Contains(wt.Branch, branch) || strings.Contains(branch, strings.TrimPrefix(wt.Branch, "refs/heads/")) {
			log.Warnf("Found existing worktree using branch %s at path: %s", branch, wt.Worktree)

			// 如果目标路径和现有路径不同，需要清理现有的 worktree
			if wt.Worktree != targetPath {
				log.Infof("Removing conflicting worktree: %s", wt.Worktree)

				// 强制删除现有的 worktree
				removeCmd := exec.Command("git", "worktree", "remove", "--force", wt.Worktree)
				removeCmd.Dir = r.repoPath
				removeOutput, removeErr := removeCmd.CombinedOutput()
				if removeErr != nil {
					log.Errorf("Failed to remove existing worktree %s: %v, output: %s",
						wt.Worktree, removeErr, string(removeOutput))

					// 尝试手动删除目录
					if err := r.forceRemoveDirectory(wt.Worktree); err != nil {
						log.Errorf("Failed to manually remove directory %s: %v", wt.Worktree, err)
						return fmt.Errorf("failed to clean up conflicting worktree: %w", err)
					}
				} else {
					log.Infof("Successfully removed conflicting worktree: %s", wt.Worktree)
				}

				// 尝试删除相关的本地分支（如果存在且不是主分支）
				branchName := strings.TrimPrefix(wt.Branch, "refs/heads/")
				if branchName != "main" && branchName != "master" && branchName != "" {
					log.Infof("Attempting to delete local branch: %s", branchName)
					branchCmd := exec.Command("git", "branch", "-D", branchName)
					branchCmd.Dir = r.repoPath
					branchOutput, err := branchCmd.CombinedOutput()
					if err != nil {
						log.Warnf("Failed to delete local branch %s: %v, output: %s",
							branchName, err, string(branchOutput))
					} else {
						log.Infof("Successfully deleted local branch: %s", branchName)
					}
				}
			} else {
				// 目标路径相同，检查是否可以复用
				if _, err := os.Stat(wt.Worktree); err == nil {
					log.Infof("Worktree already exists at target path, will reuse: %s", wt.Worktree)
					return fmt.Errorf("worktree_exists_at_target_path")
				}
			}
		}
	}

	return nil
}

// forceRemoveDirectory 强制删除目录
func (r *RepoManager) forceRemoveDirectory(dirPath string) error {
	log.Infof("Force removing directory: %s", dirPath)

	// 检查目录是否存在
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		log.Infof("Directory does not exist: %s", dirPath)
		return nil
	}

	// 尝试删除目录
	if err := os.RemoveAll(dirPath); err != nil {
		log.Errorf("Failed to remove directory %s: %v", dirPath, err)
		return err
	}

	log.Infof("Successfully removed directory: %s", dirPath)
	return nil
}

// RestoreWorktrees 扫描磁盘上的 worktree 并注册到内存
func (r *RepoManager) RestoreWorktrees() error {
	worktrees, err := r.ListWorktrees()
	if err != nil {
		return err
	}
	for _, wt := range worktrees {
		// 只处理含 -pr- 的 worktree 目录
		base := filepath.Base(wt.Worktree)
		if strings.Contains(base, "-pr-") {
			log.Infof("Parsing worktree directory name: %s", base)

			// 解析目录名格式：{aiModel}-{repo-name-with-dashes}-pr-{prNumber}-{timestamp}
			// 使用 -pr- 作为分隔符来准确分割
			prIndex := strings.Index(base, "-pr-")
			if prIndex == -1 {
				log.Warnf("Invalid worktree name format (no -pr- found): %s", base)
				continue
			}

			// 提取前缀部分（AI模型和仓库名）
			prefix := base[:prIndex]

			// 提取后缀部分（PR编号和时间戳）
			suffix := base[prIndex+4:] // 跳过 "-pr-"
			suffixParts := strings.Split(suffix, "-")
			if len(suffixParts) < 2 {
				log.Warnf("Invalid worktree name format (insufficient suffix parts): %s", base)
				continue
			}

			// 解析PR编号
			prNumber, err := strconv.Atoi(suffixParts[0])
			if err != nil {
				log.Warnf("Invalid PR number in worktree name: %s, error: %v", base, err)
				continue
			}

			// 提取AI模型（从前缀的第一部分）
			prefixParts := strings.Split(prefix, "-")
			if len(prefixParts) < 2 {
				log.Warnf("Invalid worktree name format (insufficient prefix parts): %s", base)
				continue
			}

			aiModel := prefixParts[0]
			repoName := strings.Join(prefixParts[1:], "-")

			log.Infof("Parsed worktree: aiModel=%s, repo=%s, prNumber=%d", aiModel, repoName, prNumber)

			// 验证AI模型是否有效
			if aiModel == "gemini" || aiModel == "claude" {
				r.RegisterWorktreeWithAI(prNumber, aiModel, wt)
				log.Infof("Restored worktree for PR #%d with AI model %s: %s", prNumber, aiModel, wt.Worktree)
			} else {
				// 如果第一部分不是有效的AI模型，可能是旧格式或者其他格式
				// 尝试向后兼容处理
				log.Warnf("Unknown AI model '%s' in worktree name: %s", aiModel, base)

				// 检查是否是旧格式（没有AI模型前缀）
				if strings.Contains(base, "issue-") {
					// 可能是Issue工作空间，跳过
					log.Infof("Skipping Issue worktree: %s", base)
					continue
				}

				// 使用默认方式注册
				r.RegisterWorktree(prNumber, wt)
				log.Infof("Restored worktree for PR #%d (unknown AI model): %s", prNumber, wt.Worktree)
			}
		} else if strings.Contains(base, "issue-") {
			// 处理Issue工作空间（可选，用于调试）
			log.Infof("Found Issue worktree (not registering): %s", base)
		}
	}
	return nil
}
