package workspace

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/qiniu/x/log"
)

// RepoManager 仓库管理器，负责管理单个仓库的 worktree
type RepoManager struct {
	repoPath  string
	repoURL   string
	worktrees map[int]*WorktreeInfo
	mutex     sync.RWMutex
}

// WorktreeInfo worktree 信息
type WorktreeInfo struct {
	PRNumber  int
	Path      string
	Branch    string
	CreatedAt time.Time
}

// NewRepoManager 创建新的仓库管理器
func NewRepoManager(repoPath, repoURL string) *RepoManager {
	return &RepoManager{
		repoPath:  repoPath,
		repoURL:   repoURL,
		worktrees: make(map[int]*WorktreeInfo),
	}
}

// Initialize 初始化仓库（首次克隆）
func (r *RepoManager) Initialize() error {
	// 检查是否已经初始化
	if r.isInitialized() {
		log.Infof("Repository already initialized: %s", r.repoPath)
		return nil
	}

	log.Infof("Starting repository initialization: %s -> %s", r.repoURL, r.repoPath)

	// 创建仓库目录
	if err := os.MkdirAll(r.repoPath, 0755); err != nil {
		return fmt.Errorf("failed to create repo directory: %w", err)
	}

	// 克隆仓库（带超时）
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
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

	log.Infof("Successfully initialized repository: %s", r.repoPath)
	return nil
}

// isInitialized 检查仓库是否已初始化
func (r *RepoManager) isInitialized() bool {
	gitDir := filepath.Join(r.repoPath, ".git")
	_, err := os.Stat(gitDir)
	return err == nil
}

// GetWorktree 获取指定 PR 的 worktree
func (r *RepoManager) GetWorktree(prNumber int) *WorktreeInfo {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return r.worktrees[prNumber]
}

// CreateWorktree 为指定 PR 创建 worktree
func (r *RepoManager) CreateWorktree(prNumber int, branch string, createNewBranch bool) (*WorktreeInfo, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	log.Infof("Creating worktree for PR #%d, branch: %s, createNewBranch: %v", prNumber, branch, createNewBranch)

	// 检查是否已存在
	if existing := r.worktrees[prNumber]; existing != nil {
		log.Infof("Worktree for PR #%d already exists: %s", prNumber, existing.Path)
		return existing, nil
	}

	// 确保仓库已初始化
	if !r.isInitialized() {
		log.Infof("Repository not initialized, initializing: %s", r.repoPath)
		if err := r.Initialize(); err != nil {
			return nil, err
		}
	}

	// 创建 worktree 路径
	worktreePath := filepath.Join(r.repoPath, fmt.Sprintf("pr-%d", prNumber))
	log.Infof("Worktree path: %s", worktreePath)

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
			log.Warnf("Failed to get default branch, using 'main': %v", err)
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
		// 首先检查远程分支是否存在
		log.Infof("Checking if remote branch exists: origin/%s", branch)
		checkCmd := exec.Command("git", "ls-remote", "--heads", "origin", branch)
		checkCmd.Dir = r.repoPath
		checkOutput, err := checkCmd.CombinedOutput()
		if err != nil {
			log.Warnf("Failed to check remote branch: %v, output: %s", err, string(checkOutput))
		} else if strings.TrimSpace(string(checkOutput)) == "" {
			log.Warnf("Remote branch origin/%s does not exist, will create new branch", branch)
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
			log.Infof("Remote branch exists, creating worktree: git worktree add %s origin/%s", worktreePath, branch)
			cmd = exec.Command("git", "worktree", "add", worktreePath, fmt.Sprintf("origin/%s", branch))
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

	log.Infof("Worktree creation output: %s", string(output))

	// 创建 worktree 信息
	worktree := &WorktreeInfo{
		PRNumber:  prNumber,
		Path:      worktreePath,
		Branch:    branch,
		CreatedAt: time.Now(),
	}

	// 注册到映射
	r.worktrees[prNumber] = worktree

	log.Infof("Successfully created worktree for PR #%d: %s", prNumber, worktreePath)
	return worktree, nil
}

// RemoveWorktree 移除指定 PR 的 worktree
func (r *RepoManager) RemoveWorktree(prNumber int) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	worktree := r.worktrees[prNumber]
	if worktree == nil {
		return nil // 已经不存在
	}

	// 删除 worktree
	cmd := exec.Command("git", "worktree", "remove", "--force", worktree.Path)
	cmd.Dir = r.repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Warnf("Failed to remove worktree: %v, output: %s", err, string(output))
		// 即使删除失败，也从映射中移除
	}

	// 从映射中移除
	delete(r.worktrees, prNumber)

	log.Infof("Removed worktree for PR #%d", prNumber)
	return nil
}

// ListWorktrees 列出所有 worktree
func (r *RepoManager) ListWorktrees() ([]*WorktreeInfo, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	if !r.isInitialized() {
		return []*WorktreeInfo{}, nil
	}

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

	for i := 0; i < len(lines); i += 3 {
		if i+2 >= len(lines) {
			break
		}

		// 解析 worktree 路径
		pathLine := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(pathLine, "worktree ") {
			continue
		}
		path := strings.TrimPrefix(pathLine, "worktree ")

		// 解析分支信息
		branchLine := strings.TrimSpace(lines[i+1])
		if !strings.HasPrefix(branchLine, "branch ") {
			continue
		}
		branch := strings.TrimPrefix(branchLine, "branch ")

		// 从路径中提取 PR 号
		prNumber := r.extractPRNumberFromPath(path)
		if prNumber == 0 {
			continue // 不是 PR worktree
		}

		worktree := &WorktreeInfo{
			PRNumber:  prNumber,
			Path:      path,
			Branch:    branch,
			CreatedAt: time.Now(), // 恢复时无法获取准确时间，使用当前时间
		}

		worktrees = append(worktrees, worktree)
	}

	return worktrees, nil
}

// extractPRNumberFromPath 从路径中提取 PR 号
func (r *RepoManager) extractPRNumberFromPath(path string) int {
	// 路径格式: /path/to/repo/pr-{number}
	base := filepath.Base(path)
	if strings.HasPrefix(base, "pr-") {
		parts := strings.Split(base, "-")
		if len(parts) >= 2 {
			if number, err := strconv.Atoi(parts[1]); err == nil {
				return number
			}
		}
	}
	return 0
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
