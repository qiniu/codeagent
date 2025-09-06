package workspace

import (
	"testing"
)

func TestDirectoryFormat_GenerateIssueDirName(t *testing.T) {
	df := NewDirFormatter()

	tests := []struct {
		name        string
		aiModel     string
		repo        string
		issueNumber int
		timestamp   int64
		expected    string
	}{
		{
			name:        "with AI model",
			aiModel:     "gemini",
			repo:        "codeagent",
			issueNumber: 123,
			timestamp:   1752829201,
			expected:    "gemini__codeagent__issue__123__1752829201",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := df.GenerateIssueDirName(tt.aiModel, tt.repo, tt.issueNumber, tt.timestamp)
			if result != tt.expected {
				t.Errorf("GenerateIssueDirName() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestDirectoryFormat_GeneratePRDirName(t *testing.T) {
	df := NewDirFormatter()

	tests := []struct {
		name      string
		aiModel   string
		repo      string
		prNumber  int
		timestamp int64
		expected  string
	}{
		{
			name:      "with AI model",
			aiModel:   "gemini",
			repo:      "codeagent",
			prNumber:  161,
			timestamp: 1752829201,
			expected:  "gemini__codeagent__pr__161__1752829201",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := df.GeneratePRDirName(tt.aiModel, tt.repo, tt.prNumber, tt.timestamp)
			if result != tt.expected {
				t.Errorf("GeneratePRDirName() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestDirectoryFormat_GenerateSessionDirName(t *testing.T) {
	df := NewDirFormatter()

	tests := []struct {
		name      string
		aiModel   string
		repo      string
		prNumber  int
		timestamp int64
		expected  string
	}{
		{
			name:      "with AI model",
			aiModel:   "gemini",
			repo:      "codeagent",
			prNumber:  161,
			timestamp: 1752829201,
			expected:  "gemini-codeagent-session-161-1752829201",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := df.GenerateSessionDirName(tt.aiModel, tt.repo, tt.prNumber, tt.timestamp)
			if result != tt.expected {
				t.Errorf("GenerateSessionDirName() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestDirectoryFormat_ParsePRDirName(t *testing.T) {
	df := NewDirFormatter()

	tests := []struct {
		name     string
		dirName  string
		expected *PRDirFormat
		wantErr  bool
	}{
		{
			name:    "with AI model",
			dirName: "gemini__codeagent__pr__161__1752829201",
			expected: &PRDirFormat{
				AIModel:   "gemini",
				Repo:      "codeagent",
				PRNumber:  161,
				Timestamp: 1752829201,
			},
			wantErr: false,
		},

		{
			name:     "invalid format",
			dirName:  "invalid-format",
			expected: nil,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := df.ParsePRDirName(tt.dirName)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParsePRDirName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if result.AIModel != tt.expected.AIModel {
				t.Errorf("ParsePRDirName() AIModel = %v, want %v", result.AIModel, tt.expected.AIModel)
			}
			if result.Repo != tt.expected.Repo {
				t.Errorf("ParsePRDirName() Repo = %v, want %v", result.Repo, tt.expected.Repo)
			}
			if result.PRNumber != tt.expected.PRNumber {
				t.Errorf("ParsePRDirName() PRNumber = %v, want %v", result.PRNumber, tt.expected.PRNumber)
			}
			if result.Timestamp != tt.expected.Timestamp {
				t.Errorf("ParsePRDirName() Timestamp = %v, want %v", result.Timestamp, tt.expected.Timestamp)
			}
		})
	}
}

func TestDirectoryFormat_ExtractSuffixFromPRDir(t *testing.T) {
	df := NewDirFormatter()

	tests := []struct {
		name     string
		aiModel  string
		repo     string
		prNumber int
		dirName  string
		expected string
	}{
		{
			name:     "with AI model",
			aiModel:  "gemini",
			repo:     "codeagent",
			prNumber: 161,
			dirName:  "gemini__codeagent__pr__161__1752829201",
			expected: "1752829201",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := df.ExtractSuffixFromPRDir(tt.aiModel, tt.repo, tt.prNumber, tt.dirName)
			if result != tt.expected {
				t.Errorf("ExtractSuffixFromPRDir() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestDirectoryFormat_ParseIssueDirName(t *testing.T) {
	df := NewDirFormatter()

	tests := []struct {
		name        string
		dirName     string
		expected    *IssueDirFormat
		expectError bool
	}{
		{
			name:    "valid issue directory",
			dirName: "claude__codeagent__issue__123__1752829201",
			expected: &IssueDirFormat{
				AIModel:     "claude",
				Repo:        "codeagent",
				IssueNumber: 123,
				Timestamp:   1752829201,
			},
		},
		{
			name:    "gemini issue directory",
			dirName: "gemini__myrepo__issue__456__1752829202",
			expected: &IssueDirFormat{
				AIModel:     "gemini",
				Repo:        "myrepo",
				IssueNumber: 456,
				Timestamp:   1752829202,
			},
		},
		{
			name:        "invalid format - no issue",
			dirName:     "claude__codeagent__pr__123__1752829201",
			expectError: true,
		},
		{
			name:        "invalid format - too few parts",
			dirName:     "claude__issue__123",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := df.ParseIssueDirName(tt.dirName)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if result.AIModel != tt.expected.AIModel {
				t.Errorf("AIModel = %v, want %v", result.AIModel, tt.expected.AIModel)
			}

			if result.Repo != tt.expected.Repo {
				t.Errorf("Repo = %v, want %v", result.Repo, tt.expected.Repo)
			}

			if result.IssueNumber != tt.expected.IssueNumber {
				t.Errorf("IssueNumber = %v, want %v", result.IssueNumber, tt.expected.IssueNumber)
			}

			if result.Timestamp != tt.expected.Timestamp {
				t.Errorf("Timestamp = %v, want %v", result.Timestamp, tt.expected.Timestamp)
			}
		})
	}
}
