package models

import (
	"time"
)

// MCPRequest MCP协议请求
type MCPRequest struct {
	ID     string                 `json:"id"`
	Method string                 `json:"method"`
	Params map[string]interface{} `json:"params"`
}

// MCPResponse MCP协议响应
type MCPResponse struct {
	ID     string      `json:"id"`
	Result interface{} `json:"result,omitempty"`
	Error  *MCPError   `json:"error,omitempty"`
}

// MCPError MCP错误
type MCPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data,omitempty"`
}

// Tool MCP工具定义
type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema *JSONSchema `json:"input_schema"`
}

// JSONSchema JSON Schema定义
type JSONSchema struct {
	Type                 string                 `json:"type"`
	Properties           map[string]*JSONSchema `json:"properties,omitempty"`
	Required             []string               `json:"required,omitempty"`
	Items                *JSONSchema            `json:"items,omitempty"`
	Description          string                 `json:"description,omitempty"`
	Enum                 []interface{}          `json:"enum,omitempty"`
	AdditionalProperties bool                   `json:"additionalProperties,omitempty"`
}

// ToolCall 工具调用
type ToolCall struct {
	ID       string                 `json:"id"`
	Function ToolFunction           `json:"function"`
	Context  map[string]interface{} `json:"context,omitempty"`
}

// ToolFunction 工具函数
type ToolFunction struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// ToolResult 工具执行结果
type ToolResult struct {
	ID      string      `json:"id"`
	Success bool        `json:"success"`
	Content interface{} `json:"content,omitempty"`
	Error   string      `json:"error,omitempty"`
	Type    string      `json:"type,omitempty"` // text, json, image, etc.
}

// MCPServerCapabilities MCP服务器能力声明
type MCPServerCapabilities struct {
	Tools     []Tool `json:"tools"`
	Resources []any  `json:"resources,omitempty"`
	Prompts   []any  `json:"prompts,omitempty"`
}

// MCPContext MCP执行上下文
type MCPContext struct {
	// 基础上下文
	Repository  GitHubContext `json:"repository"`
	Issue       interface{}   `json:"issue,omitempty"`
	PullRequest interface{}   `json:"pull_request,omitempty"`
	User        interface{}   `json:"user,omitempty"`

	// 工作环境
	WorkspacePath string            `json:"workspace_path,omitempty"`
	BranchName    string            `json:"branch_name,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`

	// 权限控制
	Permissions []string `json:"permissions,omitempty"`
	Constraints []string `json:"constraints,omitempty"`
}

// MCPServerInfo MCP服务器信息
type MCPServerInfo struct {
	Name         string                `json:"name"`
	Version      string                `json:"version"`
	Description  string                `json:"description"`
	Capabilities MCPServerCapabilities `json:"capabilities"`
	CreatedAt    time.Time             `json:"created_at"`
}

// ExecutionMetrics 执行指标
type ExecutionMetrics struct {
	ToolCalls     int           `json:"tool_calls"`
	Duration      time.Duration `json:"duration"`
	Success       int           `json:"success"`
	Errors        int           `json:"errors"`
	LastExecution time.Time     `json:"last_execution"`
}
