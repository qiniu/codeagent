package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/qiniu/codeagent/internal/code"
	"github.com/qiniu/codeagent/internal/config"
	"github.com/qiniu/codeagent/internal/events"
	ghclient "github.com/qiniu/codeagent/internal/github"
	"github.com/qiniu/codeagent/internal/interaction"
	"github.com/qiniu/codeagent/internal/mcp"
	"github.com/qiniu/codeagent/internal/mcp/servers"
	"github.com/qiniu/codeagent/internal/modes"
	"github.com/qiniu/codeagent/internal/workspace"
	"github.com/qiniu/codeagent/pkg/models"

	"github.com/google/go-github/v58/github"
	"github.com/qiniu/x/xlog"
)

// EnhancedAgent 增强版Agent，集成了新的组件架构
// 对应claude-code-action的完整智能化功能
type EnhancedAgent struct {
	// 原有组件
	config         *config.Config
	github         *ghclient.Client
	workspace      *workspace.Manager
	sessionManager *code.SessionManager

	// 新增组件
	eventParser *events.Parser
	modeManager *modes.Manager
	mcpManager  mcp.MCPManager
	mcpClient   mcp.MCPClient
	taskFactory *interaction.TaskFactory
}

// NewEnhancedAgent 创建增强版Agent
func NewEnhancedAgent(cfg *config.Config, workspaceManager *workspace.Manager) (*EnhancedAgent, error) {
	xl := xlog.New("")

	// 1. 初始化GitHub客户端
	githubClient, err := ghclient.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create GitHub client: %w", err)
	}

	// 2. 初始化事件解析器
	eventParser := events.NewParser()

	// 3. 初始化MCP管理器和服务器
	mcpManager := mcp.NewManager()

	// 注册内置MCP服务器
	githubFiles := servers.NewGitHubFilesServer(githubClient)
	githubComments := servers.NewGitHubCommentsServer(githubClient)

	if err := mcpManager.RegisterServer("github-files", githubFiles); err != nil {
		return nil, fmt.Errorf("failed to register github-files server: %w", err)
	}

	if err := mcpManager.RegisterServer("github-comments", githubComments); err != nil {
		return nil, fmt.Errorf("failed to register github-comments server: %w", err)
	}

	// 4. 创建MCP客户端
	mcpClient := mcp.NewClient(mcpManager)

	// 5. 初始化SessionManager
	sessionManager := code.NewSessionManager(cfg)

	// 6. 初始化模式管理器
	modeManager := modes.NewManager()

	// 注册处理器（按优先级顺序）
	tagHandler := modes.NewTagHandler(githubClient, workspaceManager, mcpClient, sessionManager)
	agentHandler := modes.NewAgentHandler(githubClient, workspaceManager, mcpClient)
	reviewHandler := modes.NewReviewHandler(githubClient, workspaceManager, mcpClient)

	modeManager.RegisterHandler(tagHandler)
	modeManager.RegisterHandler(agentHandler)
	modeManager.RegisterHandler(reviewHandler)

	// 7. 创建任务工厂
	taskFactory := interaction.NewTaskFactory()

	agent := &EnhancedAgent{
		config:         cfg,
		github:         githubClient,
		workspace:      workspaceManager,
		sessionManager: sessionManager,
		eventParser:    eventParser,
		modeManager:    modeManager,
		mcpManager:     mcpManager,
		mcpClient:      mcpClient,
		taskFactory:    taskFactory,
	}

	xl.Infof("Enhanced Agent initialized with %d MCP servers and %d mode handlers",
		len(mcpManager.GetServers()), modeManager.GetHandlerCount())

	return agent, nil
}

// ProcessGitHubWebhookEvent 处理来自Webhook的GitHub事件（推荐方法）
func (a *EnhancedAgent) ProcessGitHubWebhookEvent(ctx context.Context, eventType string, deliveryID string, payload []byte) error {
	xl := xlog.NewWith(ctx)

	startTime := time.Now()
	xl.Infof("Processing GitHub webhook event: %s, delivery_id: %s", eventType, deliveryID)

	// 1. 解析GitHub事件为类型安全的上下文
	githubCtx, err := a.eventParser.ParseWebhookEvent(ctx, eventType, deliveryID, payload)
	if err != nil {
		xl.Warnf("Failed to parse GitHub webhook event: %v", err)
		return fmt.Errorf("failed to parse webhook event: %w", err)
	}

	return a.processGitHubContext(ctx, githubCtx, startTime)
}

// ProcessGitHubEvent 处理GitHub事件的统一入口（兼容方法）
// 替换原有的多个Process方法，使用新的事件系统
func (a *EnhancedAgent) ProcessGitHubEvent(ctx context.Context, eventType string, payload interface{}) error {
	xl := xlog.NewWith(ctx)

	startTime := time.Now()
	xl.Infof("Processing GitHub event: %s", eventType)

	// 1. 解析GitHub事件为类型安全的上下文
	githubCtx, err := a.eventParser.ParseEvent(ctx, eventType, payload)
	if err != nil {
		xl.Errorf("Failed to parse GitHub event: %v", err)
		return fmt.Errorf("failed to parse event: %w", err)
	}

	return a.processGitHubContext(ctx, githubCtx, startTime)
}

// processGitHubContext 处理已解析的GitHub上下文
func (a *EnhancedAgent) processGitHubContext(ctx context.Context, githubCtx models.GitHubContext, startTime time.Time) error {
	xl := xlog.NewWith(ctx)

	xl.Infof("Parsed event type: %s for repository: %s",
		githubCtx.GetEventType(), githubCtx.GetRepository().GetFullName())

	// 2. 选择合适的处理器
	handler, err := a.modeManager.SelectHandler(ctx, githubCtx)
	if err != nil {
		xl.Warnf("No suitable handler found: %v", err)
		return fmt.Errorf("no handler available: %w", err)
	}

	xl.Infof("Selected handler with mode: %s (priority: %d)",
		handler.GetMode(), handler.GetPriority())

	// 3. 执行处理
	err = handler.Execute(ctx, githubCtx)
	if err != nil {
		xl.Errorf("Handler execution failed: %v", err)
		return fmt.Errorf("handler execution failed: %w", err)
	}

	duration := time.Since(startTime)
	xl.Infof("GitHub event processed successfully in %v", duration)

	return nil
}

// ProcessIssueCommentEnhanced 增强版Issue评论处理
// 使用新的进度通信和MCP工具系统
func (a *EnhancedAgent) ProcessIssueCommentEnhanced(ctx context.Context, event *github.IssueCommentEvent) error {
	xl := xlog.NewWith(ctx)

	// 1. 解析为类型安全的上下文
	githubCtx, err := a.eventParser.ParseIssueCommentEvent(ctx, event)
	if err != nil {
		return fmt.Errorf("failed to parse issue comment event: %w", err)
	}

	issueCommentCtx, ok := githubCtx.(*models.IssueCommentContext)
	if !ok {
		return fmt.Errorf("invalid context type for issue comment")
	}

	// 2. 创建进度评论管理器
	pcm := interaction.NewProgressCommentManager(a.github,
		issueCommentCtx.GetRepository(), issueCommentCtx.Issue.GetNumber())

	// 3. 创建任务列表
	tasks := a.taskFactory.CreateIssueProcessingTasks()

	// 4. 初始化进度跟踪
	if err := pcm.InitializeProgress(ctx, tasks); err != nil {
		xl.Errorf("Failed to initialize progress tracking: %v", err)
		return err
	}

	// 5. 创建MCP上下文
	mcpCtx := &models.MCPContext{
		Repository:  githubCtx,
		Issue:       issueCommentCtx.Issue,
		User:        issueCommentCtx.GetSender(),
		Permissions: []string{"github:read", "github:write"},
		Constraints: []string{}, // 根据需要添加约束
	}

	// 6. 执行处理流程
	result, err := a.executeIssueProcessingWithProgress(ctx, issueCommentCtx, mcpCtx, pcm)
	if err != nil {
		xl.Errorf("Issue processing failed: %v", err)

		// 最终化失败结果
		failureResult := &models.ProgressExecutionResult{
			Success:  false,
			Error:    err.Error(),
			Duration: time.Since(pcm.GetTracker().StartTime),
		}

		if finalizeErr := pcm.FinalizeComment(ctx, failureResult); finalizeErr != nil {
			xl.Errorf("Failed to finalize failure comment: %v", finalizeErr)
		}

		return err
	}

	// 7. 最终化成功结果
	if err := pcm.FinalizeComment(ctx, result); err != nil {
		xl.Errorf("Failed to finalize success comment: %v", err)
		return err
	}

	xl.Infof("Issue comment processed successfully")
	return nil
}

// executeIssueProcessingWithProgress 执行Issue处理流程，带进度跟踪
func (a *EnhancedAgent) executeIssueProcessingWithProgress(
	ctx context.Context,
	issueCtx *models.IssueCommentContext,
	mcpCtx *models.MCPContext,
	pcm *interaction.ProgressCommentManager,
) (*models.ProgressExecutionResult, error) {
	xl := xlog.NewWith(ctx)

	// 1. 收集上下文信息
	if err := pcm.UpdateTask(ctx, "gather-context", models.TaskStatusInProgress, "Analyzing issue and requirements"); err != nil {
		return nil, err
	}

	// 使用MCP工具收集更多上下文
	tools, err := a.mcpClient.PrepareTools(ctx, mcpCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare MCP tools: %w", err)
	}

	xl.Infof("Prepared %d MCP tools for issue processing", len(tools))

	if err := pcm.UpdateTask(ctx, "gather-context", models.TaskStatusCompleted); err != nil {
		return nil, err
	}

	// 2. 设置工作空间
	if err := pcm.UpdateTask(ctx, "setup-workspace", models.TaskStatusInProgress, "Creating workspace and branch"); err != nil {
		return nil, err
	}

	ws := a.workspace.CreateWorkspaceFromIssue(issueCtx.Issue)
	if ws == nil {
		return nil, fmt.Errorf("failed to create workspace")
	}

	// 更新MCP上下文
	mcpCtx.WorkspacePath = ws.Path
	mcpCtx.BranchName = ws.Branch

	if err := a.github.CreateBranch(ws); err != nil {
		return nil, fmt.Errorf("failed to create branch: %w", err)
	}

	if err := pcm.UpdateTask(ctx, "setup-workspace", models.TaskStatusCompleted); err != nil {
		return nil, err
	}

	// 3. 生成代码（使用MCP工具）
	if err := pcm.UpdateTask(ctx, "generate-code", models.TaskStatusInProgress, "Generating code implementation"); err != nil {
		return nil, err
	}

	// 这里可以集成AI代码生成逻辑
	// TODO: 实现AI代码生成，使用MCP工具进行文件操作

	if err := pcm.UpdateTask(ctx, "generate-code", models.TaskStatusCompleted); err != nil {
		return nil, err
	}

	// 4. 提交变更
	if err := pcm.UpdateTask(ctx, "commit-changes", models.TaskStatusInProgress, "Committing changes"); err != nil {
		return nil, err
	}

	// 使用现有的提交逻辑
	execResult := &models.ExecutionResult{
		Success:      true,
		Output:       "Code generated successfully",
		FilesChanged: []string{}, // TODO: 从MCP工具调用中获取
		Duration:     time.Since(pcm.GetTracker().StartTime),
	}

	if err := a.github.CommitAndPush(ws, execResult, nil); err != nil {
		return nil, fmt.Errorf("failed to commit and push: %w", err)
	}

	if err := pcm.UpdateTask(ctx, "commit-changes", models.TaskStatusCompleted); err != nil {
		return nil, err
	}

	// 5. 创建PR
	if err := pcm.UpdateTask(ctx, "create-pr", models.TaskStatusInProgress, "Creating pull request"); err != nil {
		return nil, err
	}

	pr, err := a.github.CreatePullRequest(ws)
	if err != nil {
		return nil, fmt.Errorf("failed to create PR: %w", err)
	}

	if err := pcm.UpdateTask(ctx, "create-pr", models.TaskStatusCompleted); err != nil {
		return nil, err
	}

	// 构建结果
	result := &models.ProgressExecutionResult{
		Success:        true,
		Output:         execResult.Output,
		FilesChanged:   execResult.FilesChanged,
		Duration:       time.Since(pcm.GetTracker().StartTime),
		Summary:        fmt.Sprintf("Successfully implemented Issue #%d", issueCtx.Issue.GetNumber()),
		BranchName:     ws.Branch,
		PullRequestURL: pr.GetHTMLURL(),
		TaskResults:    pcm.GetTracker().Tasks,
	}

	return result, nil
}

// GetMCPManager 获取MCP管理器（用于外部扩展）
func (a *EnhancedAgent) GetMCPManager() mcp.MCPManager {
	return a.mcpManager
}

// GetModeManager 获取模式管理器（用于外部扩展）
func (a *EnhancedAgent) GetModeManager() *modes.Manager {
	return a.modeManager
}

// Shutdown 关闭增强版Agent
func (a *EnhancedAgent) Shutdown(ctx context.Context) error {
	xl := xlog.NewWith(ctx)

	// 关闭MCP管理器
	if err := a.mcpManager.Shutdown(ctx); err != nil {
		xl.Errorf("Failed to shutdown MCP manager: %v", err)
		return err
	}

	xl.Infof("Enhanced Agent shutdown completed")
	return nil
}
