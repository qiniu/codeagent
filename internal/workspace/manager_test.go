package workspace

import (
	"testing"
)

func TestExtractOrgRepoPath(t *testing.T) {
	tests := []struct {
		name     string
		repoURL  string
		expected string
	}{
		{
			name:     "GitHub HTTPS URL",
			repoURL:  "https://github.com/org1/repo1.git",
			expected: "org1/repo1",
		},
		{
			name:     "GitHub HTTPS URL without .git",
			repoURL:  "https://github.com/org2/repo2",
			expected: "org2/repo2",
		},
		{
			name:     "GitHub SSH URL",
			repoURL:  "git@github.com:org3/repo3.git",
			expected: "org3/repo3",
		},
		{
			name:     "Invalid URL",
			repoURL:  "invalid-url",
			expected: "unknown/unknown",
		},
		{
			name:     "Empty URL",
			repoURL:  "",
			expected: "unknown/unknown",
		},
	}

	manager := &Manager{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.extractOrgRepoPath(tt.repoURL)
			if result != tt.expected {
				t.Errorf("extractOrgRepoPath(%s) = %s, want %s", tt.repoURL, result, tt.expected)
			}
		})
	}
}

func TestKeyFunction(t *testing.T) {
	tests := []struct {
		name     string
		orgRepo  string
		prNumber int
		expected string
	}{
		{
			name:     "Normal case",
			orgRepo:  "org1/repo1",
			prNumber: 123,
			expected: "org1/repo1/123",
		},
		{
			name:     "Another case",
			orgRepo:  "org2/repo2",
			prNumber: 456,
			expected: "org2/repo2/456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := key(tt.orgRepo, tt.prNumber)
			if result != tt.expected {
				t.Errorf("key(%s, %d) = %s, want %s", tt.orgRepo, tt.prNumber, result, tt.expected)
			}
		})
	}
}
