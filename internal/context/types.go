package context

import (
	"time"

	"github.com/google/go-github/v58/github"
)

// ContextType 上下文类型
type ContextType string

const (
	ContextTypePR            ContextType = "pull_request"
	ContextTypeIssue         ContextType = "issue"
	ContextTypePRComment     ContextType = "pr_comment"
	ContextTypeReviewComment ContextType = "review_comment"
	ContextTypeReview        ContextType = "review"
)

// ContextPriority 上下文优先级
type ContextPriority int

const (
	PriorityLow      ContextPriority = 1
	PriorityMedium   ContextPriority = 2
	PriorityHigh     ContextPriority = 3
	PriorityCritical ContextPriority = 4
)

// FileChange 文件变更信息
type FileChange struct {
	Path         string `json:"path"`
	Status       string `json:"status"` // added, modified, deleted, renamed
	Additions    int    `json:"additions"`
	Deletions    int    `json:"deletions"`
	Changes      int    `json:"changes"`
	Patch        string `json:"patch,omitempty"`
	PreviousPath string `json:"previous_path,omitempty"` // for renamed files
	SHA          string `json:"sha,omitempty"`
}

// CodeContext 代码上下文
type CodeContext struct {
	Repository   string       `json:"repository"`
	BaseBranch   string       `json:"base_branch"`
	HeadBranch   string       `json:"head_branch,omitempty"`
	Files        []FileChange `json:"files"`
	TotalChanges struct {
		Additions int `json:"additions"`
		Deletions int `json:"deletions"`
		Files     int `json:"files"`
	} `json:"total_changes"`
}

// CommentContext 评论上下文
type CommentContext struct {
	ID        int64     `json:"id"`
	Type      string    `json:"type"` // comment, review_comment, review
	Author    string    `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	// For review comments
	FilePath   string `json:"file_path,omitempty"`
	LineNumber int    `json:"line_number,omitempty"`
	StartLine  int    `json:"start_line,omitempty"`
	// For reviews
	ReviewState string `json:"review_state,omitempty"`
}

// GitHubContext GitHub原生上下文

type GitHubContext struct {
	Repository  string              `json:"repository"`
	PRNumber    int                 `json:"pr_number,omitempty"`
	IssueNumber int                 `json:"issue_number,omitempty"`
	PR          *PullRequestContext `json:"pr,omitempty"`
	Issue       *IssueContext       `json:"issue,omitempty"`
	Files       []FileChange        `json:"files"`
	Comments    []CommentContext    `json:"comments"`
}

type PullRequestContext struct {
	Number     int    `json:"number"`
	Title      string `json:"title"`
	Body       string `json:"body"`
	State      string `json:"state"`
	Author     string `json:"author"`
	BaseBranch string `json:"base_branch"`
	HeadBranch string `json:"head_branch"`
	Additions  int    `json:"additions"`
	Deletions  int    `json:"deletions"`
	Commits    int    `json:"commits"`
}

type IssueContext struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	State  string `json:"state"`
	Author string `json:"author"`
}

// EnhancedContext 增强上下文
type EnhancedContext struct {
	// 基础信息
	Type      ContextType     `json:"type"`
	Priority  ContextPriority `json:"priority"`
	Timestamp time.Time       `json:"timestamp"`

	// 核心数据
	Subject  interface{}      `json:"subject"` // PR, Issue, or Comment
	Comments []CommentContext `json:"comments"`
	Code     *CodeContext     `json:"code,omitempty"`

	// 元数据
	Metadata   map[string]interface{} `json:"metadata"`
	TokenCount int                    `json:"token_count,omitempty"`
}

// ContextCollector 上下文收集器接口
type ContextCollector interface {
	// 收集基础上下文
	CollectBasicContext(eventType string, payload interface{}) (*EnhancedContext, error)

	// 收集代码上下文
	CollectCodeContext(pr *github.PullRequest) (*CodeContext, error)

	// 收集GitHub上下文
	CollectGitHubContext(repoFullName string, prNumber int) (*GitHubContext, error)

	// 收集评论上下文
	CollectCommentContext(pr *github.PullRequest, currentCommentID int64) ([]CommentContext, error)
}

// ContextFormatter 上下文格式化器接口
type ContextFormatter interface {
	// 格式化为Markdown
	FormatToMarkdown(ctx *EnhancedContext) (string, error)

	// 格式化为结构化文本
	FormatToStructured(ctx *EnhancedContext) (string, error)

	// 智能裁剪
	TrimToTokenLimit(ctx *EnhancedContext, maxTokens int) (*EnhancedContext, error)
}

// PromptGenerator 提示词生成器接口
type PromptGenerator interface {
	// 生成基础提示词
	GeneratePrompt(ctx *EnhancedContext, mode string, args string) (string, error)

	// 生成工具列表
	GenerateToolsList(ctx *EnhancedContext, mode string) ([]string, error)

	// 生成系统提示词
	GenerateSystemPrompt(ctx *EnhancedContext) (string, error)
}

// ContextManager 上下文管理器
type ContextManager struct {
	Collector ContextCollector
	Formatter ContextFormatter
	Generator PromptGenerator
}
