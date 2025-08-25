package workspace

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRepoCacheService_GetCachedRepoPath(t *testing.T) {
	baseDir := "/tmp/test-cache"
	gitService := NewGitService()
	cacheService := NewRepoCacheService(baseDir, gitService)

	tests := []struct {
		name     string
		org      string
		repo     string
		expected string
	}{
		{
			name:     "Basic path",
			org:      "qiniu",
			repo:     "codeagent",
			expected: "/tmp/test-cache/_cache/qiniu/codeagent",
		},
		{
			name:     "Different org/repo",
			org:      "anthropic",
			repo:     "claude",
			expected: "/tmp/test-cache/_cache/anthropic/claude",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cacheService.GetCachedRepoPath(tt.org, tt.repo)
			if result != tt.expected {
				t.Errorf("GetCachedRepoPath(%s, %s) = %s, want %s",
					tt.org, tt.repo, result, tt.expected)
			}
		})
	}
}

func TestRepoCacheService_CachedRepoExists(t *testing.T) {
	// Create temporary directory for testing
	tempDir := os.TempDir()
	testBaseDir := filepath.Join(tempDir, "test-cache-"+time.Now().Format("20060102150405"))
	defer os.RemoveAll(testBaseDir) // Clean up after test

	gitService := NewGitService()
	cacheService := NewRepoCacheService(testBaseDir, gitService)

	// Test non-existent repo
	exists := cacheService.CachedRepoExists("test-org", "test-repo")
	if exists {
		t.Error("CachedRepoExists should return false for non-existent repo")
	}

	// Create a mock cached repo directory with .git folder
	cachedPath := cacheService.GetCachedRepoPath("test-org", "test-repo")
	err := os.MkdirAll(filepath.Join(cachedPath, ".git"), 0755)
	if err != nil {
		t.Fatalf("Failed to create mock cached repo: %v", err)
	}

	// Test existing repo
	exists = cacheService.CachedRepoExists("test-org", "test-repo")
	if !exists {
		t.Error("CachedRepoExists should return true for existing repo with .git folder")
	}
}

func TestGenerateWorkspaceKey_Integration(t *testing.T) {
	// This tests the integration between different components
	tests := []struct {
		name     string
		org      string
		repo     string
		prNumber int
		aiModel  string
		expected string
	}{
		{
			name:     "Full integration test",
			org:      "qiniu",
			repo:     "codeagent",
			prNumber: 123,
			aiModel:  "claude",
			expected: "claude/qiniu/codeagent/123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateWorkspaceKey(tt.org, tt.repo, tt.prNumber, tt.aiModel)
			if result != tt.expected {
				t.Errorf("generateWorkspaceKey integration test failed: got %s, want %s",
					result, tt.expected)
			}
		})
	}
}
