package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/qiniu/codeagent/pkg/models"
	"github.com/qiniu/x/xlog"
)

// Client MCP客户端实现
// 用于与AI提供商集成，为AI会话准备工具
type Client struct {
	manager MCPManager
}

// NewClient 创建MCP客户端
func NewClient(manager MCPManager) *Client {
	return &Client{
		manager: manager,
	}
}

// PrepareTools 为AI会话准备工具
func (c *Client) PrepareTools(ctx context.Context, mcpCtx *models.MCPContext) ([]models.Tool, error) {
	xl := xlog.NewWith(ctx)

	tools, err := c.manager.GetAvailableTools(ctx, mcpCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to get available tools: %w", err)
	}

	xl.Infof("Prepared %d tools for AI session", len(tools))
	return tools, nil
}

// ExecuteToolCalls 执行AI返回的工具调用
func (c *Client) ExecuteToolCalls(ctx context.Context, calls []*models.ToolCall, mcpCtx *models.MCPContext) ([]*models.ToolResult, error) {
	xl := xlog.NewWith(ctx)

	if len(calls) == 0 {
		return []*models.ToolResult{}, nil
	}

	xl.Infof("Executing %d tool calls", len(calls))

	var results []*models.ToolResult

	for _, call := range calls {
		xl.Infof("Executing tool call: %s", call.Function.Name)

		result, err := c.manager.HandleToolCall(ctx, call, mcpCtx)
		if err != nil {
			xl.Errorf("Tool call %s failed: %v", call.Function.Name, err)

			// 创建错误结果
			errorResult := &models.ToolResult{
				ID:      call.ID,
				Success: false,
				Error:   err.Error(),
				Type:    "error",
			}
			results = append(results, errorResult)
		} else {
			results = append(results, result)
		}
	}

	successCount := 0
	for _, result := range results {
		if result.Success {
			successCount++
		}
	}

	xl.Infof("Tool execution completed: %d/%d successful", successCount, len(results))
	return results, nil
}

// BuildPrompt 构建包含工具信息的提示
func (c *Client) BuildPrompt(ctx context.Context, userPrompt string, mcpCtx *models.MCPContext) (string, error) {
	tools, err := c.PrepareTools(ctx, mcpCtx)
	if err != nil {
		return "", fmt.Errorf("failed to prepare tools: %w", err)
	}

	if len(tools) == 0 {
		return userPrompt, nil
	}

	var promptBuilder strings.Builder

	// 添加系统提示
	promptBuilder.WriteString("You have access to the following tools via MCP (Model Context Protocol):\n\n")

	// 添加工具列表
	for _, tool := range tools {
		promptBuilder.WriteString(fmt.Sprintf("## %s\n", tool.Name))
		promptBuilder.WriteString(fmt.Sprintf("%s\n\n", tool.Description))

		if tool.InputSchema != nil {
			schemaJSON, err := json.MarshalIndent(tool.InputSchema, "", "  ")
			if err == nil {
				promptBuilder.WriteString("**Input Schema:**\n")
				promptBuilder.WriteString(fmt.Sprintf("```json\n%s\n```\n\n", string(schemaJSON)))
			}
		}
	}

	// 添加使用说明
	promptBuilder.WriteString("**Tool Usage Guidelines:**\n")
	promptBuilder.WriteString("1. Always use the provided MCP tools for file operations and GitHub interactions\n")
	promptBuilder.WriteString("2. Validate tool arguments before making calls\n")
	promptBuilder.WriteString("3. Handle tool errors gracefully and provide meaningful feedback\n")
	promptBuilder.WriteString("4. Use appropriate tools for the context (e.g., github-files for file operations)\n\n")

	// 添加权限信息
	if mcpCtx.Permissions != nil && len(mcpCtx.Permissions) > 0 {
		promptBuilder.WriteString("**Available Permissions:**\n")
		for _, perm := range mcpCtx.Permissions {
			promptBuilder.WriteString(fmt.Sprintf("- %s\n", perm))
		}
		promptBuilder.WriteString("\n")
	}

	// 添加约束信息
	if mcpCtx.Constraints != nil && len(mcpCtx.Constraints) > 0 {
		promptBuilder.WriteString("**Constraints:**\n")
		for _, constraint := range mcpCtx.Constraints {
			promptBuilder.WriteString(fmt.Sprintf("- %s\n", constraint))
		}
		promptBuilder.WriteString("\n")
	}

	// 添加上下文信息
	if mcpCtx.Repository != nil {
		promptBuilder.WriteString("**Repository Context:**\n")
		repo := mcpCtx.Repository.GetRepository()
		promptBuilder.WriteString(fmt.Sprintf("- Repository: %s\n", repo.GetFullName()))
		if mcpCtx.BranchName != "" {
			promptBuilder.WriteString(fmt.Sprintf("- Branch: %s\n", mcpCtx.BranchName))
		}
		if mcpCtx.WorkspacePath != "" {
			promptBuilder.WriteString(fmt.Sprintf("- Workspace: %s\n", mcpCtx.WorkspacePath))
		}
		promptBuilder.WriteString("\n")
	}

	// 添加用户提示
	promptBuilder.WriteString("---\n\n")
	promptBuilder.WriteString("**User Request:**\n")
	promptBuilder.WriteString(userPrompt)

	return promptBuilder.String(), nil
}

// FormatToolResults 格式化工具执行结果为可读文本
func (c *Client) FormatToolResults(results []*models.ToolResult) string {
	if len(results) == 0 {
		return "No tool results to display."
	}

	var builder strings.Builder

	for i, result := range results {
		if i > 0 {
			builder.WriteString("\n---\n\n")
		}

		builder.WriteString(fmt.Sprintf("**Tool Result %d** (ID: %s)\n", i+1, result.ID))

		if result.Success {
			builder.WriteString("✅ **Status:** Success\n")
			if result.Content != nil {
				builder.WriteString("**Content:**\n")

				switch result.Type {
				case "json":
					if jsonBytes, err := json.MarshalIndent(result.Content, "", "  "); err == nil {
						builder.WriteString(fmt.Sprintf("```json\n%s\n```\n", string(jsonBytes)))
					} else {
						builder.WriteString(fmt.Sprintf("%v\n", result.Content))
					}
				case "text":
					builder.WriteString(fmt.Sprintf("```\n%v\n```\n", result.Content))
				default:
					builder.WriteString(fmt.Sprintf("%v\n", result.Content))
				}
			}
		} else {
			builder.WriteString("❌ **Status:** Failed\n")
			if result.Error != "" {
				builder.WriteString(fmt.Sprintf("**Error:** %s\n", result.Error))
			}
		}
	}

	return builder.String()
}

// GetToolDefinitions 获取工具定义的JSON表示（用于AI集成）
func (c *Client) GetToolDefinitions(ctx context.Context, mcpCtx *models.MCPContext) ([]map[string]interface{}, error) {
	tools, err := c.PrepareTools(ctx, mcpCtx)
	if err != nil {
		return nil, err
	}

	var definitions []map[string]interface{}

	for _, tool := range tools {
		definition := map[string]interface{}{
			"name":        tool.Name,
			"description": tool.Description,
		}

		if tool.InputSchema != nil {
			definition["input_schema"] = tool.InputSchema
		}

		definitions = append(definitions, definition)
	}

	return definitions, nil
}

// ValidateToolCall 验证工具调用的格式和参数
func (c *Client) ValidateToolCall(call *models.ToolCall, mcpCtx *models.MCPContext) error {
	if call == nil {
		return fmt.Errorf("tool call is nil")
	}

	if call.Function.Name == "" {
		return fmt.Errorf("tool name is empty")
	}

	if call.Function.Arguments == nil {
		call.Function.Arguments = make(map[string]interface{})
	}

	// 获取可用工具列表进行验证
	tools, err := c.manager.GetAvailableTools(context.Background(), mcpCtx)
	if err != nil {
		return fmt.Errorf("failed to get available tools for validation: %w", err)
	}

	// 检查工具是否存在
	var targetTool *models.Tool
	for _, tool := range tools {
		if tool.Name == call.Function.Name {
			targetTool = &tool
			break
		}
	}

	if targetTool == nil {
		return fmt.Errorf("tool %s not found or not available", call.Function.Name)
	}

	return nil
}
