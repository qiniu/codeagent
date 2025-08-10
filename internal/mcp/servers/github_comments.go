package servers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/qiniu/codeagent/internal/github"
	"github.com/qiniu/codeagent/pkg/models"

	githubapi "github.com/google/go-github/v58/github"
	"github.com/qiniu/x/xlog"
)

// safeExtractInt 安全地从 interface{} 中提取 int 值
func safeExtractInt(value interface{}, paramName string) (int, error) {
	switch v := value.(type) {
	case float64:
		return int(v), nil
	case int:
		return v, nil
	case int64:
		return int(v), nil
	case int32:
		return int(v), nil
	default:
		return 0, fmt.Errorf("invalid %s type: %T", paramName, v)
	}
}

// safeExtractInt64 安全地从 interface{} 中提取 int64 值
func safeExtractInt64(value interface{}, paramName string) (int64, error) {
	switch v := value.(type) {
	case float64:
		return int64(v), nil
	case int:
		return int64(v), nil
	case int64:
		return v, nil
	case int32:
		return int64(v), nil
	default:
		return 0, fmt.Errorf("invalid %s type: %T", paramName, v)
	}
}

// GitHubCommentsServer GitHub评论操作MCP服务器
type GitHubCommentsServer struct {
	client *github.Client
	info   *models.MCPServerInfo
}

// NewGitHubCommentsServer 创建GitHub评论操作服务器
func NewGitHubCommentsServer(client *github.Client) *GitHubCommentsServer {
	return &GitHubCommentsServer{
		client: client,
		info: &models.MCPServerInfo{
			Name:        "github-comments",
			Version:     "1.0.0",
			Description: "GitHub repository comment operations via API",
			Capabilities: models.MCPServerCapabilities{
				Tools: []models.Tool{
					{
						Name:        "create_comment",
						Description: "Create a comment on an issue or pull request",
						InputSchema: &models.JSONSchema{
							Type: "object",
							Properties: map[string]*models.JSONSchema{
								"issue_number": {
									Type:        "integer",
									Description: "Issue or PR number",
								},
								"body": {
									Type:        "string",
									Description: "Comment body (Markdown supported)",
								},
							},
							Required: []string{"issue_number", "body"},
						},
					},
					{
						Name:        "update_comment",
						Description: "Update an existing comment",
						InputSchema: &models.JSONSchema{
							Type: "object",
							Properties: map[string]*models.JSONSchema{
								"comment_id": {
									Type:        "integer",
									Description: "Comment ID to update",
								},
								"body": {
									Type:        "string",
									Description: "New comment body (Markdown supported)",
								},
							},
							Required: []string{"comment_id", "body"},
						},
					},
					{
						Name:        "update_pr_description",
						Description: "Update pull request description/body",
						InputSchema: &models.JSONSchema{
							Type: "object",
							Properties: map[string]*models.JSONSchema{
								"pr_number": {
									Type:        "integer",
									Description: "Pull request number",
								},
								"body": {
									Type:        "string",
									Description: "New PR description/body (Markdown supported)",
								},
							},
							Required: []string{"pr_number", "body"},
						},
					},
					{
						Name:        "list_comments",
						Description: "List comments on an issue or pull request",
						InputSchema: &models.JSONSchema{
							Type: "object",
							Properties: map[string]*models.JSONSchema{
								"issue_number": {
									Type:        "integer",
									Description: "Issue or PR number",
								},
								"since": {
									Type:        "string",
									Description: "Only comments updated after this time (ISO 8601)",
								},
							},
							Required: []string{"issue_number"},
						},
					},
					{
						Name:        "create_review_comment",
						Description: "Create a review comment on a pull request line",
						InputSchema: &models.JSONSchema{
							Type: "object",
							Properties: map[string]*models.JSONSchema{
								"pull_number": {
									Type:        "integer",
									Description: "Pull request number",
								},
								"body": {
									Type:        "string",
									Description: "Review comment body",
								},
								"commit_id": {
									Type:        "string",
									Description: "SHA of the commit to comment on",
								},
								"path": {
									Type:        "string",
									Description: "File path to comment on",
								},
								"line": {
									Type:        "integer",
									Description: "Line number to comment on",
								},
							},
							Required: []string{"pull_number", "body", "commit_id", "path", "line"},
						},
					},
					{
						Name:        "list_pr_comments",
						Description: "List all comments on a pull request (issue + review comments)",
						InputSchema: &models.JSONSchema{
							Type: "object",
							Properties: map[string]*models.JSONSchema{
								"pull_number": {
									Type:        "integer",
									Description: "Pull request number",
								},
							},
							Required: []string{"pull_number"},
						},
					},
				},
			},
			CreatedAt: time.Now(),
		},
	}
}

// GetInfo 获取服务器信息
func (s *GitHubCommentsServer) GetInfo() *models.MCPServerInfo {
	return s.info
}

// GetTools 获取服务器提供的工具列表
func (s *GitHubCommentsServer) GetTools() []models.Tool {
	return s.info.Capabilities.Tools
}

// IsAvailable 检查服务器是否在当前上下文中可用
func (s *GitHubCommentsServer) IsAvailable(ctx context.Context, mcpCtx *models.MCPContext) bool {
	if mcpCtx == nil || mcpCtx.Repository == nil {
		return false
	}

	// 检查是否有GitHub访问权限
	if mcpCtx.Permissions != nil {
		hasReadPerm := false
		for _, perm := range mcpCtx.Permissions {
			if perm == "github:read" || perm == "github:write" || perm == "github:admin" {
				hasReadPerm = true
				break
			}
		}
		if !hasReadPerm {
			return false
		}
	}

	return true
}

// HandleToolCall 处理工具调用
func (s *GitHubCommentsServer) HandleToolCall(ctx context.Context, call *models.ToolCall, mcpCtx *models.MCPContext) (*models.ToolResult, error) {
	xl := xlog.NewWith(ctx)

	if mcpCtx.Repository == nil {
		return nil, fmt.Errorf("no repository context available")
	}

	owner := mcpCtx.Repository.GetRepository().Owner.GetLogin()
	repo := mcpCtx.Repository.GetRepository().GetName()

	// 解析工具名称，去掉服务器前缀
	toolName := call.Function.Name
	if parts := strings.SplitN(call.Function.Name, "_", 2); len(parts) == 2 {
		toolName = parts[1] // 获取去掉前缀的工具名称
	}

	xl.Infof("Executing GitHub comments tool: %s (parsed: %s) on %s/%s", call.Function.Name, toolName, owner, repo)

	switch toolName {
	case "create_comment":
		return s.createComment(ctx, call, owner, repo, mcpCtx)
	case "update_comment":
		return s.updateComment(ctx, call, owner, repo, mcpCtx)
	case "update_pr_description":
		return s.updatePRDescription(ctx, call, owner, repo, mcpCtx)
	case "list_comments":
		return s.listComments(ctx, call, owner, repo)
	case "create_review_comment":
		return s.createReviewComment(ctx, call, owner, repo, mcpCtx)
	case "list_pr_comments":
		return s.listPRComments(ctx, call, owner, repo)
	default:
		return nil, fmt.Errorf("unknown tool: %s", toolName)
	}
}

// Initialize 初始化服务器
func (s *GitHubCommentsServer) Initialize(ctx context.Context) error {
	xl := xlog.NewWith(ctx)
	xl.Infof("Initializing GitHub Comments MCP server")
	return nil
}

// Shutdown 关闭服务器
func (s *GitHubCommentsServer) Shutdown(ctx context.Context) error {
	xl := xlog.NewWith(ctx)
	xl.Infof("Shutting down GitHub Comments MCP server")
	return nil
}

// createComment 创建评论
func (s *GitHubCommentsServer) createComment(ctx context.Context, call *models.ToolCall, owner, repo string, mcpCtx *models.MCPContext) (*models.ToolResult, error) {
	// 安全地提取 issue_number
	issueNumber, err := safeExtractInt(call.Function.Arguments["issue_number"], "issue_number")
	if err != nil {
		return &models.ToolResult{
			ID:      call.ID,
			Success: false,
			Error:   err.Error(),
			Type:    "error",
		}, nil
	}
	body := call.Function.Arguments["body"].(string)

	// 检查写权限
	if !s.hasWritePermission(mcpCtx) {
		return &models.ToolResult{
			ID:      call.ID,
			Success: false,
			Error:   "insufficient permissions for comment creation",
			Type:    "error",
		}, nil
	}

	comment, err := s.client.CreateComment(ctx, owner, repo, issueNumber, body)
	if err != nil {
		return &models.ToolResult{
			ID:      call.ID,
			Success: false,
			Error:   fmt.Sprintf("failed to create comment: %v", err),
			Type:    "error",
		}, nil
	}

	return &models.ToolResult{
		ID:      call.ID,
		Success: true,
		Content: map[string]interface{}{
			"id":           comment.GetID(),
			"url":          comment.GetHTMLURL(),
			"body":         comment.GetBody(),
			"created_at":   comment.GetCreatedAt(),
			"updated_at":   comment.GetUpdatedAt(),
			"author":       comment.User.GetLogin(),
			"issue_number": issueNumber,
		},
		Type: "json",
	}, nil
}

// updateComment 更新评论
func (s *GitHubCommentsServer) updateComment(ctx context.Context, call *models.ToolCall, owner, repo string, mcpCtx *models.MCPContext) (*models.ToolResult, error) {
	// 安全地提取 comment_id
	commentID, err := safeExtractInt64(call.Function.Arguments["comment_id"], "comment_id")
	if err != nil {
		return &models.ToolResult{
			ID:      call.ID,
			Success: false,
			Error:   err.Error(),
			Type:    "error",
		}, nil
	}
	body := call.Function.Arguments["body"].(string)

	// 检查写权限
	if !s.hasWritePermission(mcpCtx) {
		return &models.ToolResult{
			ID:      call.ID,
			Success: false,
			Error:   "insufficient permissions for comment update",
			Type:    "error",
		}, nil
	}

	updateErr := s.client.UpdateComment(ctx, owner, repo, commentID, body)
	if updateErr != nil {
		return &models.ToolResult{
			ID:      call.ID,
			Success: false,
			Error:   fmt.Sprintf("failed to update comment: %v", updateErr),
			Type:    "error",
		}, nil
	}

	return &models.ToolResult{
		ID:      call.ID,
		Success: true,
		Content: map[string]interface{}{
			"id":         commentID,
			"body":       body,
			"updated_at": time.Now(),
		},
		Type: "json",
	}, nil
}

// listComments 列出评论
func (s *GitHubCommentsServer) listComments(ctx context.Context, call *models.ToolCall, owner, repo string) (*models.ToolResult, error) {
	// 安全地提取 issue_number
	issueNumber, err := safeExtractInt(call.Function.Arguments["issue_number"], "issue_number")
	if err != nil {
		return &models.ToolResult{
			ID:      call.ID,
			Success: false,
			Error:   err.Error(),
			Type:    "error",
		}, nil
	}

	opts := &githubapi.IssueListCommentsOptions{
		ListOptions: githubapi.ListOptions{PerPage: 100},
	}

	if since, ok := call.Function.Arguments["since"].(string); ok && since != "" {
		if sinceTime, parseErr := time.Parse(time.RFC3339, since); parseErr == nil {
			opts.Since = &sinceTime
		}
	}

	comments, _, listErr := s.client.GetClient().Issues.ListComments(ctx, owner, repo, issueNumber, opts)
	if listErr != nil {
		return &models.ToolResult{
			ID:      call.ID,
			Success: false,
			Error:   fmt.Sprintf("failed to list comments: %v", listErr),
			Type:    "error",
		}, nil
	}

	var commentList []map[string]interface{}
	for _, comment := range comments {
		commentInfo := map[string]interface{}{
			"id":         comment.GetID(),
			"url":        comment.GetHTMLURL(),
			"body":       comment.GetBody(),
			"created_at": comment.GetCreatedAt(),
			"updated_at": comment.GetUpdatedAt(),
			"author":     comment.User.GetLogin(),
		}
		commentList = append(commentList, commentInfo)
	}

	return &models.ToolResult{
		ID:      call.ID,
		Success: true,
		Content: map[string]interface{}{
			"issue_number": issueNumber,
			"comments":     commentList,
			"count":        len(commentList),
		},
		Type: "json",
	}, nil
}

// createReviewComment 创建review评论
func (s *GitHubCommentsServer) createReviewComment(ctx context.Context, call *models.ToolCall, owner, repo string, mcpCtx *models.MCPContext) (*models.ToolResult, error) {
	// 安全地提取 pull_number
	pullNumber, err := safeExtractInt(call.Function.Arguments["pull_number"], "pull_number")
	if err != nil {
		return &models.ToolResult{
			ID:      call.ID,
			Success: false,
			Error:   err.Error(),
			Type:    "error",
		}, nil
	}

	body := call.Function.Arguments["body"].(string)
	commitID := call.Function.Arguments["commit_id"].(string)
	path := call.Function.Arguments["path"].(string)

	// 安全地提取 line
	line, lineErr := safeExtractInt(call.Function.Arguments["line"], "line")
	if lineErr != nil {
		return &models.ToolResult{
			ID:      call.ID,
			Success: false,
			Error:   lineErr.Error(),
			Type:    "error",
		}, nil
	}

	// 检查写权限
	if !s.hasWritePermission(mcpCtx) {
		return &models.ToolResult{
			ID:      call.ID,
			Success: false,
			Error:   "insufficient permissions for review comment creation",
			Type:    "error",
		}, nil
	}

	comment := &githubapi.PullRequestComment{
		Body:     &body,
		CommitID: &commitID,
		Path:     &path,
		Line:     &line,
	}

	createdComment, _, err := s.client.GetClient().PullRequests.CreateComment(ctx, owner, repo, pullNumber, comment)
	if err != nil {
		return &models.ToolResult{
			ID:      call.ID,
			Success: false,
			Error:   fmt.Sprintf("failed to create review comment: %v", err),
			Type:    "error",
		}, nil
	}

	return &models.ToolResult{
		ID:      call.ID,
		Success: true,
		Content: map[string]interface{}{
			"id":          createdComment.GetID(),
			"url":         createdComment.GetHTMLURL(),
			"body":        createdComment.GetBody(),
			"path":        createdComment.GetPath(),
			"line":        createdComment.GetLine(),
			"commit_id":   createdComment.GetCommitID(),
			"created_at":  createdComment.GetCreatedAt(),
			"author":      createdComment.User.GetLogin(),
			"pull_number": pullNumber,
		},
		Type: "json",
	}, nil
}

// listPRComments 列出PR的所有评论
func (s *GitHubCommentsServer) listPRComments(ctx context.Context, call *models.ToolCall, owner, repo string) (*models.ToolResult, error) {
	// 安全地提取 pull_number，支持多种数字类型
	var pullNumber int
	switch v := call.Function.Arguments["pull_number"].(type) {
	case float64:
		pullNumber = int(v)
	case int:
		pullNumber = v
	case int64:
		pullNumber = int(v)
	case int32:
		pullNumber = int(v)
	default:
		return &models.ToolResult{
			ID:      call.ID,
			Success: false,
			Error:   fmt.Sprintf("invalid pull_number type: %T", v),
			Type:    "error",
		}, nil
	}

	// 获取PR详情
	pr, _, err := s.client.GetClient().PullRequests.Get(ctx, owner, repo, pullNumber)
	if err != nil {
		return &models.ToolResult{
			ID:      call.ID,
			Success: false,
			Error:   fmt.Sprintf("failed to get PR details: %v", err),
			Type:    "error",
		}, nil
	}

	// 获取所有评论
	allComments, err := s.client.GetAllPRComments(pr)
	if err != nil {
		return &models.ToolResult{
			ID:      call.ID,
			Success: false,
			Error:   fmt.Sprintf("failed to get all PR comments: %v", err),
			Type:    "error",
		}, nil
	}

	// 转换为统一格式
	var issueComments []map[string]interface{}
	for _, comment := range allComments.IssueComments {
		commentInfo := map[string]interface{}{
			"type":       "issue_comment",
			"id":         comment.GetID(),
			"url":        comment.GetHTMLURL(),
			"body":       comment.GetBody(),
			"created_at": comment.GetCreatedAt(),
			"updated_at": comment.GetUpdatedAt(),
			"author":     comment.User.GetLogin(),
		}
		issueComments = append(issueComments, commentInfo)
	}

	var reviewComments []map[string]interface{}
	for _, comment := range allComments.ReviewComments {
		commentInfo := map[string]interface{}{
			"type":       "review_comment",
			"id":         comment.GetID(),
			"url":        comment.GetHTMLURL(),
			"body":       comment.GetBody(),
			"path":       comment.GetPath(),
			"line":       comment.GetLine(),
			"commit_id":  comment.GetCommitID(),
			"created_at": comment.GetCreatedAt(),
			"updated_at": comment.GetUpdatedAt(),
			"author":     comment.User.GetLogin(),
		}
		reviewComments = append(reviewComments, commentInfo)
	}

	var reviews []map[string]interface{}
	for _, review := range allComments.Reviews {
		reviewInfo := map[string]interface{}{
			"type":       "review",
			"id":         review.GetID(),
			"url":        review.GetHTMLURL(),
			"body":       review.GetBody(),
			"state":      review.GetState(),
			"commit_id":  review.GetCommitID(),
			"created_at": review.GetSubmittedAt(),
			"author":     review.User.GetLogin(),
		}
		reviews = append(reviews, reviewInfo)
	}

	return &models.ToolResult{
		ID:      call.ID,
		Success: true,
		Content: map[string]interface{}{
			"pull_number":     pullNumber,
			"pr_body":         allComments.PRBody,
			"issue_comments":  issueComments,
			"review_comments": reviewComments,
			"reviews":         reviews,
			"totals": map[string]int{
				"issue_comments":  len(issueComments),
				"review_comments": len(reviewComments),
				"reviews":         len(reviews),
			},
		},
		Type: "json",
	}, nil
}

// updatePRDescription 更新PR描述
func (s *GitHubCommentsServer) updatePRDescription(ctx context.Context, call *models.ToolCall, owner, repo string, mcpCtx *models.MCPContext) (*models.ToolResult, error) {
	xl := xlog.NewWith(ctx)

	// 检查写权限
	if !s.hasWritePermission(mcpCtx) {
		return nil, fmt.Errorf("insufficient permissions: github:write required")
	}

	// 解析参数
	xl.Infof("Received pr_number: %v (type: %T)", call.Function.Arguments["pr_number"], call.Function.Arguments["pr_number"])

	var prNumber int
	switch v := call.Function.Arguments["pr_number"].(type) {
	case float64:
		prNumber = int(v)
	case int:
		prNumber = v
	case int64:
		prNumber = int(v)
	default:
		return nil, fmt.Errorf("pr_number must be a number, got %T: %v", v, v)
	}

	body, ok := call.Function.Arguments["body"].(string)
	if !ok {
		return nil, fmt.Errorf("body must be a string")
	}

	xl.Infof("Updating PR #%d description in %s/%s", prNumber, owner, repo)

	// 先获取PR对象
	pr, err := s.client.GetPullRequest(owner, repo, prNumber)
	if err != nil {
		xl.Errorf("Failed to get PR: %v", err)
		return nil, fmt.Errorf("failed to get PR: %w", err)
	}

	// 更新PR描述
	err = s.client.UpdatePullRequest(pr, body)
	if err != nil {
		xl.Errorf("Failed to update PR description: %v", err)
		return nil, fmt.Errorf("failed to update PR description: %w", err)
	}

	xl.Infof("Successfully updated PR #%d description", prNumber)

	return &models.ToolResult{
		ID:      call.ID,
		Success: true,
		Content: map[string]interface{}{
			"pr_number":   prNumber,
			"url":         pr.GetHTMLURL(),
			"body_length": len(body),
			"updated_at":  time.Now(),
		},
		Type: "json",
	}, nil
}

// hasWritePermission 检查是否有写权限
func (s *GitHubCommentsServer) hasWritePermission(mcpCtx *models.MCPContext) bool {
	if mcpCtx.Permissions == nil {
		return false
	}

	for _, perm := range mcpCtx.Permissions {
		if perm == "github:write" || perm == "github:admin" {
			return true
		}
	}
	return false
}
