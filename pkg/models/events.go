package models

import (
	"regexp"
	"strings"
	"time"

	"github.com/google/go-github/v58/github"
)

// EventType defines GitHub event types
type EventType string

const (
	EventIssueComment             EventType = "issue_comment"
	EventPullRequestReview        EventType = "pull_request_review"
	EventPullRequestReviewComment EventType = "pull_request_review_comment"
	EventIssues                   EventType = "issues"
	EventPullRequest              EventType = "pull_request"
	EventWorkflowDispatch         EventType = "workflow_dispatch"
	EventSchedule                 EventType = "schedule"
	EventPush                     EventType = "push"
)

// GitHubContext is the interface for all GitHub event contexts
type GitHubContext interface {
	GetEventType() EventType
	GetRepository() *github.Repository
	GetSender() *github.User
	GetRawEvent() interface{}
	GetEventAction() string
	GetDeliveryID() string
	GetTimestamp() time.Time
}

// BaseContext provides base implementation for all event contexts
type BaseContext struct {
	Type       EventType          `json:"type"`
	Repository *github.Repository `json:"repository"`
	Sender     *github.User       `json:"sender"`
	RawEvent   interface{}        `json:"raw_event"`
	Action     string             `json:"action"`
	DeliveryID string             `json:"delivery_id"`
	Timestamp  time.Time          `json:"timestamp"`
}

func (bc *BaseContext) GetEventType() EventType {
	return bc.Type
}

func (bc *BaseContext) GetRepository() *github.Repository {
	return bc.Repository
}

func (bc *BaseContext) GetSender() *github.User {
	return bc.Sender
}

func (bc *BaseContext) GetRawEvent() interface{} {
	return bc.RawEvent
}

func (bc *BaseContext) GetEventAction() string {
	return bc.Action
}

func (bc *BaseContext) GetDeliveryID() string {
	return bc.DeliveryID
}

func (bc *BaseContext) GetTimestamp() time.Time {
	return bc.Timestamp
}

// IssueCommentContext Issue评论事件上下文
type IssueCommentContext struct {
	BaseContext
	Issue   *github.Issue        `json:"issue"`
	Comment *github.IssueComment `json:"comment"`
	// 是否是PR评论（通过Issue.PullRequestLinks判断）
	IsPRComment bool `json:"is_pr_comment"`
}

// PullRequestReviewContext PR Review事件上下文
type PullRequestReviewContext struct {
	BaseContext
	PullRequest *github.PullRequest       `json:"pull_request"`
	Review      *github.PullRequestReview `json:"review"`
}

// PullRequestReviewCommentContext PR Review评论事件上下文
type PullRequestReviewCommentContext struct {
	BaseContext
	PullRequest *github.PullRequest        `json:"pull_request"`
	Comment     *github.PullRequestComment `json:"comment"`
}

// IssuesContext Issue事件上下文
type IssuesContext struct {
	BaseContext
	Issue *github.Issue `json:"issue"`
}

// PullRequestContext PR事件上下文
type PullRequestContext struct {
	BaseContext
	PullRequest *github.PullRequest `json:"pull_request"`
}

// WorkflowDispatchContext workflow_dispatch事件上下文
type WorkflowDispatchContext struct {
	BaseContext
	Inputs map[string]interface{} `json:"inputs"`
}

// ScheduleContext schedule事件上下文
type ScheduleContext struct {
	BaseContext
	Cron string `json:"cron"`
}

// PushContext push事件上下文
type PushContext struct {
	BaseContext
	Ref     string               `json:"ref"`
	Commits []*github.HeadCommit `json:"commits"`
	Before  string               `json:"before"`
	After   string               `json:"after"`
}

// Repository 简单的仓库信息结构体
type Repository struct {
	Owner string `json:"owner"` // 仓库所有者（组织或用户）
	Name  string `json:"name"`  // 仓库名称
}

// CommandInfo 提取的命令信息
type CommandInfo struct {
	Command string `json:"command"`  // 任意斜杠命令，如 /analyze, /deploy, /test
	AIModel string `json:"ai_model"` // claude, gemini
	Args    string `json:"args"`     // 命令参数
	RawText string `json:"raw_text"` // 原始文本
}

// 命令类型
const (
	CommandCode     = "/code"
	CommandContinue = "/continue"
	CommandClaude   = "@qiniu-ci"
	CommandReview   = "/review"
)

// AI模型类型
const (
	AIModelClaude = "claude"
	AIModelGemini = "gemini"
)

// MentionConfig 提及配置接口
type MentionConfig interface {
	GetTriggers() []string
	GetDefaultTrigger() string
}

// ConfigMentionAdapter 从内部config包适配到models包
type ConfigMentionAdapter struct {
	Triggers       []string
	DefaultTrigger string
}

func (c *ConfigMentionAdapter) GetTriggers() []string {
	return c.Triggers
}

func (c *ConfigMentionAdapter) GetDefaultTrigger() string {
	return c.DefaultTrigger
}

// HasCommand 检查上下文是否包含命令（使用默认mention配置）
func HasCommand(ctx GitHubContext) (*CommandInfo, bool) {
	return HasCommandWithConfig(ctx, nil)
}

// HasCommandWithConfig 检查上下文是否包含命令（支持自定义mention配置）
func HasCommandWithConfig(ctx GitHubContext, mentionConfig MentionConfig) (*CommandInfo, bool) {
	var content string

	switch c := ctx.(type) {
	case *IssueCommentContext:
		if c.Comment != nil {
			content = c.Comment.GetBody()
		}
	case *PullRequestReviewContext:
		if c.Review != nil {
			content = c.Review.GetBody()
		}
	case *PullRequestReviewCommentContext:
		if c.Comment != nil {
			content = c.Comment.GetBody()
		}
	case *IssuesContext:
		// 支持Issue中的@qiniu-ci命令
		if c.Issue != nil {
			content = c.Issue.GetBody()
		}
	default:
		return nil, false
	}
	// First try to parse as slash command
	if cmdInfo, hasCmd := parseCommand(content); hasCmd {
		return cmdInfo, true
	}

	// Then try to parse as mention with config
	if mentionConfig != nil {
		return parseMentionWithConfig(content, mentionConfig)
	}
	// Fallback to default mention parsing
	return parseMention(content)
}

// parseCommand 解析命令字符串
func parseCommand(content string) (*CommandInfo, bool) {
	content = strings.TrimSpace(content)

	// 检查是否以斜杠开头
	if !strings.HasPrefix(content, "/") {
		return nil, false
	}

	// 提取命令名（第一个空格之前的部分，去掉斜杠）
	parts := strings.SplitN(content[1:], " ", 2)
	command := "/" + parts[0] // 重新添加斜杠前缀

	var remaining string
	if len(parts) > 1 {
		remaining = strings.TrimSpace(parts[1])
	}

	// 解析AI模型和参数
	var aiModel string
	var args string

	if strings.HasPrefix(remaining, "-claude") {
		aiModel = AIModelClaude
		args = strings.TrimSpace(strings.TrimPrefix(remaining, "-claude"))
	} else if strings.HasPrefix(remaining, "-gemini") {
		aiModel = AIModelGemini
		args = strings.TrimSpace(strings.TrimPrefix(remaining, "-gemini"))
	} else {
		aiModel = ""
		args = remaining
	}

	return &CommandInfo{
		Command: command,
		AIModel: aiModel,
		Args:    args,
		RawText: content,
	}, true
}

// parseMention 解析@qiniu-ci提及（默认触发词，向后兼容）
func parseMention(content string) (*CommandInfo, bool) {
	return parseMentionWithTrigger(content, CommandClaude)
}

// parseMentionWithConfig 使用配置解析mention
func parseMentionWithConfig(content string, config MentionConfig) (*CommandInfo, bool) {
	triggers := config.GetTriggers()
	if len(triggers) == 0 {
		triggers = []string{config.GetDefaultTrigger()}
	}

	// 尝试所有配置的触发词
	for _, trigger := range triggers {
		if trigger == "" {
			continue
		}
		if cmdInfo, found := parseMentionWithTrigger(content, trigger); found {
			return cmdInfo, true
		}
	}

	return nil, false
}

// parseMentionWithTrigger 使用指定触发词解析mention，传递完整评论内容
func parseMentionWithTrigger(content string, trigger string) (*CommandInfo, bool) {
	content = strings.TrimSpace(content)
	pattern := `(^|\s)` + regexp.QuoteMeta(trigger) + `([\s.,!?;:]|$)`
	re := regexp.MustCompile(pattern)

	match := re.FindStringSubmatch(content)
	if match == nil {
		return nil, false
	}

	// NOTE(CarlJin): mention 模式传递暂时完整评论内容
	fullContent := content

	var aiModel string

	// 查找模型指定标志
	if strings.Contains(fullContent, "-claude") {
		aiModel = AIModelClaude
	} else if strings.Contains(fullContent, "-gemini") {
		aiModel = AIModelGemini
	}

	return &CommandInfo{
		Command: CommandClaude, // 总是使用CommandClaude作为mention的标识
		AIModel: aiModel,
		Args:    fullContent, // 传递完整评论内容
		RawText: content,
	}, true
}

// IsValidEventType 检查事件类型是否有效
func IsValidEventType(eventType string) bool {
	switch EventType(eventType) {
	case EventIssueComment, EventPullRequestReview, EventPullRequestReviewComment,
		EventIssues, EventPullRequest, EventWorkflowDispatch, EventSchedule, EventPush:
		return true
	default:
		return false
	}
}
