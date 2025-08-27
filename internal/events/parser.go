package events

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/qiniu/codeagent/pkg/models"

	"github.com/google/go-github/v58/github"
)

// EventParser 事件解析器
type EventParser struct{}

// Parser 事件解析器接口（向前兼容）
type Parser = EventParser

// NewParser 创建新的事件解析器（向前兼容）
func NewParser() *Parser {
	return NewEventParser()
}

// NewEventParser 创建新的事件解析器
func NewEventParser() *EventParser {
	return &EventParser{}
}

// ParseEvent 解析通用事件（兼容性方法）
func (p *EventParser) ParseEvent(ctx context.Context, eventType string, rawEvent interface{}) (models.GitHubContext, error) {
	switch models.EventType(eventType) {
	case models.EventIssueComment:
		if event, ok := rawEvent.(*github.IssueCommentEvent); ok {
			return p.ParseIssueCommentEvent(ctx, event)
		}
	}
	return nil, UnsupportedEventTypeError(eventType)
}

// ParseIssueCommentEvent 解析Issue评论事件（从原始GitHub事件）
func (p *EventParser) ParseIssueCommentEvent(ctx context.Context, event *github.IssueCommentEvent) (models.GitHubContext, error) {
	if event == nil {
		return nil, fmt.Errorf("event is nil")
	}

	// 检查是否是PR评论
	isPRComment := event.Issue != nil && event.Issue.PullRequestLinks != nil

	return &models.IssueCommentContext{
		BaseContext: models.BaseContext{
			Type:       models.EventIssueComment,
			Repository: event.Repo,
			Sender:     event.Sender,
			Action:     event.GetAction(),
			RawEvent:   event,
			Timestamp:  time.Now(),
		},
		Issue:       event.Issue,
		Comment:     event.Comment,
		IsPRComment: isPRComment,
	}, nil
}

// ParseWebhookEvent 解析webhook事件为统一的GitHubContext
func (p *EventParser) ParseWebhookEvent(
	ctx context.Context,
	eventType string,
	deliveryID string,
	payload []byte,
) (models.GitHubContext, error) {
	// 验证事件类型
	if !models.IsValidEventType(eventType) {
		return nil, UnsupportedEventTypeError(eventType)
	}

	// 根据事件类型解析
	switch models.EventType(eventType) {
	case models.EventIssueComment:
		return p.parseIssueCommentEvent(ctx, payload, deliveryID)
	case models.EventPullRequestReview:
		return p.parsePullRequestReviewEvent(ctx, payload, deliveryID)
	case models.EventPullRequestReviewComment:
		return p.parsePullRequestReviewCommentEvent(ctx, payload, deliveryID)
	case models.EventIssues:
		return p.parseIssuesEvent(ctx, payload, deliveryID)
	case models.EventPullRequest:
		return p.parsePullRequestEvent(ctx, payload, deliveryID)
	case models.EventPush:
		return p.parsePushEvent(ctx, payload, deliveryID)
	default:
		return nil, UnsupportedEventTypeError(eventType)
	}
}

// parseIssueCommentEvent 解析Issue评论事件
func (p *EventParser) parseIssueCommentEvent(
	ctx context.Context,
	payload []byte,
	deliveryID string,
) (*models.IssueCommentContext, error) {
	var event github.IssueCommentEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, ParsingError("issue_comment", err)
	}

	// 检查必需字段
	if event.Repo == nil {
		return nil, ValidationError("issue_comment", ErrMissingRepository, "")
	}
	if event.Sender == nil {
		return nil, ValidationError("issue_comment", ErrMissingSender, "")
	}
	if event.Issue == nil {
		return nil, ValidationError("issue_comment", ErrMissingIssue, "")
	}
	if event.Comment == nil {
		return nil, ValidationError("issue_comment", ErrMissingComment, "")
	}

	// 判断是否是PR评论
	isPRComment := event.Issue.PullRequestLinks != nil

	return &models.IssueCommentContext{
		BaseContext: models.BaseContext{
			Type:       models.EventIssueComment,
			Repository: event.Repo,
			Sender:     event.Sender,
			RawEvent:   &event,
			Action:     event.GetAction(),
			DeliveryID: deliveryID,
			Timestamp:  time.Now(),
		},
		Issue:       event.Issue,
		Comment:     event.Comment,
		IsPRComment: isPRComment,
	}, nil
}

// parsePullRequestReviewEvent 解析PR Review事件
func (p *EventParser) parsePullRequestReviewEvent(
	ctx context.Context,
	payload []byte,
	deliveryID string,
) (*models.PullRequestReviewContext, error) {
	var event github.PullRequestReviewEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, fmt.Errorf("failed to unmarshal pull request review event: %w", err)
	}

	// 检查必需字段
	if event.Repo == nil {
		return nil, fmt.Errorf("missing repository in pull request review event")
	}
	if event.Sender == nil {
		return nil, fmt.Errorf("missing sender in pull request review event")
	}
	if event.PullRequest == nil {
		return nil, fmt.Errorf("missing pull request in pull request review event")
	}
	if event.Review == nil {
		return nil, fmt.Errorf("missing review in pull request review event")
	}

	return &models.PullRequestReviewContext{
		BaseContext: models.BaseContext{
			Type:       models.EventPullRequestReview,
			Repository: event.Repo,
			Sender:     event.Sender,
			RawEvent:   &event,
			Action:     event.GetAction(),
			DeliveryID: deliveryID,
			Timestamp:  time.Now(),
		},
		PullRequest: event.PullRequest,
		Review:      event.Review,
	}, nil
}

// parsePullRequestReviewCommentEvent 解析PR Review评论事件
func (p *EventParser) parsePullRequestReviewCommentEvent(
	ctx context.Context,
	payload []byte,
	deliveryID string,
) (*models.PullRequestReviewCommentContext, error) {
	var event github.PullRequestReviewCommentEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, fmt.Errorf("failed to unmarshal pull request review comment event: %w", err)
	}

	// 检查必需字段
	if event.Repo == nil {
		return nil, fmt.Errorf("missing repository in pull request review comment event")
	}
	if event.Sender == nil {
		return nil, fmt.Errorf("missing sender in pull request review comment event")
	}
	if event.PullRequest == nil {
		return nil, fmt.Errorf("missing pull request in pull request review comment event")
	}
	if event.Comment == nil {
		return nil, fmt.Errorf("missing comment in pull request review comment event")
	}

	return &models.PullRequestReviewCommentContext{
		BaseContext: models.BaseContext{
			Type:       models.EventPullRequestReviewComment,
			Repository: event.Repo,
			Sender:     event.Sender,
			RawEvent:   &event,
			Action:     event.GetAction(),
			DeliveryID: deliveryID,
			Timestamp:  time.Now(),
		},
		PullRequest: event.PullRequest,
		Comment:     event.Comment,
	}, nil
}

// parseIssuesEvent 解析Issues事件
func (p *EventParser) parseIssuesEvent(
	ctx context.Context,
	payload []byte,
	deliveryID string,
) (*models.IssuesContext, error) {
	var event github.IssuesEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, fmt.Errorf("failed to unmarshal issues event: %w", err)
	}

	// 检查必需字段
	if event.Repo == nil {
		return nil, fmt.Errorf("missing repository in issues event")
	}
	if event.Sender == nil {
		return nil, fmt.Errorf("missing sender in issues event")
	}
	if event.Issue == nil {
		return nil, fmt.Errorf("missing issue in issues event")
	}

	return &models.IssuesContext{
		BaseContext: models.BaseContext{
			Type:       models.EventIssues,
			Repository: event.Repo,
			Sender:     event.Sender,
			RawEvent:   &event,
			Action:     event.GetAction(),
			DeliveryID: deliveryID,
			Timestamp:  time.Now(),
		},
		Issue: event.Issue,
	}, nil
}

// parsePullRequestEvent 解析PR事件
func (p *EventParser) parsePullRequestEvent(
	ctx context.Context,
	payload []byte,
	deliveryID string,
) (*models.PullRequestContext, error) {
	var event github.PullRequestEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, fmt.Errorf("failed to unmarshal pull request event: %w", err)
	}

	// 检查必需字段
	if event.Repo == nil {
		return nil, fmt.Errorf("missing repository in pull request event")
	}
	if event.Sender == nil {
		return nil, fmt.Errorf("missing sender in pull request event")
	}
	if event.PullRequest == nil {
		return nil, fmt.Errorf("missing pull request in pull request event")
	}

	return &models.PullRequestContext{
		BaseContext: models.BaseContext{
			Type:       models.EventPullRequest,
			Repository: event.Repo,
			Sender:     event.Sender,
			RawEvent:   &event,
			Action:     event.GetAction(),
			DeliveryID: deliveryID,
			Timestamp:  time.Now(),
		},
		PullRequest: event.PullRequest,
	}, nil
}

// parsePushEvent 解析Push事件
func (p *EventParser) parsePushEvent(
	ctx context.Context,
	payload []byte,
	deliveryID string,
) (*models.PushContext, error) {
	var event github.PushEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, fmt.Errorf("failed to unmarshal push event: %w", err)
	}

	// 检查必需字段
	if event.Repo == nil {
		return nil, fmt.Errorf("missing repository in push event")
	}
	if event.Sender == nil {
		return nil, fmt.Errorf("missing sender in push event")
	}

	// Push事件的Repository类型是PushEventRepository，需要转换
	var repo *github.Repository
	if event.Repo != nil {
		repo = &github.Repository{
			ID:       event.Repo.ID,
			Name:     event.Repo.Name,
			FullName: event.Repo.FullName,
			Owner:    event.Repo.Owner,
		}
	}

	return &models.PushContext{
		BaseContext: models.BaseContext{
			Type:       models.EventPush,
			Repository: repo,
			Sender:     event.Sender,
			RawEvent:   &event,
			Action:     "", // Push事件没有action
			DeliveryID: deliveryID,
			Timestamp:  time.Now(),
		},
		Ref:     event.GetRef(),
		Commits: event.Commits,
		Before:  event.GetBefore(),
		After:   event.GetAfter(),
	}, nil
}
