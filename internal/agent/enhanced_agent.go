package agent

import (
	"context"
	"fmt"
	"io"
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
	reviewHandler := modes.NewReviewHandler(githubClient, workspaceManager, mcpClient, sessionManager)

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
// 使用MCP工具系统，不带进度跟踪
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

	// 2. 创建MCP上下文
	mcpCtx := &models.MCPContext{
		Repository:  githubCtx,
		Issue:       issueCommentCtx.Issue,
		User:        issueCommentCtx.GetSender(),
		Permissions: []string{"github:read", "github:write"},
		Constraints: []string{}, // 根据需要添加约束
	}

	// 3. 执行处理流程
	result, err := a.executeIssueProcessing(ctx, issueCommentCtx, mcpCtx)
	if err != nil {
		xl.Errorf("Issue processing failed: %v", err)
		return err
	}

	xl.Infof("Issue comment processed successfully: %s", result.Summary)
	return nil
}

// executeIssueProcessing 执行Issue处理流程，不带进度跟踪
func (a *EnhancedAgent) executeIssueProcessing(
	ctx context.Context,
	issueCtx *models.IssueCommentContext,
	mcpCtx *models.MCPContext,
) (*models.ProgressExecutionResult, error) {
	xl := xlog.NewWith(ctx)
	startTime := time.Now()

	// 1. 收集上下文信息
	xl.Infof("Analyzing issue and requirements")

	// 使用MCP工具收集更多上下文
	tools, err := a.mcpClient.PrepareTools(ctx, mcpCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare MCP tools: %w", err)
	}

	xl.Infof("Prepared %d MCP tools for issue processing", len(tools))

	// 2. 设置工作空间
	xl.Infof("Creating workspace and branch")

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

	// 3. 创建PR（在代码生成之前）
	xl.Infof("Creating pull request")

	pr, err := a.github.CreatePullRequest(ws)
	if err != nil {
		return nil, fmt.Errorf("failed to create PR: %w", err)
	}

	// 4. 移动工作空间从 Issue 到 PR
	if err := a.workspace.MoveIssueToPR(ws, pr.GetNumber()); err != nil {
		xl.Errorf("Failed to move workspace: %v", err)
	}
	ws.PRNumber = pr.GetNumber()
	ws.PullRequest = pr

	// 5. 生成代码（使用AI会话）
	xl.Infof("Generating code implementation for Issue #%d", issueCtx.Issue.GetNumber())

	// 创建AI会话进行代码生成
	session, err := a.sessionManager.GetSession(ws)
	if err != nil {
		return nil, fmt.Errorf("failed to create AI session: %w", err)
	}

	// 构建简洁的代码生成提示，仅基于Issue内容（不包含额外项目上下文）
	codePrompt := fmt.Sprintf(`You are an AI coding assistant working on GitHub issue #%d.

Issue Title: %s
Issue Description: %s

Please implement the requested functionality by creating or modifying the necessary files.

Instructions:
1. Analyze the issue requirements carefully
2. Create well-structured, maintainable code
3. Follow best practices for the project's programming language
4. Include appropriate error handling
5. Add comments for complex logic
6. Ensure code is production-ready

Provide your implementation with clear explanations of the changes made.

Focus only on the specific issue requirements, do not make broad changes to the codebase.`,
		issueCtx.Issue.GetNumber(),
		issueCtx.Issue.GetTitle(),
		issueCtx.Issue.GetBody())

	// 执行AI代码生成
	resp, err := session.Prompt(codePrompt)
	if err != nil {
		return nil, fmt.Errorf("failed to generate code with AI: %w", err)
	}

	// 读取AI生成的响应
	aiOutput, err := io.ReadAll(resp.Out)
	if err != nil {
		return nil, fmt.Errorf("failed to read AI response: %w", err)
	}

	aiOutputStr := string(aiOutput)
	xl.Infof("AI code generation completed, output length: %d", len(aiOutputStr))
	xl.Debugf("AI Output: %s", aiOutputStr)

	// 6. 提交变更
	xl.Infof("Committing changes")

	// 使用现有的提交逻辑
	execResult := &models.ExecutionResult{
		Success:      true,
		Output:       aiOutputStr,
		FilesChanged: []string{}, // TODO: 从AI响应中解析
		Duration:     time.Since(startTime),
	}

	if err := a.github.CommitAndPush(ws, execResult, nil); err != nil {
		return nil, fmt.Errorf("failed to commit and push: %w", err)
	}

	// 构建结果
	result := &models.ProgressExecutionResult{
		Success:        true,
		Output:         execResult.Output,
		FilesChanged:   execResult.FilesChanged,
		Duration:       time.Since(startTime),
		Summary:        fmt.Sprintf("Successfully implemented Issue #%d", issueCtx.Issue.GetNumber()),
		BranchName:     ws.Branch,
		PullRequestURL: pr.GetHTMLURL(),
		TaskResults:    []*models.Task{}, // No progress tracking
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
