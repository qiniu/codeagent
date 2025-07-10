package workspace

import (
	"testing"
	"time"
)

func TestParseWorktreeList(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []*WorktreeInfo
	}{
		{
			name: "正常 worktree 列表",
			input: `worktree /Users/jicarl/codeagent/codeagent
HEAD 6446817fba0a257f73b311c93126041b63ab6f78
branch refs/heads/main

worktree /Users/jicarl/codeagent/codeagent/pr-11
HEAD d68cdc8c659651ea450449a3d4818594c6c0e33a
branch refs/heads/codeagent/issue-11-1752130181`,
			expected: []*WorktreeInfo{
				{
					PRNumber:  11,
					Path:      "/Users/jicarl/codeagent/codeagent/pr-11",
					Branch:    "codeagent/issue-11-1752130181",
					CreatedAt: time.Now(), // 这个会在测试中忽略
				},
			},
		},
		{
			name: "多个 PR worktree",
			input: `worktree /Users/jicarl/codeagent/codeagent
HEAD 6446817fba0a257f73b311c93126041b63ab6f78
branch refs/heads/main

worktree /Users/jicarl/codeagent/codeagent/pr-11
HEAD d68cdc8c659651ea450449a3d4818594c6c0e33a
branch refs/heads/codeagent/issue-11-1752130181

worktree /Users/jicarl/codeagent/codeagent/pr-12
HEAD e79cdc8c659651ea450449a3d4818594c6c0e33b
branch refs/heads/codeagent/issue-12-1752130182`,
			expected: []*WorktreeInfo{
				{
					PRNumber:  11,
					Path:      "/Users/jicarl/codeagent/codeagent/pr-11",
					Branch:    "codeagent/issue-11-1752130181",
					CreatedAt: time.Now(),
				},
				{
					PRNumber:  12,
					Path:      "/Users/jicarl/codeagent/codeagent/pr-12",
					Branch:    "codeagent/issue-12-1752130182",
					CreatedAt: time.Now(),
				},
			},
		},
		{
			name: "只有主仓库",
			input: `worktree /Users/jicarl/codeagent/codeagent
HEAD 6446817fba0a257f73b311c93126041b63ab6f78
branch refs/heads/main`,
			expected: []*WorktreeInfo{},
		},
		{
			name: "detached HEAD worktree",
			input: `worktree /Users/jicarl/codeagent/codeagent
HEAD 6446817fba0a257f73b311c93126041b63ab6f78
branch refs/heads/main

worktree /Users/jicarl/codeagent/codeagent/pr-13
HEAD f89cdc8c659651ea450449a3d4818594c6c0e33c
detached`,
			expected: []*WorktreeInfo{}, // detached HEAD 应该被跳过
		},
		{
			name:     "空输入",
			input:    "",
			expected: []*WorktreeInfo{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoManager := &RepoManager{}
			result, err := repoManager.parseWorktreeList(tt.input)

			if err != nil {
				t.Errorf("parseWorktreeList() error = %v", err)
				return
			}

			if len(result) != len(tt.expected) {
				t.Errorf("parseWorktreeList() returned %d worktrees, expected %d", len(result), len(tt.expected))
				return
			}

			for i, worktree := range result {
				expected := tt.expected[i]

				if worktree.PRNumber != expected.PRNumber {
					t.Errorf("worktree[%d].PRNumber = %d, expected %d", i, worktree.PRNumber, expected.PRNumber)
				}

				if worktree.Path != expected.Path {
					t.Errorf("worktree[%d].Path = %s, expected %s", i, worktree.Path, expected.Path)
				}

				if worktree.Branch != expected.Branch {
					t.Errorf("worktree[%d].Branch = %s, expected %s", i, worktree.Branch, expected.Branch)
				}
			}
		})
	}
}

func TestExtractPRNumberFromPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected int
	}{
		{
			name:     "正常 PR 路径",
			path:     "/Users/jicarl/codeagent/codeagent/pr-11",
			expected: 11,
		},
		{
			name:     "主仓库路径",
			path:     "/Users/jicarl/codeagent/codeagent",
			expected: 0,
		},
		{
			name:     "其他目录路径",
			path:     "/Users/jicarl/codeagent/codeagent/session-11",
			expected: 0,
		},
		{
			name:     "无效 PR 号",
			path:     "/Users/jicarl/codeagent/codeagent/pr-abc",
			expected: 0,
		},
		{
			name:     "空路径",
			path:     "",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoManager := &RepoManager{}
			result := repoManager.extractPRNumberFromPath(tt.path)

			if result != tt.expected {
				t.Errorf("extractPRNumberFromPath(%s) = %d, expected %d", tt.path, result, tt.expected)
			}
		})
	}
}
