package context

import (
	"context"
	"fmt"
	"strings"
	"time"

	ghclient "github.com/qiniu/codeagent/internal/github"
	"github.com/qiniu/codeagent/pkg/models"

	"github.com/google/go-github/v58/github"
)

// DefaultContextCollector 默认上下文收集器实现
type DefaultContextCollector struct {
	clientManager ghclient.ClientManagerInterface
}

// NewDefaultContextCollector 创建默认上下文收集器
func NewDefaultContextCollector(clientManager ghclient.ClientManagerInterface) *DefaultContextCollector {
	return &DefaultContextCollector{
		clientManager: clientManager,
	}
}

// extractRepoFromPR 从PR中提取repository信息
func extractRepoFromPR(pr *github.PullRequest) *models.Repository {
	if pr == nil || pr.GetBase() == nil || pr.GetBase().GetRepo() == nil {
		return nil
	}

	repo := pr.GetBase().GetRepo()
	return &models.Repository{
		Owner: repo.GetOwner().GetLogin(),
		Name:  repo.GetName(),
	}
}

// extractRepoFromFullName 从"owner/repo"格式字符串提取repository信息
func extractRepoFromFullName(repoFullName string) *models.Repository {
	parts := strings.Split(repoFullName, "/")
	if len(parts) != 2 {
		return nil
	}

	return &models.Repository{
		Owner: parts[0],
		Name:  parts[1],
	}
}

// CollectBasicContext 收集基础上下文
func (c *DefaultContextCollector) CollectBasicContext(eventType string, payload interface{}) (*EnhancedContext, error) {
	ctx := &EnhancedContext{
		Timestamp: time.Now(),
		Metadata:  make(map[string]interface{}),
	}

	switch eventType {
	case "issue_comment":
		if event, ok := payload.(*github.IssueCommentEvent); ok {
			ctx.Type = ContextTypeIssue
			ctx.Subject = event
			ctx.Priority = PriorityHigh
			ctx.Metadata["issue_number"] = event.Issue.GetNumber()
			ctx.Metadata["issue_title"] = event.Issue.GetTitle()
			ctx.Metadata["issue_body"] = event.Issue.GetBody()
			ctx.Metadata["comment_id"] = event.Comment.GetID()
			ctx.Metadata["repository"] = event.Repo.GetFullName()
			ctx.Metadata["sender"] = event.GetSender().GetLogin()
		}
	case "pull_request_review_comment":
		if event, ok := payload.(*github.PullRequestReviewCommentEvent); ok {
			ctx.Type = ContextTypeReviewComment
			ctx.Subject = event
			ctx.Priority = PriorityMedium
			ctx.Metadata["pr_number"] = event.PullRequest.GetNumber()
			ctx.Metadata["comment_id"] = event.Comment.GetID()
			ctx.Metadata["file_path"] = event.Comment.GetPath()
			ctx.Metadata["line_number"] = event.Comment.GetLine()
		}
	case "pull_request_review":
		if event, ok := payload.(*github.PullRequestReviewEvent); ok {
			ctx.Type = ContextTypeReview
			ctx.Subject = event
			ctx.Priority = PriorityHigh
			ctx.Metadata["pr_number"] = event.PullRequest.GetNumber()
			ctx.Metadata["review_id"] = event.Review.GetID()
			ctx.Metadata["review_state"] = event.Review.GetState()
		}
	case "pull_request":
		if event, ok := payload.(*github.PullRequestEvent); ok {
			ctx.Type = ContextTypePR
			ctx.Subject = event
			ctx.Priority = PriorityCritical
			ctx.Metadata["pr_number"] = event.PullRequest.GetNumber()
			ctx.Metadata["action"] = event.GetAction()
		}
	case "issues":
		if event, ok := payload.(*github.IssuesEvent); ok {
			ctx.Type = ContextTypeIssue
			ctx.Subject = event
			ctx.Priority = PriorityMedium
			ctx.Metadata["issue_number"] = event.Issue.GetNumber()
			ctx.Metadata["action"] = event.GetAction()
		}
	default:
		return nil, fmt.Errorf("unsupported event type: %s", eventType)
	}

	return ctx, nil
}

// CollectCodeContext 收集代码上下文
func (c *DefaultContextCollector) CollectCodeContext(pr *github.PullRequest) (*CodeContext, error) {
	// 提取repository信息
	repo := extractRepoFromPR(pr)
	if repo == nil {
		return nil, fmt.Errorf("failed to extract repository info from PR")
	}

	// 获取对应的GitHub客户端
	client, err := c.clientManager.GetClient(context.Background(), repo)
	if err != nil {
		return nil, fmt.Errorf("failed to get GitHub client: %w", err)
	}

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

	files, _, err := client.GetClient().PullRequests.ListFiles(
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
	// 提取repository信息
	repo := extractRepoFromFullName(repoFullName)
	if repo == nil {
		return nil, fmt.Errorf("invalid repository format: %s", repoFullName)
	}

	// 获取对应的GitHub客户端
	client, err := c.clientManager.GetClient(context.Background(), repo)
	if err != nil {
		return nil, fmt.Errorf("failed to get GitHub client: %w", err)
	}

	ctx := &GitHubContext{
		Repository: repoFullName,
		PRNumber:   prNumber,
	}

	// 获取PR详情
	pr, _, err := client.GetClient().PullRequests.Get(nil, repo.Owner, repo.Name, prNumber)
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
	files, _, err := client.GetClient().PullRequests.ListFiles(
		nil, repo.Owner, repo.Name, prNumber, nil,
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
	// 提取repository信息
	repo := extractRepoFromPR(pr)
	if repo == nil {
		return nil, fmt.Errorf("failed to extract repository info from PR")
	}

	// 获取对应的GitHub客户端
	client, err := c.clientManager.GetClient(context.Background(), repo)
	if err != nil {
		return nil, fmt.Errorf("failed to get GitHub client: %w", err)
	}

	// 使用现有的方法获取所有评论
	allComments, err := client.GetAllPRComments(pr)
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
				Body:      comment.GetBody(),
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
				Body:       comment.GetBody(),
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
				Body:        review.GetBody(),
				CreatedAt:   review.GetSubmittedAt().Time,
				UpdatedAt:   review.GetSubmittedAt().Time,
				ReviewState: review.GetState(),
			})
		}
	}

	return comments, nil
}
