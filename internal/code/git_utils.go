package code

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/qiniu/x/log"
)

// GitWorktreeInfo contains information about a git worktree setup
type GitWorktreeInfo struct {
	IsWorktree      bool
	ParentRepoPath  string
	WorktreeName    string
	GitDirPath      string
}

// getGitWorktreeInfo 获取 git worktree 的详细信息
// 相比原来的 getParentRepoPath，这个函数提供更多信息和更好的错误处理
func getGitWorktreeInfo(workspacePath string) (*GitWorktreeInfo, error) {
	// 安全性检查：确保工作空间路径是绝对路径
	absWorkspacePath, err := filepath.Abs(workspacePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute workspace path: %w", err)
	}
	
	gitPath := filepath.Join(absWorkspacePath, ".git")
	
	// 检查 .git 是文件还是目录
	info, err := os.Stat(gitPath)
	if err != nil {
		if os.IsNotExist(err) {
			// 不是git仓库
			return &GitWorktreeInfo{IsWorktree: false}, nil
		}
		return nil, fmt.Errorf("failed to stat .git path: %w", err)
	}
	
	if info.IsDir() {
		// .git 是目录，说明是主仓库，不是worktree
		return &GitWorktreeInfo{IsWorktree: false}, nil
	}
	
	// .git 是文件，读取其内容获取实际的 git 目录路径
	content, err := os.ReadFile(gitPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read .git file: %w", err)
	}
	
	gitDirLine := strings.TrimSpace(string(content))
	if !strings.HasPrefix(gitDirLine, "gitdir: ") {
		return nil, fmt.Errorf("invalid .git file format, expected 'gitdir: ' prefix, got: %s", gitDirLine)
	}
	
	// 提取 git 目录路径
	gitDir := strings.TrimPrefix(gitDirLine, "gitdir: ")
	
	// 如果是相对路径，转换为绝对路径
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(absWorkspacePath, gitDir)
	}
	
	// 规范化路径，解决路径中的 ".." 等问题
	gitDir, err = filepath.Abs(gitDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve git directory path: %w", err)
	}
	
	// 验证gitDir路径的安全性：确保它指向一个合理的位置
	if !isSecurePath(gitDir) {
		return nil, fmt.Errorf("git directory path appears to be unsafe: %s", gitDir)
	}
	
	// git worktree 的路径格式: /path/to/parent/.git/worktrees/worktree_name
	worktreesPattern := string(filepath.Separator) + ".git" + string(filepath.Separator) + "worktrees" + string(filepath.Separator)
	
	if !strings.Contains(gitDir, worktreesPattern) {
		return nil, fmt.Errorf("git directory does not appear to be a worktree: %s", gitDir)
	}
	
	// 找到 .git/worktrees/ 的位置，提取父仓库路径
	parts := strings.Split(gitDir, worktreesPattern)
	if len(parts) < 2 {
		return nil, fmt.Errorf("unable to parse worktree path structure: %s", gitDir)
	}
	
	parentRepo := parts[0]
	worktreeName := strings.Split(parts[1], string(filepath.Separator))[0]
	
	// 验证父仓库路径是否存在且合法
	parentGitDir := filepath.Join(parentRepo, ".git")
	if _, err := os.Stat(parentGitDir); err != nil {
		return nil, fmt.Errorf("parent repository .git directory not found: %s", parentGitDir)
	}
	
	// 安全性检查：确保父仓库路径不会导致路径遍历
	if !isSecurePath(parentRepo) {
		return nil, fmt.Errorf("parent repository path appears to be unsafe: %s", parentRepo)
	}
	
	log.Infof("Detected git worktree: %s, parent repository: %s", worktreeName, parentRepo)
	
	return &GitWorktreeInfo{
		IsWorktree:     true,
		ParentRepoPath: parentRepo,
		WorktreeName:   worktreeName,
		GitDirPath:     gitDir,
	}, nil
}

// isSecurePath 检查路径是否安全，防止路径遍历攻击
func isSecurePath(path string) bool {
	// 检查路径是否包含危险的路径遍历模式
	dangerousPatterns := []string{
		"..",
		"~",
		"/etc/",
		"/var/",
		"/usr/",
		"/bin/",
		"/sbin/",
		"/root/",
	}
	
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	
	// 检查是否包含危险模式
	for _, pattern := range dangerousPatterns {
		if strings.Contains(absPath, pattern) {
			// 允许合理的上级目录导航（在预期的工作目录范围内）
			if pattern == ".." {
				// 只有在合理的深度内才允许
				if strings.Count(absPath, "..") > 3 {
					return false
				}
			} else {
				return false
			}
		}
	}
	
	return true
}

// getParentRepoPath 保持向后兼容性的包装函数
// 如果不是 worktree，返回空字符串
func getParentRepoPath(workspacePath string) (string, error) {
	info, err := getGitWorktreeInfo(workspacePath)
	if err != nil {
		return "", err
	}
	
	if !info.IsWorktree {
		return "", nil
	}
	
	return info.ParentRepoPath, nil
}