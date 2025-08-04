package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/qiniu/codeagent/pkg/models"

	githubapi "github.com/google/go-github/v58/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockMCPServer 模拟MCP服务器
type MockMCPServer struct {
	name      string
	tools     []models.Tool
	available bool
	responses map[string]*models.ToolResult
}

func NewMockMCPServer(name string) *MockMCPServer {
	return &MockMCPServer{
		name:      name,
		tools:     []models.Tool{},
		available: true,
		responses: make(map[string]*models.ToolResult),
	}
}

func (m *MockMCPServer) GetInfo() *models.MCPServerInfo {
	return &models.MCPServerInfo{
		Name:        m.name,
		Version:     "1.0.0-test",
		Description: "Mock MCP server for testing",
		Capabilities: models.MCPServerCapabilities{
			Tools: m.tools,
		},
	}
}

func (m *MockMCPServer) GetTools() []models.Tool {
	return m.tools
}

func (m *MockMCPServer) IsAvailable(ctx context.Context, mcpCtx *models.MCPContext) bool {
	return m.available
}

func (m *MockMCPServer) HandleToolCall(ctx context.Context, call *models.ToolCall, mcpCtx *models.MCPContext) (*models.ToolResult, error) {
	// 从完整工具名称中提取原始工具名称（去掉服务器前缀）
	toolName := call.Function.Name
	if parts := strings.Split(call.Function.Name, "_"); len(parts) > 1 {
		toolName = strings.Join(parts[1:], "_")
	}

	if result, exists := m.responses[toolName]; exists {
		result.ID = call.ID
		return result, nil
	}

	return &models.ToolResult{
		ID:      call.ID,
		Success: true,
		Content: map[string]interface{}{
			"tool":   call.Function.Name,
			"server": m.name,
		},
		Type: "json",
	}, nil
}

func (m *MockMCPServer) Initialize(ctx context.Context) error {
	return nil
}

func (m *MockMCPServer) Shutdown(ctx context.Context) error {
	return nil
}

func (m *MockMCPServer) AddTool(tool models.Tool) {
	m.tools = append(m.tools, tool)
}

func (m *MockMCPServer) SetAvailable(available bool) {
	m.available = available
}

func (m *MockMCPServer) SetResponse(toolName string, result *models.ToolResult) {
	m.responses[toolName] = result
}

func TestMCPManager_RegisterServer(t *testing.T) {
	manager := NewManager()
	mockServer := NewMockMCPServer("test-server")

	// 注册服务器
	err := manager.RegisterServer("test", mockServer)
	require.NoError(t, err)

	// 验证服务器已注册
	servers := manager.GetServers()
	assert.Len(t, servers, 1)
	assert.Contains(t, servers, "test")

	// 验证指标已初始化
	metrics := manager.GetMetrics()
	assert.Len(t, metrics, 1)
	assert.Contains(t, metrics, "test")
}

func TestMCPManager_RegisterDuplicateServer(t *testing.T) {
	manager := NewManager()
	mockServer1 := NewMockMCPServer("test-server-1")
	mockServer2 := NewMockMCPServer("test-server-2")

	// 注册第一个服务器
	err := manager.RegisterServer("test", mockServer1)
	require.NoError(t, err)

	// 尝试注册重名服务器
	err = manager.RegisterServer("test", mockServer2)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}

func TestMCPManager_GetAvailableTools(t *testing.T) {
	manager := NewManager()

	// 创建两个模拟服务器
	server1 := NewMockMCPServer("server1")
	server1.AddTool(models.Tool{
		Name:        "tool1",
		Description: "Test tool 1",
	})
	server1.AddTool(models.Tool{
		Name:        "tool2",
		Description: "Test tool 2",
	})

	server2 := NewMockMCPServer("server2")
	server2.AddTool(models.Tool{
		Name:        "tool3",
		Description: "Test tool 3",
	})

	// 注册服务器
	err := manager.RegisterServer("srv1", server1)
	require.NoError(t, err)
	err = manager.RegisterServer("srv2", server2)
	require.NoError(t, err)

	// 创建测试上下文
	mcpCtx := &models.MCPContext{
		Repository: &models.IssueCommentContext{
			BaseContext: models.BaseContext{
				Repository: &githubapi.Repository{
					Name: githubapi.String("test-repo"),
					Owner: &githubapi.User{
						Login: githubapi.String("test-owner"),
					},
				},
			},
		},
		Permissions: []string{"github:read"},
	}

	// 获取可用工具
	tools, err := manager.GetAvailableTools(context.Background(), mcpCtx)
	require.NoError(t, err)

	// 验证工具数量和命名
	assert.Len(t, tools, 3)

	toolNames := make([]string, len(tools))
	for i, tool := range tools {
		toolNames[i] = tool.Name
	}

	assert.Contains(t, toolNames, "srv1_tool1")
	assert.Contains(t, toolNames, "srv1_tool2")
	assert.Contains(t, toolNames, "srv2_tool3")
}

func TestMCPManager_GetAvailableTools_UnavailableServer(t *testing.T) {
	manager := NewManager()

	// 创建不可用的服务器
	server := NewMockMCPServer("server")
	server.AddTool(models.Tool{
		Name:        "tool1",
		Description: "Test tool 1",
	})
	server.SetAvailable(false) // 设置为不可用

	err := manager.RegisterServer("srv", server)
	require.NoError(t, err)

	mcpCtx := &models.MCPContext{}

	// 获取可用工具
	tools, err := manager.GetAvailableTools(context.Background(), mcpCtx)
	require.NoError(t, err)

	// 不可用的服务器不应该提供工具
	assert.Len(t, tools, 0)
}

func TestMCPManager_HandleToolCall(t *testing.T) {
	manager := NewManager()

	// 创建模拟服务器
	server := NewMockMCPServer("server")
	server.AddTool(models.Tool{
		Name:        "test_tool",
		Description: "Test tool",
	})

	// 设置预期响应
	expectedResult := &models.ToolResult{
		Success: true,
		Content: map[string]interface{}{
			"result": "success",
		},
		Type: "json",
	}
	server.SetResponse("test_tool", expectedResult)

	err := manager.RegisterServer("srv", server)
	require.NoError(t, err)

	// 创建工具调用
	call := &models.ToolCall{
		ID: "test-call-1",
		Function: models.ToolFunction{
			Name:      "srv_test_tool",
			Arguments: map[string]interface{}{},
		},
	}

	mcpCtx := &models.MCPContext{}

	// 执行工具调用
	result, err := manager.HandleToolCall(context.Background(), call, mcpCtx)
	require.NoError(t, err)

	assert.Equal(t, "test-call-1", result.ID)
	assert.True(t, result.Success)
	assert.Equal(t, expectedResult.Content, result.Content)
}

func TestMCPManager_HandleToolCall_UnknownServer(t *testing.T) {
	manager := NewManager()

	call := &models.ToolCall{
		ID: "test-call-1",
		Function: models.ToolFunction{
			Name:      "unknown_server_tool",
			Arguments: map[string]interface{}{},
		},
	}

	mcpCtx := &models.MCPContext{}

	// 执行未知服务器的工具调用
	result, err := manager.HandleToolCall(context.Background(), call, mcpCtx)
	require.NoError(t, err)

	assert.Equal(t, "test-call-1", result.ID)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "unknown MCP server")
}

func TestMCPManager_HandleToolCall_InvalidToolName(t *testing.T) {
	manager := NewManager()

	call := &models.ToolCall{
		ID: "test-call-1",
		Function: models.ToolFunction{
			Name:      "invalid-tool-name", // 没有下划线分隔符
			Arguments: map[string]interface{}{},
		},
	}

	mcpCtx := &models.MCPContext{}

	result, err := manager.HandleToolCall(context.Background(), call, mcpCtx)
	require.NoError(t, err)

	assert.Equal(t, "test-call-1", result.ID)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "invalid tool name format")
}

func TestMCPManager_UnregisterServer(t *testing.T) {
	manager := NewManager()
	mockServer := NewMockMCPServer("test-server")

	// 注册服务器
	err := manager.RegisterServer("test", mockServer)
	require.NoError(t, err)

	// 验证服务器已注册
	servers := manager.GetServers()
	assert.Len(t, servers, 1)

	// 取消注册服务器
	err = manager.UnregisterServer("test")
	require.NoError(t, err)

	// 验证服务器已被移除
	servers = manager.GetServers()
	assert.Len(t, servers, 0)

	metrics := manager.GetMetrics()
	assert.Len(t, metrics, 0)
}

func TestMCPManager_UnregisterNonexistentServer(t *testing.T) {
	manager := NewManager()

	err := manager.UnregisterServer("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestMCPManager_Shutdown(t *testing.T) {
	manager := NewManager()

	// 注册多个服务器
	server1 := NewMockMCPServer("server1")
	server2 := NewMockMCPServer("server2")

	err := manager.RegisterServer("srv1", server1)
	require.NoError(t, err)
	err = manager.RegisterServer("srv2", server2)
	require.NoError(t, err)

	// 验证服务器已注册
	servers := manager.GetServers()
	assert.Len(t, servers, 2)

	// 关闭管理器
	err = manager.Shutdown(context.Background())
	require.NoError(t, err)

	// 验证所有服务器已被清理
	servers = manager.GetServers()
	assert.Len(t, servers, 0)

	metrics := manager.GetMetrics()
	assert.Len(t, metrics, 0)
}
