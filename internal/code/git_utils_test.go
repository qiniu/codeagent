package code

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsSecurePath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name:     "normal path",
			path:     "/tmp/workspace/repo",
			expected: true,
		},
		{
			name:     "path with single parent",
			path:     "/tmp/workspace/../parent",
			expected: true,
		},
		{
			name:     "dangerous system path",
			path:     "/etc/passwd",
			expected: false,
		},
		{
			name:     "root path",
			path:     "/root/secret",
			expected: false,
		},
		{
			name:     "excessive parent traversal",
			path:     "/tmp/../../../../../../../etc/passwd",
			expected: false,
		},
		{
			name:     "home expansion",
			path:     "~/malicious",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isSecurePath(tt.path)
			if result != tt.expected {
				t.Errorf("isSecurePath(%s) = %v, expected %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestGetGitWorktreeInfo(t *testing.T) {
	// 创建临时目录结构进行测试
	tmpDir := t.TempDir()
	
	// 测试非Git目录
	t.Run("non-git directory", func(t *testing.T) {
		info, err := getGitWorktreeInfo(tmpDir)
		if err != nil {
			t.Errorf("getGitWorktreeInfo() error = %v", err)
		}
		if info.IsWorktree {
			t.Error("expected IsWorktree to be false for non-git directory")
		}
	})
	
	// 测试普通Git仓库（.git目录）
	t.Run("regular git repo", func(t *testing.T) {
		gitDir := filepath.Join(tmpDir, "regular_repo", ".git")
		if err := os.MkdirAll(gitDir, 0755); err != nil {
			t.Fatalf("failed to create .git directory: %v", err)
		}
		
		info, err := getGitWorktreeInfo(filepath.Dir(gitDir))
		if err != nil {
			t.Errorf("getGitWorktreeInfo() error = %v", err)
		}
		if info.IsWorktree {
			t.Error("expected IsWorktree to be false for regular git repo")
		}
	})
	
	// 测试Git worktree
	t.Run("git worktree", func(t *testing.T) {
		// 创建父仓库结构
		parentRepo := filepath.Join(tmpDir, "parent_repo")
		parentGitDir := filepath.Join(parentRepo, ".git")
		worktreesDir := filepath.Join(parentGitDir, "worktrees", "test_worktree")
		if err := os.MkdirAll(worktreesDir, 0755); err != nil {
			t.Fatalf("failed to create worktrees directory: %v", err)
		}
		
		// 创建worktree目录和.git文件
		worktreeDir := filepath.Join(tmpDir, "test_worktree")
		if err := os.MkdirAll(worktreeDir, 0755); err != nil {
			t.Fatalf("failed to create worktree directory: %v", err)
		}
		
		gitFile := filepath.Join(worktreeDir, ".git")
		gitContent := "gitdir: " + worktreesDir
		if err := os.WriteFile(gitFile, []byte(gitContent), 0644); err != nil {
			t.Fatalf("failed to write .git file: %v", err)
		}
		
		info, err := getGitWorktreeInfo(worktreeDir)
		if err != nil {
			t.Errorf("getGitWorktreeInfo() error = %v", err)
		}
		if !info.IsWorktree {
			t.Error("expected IsWorktree to be true for git worktree")
		}
		if info.ParentRepoPath != parentRepo {
			t.Errorf("expected ParentRepoPath = %s, got %s", parentRepo, info.ParentRepoPath)
		}
		if info.WorktreeName != "test_worktree" {
			t.Errorf("expected WorktreeName = test_worktree, got %s", info.WorktreeName)
		}
	})
}

func TestGetParentRepoPath(t *testing.T) {
	// 测试向后兼容性
	tmpDir := t.TempDir()
	
	// 测试非worktree应该返回空字符串
	result, err := getParentRepoPath(tmpDir)
	if err != nil {
		t.Errorf("getParentRepoPath() error = %v", err)
	}
	if result != "" {
		t.Errorf("expected empty string for non-worktree, got %s", result)
	}
}