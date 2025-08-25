package workspace

import (
	"testing"

	"github.com/qiniu/codeagent/internal/config"
)

func TestGenerateWorkspaceKey(t *testing.T) {
	tests := []struct {
		name     string
		org      string
		repo     string
		prNumber int
		aiModel  string
		expected string
	}{
		{
			name:     "Without AI model",
			org:      "org1",
			repo:     "repo1",
			prNumber: 123,
			aiModel:  "",
			expected: "org1/repo1/123",
		},
		{
			name:     "With AI model",
			org:      "org2",
			repo:     "repo2",
			prNumber: 456,
			aiModel:  "claude",
			expected: "claude/org2/repo2/456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateWorkspaceKey(tt.org, tt.repo, tt.prNumber, tt.aiModel)
			if result != tt.expected {
				t.Errorf("generateWorkspaceKey(%s, %s, %d, %s) = %s, want %s", 
					tt.org, tt.repo, tt.prNumber, tt.aiModel, result, tt.expected)
			}
		})
	}
}

func TestExtractAIModelFromBranch(t *testing.T) {
	cfg := &config.Config{
		CodeProvider: "claude",
	}
	manager := NewManager(cfg)

	tests := []struct {
		name       string
		branchName string
		expected   string
	}{
		{
			name:       "Claude branch",
			branchName: "codeagent/claude/issue-123-456",
			expected:   "claude",
		},
		{
			name:       "Gemini branch", 
			branchName: "codeagent/gemini/pr-789-101",
			expected:   "gemini",
		},
		{
			name:       "Invalid AI model",
			branchName: "codeagent/invalid/issue-123-456",
			expected:   "claude", // Falls back to config default
		},
		{
			name:       "Not a codeagent branch",
			branchName: "main",
			expected:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.ExtractAIModelFromBranch(tt.branchName)
			if result != tt.expected {
				t.Errorf("ExtractAIModelFromBranch(%s) = %s, want %s", 
					tt.branchName, result, tt.expected)
			}
		})
	}
}
