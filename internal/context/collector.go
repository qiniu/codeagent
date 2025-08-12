package context

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/qiniu/codeagent/internal/content"
	ghclient "github.com/qiniu/codeagent/internal/github"

	"github.com/google/go-github/v58/github"
)

// DefaultContextCollector 默认上下文收集器实现
type DefaultContextCollector struct {
	githubClient     *ghclient.Client
	contentProcessor *content.Processor
}

// NewDefaultContextCollector 创建默认上下文收集器
func NewDefaultContextCollector(githubClient *ghclient.Client) *DefaultContextCollector {
	return &DefaultContextCollector{
		githubClient:     githubClient,
		contentProcessor: content.NewProcessor(),
	}
}

// processContentSafely safely processes content through rich content processor
func (c *DefaultContextCollector) processContentSafely(ctx context.Context, body string) string {
	if body == "" {
		return body
	}
	
	richContent, err := c.contentProcessor.ProcessContent(ctx, body)
	if err != nil {
		// Log error but return original content as fallback
		return body
	}
	
	return c.contentProcessor.FormatForAI(richContent)
}

// CollectBasicContext 收集基础上下文
func (c *DefaultContextCollector) CollectBasicContext(eventType string, payload interface{}) (*EnhancedContext, error) {
	return c.CollectBasicContextWithProcessor(context.Background(), eventType, payload)
}

// CollectBasicContextWithProcessor 收集基础上下文（支持富内容处理）
func (c *DefaultContextCollector) CollectBasicContextWithProcessor(ctx context.Context, eventType string, payload interface{}) (*EnhancedContext, error) {
	enhancedCtx := &EnhancedContext{
		Timestamp: time.Now(),
		Metadata:  make(map[string]interface{}),
	}

	switch eventType {
	case "issue_comment":
		if event, ok := payload.(*github.IssueCommentEvent); ok {
			enhancedCtx.Type = ContextTypeIssue
			enhancedCtx.Subject = event
			enhancedCtx.Priority = PriorityHigh
			enhancedCtx.Metadata["issue_number"] = event.Issue.GetNumber()
			enhancedCtx.Metadata["issue_title"] = event.Issue.GetTitle()
			enhancedCtx.Metadata["issue_body"] = c.processContentSafely(ctx, event.Issue.GetBody())
			enhancedCtx.Metadata["comment_body"] = c.processContentSafely(ctx, event.Comment.GetBody())
			enhancedCtx.Metadata["comment_id"] = event.Comment.GetID()
			enhancedCtx.Metadata["repository"] = event.Repo.GetFullName()
			enhancedCtx.Metadata["sender"] = event.GetSender().GetLogin()
		}
	case "pull_request_review_comment":
		if event, ok := payload.(*github.PullRequestReviewCommentEvent); ok {
			enhancedCtx.Type = ContextTypeReviewComment
			enhancedCtx.Subject = event
			enhancedCtx.Priority = PriorityMedium
			enhancedCtx.Metadata["pr_number"] = event.PullRequest.GetNumber()
			enhancedCtx.Metadata["comment_id"] = event.Comment.GetID()
			enhancedCtx.Metadata["comment_body"] = c.processContentSafely(ctx, event.Comment.GetBody())
			enhancedCtx.Metadata["file_path"] = event.Comment.GetPath()
			enhancedCtx.Metadata["line_number"] = event.Comment.GetLine()
		}
	case "pull_request_review":
		if event, ok := payload.(*github.PullRequestReviewEvent); ok {
			enhancedCtx.Type = ContextTypeReview
			enhancedCtx.Subject = event
			enhancedCtx.Priority = PriorityHigh
			enhancedCtx.Metadata["pr_number"] = event.PullRequest.GetNumber()
			enhancedCtx.Metadata["review_id"] = event.Review.GetID()
			enhancedCtx.Metadata["review_state"] = event.Review.GetState()
			enhancedCtx.Metadata["review_body"] = c.processContentSafely(ctx, event.Review.GetBody())
		}
	case "pull_request":
		if event, ok := payload.(*github.PullRequestEvent); ok {
			enhancedCtx.Type = ContextTypePR
			enhancedCtx.Subject = event
			enhancedCtx.Priority = PriorityCritical
			enhancedCtx.Metadata["pr_number"] = event.PullRequest.GetNumber()
			enhancedCtx.Metadata["pr_body"] = c.processContentSafely(ctx, event.PullRequest.GetBody())
			enhancedCtx.Metadata["action"] = event.GetAction()
		}
	case "issues":
		if event, ok := payload.(*github.IssuesEvent); ok {
			enhancedCtx.Type = ContextTypeIssue
			enhancedCtx.Subject = event
			enhancedCtx.Priority = PriorityMedium
			enhancedCtx.Metadata["issue_number"] = event.Issue.GetNumber()
			enhancedCtx.Metadata["issue_body"] = c.processContentSafely(ctx, event.Issue.GetBody())
			enhancedCtx.Metadata["action"] = event.GetAction()
		}
	default:
		return nil, fmt.Errorf("unsupported event type: %s", eventType)
	}

	return enhancedCtx, nil
}

// CollectCodeContext 收集代码上下文
func (c *DefaultContextCollector) CollectCodeContext(pr *github.PullRequest) (*CodeContext, error) {
	codeCtx := &CodeContext{
		Repository: pr.GetBase().GetRepo().GetFullName(),
		BaseBranch: pr.GetBase().GetRef(),
		HeadBranch: pr.GetHead().GetRef(),
		Files:      []FileChange{},
	}

	// 获取PR文件列表
	repoOwner := pr.GetBase().GetRepo().GetOwner().GetLogin()
	repoName := pr.GetBase().GetRepo().GetName()
	prNumber := pr.GetNumber()

	files, _, err := c.githubClient.GetClient().PullRequests.ListFiles(
		nil, repoOwner, repoName, prNumber, nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get PR files: %w", err)
	}

	totalAdditions := 0
	totalDeletions := 0

	for _, file := range files {
		change := FileChange{
			Path:      file.GetFilename(),
			Status:    file.GetStatus(),
			Additions: file.GetAdditions(),
			Deletions: file.GetDeletions(),
			Changes:   file.GetChanges(),
			SHA:       file.GetSHA(),
		}

		if file.GetPatch() != "" {
			// 只保留前1000字符的patch信息，避免上下文过长
			patch := file.GetPatch()
			if len(patch) > 1000 {
				patch = patch[:1000] + "\n... (truncated)"
			}
			change.Patch = patch
		}

		if file.GetPreviousFilename() != "" {
			change.PreviousPath = file.GetPreviousFilename()
		}

		codeCtx.Files = append(codeCtx.Files, change)
		totalAdditions += file.GetAdditions()
		totalDeletions += file.GetDeletions()
	}

	codeCtx.TotalChanges.Additions = totalAdditions
	codeCtx.TotalChanges.Deletions = totalDeletions
	codeCtx.TotalChanges.Files = len(files)

	return codeCtx, nil
}

// CollectGitHubContext 收集GitHub上下文信息
// 专注于GitHub原生数据收集
func (c *DefaultContextCollector) CollectGitHubContext(repoFullName string, prNumber int) (*GitHubContext, error) {
	ctx := &GitHubContext{
		Repository: repoFullName,
		PRNumber:   prNumber,
	}

	// 解析仓库信息
	parts := strings.Split(repoFullName, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repository format: %s", repoFullName)
	}
	owner, repo := parts[0], parts[1]

	// 获取PR详情
	pr, _, err := c.githubClient.GetClient().PullRequests.Get(nil, owner, repo, prNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to get PR: %w", err)
	}

	ctx.PR = &PullRequestContext{
		Number:     pr.GetNumber(),
		Title:      pr.GetTitle(),
		Body:       pr.GetBody(),
		State:      pr.GetState(),
		Author:     pr.GetUser().GetLogin(),
		BaseBranch: pr.GetBase().GetRef(),
		HeadBranch: pr.GetHead().GetRef(),
		Additions:  pr.GetAdditions(),
		Deletions:  pr.GetDeletions(),
		Commits:    pr.GetCommits(),
	}

	// 获取PR文件变更
	files, _, err := c.githubClient.GetClient().PullRequests.ListFiles(
		nil, owner, repo, prNumber, nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get PR files: %w", err)
	}

	for _, file := range files {
		ctx.Files = append(ctx.Files, FileChange{
			Path:      file.GetFilename(),
			Status:    file.GetStatus(),
			Additions: file.GetAdditions(),
			Deletions: file.GetDeletions(),
			Changes:   file.GetChanges(),
			SHA:       file.GetSHA(),
		})
	}

	return ctx, nil
}

// CollectCommentContext 收集评论上下文
func (c *DefaultContextCollector) CollectCommentContext(pr *github.PullRequest, currentCommentID int64) ([]CommentContext, error) {
	return c.CollectCommentContextWithProcessor(context.Background(), pr, currentCommentID)
}

// CollectCommentContextWithProcessor 收集评论上下文（支持富内容处理）
func (c *DefaultContextCollector) CollectCommentContextWithProcessor(ctx context.Context, pr *github.PullRequest, currentCommentID int64) ([]CommentContext, error) {
	// 使用现有的方法获取所有评论
	allComments, err := c.githubClient.GetAllPRComments(pr)
	if err != nil {
		return nil, fmt.Errorf("failed to get all PR comments: %w", err)
	}

	var comments []CommentContext

	// 处理一般评论
	for _, comment := range allComments.IssueComments {
		if comment.GetID() != currentCommentID {
			comments = append(comments, CommentContext{
				ID:        comment.GetID(),
				Type:      "comment",
				Author:    comment.GetUser().GetLogin(),
				Body:      c.processContentSafely(ctx, comment.GetBody()),
				CreatedAt: comment.GetCreatedAt().Time,
				UpdatedAt: comment.GetUpdatedAt().Time,
			})
		}
	}

	// 处理代码行评论
	for _, comment := range allComments.ReviewComments {
		if comment.GetID() != currentCommentID {
			comments = append(comments, CommentContext{
				ID:         comment.GetID(),
				Type:       "review_comment",
				Author:     comment.GetUser().GetLogin(),
				Body:       c.processContentSafely(ctx, comment.GetBody()),
				CreatedAt:  comment.GetCreatedAt().Time,
				UpdatedAt:  comment.GetUpdatedAt().Time,
				FilePath:   comment.GetPath(),
				LineNumber: comment.GetLine(),
				StartLine:  comment.GetStartLine(),
			})
		}
	}

	// 处理Review评论
	for _, review := range allComments.Reviews {
		if review.GetBody() != "" {
			comments = append(comments, CommentContext{
				ID:          review.GetID(),
				Type:        "review",
				Author:      review.GetUser().GetLogin(),
				Body:        c.processContentSafely(ctx, review.GetBody()),
				CreatedAt:   review.GetSubmittedAt().Time,
				UpdatedAt:   review.GetSubmittedAt().Time,
				ReviewState: review.GetState(),
			})
		}
	}

	return comments, nil
}
