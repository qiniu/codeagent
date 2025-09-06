package workspace

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// DirFormatter handles directory name formatting and parsing
type DirFormatter interface {
	GenerateIssueDirName(aiModel, repo string, issueNumber int, timestamp int64) string
	GeneratePRDirName(aiModel, repo string, prNumber int, timestamp int64) string
	GenerateSessionDirName(aiModel, repo string, prNumber int, timestamp int64) string
	ParsePRDirName(dirName string) (*PRDirFormat, error)
	ParseIssueDirName(dirName string) (*IssueDirFormat, error)
	ExtractSuffixFromPRDir(aiModel, repo string, prNumber int, dirName string) string
	ExtractSuffixFromIssueDir(aiModel, repo string, issueNumber int, dirName string) string
	CreateSessionPath(underPath, aiModel, repo string, prNumber int, suffix string) string
	CreateSessionPathWithTimestamp(underPath, aiModel, repo string, prNumber int, timestamp int64) string
}

// dirFormatter 目录格式管理器
type dirFormatter struct{}

// NewDirFormatter 创建目录格式管理器
func NewDirFormatter() DirFormatter {
	return &dirFormatter{}
}

// GenerateIssueDirName 生成Issue目录名
func (f *dirFormatter) GenerateIssueDirName(aiModel, repo string, issueNumber int, timestamp int64) string {
	return fmt.Sprintf("%s__%s__issue__%d__%d", aiModel, repo, issueNumber, timestamp)
}

// GeneratePRDirName 生成PR目录名
func (f *dirFormatter) GeneratePRDirName(aiModel, repo string, prNumber int, timestamp int64) string {
	return fmt.Sprintf("%s__%s__pr__%d__%d", aiModel, repo, prNumber, timestamp)
}

// GenerateSessionDirName 生成Session目录名
func (f *dirFormatter) GenerateSessionDirName(aiModel, repo string, prNumber int, timestamp int64) string {
	if aiModel != "" {
		return fmt.Sprintf("%s-%s-session-%d-%d", aiModel, repo, prNumber, timestamp)
	}
	return fmt.Sprintf("%s-session-%d-%d", repo, prNumber, timestamp)
}

// generateSessionDirNameWithSuffix 生成Session目录名（使用suffix）
func (f *dirFormatter) generateSessionDirNameWithSuffix(aiModel, repo string, prNumber int, suffix string) string {
	return fmt.Sprintf("%s__%s__session__%d__%s", aiModel, repo, prNumber, suffix)
}

// ExtractSuffixFromPRDir 从PR目录名中提取suffix（时间戳）
func (f *dirFormatter) ExtractSuffixFromPRDir(aiModel, repo string, prNumber int, dirName string) string {
	expectedPrefix := fmt.Sprintf("%s__%s__pr__%d__", aiModel, repo, prNumber)
	return strings.TrimPrefix(dirName, expectedPrefix)
}

// ExtractSuffixFromIssueDir 从Issue目录名中提取suffix（时间戳）
func (f *dirFormatter) ExtractSuffixFromIssueDir(aiModel, repo string, issueNumber int, dirName string) string {
	expectedPrefix := fmt.Sprintf("%s__%s__issue__%d__", aiModel, repo, issueNumber)
	return strings.TrimPrefix(dirName, expectedPrefix)
}

// CreateSessionPath 创建Session目录路径
func (f *dirFormatter) CreateSessionPath(underPath, aiModel, repo string, prNumber int, suffix string) string {
	dirName := f.generateSessionDirNameWithSuffix(aiModel, repo, prNumber, suffix)
	return filepath.Join(underPath, dirName)
}

// CreateSessionPathWithTimestamp 创建Session目录路径（使用时间戳）
func (f *dirFormatter) CreateSessionPathWithTimestamp(underPath, aiModel, repo string, prNumber int, timestamp int64) string {
	dirName := f.GenerateSessionDirName(aiModel, repo, prNumber, timestamp)
	return filepath.Join(underPath, dirName)
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

// ParsePRDirName 解析PR目录名
func (f *dirFormatter) ParsePRDirName(dirName string) (*PRDirFormat, error) {
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

// ParseIssueDirName 解析Issue目录名
func (f *dirFormatter) ParseIssueDirName(dirName string) (*IssueDirFormat, error) {
	parts := strings.Split(dirName, "__")
	if len(parts) < 5 {
		return nil, fmt.Errorf("invalid Issue directory format: %s", dirName)
	}

	// 格式: {aiModel}__{repo}__issue__{issueNumber}__{timestamp}
	// 找到 "issue" 的位置
	issueIndex := -1
	for i, part := range parts {
		if part == "issue" {
			issueIndex = i
			break
		}
	}

	if issueIndex == -1 || issueIndex < 2 || issueIndex >= len(parts)-2 {
		return nil, fmt.Errorf("invalid Issue directory format: %s", dirName)
	}

	// 提取AI模型和仓库名
	aiModel := parts[issueIndex-2]
	repo := parts[issueIndex-1]

	// 提取Issue编号
	issueNumber, err := strconv.Atoi(parts[issueIndex+1])
	if err != nil {
		return nil, fmt.Errorf("invalid issue number: %s", parts[issueIndex+1])
	}

	// 提取时间戳
	timestamp, err := strconv.ParseInt(parts[issueIndex+2], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid timestamp: %s", parts[issueIndex+2])
	}

	return &IssueDirFormat{
		AIModel:     aiModel,
		Repo:        repo,
		IssueNumber: issueNumber,
		Timestamp:   timestamp,
	}, nil
}
