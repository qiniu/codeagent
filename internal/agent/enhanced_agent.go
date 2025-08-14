package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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
func NewEnhancedAgent(cfg *config.Config, workspaceManager *workspace.Manager, installationID int64) (*EnhancedAgent, error) {

	// 1. 初始化GitHub客户端并绑定到特定installation
	githubClient, err := ghclient.NewClientWithInstallation(cfg, installationID)
	if err != nil {
		return nil, fmt.Errorf("failed to create GitHub client for installation %d: %w", installationID, err)
	}

	// 2. 初始化事件解析器
	eventParser := events.NewParser()

	// 3. 初始化MCP管理器和服务器
	mcpManager := mcp.NewManager()

	// 注册内置MCP服务器（直接使用重构后的客户端）
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

	// 注册处理器（按优先级顺序，直接使用重构后的客户端）
	tagHandler := modes.NewTagHandler(cfg.CodeProvider, githubClient, workspaceManager, mcpClient, sessionManager)
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

	return agent, nil
}

// ProcessGitHubWebhookEvent 处理来自Webhook的GitHub事件（推荐方法）
func (a *EnhancedAgent) ProcessGitHubWebhookEvent(ctx context.Context, eventType string, deliveryID string, payload []byte) error {
	xl := xlog.NewWith(ctx)

	startTime := time.Now()

	// 解析GitHub事件为类型安全的上下文（不需要再提取installation ID，已经在创建时绑定）
	githubCtx, err := a.eventParser.ParseWebhookEvent(ctx, eventType, deliveryID, payload)
	if err != nil {
		// 对于不支持的事件类型，使用DEBUG级别，避免日志冗余
		if strings.Contains(err.Error(), "unsupported event type") {
			xl.Debugf("Skipping unsupported event type: %s", eventType)
		} else {
			xl.Warnf("Failed to parse GitHub webhook event: %v", err)
		}
		return fmt.Errorf("failed to parse webhook event: %w", err)
	}

	return a.processGitHubContext(ctx, githubCtx, startTime)
}

// processGitHubContext 处理已解析的GitHub上下文
func (a *EnhancedAgent) processGitHubContext(ctx context.Context, githubCtx models.GitHubContext, startTime time.Time) error {
	xl := xlog.NewWith(ctx)

	xl.Debugf("Processing event: %s for repository: %s",
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

	xl.Debugf("Enhanced Agent shutdown completed")
	return nil
}

// ExtractInstallationIDFromPayload 从 webhook payload 中提取 installation ID（用于工厂函数）
func ExtractInstallationIDFromPayload(payload []byte) (int64, error) {
	// 解析payload为通用map，提取installation信息
	var data map[string]interface{}
	if err := json.Unmarshal(payload, &data); err != nil {
		return 0, fmt.Errorf("failed to parse webhook payload: %w", err)
	}

	// 检查是否存在installation字段
	if installation, ok := data["installation"]; ok {
		if installationMap, ok := installation.(map[string]interface{}); ok {
			if idInterface, ok := installationMap["id"]; ok {
				var installationID int64
				switch v := idInterface.(type) {
				case float64:
					installationID = int64(v)
				case int64:
					installationID = v
				case int:
					installationID = int64(v)
				default:
					return 0, fmt.Errorf("invalid installation ID type: %T", v)
				}

				return installationID, nil
			}
		}
	}

	// 如果没有找到installation字段，返回0（PAT模式）
	return 0, fmt.Errorf("no installation found in payload")
}
