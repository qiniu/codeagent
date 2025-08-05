package modes

import (
	"context"
	"fmt"
	"strings"

	"github.com/qiniu/codeagent/internal/code"
	ghclient "github.com/qiniu/codeagent/internal/github"
	"github.com/qiniu/codeagent/internal/mcp"
	"github.com/qiniu/codeagent/internal/workspace"
	"github.com/qiniu/codeagent/pkg/models"

	"github.com/qiniu/x/xlog"
)

// ReviewHandler Review模式处理器
// 处理自动代码审查相关的事件
type ReviewHandler struct {
	*BaseHandler
	github         *ghclient.Client
	workspace      *workspace.Manager
	mcpClient      mcp.MCPClient
	sessionManager *code.SessionManager
}

// NewReviewHandler 创建Review模式处理器
func NewReviewHandler(github *ghclient.Client, workspace *workspace.Manager, mcpClient mcp.MCPClient, sessionManager *code.SessionManager) *ReviewHandler {
	return &ReviewHandler{
		BaseHandler: NewBaseHandler(
			ReviewMode,
			30, // 最低优先级
			"Handle automatic code review events",
		),
		github:         github,
		workspace:      workspace,
		mcpClient:      mcpClient,
		sessionManager: sessionManager,
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

	case "closed":
		// PR关闭时清理资源
		xl.Infof("Review mode can handle PR closed event")
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

	case "closed":
		return rh.handlePRClosed(ctx, event)

	default:
		return fmt.Errorf("unsupported action for PR event in ReviewHandler: %s", event.GetEventAction())
	}
}

// handlePRClosed 处理PR关闭事件
func (rh *ReviewHandler) handlePRClosed(ctx context.Context, event *models.PullRequestContext) error {
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
		err := rh.github.DeleteCodeAgentBranch(ctx, owner, repoName, prBranch)
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

// handlePushEvent 处理Push事件
func (rh *ReviewHandler) handlePushEvent(ctx context.Context, event *models.PushContext) error {
	xl := xlog.NewWith(ctx)
	xl.Infof("Processing push event to %s with %d commits", event.Ref, len(event.Commits))

	// 这里可以实现对主分支Push的自动分析
	// 例如：代码质量检查、安全扫描、性能分析等

	// 暂时返回未实现错误
	return fmt.Errorf("push event handling in ReviewHandler not implemented yet")
}
