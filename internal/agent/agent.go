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

	"github.com/qiniu/x/xlog"
)

// EnhancedAgent 增强版Agent，集成了新的组件架构
type EnhancedAgent struct {
	// 原有组件
	config         *config.Config
	clientManager  ghclient.ClientManagerInterface
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
	xl.Infof("NewEnhancedAgent: %+v", cfg)

	// 1. 初始化GitHub客户端管理器
	clientManager, err := ghclient.NewClientManager(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create GitHub client manager: %w", err)
	}

	// 2. 初始化事件解析器
	eventParser := events.NewParser()

	// 3. 初始化MCP管理器和服务器
	mcpManager := mcp.NewManager()

	// 注册内置MCP服务器
	githubFiles := servers.NewGitHubFilesServer(clientManager)
	githubComments := servers.NewGitHubCommentsServer(clientManager)

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
	// Custom command handler has highest priority to handle custom commands first
	var customCmdHandler *modes.CustomCommandHandler
	if cfg.Commands.GlobalPath != "" {
		customCmdHandler = modes.NewCustomCommandHandler(clientManager, workspaceManager, sessionManager, mcpClient, cfg.Commands.GlobalPath, cfg.CodeProvider)
		modeManager.RegisterHandler(customCmdHandler)
		xl.Infof("Custom command handler registered with global config path: %s", cfg.Commands.GlobalPath)
	}

	tagHandler := modes.NewTagHandler(cfg.CodeProvider, clientManager, workspaceManager, mcpClient, sessionManager)
	agentHandler := modes.NewAgentHandler(clientManager, workspaceManager, mcpClient)
	reviewHandler := modes.NewReviewHandler(clientManager, workspaceManager, mcpClient, sessionManager, cfg)

	modeManager.RegisterHandler(tagHandler)
	modeManager.RegisterHandler(agentHandler)
	modeManager.RegisterHandler(reviewHandler)

	// 7. 创建任务工厂
	taskFactory := interaction.NewTaskFactory()

	agent := &EnhancedAgent{
		config:         cfg,
		clientManager:  clientManager,
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

// processGitHubContext 处理已解析的GitHub上下文
func (a *EnhancedAgent) processGitHubContext(ctx context.Context, githubCtx models.GitHubContext, startTime time.Time) error {
	xl := xlog.NewWith(ctx)

	xl.Infof("Parsed event type: %s for repository: %s",
		githubCtx.GetEventType(), githubCtx.GetRepository().GetFullName())

	// 2. 选择合适的处理器
	handler, err := a.modeManager.SelectHandler(ctx, githubCtx)
	if err != nil {
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
