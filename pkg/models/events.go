package models

import (
	"strings"
	"time"

	"github.com/google/go-github/v58/github"
)

// EventType 定义GitHub事件类型
type EventType string

const (
	EventIssueComment           EventType = "issue_comment"
	EventPullRequestReview      EventType = "pull_request_review"
	EventPullRequestReviewComment EventType = "pull_request_review_comment"
	EventIssues                 EventType = "issues"
	EventPullRequest            EventType = "pull_request"
	EventWorkflowDispatch       EventType = "workflow_dispatch"
	EventSchedule               EventType = "schedule"
	EventPush                   EventType = "push"
)

// GitHubContext 是所有GitHub事件上下文的接口
// 对应claude-code-action中的判别式联合类型
type GitHubContext interface {
	GetEventType() EventType
	GetRepository() *github.Repository
	GetSender() *github.User
	GetRawEvent() interface{}
	GetEventAction() string
	GetDeliveryID() string
	GetTimestamp() time.Time
}

// BaseContext 提供所有事件上下文的基础实现
type BaseContext struct {
	Type       EventType           `json:"type"`
	Repository *github.Repository  `json:"repository"`
	Sender     *github.User        `json:"sender"`
	RawEvent   interface{}         `json:"raw_event"`
	Action     string              `json:"action"`
	DeliveryID string              `json:"delivery_id"`
	Timestamp  time.Time           `json:"timestamp"`
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
	Issue   *github.Issue       `json:"issue"`
	Comment *github.IssueComment `json:"comment"`
	// 是否是PR评论（通过Issue.PullRequestLinks判断）
	IsPRComment bool `json:"is_pr_comment"`
}

// PullRequestReviewContext PR Review事件上下文
type PullRequestReviewContext struct {
	BaseContext
	PullRequest *github.PullRequest     `json:"pull_request"`
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
	Ref     string                    `json:"ref"`
	Commits []*github.HeadCommit      `json:"commits"`
	Before  string                    `json:"before"`
	After   string                    `json:"after"`
}

// CommandInfo 提取的命令信息
type CommandInfo struct {
	Command   string `json:"command"`    // /code, /continue, /fix
	AIModel   string `json:"ai_model"`   // claude, gemini
	Args      string `json:"args"`       // 命令参数
	RawText   string `json:"raw_text"`   // 原始文本
}

// 命令类型
const (
	CommandCode     = "/code"
	CommandContinue = "/continue"
	CommandFix      = "/fix"
)

// AI模型类型
const (
	AIModelClaude = "claude"
	AIModelGemini = "gemini"
)

// HasCommand 检查上下文是否包含命令
func HasCommand(ctx GitHubContext) (*CommandInfo, bool) {
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
	default:
		return nil, false
	}
	
	return parseCommand(content)
}

// parseCommand 解析命令字符串
func parseCommand(content string) (*CommandInfo, bool) {
	content = strings.TrimSpace(content)
	
	var command string
	var remaining string
	
	// 检测命令类型
	if strings.HasPrefix(content, CommandCode) {
		command = CommandCode
		remaining = strings.TrimSpace(strings.TrimPrefix(content, CommandCode))
	} else if strings.HasPrefix(content, CommandContinue) {
		command = CommandContinue
		remaining = strings.TrimSpace(strings.TrimPrefix(content, CommandContinue))
	} else if strings.HasPrefix(content, CommandFix) {
		command = CommandFix
		remaining = strings.TrimSpace(strings.TrimPrefix(content, CommandFix))
	} else {
		return nil, false
	}
	
	// 解析AI模型
	var aiModel string
	var args string
	
	if strings.HasPrefix(remaining, "-claude") {
		aiModel = AIModelClaude
		args = strings.TrimSpace(strings.TrimPrefix(remaining, "-claude"))
	} else if strings.HasPrefix(remaining, "-gemini") {
		aiModel = AIModelGemini
		args = strings.TrimSpace(strings.TrimPrefix(remaining, "-gemini"))
	} else {
		// 没有指定AI模型，使用默认
		aiModel = "" // 将在后续处理中设置默认值
		args = remaining
	}
	
	return &CommandInfo{
		Command: command,
		AIModel: aiModel,
		Args:    args,
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