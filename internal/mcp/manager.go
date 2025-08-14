package mcp

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/qiniu/codeagent/pkg/models"
	"github.com/qiniu/x/xlog"
)

// Manager MCP管理器实现
type Manager struct {
	servers   map[string]MCPServer
	metrics   map[string]*models.ExecutionMetrics
	validator ToolValidator
	mutex     sync.RWMutex
}

// NewManager 创建MCP管理器
func NewManager() *Manager {
	return &Manager{
		servers:   make(map[string]MCPServer),
		metrics:   make(map[string]*models.ExecutionMetrics),
		validator: NewToolValidator(),
	}
}

// RegisterServer 注册MCP服务器
func (m *Manager) RegisterServer(name string, server MCPServer) error {
	xl := xlog.New("")

	m.mutex.Lock()
	defer m.mutex.Unlock()

	if _, exists := m.servers[name]; exists {
		return fmt.Errorf("server %s already registered", name)
	}

	// 初始化服务器
	if err := server.Initialize(context.Background()); err != nil {
		return fmt.Errorf("failed to initialize server %s: %w", name, err)
	}

	m.servers[name] = server
	m.metrics[name] = &models.ExecutionMetrics{
		ToolCalls:     0,
		Duration:      0,
		Success:       0,
		Errors:        0,
		LastExecution: time.Time{},
	}

	info := server.GetInfo()
	xl.Debugf("Registered MCP server: %s v%s (%d tools)",
		info.Name, info.Version, len(info.Capabilities.Tools))

	return nil
}

// UnregisterServer 取消注册MCP服务器
func (m *Manager) UnregisterServer(name string) error {
	xl := xlog.New("")

	m.mutex.Lock()
	defer m.mutex.Unlock()

	server, exists := m.servers[name]
	if !exists {
		return fmt.Errorf("server %s not found", name)
	}

	// 关闭服务器
	if err := server.Shutdown(context.Background()); err != nil {
		xl.Warnf("Failed to shutdown server %s: %v", name, err)
	}

	delete(m.servers, name)
	delete(m.metrics, name)

	xl.Infof("Unregistered MCP server: %s", name)
	return nil
}

// GetAvailableTools 获取可用工具列表
func (m *Manager) GetAvailableTools(ctx context.Context, mcpCtx *models.MCPContext) ([]models.Tool, error) {
	xl := xlog.NewWith(ctx)

	m.mutex.RLock()
	defer m.mutex.RUnlock()

	var tools []models.Tool

	for serverName, server := range m.servers {
		if !server.IsAvailable(ctx, mcpCtx) {
			xl.Debugf("Server %s not available for current context", serverName)
			continue
		}

		serverTools := server.GetTools()

		// 为工具名称添加服务器前缀，避免冲突
		for _, tool := range serverTools {
			tool.Name = fmt.Sprintf("%s_%s", serverName, tool.Name)
			tools = append(tools, tool)
		}

		xl.Debugf("Added %d tools from server %s", len(serverTools), serverName)
	}

	xl.Infof("Total available tools: %d", len(tools))
	return tools, nil
}

// HandleToolCall 处理工具调用
func (m *Manager) HandleToolCall(ctx context.Context, call *models.ToolCall, mcpCtx *models.MCPContext) (*models.ToolResult, error) {
	xl := xlog.NewWith(ctx)
	startTime := time.Now()

	// 解析服务器名称和工具名称
	serverName, toolName, err := m.parseToolName(call.Function.Name)
	if err != nil {
		return m.errorResult(call.ID, err), nil
	}

	m.mutex.RLock()
	server, exists := m.servers[serverName]
	metrics := m.metrics[serverName]
	m.mutex.RUnlock()

	if !exists {
		err := fmt.Errorf("unknown MCP server: %s", serverName)
		return m.errorResult(call.ID, err), nil
	}

	// 验证工具调用
	tools := server.GetTools()
	var targetTool *models.Tool
	for _, tool := range tools {
		if tool.Name == toolName {
			targetTool = &tool
			break
		}
	}

	if targetTool == nil {
		err := fmt.Errorf("tool %s not found in server %s", toolName, serverName)
		return m.errorResult(call.ID, err), nil
	}

	// 验证调用参数和权限
	if err := m.validator.ValidateCall(call, targetTool); err != nil {
		return m.errorResult(call.ID, err), nil
	}

	if err := m.validator.ValidatePermissions(call, mcpCtx); err != nil {
		return m.errorResult(call.ID, err), nil
	}

	xl.Infof("Executing tool call: %s.%s", serverName, toolName)

	// 执行工具调用
	result, err := server.HandleToolCall(ctx, call, mcpCtx)
	if err != nil {
		xl.Errorf("Tool call failed: %v", err)
		m.updateMetrics(metrics, startTime, false)
		return m.errorResult(call.ID, err), nil
	}

	// 更新指标
	m.updateMetrics(metrics, startTime, true)

	xl.Infof("Tool call completed successfully in %v", time.Since(startTime))
	return result, nil
}

// GetServers 获取已注册的服务器列表
func (m *Manager) GetServers() map[string]MCPServer {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	servers := make(map[string]MCPServer)
	for name, server := range m.servers {
		servers[name] = server
	}
	return servers
}

// GetMetrics 获取执行指标
func (m *Manager) GetMetrics() map[string]*models.ExecutionMetrics {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	metrics := make(map[string]*models.ExecutionMetrics)
	for name, metric := range m.metrics {
		// 创建副本避免并发问题
		metrics[name] = &models.ExecutionMetrics{
			ToolCalls:     metric.ToolCalls,
			Duration:      metric.Duration,
			Success:       metric.Success,
			Errors:        metric.Errors,
			LastExecution: metric.LastExecution,
		}
	}
	return metrics
}

// parseToolName 解析工具名称，返回服务器名称和工具名称
func (m *Manager) parseToolName(fullName string) (serverName, toolName string, err error) {
	parts := strings.SplitN(fullName, "_", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid tool name format: %s (expected: server_tool)", fullName)
	}
	return parts[0], parts[1], nil
}

// errorResult 创建错误结果
func (m *Manager) errorResult(id string, err error) *models.ToolResult {
	return &models.ToolResult{
		ID:      id,
		Success: false,
		Error:   err.Error(),
		Type:    "error",
	}
}

// updateMetrics 更新执行指标
func (m *Manager) updateMetrics(metrics *models.ExecutionMetrics, startTime time.Time, success bool) {
	metrics.ToolCalls++
	metrics.Duration += time.Since(startTime)
	metrics.LastExecution = time.Now()

	if success {
		metrics.Success++
	} else {
		metrics.Errors++
	}
}

// Shutdown 关闭管理器，停止所有服务器
func (m *Manager) Shutdown(ctx context.Context) error {
	xl := xlog.NewWith(ctx)

	m.mutex.Lock()
	defer m.mutex.Unlock()

	var errors []string

	for name, server := range m.servers {
		if err := server.Shutdown(ctx); err != nil {
			errors = append(errors, fmt.Sprintf("server %s: %v", name, err))
		}
	}

	// 清空所有注册
	m.servers = make(map[string]MCPServer)
	m.metrics = make(map[string]*models.ExecutionMetrics)

	if len(errors) > 0 {
		xl.Warnf("Some servers failed to shutdown: %s", strings.Join(errors, "; "))
		return fmt.Errorf("shutdown errors: %s", strings.Join(errors, "; "))
	}

	xl.Debugf("All MCP servers shutdown successfully")
	return nil
}
