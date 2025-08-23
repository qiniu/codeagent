package code

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/qiniu/x/log"
)

// getParentRepoPath 获取 git worktree 的父仓库路径
// 如果不是 worktree，返回空字符串
func getParentRepoPath(workspacePath string) (string, error) {
	gitPath := filepath.Join(workspacePath, ".git")
	
	// 检查 .git 是文件还是目录
	info, err := os.Stat(gitPath)
	if err != nil {
		if os.IsNotExist(err) {
			// 不是git仓库
			return "", nil
		}
		return "", err
	}
	
	if info.IsDir() {
		// .git 是目录，说明是主仓库，不需要挂载父目录
		return "", nil
	}
	
	// .git 是文件，读取其内容获取实际的 git 目录路径
	content, err := os.ReadFile(gitPath)
	if err != nil {
		return "", fmt.Errorf("failed to read .git file: %w", err)
	}
	
	gitDirLine := strings.TrimSpace(string(content))
	if !strings.HasPrefix(gitDirLine, "gitdir: ") {
		return "", fmt.Errorf("invalid .git file format: %s", gitDirLine)
	}
	
	// 提取 git 目录路径
	gitDir := strings.TrimPrefix(gitDirLine, "gitdir: ")
	
	// 如果是相对路径，转换为绝对路径
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(workspacePath, gitDir)
	}
	
	// git worktree 的路径格式通常是: /path/to/parent/.git/worktrees/worktree_name
	// 我们需要返回父仓库的路径: /path/to/parent
	if strings.Contains(gitDir, ".git/worktrees/") {
		// 找到 .git/worktrees/ 的位置，提取父仓库路径
		parts := strings.Split(gitDir, ".git/worktrees/")
		if len(parts) >= 2 {
			parentRepo := filepath.Join(parts[0])
			// 移除末尾的路径分隔符
			parentRepo = strings.TrimSuffix(parentRepo, string(filepath.Separator))
			
			// 验证父仓库路径是否存在
			if _, err := os.Stat(filepath.Join(parentRepo, ".git")); err == nil {
				log.Infof("Detected git worktree, parent repository: %s", parentRepo)
				return parentRepo, nil
			}
		}
	}
	
	return "", fmt.Errorf("unable to determine parent repository path from: %s", gitDir)
}