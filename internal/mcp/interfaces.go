package mcp

import (
	"context"

	"github.com/qiniu/codeagent/pkg/models"
)

// MCPServer MCP服务器接口
type MCPServer interface {
	// GetInfo 获取服务器信息
	GetInfo() *models.MCPServerInfo

	// GetTools 获取服务器提供的工具列表
	GetTools() []models.Tool

	// IsAvailable 检查服务器是否在当前上下文中可用
	IsAvailable(ctx context.Context, mcpCtx *models.MCPContext) bool

	// HandleToolCall 处理工具调用
	HandleToolCall(ctx context.Context, call *models.ToolCall, mcpCtx *models.MCPContext) (*models.ToolResult, error)

	// Initialize 初始化服务器
	Initialize(ctx context.Context) error

	// Shutdown 关闭服务器
	Shutdown(ctx context.Context) error
}

// MCPManager MCP管理器接口
type MCPManager interface {
	// RegisterServer 注册MCP服务器
	RegisterServer(name string, server MCPServer) error

	// UnregisterServer 取消注册MCP服务器
	UnregisterServer(name string) error

	// GetAvailableTools 获取可用工具列表
	GetAvailableTools(ctx context.Context, mcpCtx *models.MCPContext) ([]models.Tool, error)

	// HandleToolCall 处理工具调用
	HandleToolCall(ctx context.Context, call *models.ToolCall, mcpCtx *models.MCPContext) (*models.ToolResult, error)

	// GetServers 获取已注册的服务器列表
	GetServers() map[string]MCPServer

	// GetMetrics 获取执行指标
	GetMetrics() map[string]*models.ExecutionMetrics

	// Shutdown 关闭管理器和所有服务器
	Shutdown(ctx context.Context) error
}

// MCPClient MCP客户端接口
// 用于与AI提供商集成
type MCPClient interface {
	// PrepareTools 为AI会话准备工具
	PrepareTools(ctx context.Context, mcpCtx *models.MCPContext) ([]models.Tool, error)

	// ExecuteToolCalls 执行AI返回的工具调用
	ExecuteToolCalls(ctx context.Context, calls []*models.ToolCall, mcpCtx *models.MCPContext) ([]*models.ToolResult, error)

	// BuildPrompt 构建包含工具信息的提示
	BuildPrompt(ctx context.Context, userPrompt string, mcpCtx *models.MCPContext) (string, error)
}

// ToolValidator 工具验证器接口
type ToolValidator interface {
	// ValidateCall 验证工具调用
	ValidateCall(call *models.ToolCall, tool *models.Tool) error

	// ValidatePermissions 验证权限
	ValidatePermissions(call *models.ToolCall, mcpCtx *models.MCPContext) error

	// ValidateArguments 验证参数
	ValidateArguments(args map[string]interface{}, schema *models.JSONSchema) error
}
