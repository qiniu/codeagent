package workspace

import (
	"fmt"
	"strconv"
	"strings"
)

// dirFormatter Directory format manager
type dirFormatter struct{}

// newDirFormatter creates directory format manager
func newDirFormatter() *dirFormatter {
	return &dirFormatter{}
}

// generateIssueDirName generates Issue directory name
func (f *dirFormatter) generateIssueDirName(aiModel, repo string, issueNumber int, timestamp int64) string {
	return fmt.Sprintf("%s-%s-issue-%d-%d", aiModel, repo, issueNumber, timestamp)
}

// generatePRDirName generates PR directory name
func (f *dirFormatter) generatePRDirName(aiModel, repo string, prNumber int, timestamp int64) string {
	return fmt.Sprintf("%s-%s-pr-%d-%d", aiModel, repo, prNumber, timestamp)
}

// generateSessionDirName generates Session directory name
func (f *dirFormatter) generateSessionDirName(aiModel, repo string, prNumber int, timestamp int64) string {
	return fmt.Sprintf("%s-%s-session-%d-%d", aiModel, repo, prNumber, timestamp)
}

// generateSessionDirNameWithSuffix generates Session directory name (using suffix)
func (f *dirFormatter) generateSessionDirNameWithSuffix(aiModel, repo string, prNumber int, suffix string) string {
	return fmt.Sprintf("%s-%s-session-%d-%s", aiModel, repo, prNumber, suffix)
}

// extractSuffixFromPRDir extracts suffix (timestamp) from PR directory name
func (f *dirFormatter) extractSuffixFromPRDir(aiModel, repo string, prNumber int, dirName string) string {
	expectedPrefix := fmt.Sprintf("%s-%s-pr-%d-", aiModel, repo, prNumber)
	return strings.TrimPrefix(dirName, expectedPrefix)
}

// extractSuffixFromIssueDir extracts suffix (timestamp) from Issue directory name
func (f *dirFormatter) extractSuffixFromIssueDir(aiModel, repo string, issueNumber int, dirName string) string {
	expectedPrefix := fmt.Sprintf("%s-%s-issue-%d-", aiModel, repo, issueNumber)
	return strings.TrimPrefix(dirName, expectedPrefix)
}

// createSessionPath creates Session directory path
func (f *dirFormatter) createSessionPath(underPath, aiModel, repo string, prNumber int, suffix string) string {
	dirName := f.generateSessionDirNameWithSuffix(aiModel, repo, prNumber, suffix)
	return fmt.Sprintf("%s/%s", underPath, dirName)
}

// createSessionPathWithTimestamp creates Session directory path (using timestamp)
func (f *dirFormatter) createSessionPathWithTimestamp(underPath, aiModel, repo string, prNumber int, timestamp int64) string {
	dirName := f.generateSessionDirName(aiModel, repo, prNumber, timestamp)
	return fmt.Sprintf("%s/%s", underPath, dirName)
}

// IssueDirFormat Issue directory format
type IssueDirFormat struct {
	AIModel     string
	Repo        string
	IssueNumber int
	Timestamp   int64
}

// PRDirFormat PR directory format
type PRDirFormat struct {
	AIModel   string
	Repo      string
	PRNumber  int
	Timestamp int64
}

// SessionDirFormat Session directory format
type SessionDirFormat struct {
	AIModel   string
	Repo      string
	PRNumber  int
	Timestamp int64
}

// parsePRDirName parses PR directory name
func (f *dirFormatter) parsePRDirName(dirName string) (*PRDirFormat, error) {
	parts := strings.Split(dirName, "-")
	if len(parts) < 5 {
		return nil, fmt.Errorf("invalid PR directory format: %s", dirName)
	}

	// Format: {aiModel}-{repo}-pr-{prNumber}-{timestamp}
	// Find position of "pr"
	prIndex := -1
	for i, part := range parts {
		if part == "pr" {
			prIndex = i
			break
		}
	}

	if prIndex == -1 || prIndex < 2 || prIndex >= len(parts)-2 {
		return nil, fmt.Errorf("invalid PR directory format: %s", dirName)
	}

	// Extract AI model and repository name
	aiModel := strings.Join(parts[:prIndex-1], "-")
	repo := parts[prIndex-1]

	// Extract PR number
	prNumber, err := strconv.Atoi(parts[prIndex+1])
	if err != nil {
		return nil, fmt.Errorf("invalid PR number: %s", parts[prIndex+1])
	}

	// Extract timestamp
	timestamp, err := strconv.ParseInt(parts[prIndex+2], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid timestamp: %s", parts[prIndex+2])
	}

	return &PRDirFormat{
		AIModel:   aiModel,
		Repo:      repo,
		PRNumber:  prNumber,
		Timestamp: timestamp,
	}, nil
}
