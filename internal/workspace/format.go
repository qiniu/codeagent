package workspace

import (
	"fmt"
	"strconv"
	"strings"
)

// dirFormatter 目录格式管理器
type dirFormatter struct{}

// newDirFormatter 创建目录格式管理器
func newDirFormatter() *dirFormatter {
	return &dirFormatter{}
}

// generateIssueDirName 生成Issue目录名
func (f *dirFormatter) generateIssueDirName(aiModel, repo string, issueNumber int, timestamp int64) string {
	return fmt.Sprintf("%s__%s__issue__%d__%d", aiModel, repo, issueNumber, timestamp)
}

// generatePRDirName 生成PR目录名
func (f *dirFormatter) generatePRDirName(aiModel, repo string, prNumber int, timestamp int64) string {
	return fmt.Sprintf("%s__%s__pr__%d__%d", aiModel, repo, prNumber, timestamp)
}

// generateSessionDirName 生成Session目录名
func (f *dirFormatter) generateSessionDirName(aiModel, repo string, prNumber int, timestamp int64) string {
	return fmt.Sprintf("%s__%s__session__%d__%d", aiModel, repo, prNumber, timestamp)
}

// generateSessionDirNameWithSuffix 生成Session目录名（使用suffix）
func (f *dirFormatter) generateSessionDirNameWithSuffix(aiModel, repo string, prNumber int, suffix string) string {
	return fmt.Sprintf("%s__%s__session__%d__%s", aiModel, repo, prNumber, suffix)
}

// extractSuffixFromPRDir 从PR目录名中提取suffix（时间戳）
func (f *dirFormatter) extractSuffixFromPRDir(aiModel, repo string, prNumber int, dirName string) string {
	expectedPrefix := fmt.Sprintf("%s__%s__pr__%d__", aiModel, repo, prNumber)
	return strings.TrimPrefix(dirName, expectedPrefix)
}

// extractSuffixFromIssueDir 从Issue目录名中提取suffix（时间戳）
func (f *dirFormatter) extractSuffixFromIssueDir(aiModel, repo string, issueNumber int, dirName string) string {
	expectedPrefix := fmt.Sprintf("%s__%s__issue__%d__", aiModel, repo, issueNumber)
	return strings.TrimPrefix(dirName, expectedPrefix)
}

// createSessionPath 创建Session目录路径
func (f *dirFormatter) createSessionPath(underPath, aiModel, repo string, prNumber int, suffix string) string {
	dirName := f.generateSessionDirNameWithSuffix(aiModel, repo, prNumber, suffix)
	return fmt.Sprintf("%s/%s", underPath, dirName)
}

// createSessionPathWithTimestamp 创建Session目录路径（使用时间戳）
func (f *dirFormatter) createSessionPathWithTimestamp(underPath, aiModel, repo string, prNumber int, timestamp int64) string {
	dirName := f.generateSessionDirName(aiModel, repo, prNumber, timestamp)
	return fmt.Sprintf("%s/%s", underPath, dirName)
}

// IssueDirFormat Issue目录格式
type IssueDirFormat struct {
	AIModel     string
	Repo        string
	IssueNumber int
	Timestamp   int64
}

// PRDirFormat PR目录格式
type PRDirFormat struct {
	AIModel   string
	Repo      string
	PRNumber  int
	Timestamp int64
}

// SessionDirFormat Session目录格式
type SessionDirFormat struct {
	AIModel   string
	Repo      string
	PRNumber  int
	Timestamp int64
}

// parsePRDirName 解析PR目录名
func (f *dirFormatter) parsePRDirName(dirName string) (*PRDirFormat, error) {
	parts := strings.Split(dirName, "__")
	if len(parts) < 5 {
		return nil, fmt.Errorf("invalid PR directory format: %s", dirName)
	}

	// 格式: {aiModel}__{repo}__pr__{prNumber}__{timestamp}
	// 找到 "pr" 的位置
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

	// 提取AI模型和仓库名
	aiModel := parts[prIndex-2]
	repo := parts[prIndex-1]

	// 提取PR编号
	prNumber, err := strconv.Atoi(parts[prIndex+1])
	if err != nil {
		return nil, fmt.Errorf("invalid PR number: %s", parts[prIndex+1])
	}

	// 提取时间戳
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
