package modes

import (
	"context"
	"fmt"

	ghclient "github.com/qiniu/codeagent/internal/github"
	"github.com/qiniu/codeagent/internal/mcp"
	"github.com/qiniu/codeagent/internal/workspace"
	"github.com/qiniu/codeagent/pkg/models"

	"github.com/qiniu/x/xlog"
)

// ReviewHandler Review模式处理器
// 对应claude-code-action中的ReviewMode
// 处理自动代码审查相关的事件
type ReviewHandler struct {
	*BaseHandler
	github    *ghclient.Client
	workspace *workspace.Manager
	mcpClient mcp.MCPClient
}

// NewReviewHandler 创建Review模式处理器
func NewReviewHandler(github *ghclient.Client, workspace *workspace.Manager, mcpClient mcp.MCPClient) *ReviewHandler {
	return &ReviewHandler{
		BaseHandler: NewBaseHandler(
			ReviewMode,
			30, // 最低优先级
			"Handle automatic code review events",
		),
		github:    github,
		workspace: workspace,
		mcpClient: mcpClient,
	}
}

// CanHandle 检查是否能处理给定的事件
func (rh *ReviewHandler) CanHandle(ctx context.Context, event models.GitHubContext) bool {
	xl := xlog.NewWith(ctx)
	
	switch event.GetEventType() {
	case models.EventPullRequest:
		prCtx := event.(*models.PullRequestContext)
		return rh.canHandlePREvent(ctx, prCtx)
		
	case models.EventPush:
		pushCtx := event.(*models.PushContext)
		return rh.canHandlePushEvent(ctx, pushCtx)
		
	default:
		xl.Debugf("Review mode does not handle event type: %s", event.GetEventType())
		return false
	}
}

// canHandlePREvent 检查是否能处理PR事件
func (rh *ReviewHandler) canHandlePREvent(ctx context.Context, event *models.PullRequestContext) bool {
	xl := xlog.NewWith(ctx)
	
	switch event.GetEventAction() {
	case "opened":
		// PR打开时自动审查
		xl.Infof("Review mode can handle PR opened event")
		return true
		
	case "synchronize":
		// PR有新提交时重新审查
		xl.Infof("Review mode can handle PR synchronize event")
		return true
		
	case "ready_for_review":
		// PR从draft状态变为ready时审查
		xl.Infof("Review mode can handle PR ready_for_review event")
		return true
		
	default:
		return false
	}
}

// canHandlePushEvent 检查是否能处理Push事件
func (rh *ReviewHandler) canHandlePushEvent(ctx context.Context, event *models.PushContext) bool {
	xl := xlog.NewWith(ctx)
	
	// 只处理主分支的Push事件
	if event.Ref == "refs/heads/main" || event.Ref == "refs/heads/master" {
		xl.Infof("Review mode can handle push to main branch")
		return true
	}
	
	// 可以扩展到处理其他重要分支
	return false
}

// Execute 执行Review模式处理逻辑
func (rh *ReviewHandler) Execute(ctx context.Context, event models.GitHubContext) error {
	xl := xlog.NewWith(ctx)
	xl.Infof("ReviewHandler executing for event type: %s, action: %s", 
		event.GetEventType(), event.GetEventAction())
	
	switch event.GetEventType() {
	case models.EventPullRequest:
		return rh.handlePREvent(ctx, event.(*models.PullRequestContext))
	case models.EventPush:
		return rh.handlePushEvent(ctx, event.(*models.PushContext))
	default:
		return fmt.Errorf("unsupported event type for ReviewHandler: %s", event.GetEventType())
	}
}

// handlePREvent 处理PR事件
func (rh *ReviewHandler) handlePREvent(ctx context.Context, event *models.PullRequestContext) error {
	xl := xlog.NewWith(ctx)
	
	switch event.GetEventAction() {
	case "opened", "synchronize", "ready_for_review":
		xl.Infof("Auto-reviewing PR #%d", event.PullRequest.GetNumber())
		
		// 执行自动代码审查
		// TODO: 实现自动PR审查逻辑，使用MCP工具
		xl.Infof("Auto-review for PR #%d is not yet implemented", event.PullRequest.GetNumber())
		return nil
		
	default:
		return fmt.Errorf("unsupported action for PR event in ReviewHandler: %s", event.GetEventAction())
	}
}

// handlePushEvent 处理Push事件
func (rh *ReviewHandler) handlePushEvent(ctx context.Context, event *models.PushContext) error {
	xl := xlog.NewWith(ctx)
	xl.Infof("Processing push event to %s with %d commits", event.Ref, len(event.Commits))
	
	// 这里可以实现对主分支Push的自动分析
	// 例如：代码质量检查、安全扫描、性能分析等
	
	// 暂时返回未实现错误
	return fmt.Errorf("push event handling in ReviewHandler not implemented yet")
}