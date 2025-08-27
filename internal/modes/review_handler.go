package modes

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/qiniu/codeagent/internal/code"
	"github.com/qiniu/codeagent/internal/config"
	ctxsys "github.com/qiniu/codeagent/internal/context"
	ghclient "github.com/qiniu/codeagent/internal/github"
	"github.com/qiniu/codeagent/internal/mcp"
	"github.com/qiniu/codeagent/internal/workspace"
	"github.com/qiniu/codeagent/pkg/models"

	"github.com/google/go-github/v58/github"
	"github.com/qiniu/x/xlog"
)

// ReviewHandler Review模式处理器
// 处理自动代码审查相关的事件
type ReviewHandler struct {
	*BaseHandler
	clientManager  ghclient.ClientManagerInterface
	workspace      *workspace.Manager
	mcpClient      mcp.MCPClient
	sessionManager *code.SessionManager
	config         *config.Config
	contextManager *ctxsys.ContextManager
}

// NewReviewHandler 创建Review模式处理器
func NewReviewHandler(clientManager ghclient.ClientManagerInterface, workspace *workspace.Manager, mcpClient mcp.MCPClient, sessionManager *code.SessionManager, config *config.Config) *ReviewHandler {
	// Create context manager with dynamic client support
	collector := ctxsys.NewDefaultContextCollector(clientManager)
	formatter := ctxsys.NewDefaultContextFormatter(50000) // 50k tokens limit
	generator := ctxsys.NewTemplatePromptGenerator(formatter)
	contextManager := &ctxsys.ContextManager{
		Collector: collector,
		Formatter: formatter,
		Generator: generator,
	}

	return &ReviewHandler{
		BaseHandler: NewBaseHandler(
			ReviewMode,
			0, // 最高优先级
			"Handle automatic code review events",
		),
		clientManager:  clientManager,
		workspace:      workspace,
		mcpClient:      mcpClient,
		sessionManager: sessionManager,
		config:         config,
		contextManager: contextManager,
	}
}

// CanHandle 检查是否能处理给定的事件
func (rh *ReviewHandler) CanHandle(ctx context.Context, event models.GitHubContext) bool {
	xl := xlog.NewWith(ctx)

	switch event.GetEventType() {
	case models.EventPullRequest:
		prCtx := event.(*models.PullRequestContext)
		return rh.canHandlePREvent(ctx, prCtx)

	default:
		xl.Debugf("Review mode does not handle event type: %s", event.GetEventType())
		return false
	}
}

// canHandlePREvent 检查是否能处理PR事件
func (rh *ReviewHandler) canHandlePREvent(ctx context.Context, event *models.PullRequestContext) bool {
	xl := xlog.NewWith(ctx)

	switch event.GetEventAction() {
	case "opened", "reopened":
		// PR打开时自动审查
		xl.Infof("Review mode can handle PR opened event")
		return true

	case "closed":
		// PR关闭时清理资源
		xl.Infof("Review mode can handle PR closed event")
		return true

	default:
		return false
	}
}

// Execute 执行Review模式处理逻辑
func (rh *ReviewHandler) Execute(ctx context.Context, event models.GitHubContext) error {
	xl := xlog.NewWith(ctx)
	xl.Infof("ReviewHandler executing for event type: %s, action: %s",
		event.GetEventType(), event.GetEventAction())

	// Extract repository information
	ghRepo := event.GetRepository()
	if ghRepo == nil {
		return fmt.Errorf("no repository information available")
	}

	repo := &models.Repository{
		Owner: ghRepo.Owner.GetLogin(),
		Name:  ghRepo.GetName(),
	}

	// Get dynamic GitHub client for this repository
	client, err := rh.clientManager.GetClient(ctx, repo)
	if err != nil {
		return fmt.Errorf("failed to get GitHub client for %s/%s: %w", repo.Owner, repo.Name, err)
	}

	switch event.GetEventType() {
	case models.EventPullRequest:
		return rh.handlePREvent(ctx, event.(*models.PullRequestContext), client)
	default:
		return fmt.Errorf("unsupported event type for ReviewHandler: %s", event.GetEventType())
	}
}

// handlePREvent 处理PR事件
func (rh *ReviewHandler) handlePREvent(ctx context.Context, event *models.PullRequestContext, client *ghclient.Client) error {
	xl := xlog.NewWith(ctx)

	switch event.GetEventAction() {
	case "opened", "reopened", "synchronize", "ready_for_review":
		xl.Infof("Auto-reviewing PR #%d", event.PullRequest.GetNumber())

		// 执行自动代码审查
		return rh.processCodeReview(ctx, event, client)

	case "closed":
		return rh.handlePRClosed(ctx, event, client)

	default:
		return fmt.Errorf("unsupported action for PR event in ReviewHandler: %s", event.GetEventAction())
	}
}

// handlePRClosed 处理PR关闭事件
func (rh *ReviewHandler) handlePRClosed(ctx context.Context, event *models.PullRequestContext, client *ghclient.Client) error {
	xl := xlog.NewWith(ctx)

	pr := event.PullRequest
	prNumber := pr.GetNumber()
	prBranch := pr.GetHead().GetRef()
	xl.Infof("Starting cleanup after PR #%d closed, branch: %s, merged: %v", prNumber, prBranch, pr.GetMerged())

	// 获取所有与该PR相关的工作空间（可能有多个不同AI模型的工作空间）
	workspaces := rh.workspace.GetAllWorkspacesByPR(pr)
	if len(workspaces) == 0 {
		xl.Infof("No workspaces found for PR: %s", pr.GetHTMLURL())
	} else {
		xl.Infof("Found %d workspaces for cleanup", len(workspaces))

		// 清理所有工作空间
		for _, ws := range workspaces {
			xl.Infof("Cleaning up workspace: %s (AI model: %s)", ws.Path, ws.AIModel)

			// 清理执行的 code session
			xl.Infof("Closing code session for AI model: %s", ws.AIModel)
			err := rh.sessionManager.CloseSession(ws)
			if err != nil {
				xl.Errorf("Failed to close code session for PR #%d with AI model %s: %v", prNumber, ws.AIModel, err)
				// 不返回错误，继续清理其他工作空间
			} else {
				xl.Infof("Code session closed successfully for AI model: %s", ws.AIModel)
			}

			// 清理 worktree,session 目录 和 对应的内存映射
			xl.Infof("Cleaning up workspace for AI model: %s", ws.AIModel)
			b := rh.workspace.CleanupWorkspace(ws)
			if !b {
				xl.Errorf("Failed to cleanup workspace for PR #%d with AI model %s", prNumber, ws.AIModel)
				// 不返回错误，继续清理其他工作空间
			} else {
				xl.Infof("Workspace cleaned up successfully for AI model: %s", ws.AIModel)
			}
		}
	}

	// 删除CodeAgent创建的分支
	if prBranch != "" && strings.HasPrefix(prBranch, "codeagent") {
		owner := pr.GetBase().GetRepo().GetOwner().GetLogin()
		repoName := pr.GetBase().GetRepo().GetName()

		xl.Infof("Deleting CodeAgent branch: %s from repo %s/%s", prBranch, owner, repoName)
		err := client.DeleteCodeAgentBranch(ctx, owner, repoName, prBranch)
		if err != nil {
			xl.Errorf("Failed to delete branch %s: %v", prBranch, err)
			// 不返回错误，继续完成其他清理工作
		} else {
			xl.Infof("Successfully deleted CodeAgent branch: %s", prBranch)
		}
	} else {
		xl.Infof("Branch %s is not a CodeAgent branch, skipping deletion", prBranch)
	}

	xl.Infof("Cleanup after PR closed completed: PR #%d, cleaned %d workspaces", prNumber, len(workspaces))
	return nil
}

// processCodeReview PR自动代码审查方法
func (rh *ReviewHandler) processCodeReview(ctx context.Context, prEvent *models.PullRequestContext, client *ghclient.Client) error {
	xl := xlog.NewWith(ctx)
	xl.Infof("Starting automatic code review for PR")

	// 1. 提取PR信息
	if prEvent == nil {
		return fmt.Errorf("PR event is required for PR review")
	}
	pr := prEvent.PullRequest
	// 使用配置中的默认AI模型进行自动审查
	aiModel := rh.config.CodeProvider
	xl.Infof("Processing PR #%d with AI model: %s", pr.GetNumber(), aiModel)

	// 2. 立即创建初始状态comment
	owner := pr.GetBase().GetRepo().GetOwner().GetLogin()
	repoName := pr.GetBase().GetRepo().GetName()
	prNumber := pr.GetNumber()

	initialCommentBody := "🤖 CodeAgent is working… \n\nI'll analyze this and get back to you."

	xl.Infof("Creating initial review status comment for PR #%d", prNumber)
	initialComment, err := client.CreateComment(ctx, owner, repoName, prNumber, initialCommentBody)
	if err != nil {
		xl.Errorf("Failed to create initial status comment: %v", err)
		return fmt.Errorf("failed to create initial status comment: %w", err)
	}

	commentID := initialComment.GetID()
	xl.Infof("Created initial comment with ID: %d for PR #%d", commentID, prNumber)

	// 3. 获取或创建工作空间
	ws := rh.workspace.GetOrCreateWorkspaceForPR(pr, aiModel)
	if ws == nil {
		return fmt.Errorf("failed to get or create workspace for PR review")
	}
	// 拉取最新代码
	if err := client.PullLatestChanges(ws, pr); err != nil {
		xl.Warnf("Failed to pull latest changes: %v", err)
	}
	xl.Infof("Workspace ready: %s", ws.Path)

	// 4. 初始化code client
	xl.Infof("Initializing code client for review")
	codeClient, err := rh.sessionManager.GetSession(ws)
	if err != nil {
		return fmt.Errorf("failed to get code session for review: %w", err)
	}
	xl.Infof("Code client initialized successfully")

	// 5. 构建审查上下文和提示词
	xl.Infof("Building review context and prompt")
	prompt, err := rh.buildReviewPrompt(ctx, prEvent, commentID)
	if err != nil {
		xl.Errorf("Failed to build enhanced prompt : %v", err)
	}

	// 6. 执行AI代码审查
	xl.Infof("Executing AI code review analysis")
	resp, err := rh.promptWithRetry(ctx, codeClient, prompt, 3)
	if err != nil {
		return fmt.Errorf("failed to execute code review: %w", err)
	}

	output, err := io.ReadAll(resp.Out)
	if err != nil {
		return fmt.Errorf("failed to read review output: %w", err)
	}

	xl.Infof("AI code review completed, output length: %d", len(output))
	xl.Debugf("Review Output: %s", string(output))

	xl.Infof("PR code review process completed successfully")
	return nil
}

// buildReviewPrompt 构建代码审查提示词
func (rh *ReviewHandler) buildReviewPrompt(ctx context.Context, prEvent *models.PullRequestContext, commentID int64) (string, error) {
	xl := xlog.NewWith(ctx)

	if prEvent == nil {
		return "", fmt.Errorf("PR event is required")
	}

	// 先收集代码上下文
	var codeCtx *ctxsys.CodeContext
	if prEvent.PullRequest != nil {
		var err error
		codeCtx, err = rh.contextManager.Collector.CollectCodeContext(prEvent.PullRequest)
		if err != nil {
			xl.Warnf("Failed to collect code context: %v", err)
		} else {
			xl.Infof("Successfully collected code context with %d files", len(codeCtx.Files))
		}
	}

	// 构建PR审查的上下文
	enhancedCtx := &ctxsys.EnhancedContext{
		Type:      ctxsys.ContextTypePR,
		Priority:  ctxsys.PriorityHigh,
		Timestamp: time.Now(),
		Subject:   prEvent,
		Code:      codeCtx, // 确保代码上下文被设置
		Metadata: map[string]interface{}{
			"pr_number":            prEvent.PullRequest.GetNumber(),
			"pr_title":             prEvent.PullRequest.GetTitle(),
			"pr_body":              prEvent.PullRequest.GetBody(),
			"repository":           prEvent.PullRequest.GetBase().GetRepo().GetFullName(),
			"trigger_username":     "system", // 自动审查
			"trigger_display_name": "CodeAgent Auto Review",
			"claude_comment_id":    commentID,
		},
	}

	// 使用模板生成器的Review模式生成提示词
	xl.Infof("Generating review prompt using template generator")
	return rh.contextManager.Generator.GeneratePrompt(enhancedCtx, "Review", "Perform automatic code review")
}

// promptWithRetry 带重试的提示执行
func (rh *ReviewHandler) promptWithRetry(ctx context.Context, codeClient code.Code, prompt string, maxRetries int) (*code.Response, error) {
	return code.PromptWithRetry(ctx, codeClient, prompt, maxRetries)
}

// addPRComment 使用GitHub client添加PR评论
func (rh *ReviewHandler) addPRComment(ctx context.Context, pr *github.PullRequest, comment string, client *ghclient.Client) error {
	xl := xlog.NewWith(ctx)

	// 使用GitHub client的CreateComment方法添加评论
	owner := pr.GetBase().GetRepo().GetOwner().GetLogin()
	repo := pr.GetBase().GetRepo().GetName()
	prNumber := pr.GetNumber()

	_, err := client.CreateComment(ctx, owner, repo, prNumber, comment)
	if err != nil {
		xl.Errorf("Failed to add PR comment: %v", err)
		return err
	}

	xl.Infof("Successfully added review comment to PR")
	return nil
}

// updatePRComment 使用GitHub client更新PR评论
func (rh *ReviewHandler) updatePRComment(ctx context.Context, pr *github.PullRequest, commentID int64, comment string, client *ghclient.Client) error {
	xl := xlog.NewWith(ctx)

	// 使用GitHub client的UpdateComment方法更新评论
	owner := pr.GetBase().GetRepo().GetOwner().GetLogin()
	repo := pr.GetBase().GetRepo().GetName()

	err := client.UpdateComment(ctx, owner, repo, commentID, comment)
	if err != nil {
		xl.Errorf("Failed to update PR comment: %v", err)
		return err
	}

	xl.Infof("Successfully updated review comment in PR")
	return nil
}

// ProcessManualCodeReview 处理手动代码审查请求（从PR评论触发）
func (rh *ReviewHandler) ProcessManualCodeReview(ctx context.Context, event *models.IssueCommentContext, client *ghclient.Client) error {
	xl := xlog.NewWith(ctx)
	xl.Infof("Starting manual code review from PR comment")

	// 1. 验证这是一个PR评论
	if !event.IsPRComment {
		return fmt.Errorf("manual review can only be triggered from PR comments")
	}

	// 2. 从GitHub API获取完整的PR信息
	repoOwner := event.Repository.GetOwner().GetLogin()
	repoName := event.Repository.GetName()
	prNumber := event.Issue.GetNumber()

	pr, _, err := client.GetClient().PullRequests.Get(ctx, repoOwner, repoName, prNumber)
	if err != nil {
		return fmt.Errorf("failed to get PR information: %w", err)
	}

	// 3. 构造 PullRequestContext
	prEvent := &models.PullRequestContext{
		BaseContext: models.BaseContext{
			Type:       models.EventPullRequest,
			Repository: event.Repository,
			Sender:     event.Sender,
			RawEvent:   pr, // 使用PR对象作为原始事件
			Action:     event.GetEventAction(),
			DeliveryID: event.DeliveryID,
			Timestamp:  event.Timestamp,
		},
		PullRequest: pr,
	}

	// 4. 调用统一的代码审查逻辑
	return rh.processCodeReview(ctx, prEvent, client)
}
