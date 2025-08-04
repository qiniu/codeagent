package modes

import (
	"context"
	"fmt"
	"strings"

	ghclient "github.com/qiniu/codeagent/internal/github"
	"github.com/qiniu/codeagent/internal/mcp"
	"github.com/qiniu/codeagent/internal/workspace"
	"github.com/qiniu/codeagent/pkg/models"

	"github.com/google/go-github/v58/github"
	"github.com/qiniu/x/xlog"
)

// AgentHandler Agent模式处理器
// 对应claude-code-action中的AgentMode
// 处理自动化触发的事件（Issue分配、标签添加等）
type AgentHandler struct {
	*BaseHandler
	github    *ghclient.Client
	workspace *workspace.Manager
	mcpClient mcp.MCPClient
}

// NewAgentHandler 创建Agent模式处理器
func NewAgentHandler(github *ghclient.Client, workspace *workspace.Manager, mcpClient mcp.MCPClient) *AgentHandler {
	return &AgentHandler{
		BaseHandler: NewBaseHandler(
			AgentMode,
			20, // 较低优先级，在Tag模式之后
			"Handle automated triggers (issue assignment, labels, etc.)",
		),
		github:    github,
		workspace: workspace,
		mcpClient: mcpClient,
	}
}

// CanHandle 检查是否能处理给定的事件
func (ah *AgentHandler) CanHandle(ctx context.Context, event models.GitHubContext) bool {
	xl := xlog.NewWith(ctx)

	switch event.GetEventType() {
	case models.EventIssues:
		// 处理Issue相关的自动化触发
		issuesCtx := event.(*models.IssuesContext)
		return ah.canHandleIssuesEvent(ctx, issuesCtx)

	case models.EventPullRequest:
		// 处理PR相关的自动化触发
		prCtx := event.(*models.PullRequestContext)
		return ah.canHandlePREvent(ctx, prCtx)

	case models.EventWorkflowDispatch:
		// 处理工作流调度事件
		xl.Infof("Agent mode can handle workflow_dispatch events")
		return true

	case models.EventSchedule:
		// 处理定时任务事件
		xl.Infof("Agent mode can handle schedule events")
		return true

	default:
		return false
	}
}

// canHandleIssuesEvent 检查是否能处理Issues事件
func (ah *AgentHandler) canHandleIssuesEvent(ctx context.Context, event *models.IssuesContext) bool {
	xl := xlog.NewWith(ctx)

	switch event.GetEventAction() {
	case "assigned":
		// Issue被分配给某人时自动触发
		xl.Infof("Agent mode can handle issue assignment")
		return true

	case "labeled":
		// Issue被添加特定标签时触发
		// 这里可以检查是否包含特定的标签（如"ai-assist", "codeagent"等）
		xl.Infof("Agent mode can handle issue labeling")
		return ah.hasAutoTriggerLabel(event.Issue)

	case "opened":
		// Issue创建时的自动处理（可选）
		xl.Debugf("Issue opened, checking for auto-trigger conditions")
		return ah.shouldAutoProcessIssue(event.Issue)

	default:
		return false
	}
}

// canHandlePREvent 检查是否能处理PR事件
func (ah *AgentHandler) canHandlePREvent(ctx context.Context, event *models.PullRequestContext) bool {
	xl := xlog.NewWith(ctx)

	switch event.GetEventAction() {
	case "opened":
		// PR打开时自动审查（如果启用）
		xl.Debugf("PR opened, checking for auto-review conditions")
		return ah.shouldAutoReviewPR(event.PullRequest)

	case "synchronize":
		// PR同步时重新审查
		xl.Debugf("PR synchronized, checking for auto-review conditions")
		return ah.shouldAutoReviewPR(event.PullRequest)

	default:
		return false
	}
}

// Execute 执行Agent模式处理逻辑
func (ah *AgentHandler) Execute(ctx context.Context, event models.GitHubContext) error {
	xl := xlog.NewWith(ctx)
	xl.Infof("AgentHandler executing for event type: %s, action: %s",
		event.GetEventType(), event.GetEventAction())

	switch event.GetEventType() {
	case models.EventIssues:
		return ah.handleIssuesEvent(ctx, event.(*models.IssuesContext))
	case models.EventPullRequest:
		return ah.handlePREvent(ctx, event.(*models.PullRequestContext))
	case models.EventWorkflowDispatch:
		return ah.handleWorkflowDispatch(ctx, event.(*models.WorkflowDispatchContext))
	case models.EventSchedule:
		return ah.handleSchedule(ctx, event.(*models.ScheduleContext))
	default:
		return fmt.Errorf("unsupported event type for AgentHandler: %s", event.GetEventType())
	}
}

// handleIssuesEvent 处理Issues事件
func (ah *AgentHandler) handleIssuesEvent(ctx context.Context, event *models.IssuesContext) error {
	xl := xlog.NewWith(ctx)

	switch event.GetEventAction() {
	case "assigned":
		xl.Infof("Auto-processing assigned issue #%d", event.Issue.GetNumber())
		// 自动处理被分配的Issue
		return ah.autoProcessIssue(ctx, event)

	case "labeled":
		xl.Infof("Auto-processing labeled issue #%d", event.Issue.GetNumber())
		// 自动处理被标记的Issue
		return ah.autoProcessIssue(ctx, event)

	case "opened":
		xl.Infof("Auto-processing opened issue #%d", event.Issue.GetNumber())
		// 自动处理新创建的Issue
		return ah.autoProcessIssue(ctx, event)

	default:
		return fmt.Errorf("unsupported action for Issues event: %s", event.GetEventAction())
	}
}

// handlePREvent 处理PR事件
func (ah *AgentHandler) handlePREvent(ctx context.Context, event *models.PullRequestContext) error {
	xl := xlog.NewWith(ctx)

	switch event.GetEventAction() {
	case "opened", "synchronize":
		xl.Infof("Auto-reviewing PR #%d", event.PullRequest.GetNumber())
		// 自动审查PR
		// TODO: 实现自动PR审查逻辑
		xl.Infof("Auto-review for PR #%d is not yet implemented", event.PullRequest.GetNumber())
		return nil

	default:
		return fmt.Errorf("unsupported action for PullRequest event: %s", event.GetEventAction())
	}
}

// handleWorkflowDispatch 处理工作流调度事件
func (ah *AgentHandler) handleWorkflowDispatch(ctx context.Context, event *models.WorkflowDispatchContext) error {
	xl := xlog.NewWith(ctx)
	xl.Infof("Processing workflow dispatch event with inputs: %+v", event.Inputs)

	// 这里可以根据inputs中的参数执行不同的自动化任务
	// 例如：批量处理Issues、生成报告、清理资源等

	return fmt.Errorf("workflow_dispatch handling not implemented yet")
}

// handleSchedule 处理定时任务事件
func (ah *AgentHandler) handleSchedule(ctx context.Context, event *models.ScheduleContext) error {
	xl := xlog.NewWith(ctx)
	xl.Infof("Processing schedule event with cron: %s", event.Cron)

	// 这里可以执行定时任务，例如：
	// - 清理过期的工作空间
	// - 生成周期性报告
	// - 检查待处理的Issues

	return fmt.Errorf("schedule handling not implemented yet")
}

// autoProcessIssue 自动处理Issue
func (ah *AgentHandler) autoProcessIssue(ctx context.Context, event *models.IssuesContext) error {
	xl := xlog.NewWith(ctx)

	// 将事件转换为IssueCommentEvent格式（模拟/code命令）
	// 这样可以复用现有的agent逻辑
	_ = event.RawEvent.(*github.IssuesEvent)

	xl.Infof("Auto-processing issue with new architecture")
	// TODO: 实现自动Issue处理逻辑，使用MCP工具
	xl.Infof("Auto-processing for Issue #%d is not yet implemented", event.Issue.GetNumber())
	return nil
}

// generateAutoPrompt 为Issue生成自动化提示
func (ah *AgentHandler) generateAutoPrompt(issue *github.Issue) string {
	// 基于Issue的标题和描述生成合适的提示
	title := issue.GetTitle()

	prompt := "Please implement this feature based on the issue description."

	// 可以根据标题中的关键词优化提示
	if strings.Contains(strings.ToLower(title), "bug") || strings.Contains(strings.ToLower(title), "fix") {
		prompt = "Please analyze and fix this bug based on the issue description."
	} else if strings.Contains(strings.ToLower(title), "test") {
		prompt = "Please add tests for this functionality based on the issue description."
	} else if strings.Contains(strings.ToLower(title), "refactor") {
		prompt = "Please refactor the code based on the issue description."
	}

	return prompt
}

// hasAutoTriggerLabel 检查Issue是否包含自动触发标签
func (ah *AgentHandler) hasAutoTriggerLabel(issue *github.Issue) bool {
	autoTriggerLabels := []string{"ai-assist", "codeagent", "auto-code", "ai-help"}

	for _, label := range issue.Labels {
		labelName := strings.ToLower(label.GetName())
		for _, triggerLabel := range autoTriggerLabels {
			if labelName == triggerLabel {
				return true
			}
		}
	}

	return false
}

// shouldAutoProcessIssue 检查是否应该自动处理Issue
func (ah *AgentHandler) shouldAutoProcessIssue(issue *github.Issue) bool {
	// 这里可以根据配置或其他条件决定是否自动处理
	// 例如：特定的仓库、特定的标签、特定的用户等
	return false // 默认不自动处理新创建的Issue
}

// shouldAutoReviewPR 检查是否应该自动审查PR
func (ah *AgentHandler) shouldAutoReviewPR(pr *github.PullRequest) bool {
	// 这里可以根据配置决定是否自动审查PR
	// 例如：特定的分支、特定的作者、特定的文件变更等
	return false // 默认不自动审查PR
}
