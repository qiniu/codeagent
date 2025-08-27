package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/qiniu/codeagent/internal/config"
	"github.com/qiniu/codeagent/internal/github"
	"github.com/qiniu/codeagent/internal/mcp"
	"github.com/qiniu/codeagent/internal/mcp/servers"
	"github.com/qiniu/codeagent/pkg/models"
)

var (
	mcpManager mcp.MCPManager
	mcpContext *models.MCPContext
)

func main() {
	// 创建日志文件用于调试
	logFile, err := os.OpenFile("/tmp/mcp-server.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		// 如果无法创建日志文件，回退到stderr
		log.SetOutput(os.Stderr)
	} else {
		// 同时输出到文件和stderr
		log.SetOutput(io.MultiWriter(os.Stderr, logFile))
		defer logFile.Close()
	}
	log.SetPrefix("[MCP Server] ")

	log.Println("Starting CodeAgent MCP Server...")

	// 初始化
	if err := initialize(); err != nil {
		log.Fatalf("Initialization failed: %v", err)
	}

	log.Println("MCP Server ready, listening on stdin/stdout...")

	// 处理MCP协议
	handleMCPProtocol()
}

// initialize 初始化MCP服务器
func initialize() error {
	// 获取环境变量
	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		return fmt.Errorf("GITHUB_TOKEN environment variable is required")
	}

	repoOwner := os.Getenv("REPO_OWNER")
	repoName := os.Getenv("REPO_NAME")
	if repoOwner == "" || repoName == "" {
		return fmt.Errorf("REPO_OWNER and REPO_NAME environment variables are required")
	}

	// 创建基本配置
	cfg := &config.Config{
		GitHub: config.GitHubConfig{
			Token: githubToken,
		},
	}

	// 如果有GitHub App配置
	if appIDStr := os.Getenv("GITHUB_APP_ID"); appIDStr != "" {
		if appID, err := strconv.ParseInt(appIDStr, 10, 64); err == nil {
			cfg.GitHub.App.AppID = appID
		}
	}
	if privateKey := os.Getenv("GITHUB_APP_PRIVATE_KEY"); privateKey != "" {
		cfg.GitHub.App.PrivateKey = privateKey
	}
	if privateKeyPath := os.Getenv("GITHUB_APP_PRIVATE_KEY_PATH"); privateKeyPath != "" {
		cfg.GitHub.App.PrivateKeyPath = privateKeyPath
	}

	// 创建GitHub客户端管理器
	clientManager, err := github.NewClientManager(cfg)
	if err != nil {
		return fmt.Errorf("failed to create GitHub client manager: %w", err)
	}

	// 创建MCP管理器
	mcpManager = mcp.NewManager()

	// 注册GitHub评论服务器
	githubCommentsServer := servers.NewGitHubCommentsServer(clientManager)
	if err := mcpManager.RegisterServer("github-comments", githubCommentsServer); err != nil {
		return fmt.Errorf("failed to register GitHub comments server: %w", err)
	}

	// 注册GitHub文件服务器
	githubFilesServer := servers.NewGitHubFilesServer(clientManager)
	if err := mcpManager.RegisterServer("github-files", githubFilesServer); err != nil {
		return fmt.Errorf("failed to register GitHub files server: %w", err)
	}

	// 构建MCP上下文
	mcpContext = buildMCPContext(repoOwner, repoName)

	log.Printf("Registered %d MCP servers", len(mcpManager.GetServers()))
	
	// 输出到 stderr，和官方 GitHub MCP 服务器保持一致
	fmt.Fprintln(os.Stderr, "CodeAgent MCP Server running on stdio")
	
	return nil
}

// buildMCPContext 构建MCP上下文
func buildMCPContext(owner, repo string) *models.MCPContext {
	// 创建基本的GitHub上下文
	githubCtx := models.NewGitHubContextWrapper(owner, repo)

	ctx := &models.MCPContext{
		Repository:    githubCtx,
		WorkspacePath: os.Getenv("WORKSPACE_PATH"),
		BranchName:    os.Getenv("BRANCH_NAME"),
		Metadata:      make(map[string]string),
		Permissions: []string{
			"github:read",
			"github:write",
		},
	}

	// 添加PR信息（如果有）
	if prNum := os.Getenv("PR_NUMBER"); prNum != "" {
		if num, err := strconv.Atoi(prNum); err == nil {
			ctx.Metadata["pr_number"] = strconv.Itoa(num)
		}
	}

	// 添加Issue信息（如果有）
	if issueNum := os.Getenv("ISSUE_NUMBER"); issueNum != "" {
		if num, err := strconv.Atoi(issueNum); err == nil {
			ctx.Metadata["issue_number"] = strconv.Itoa(num)
		}
	}

	return ctx
}

// handleMCPProtocol 处理MCP协议
func handleMCPProtocol() {
	scanner := bufio.NewScanner(os.Stdin)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		log.Printf("Received request: %s", line)

		var request models.MCPRequest
		if err := json.Unmarshal([]byte(line), &request); err != nil {
			log.Printf("Failed to parse request: %v", err)
			sendError("", -32700, "Parse error", err.Error())
			continue
		}

		// 处理请求
		response := handleMCPRequest(&request)

		// 发送响应（如果不为nil）
		if response != nil {
			responseJSON, err := json.Marshal(response)
			if err != nil {
				log.Printf("Failed to marshal response: %v", err)
				sendError(request.ID.Value, -32603, "Internal error", err.Error())
				continue
			}

			fmt.Println(string(responseJSON))
			log.Printf("Sent response: %s", string(responseJSON))
		} else {
			log.Printf("No response needed for method: %s", request.Method)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Scanner error: %v", err)
	}
}

// handleMCPRequest 处理MCP请求
func handleMCPRequest(request *models.MCPRequest) *models.MCPResponse {
	ctx := context.Background()

	switch request.Method {
	case "initialize":
		return handleInitialize(request)

	case "tools/list":
		return handleToolsList(ctx, request)

	case "tools/call":
		return handleToolCall(ctx, request)

	case "notifications/initialized":
		// 客户端通知初始化完成 - 这是通知，不需要响应
		log.Printf("Client sent initialized notification")
		return nil // 通知不需要响应

	default:
		return &models.MCPResponse{
			ID:      request.ID,
			JSONRPC: "2.0",
			Error: &models.MCPError{
				Code:    -32601,
				Message: "Method not found",
				Data:    fmt.Sprintf("Unknown method: %s", request.Method),
			},
		}
	}
}

// handleInitialize 处理初始化请求
func handleInitialize(request *models.MCPRequest) *models.MCPResponse {
	// 使用固定的协议版本，和官方 GitHub MCP 服务器保持一致
	protocolVersion := "2024-11-05"
	
	// 从客户端请求中提取请求的协议版本（仅用于日志）
	if params, ok := request.Params["protocolVersion"].(string); ok {
		log.Printf("Client requested protocol version: %s", params)
	}
	
	log.Printf("Server responding with protocol version: %s", protocolVersion)
	
	return &models.MCPResponse{
		ID:      request.ID,
		JSONRPC: "2.0",
		Result: map[string]interface{}{
			"protocolVersion": protocolVersion, // 使用固定协议版本
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{}, // 服务器支持工具
			},
			"serverInfo": map[string]interface{}{
				"name":    "codeagent-mcp-server",
				"version": "1.0.0",
			},
		},
	}
}

// handleToolsList 处理工具列表请求
func handleToolsList(ctx context.Context, request *models.MCPRequest) *models.MCPResponse {
	tools, err := mcpManager.GetAvailableTools(ctx, mcpContext)
	if err != nil {
		return &models.MCPResponse{
			ID:      request.ID,
			JSONRPC: "2.0",
			Error: &models.MCPError{
				Code:    -32603,
				Message: "Failed to get tools",
				Data:    err.Error(),
			},
		}
	}

	// 转换为MCP协议格式
	mcpTools := make([]interface{}, len(tools))
	for i, tool := range tools {
		mcpTools[i] = map[string]interface{}{
			"name":        tool.Name,
			"description": tool.Description,
			"inputSchema": tool.InputSchema,
		}
	}

	return &models.MCPResponse{
		ID:      request.ID,
		JSONRPC: "2.0",
		Result: map[string]interface{}{
			"tools": mcpTools,
		},
	}
}

// handleToolCall 处理工具调用请求
func handleToolCall(ctx context.Context, request *models.MCPRequest) *models.MCPResponse {
	// 从请求参数中提取工具调用信息
	params, ok := request.Params["arguments"].(map[string]interface{})
	if !ok {
		return &models.MCPResponse{
			ID:      request.ID,
			JSONRPC: "2.0",
			Error: &models.MCPError{
				Code:    -32602,
				Message: "Invalid params",
				Data:    "Missing or invalid arguments",
			},
		}
	}

	toolName, ok := request.Params["name"].(string)
	if !ok {
		return &models.MCPResponse{
			ID:      request.ID,
			JSONRPC: "2.0",
			Error: &models.MCPError{
				Code:    -32602,
				Message: "Invalid params",
				Data:    "Missing or invalid tool name",
			},
		}
	}

	// 构建工具调用
	toolCall := &models.ToolCall{
		ID: request.ID,
		Function: models.ToolFunction{
			Name:      toolName,
			Arguments: params,
		},
	}

	log.Printf("Executing tool call: %s with params: %+v", toolName, params)

	// 执行工具调用
	result, err := mcpManager.HandleToolCall(ctx, toolCall, mcpContext)
	
	log.Printf("Tool call result - Success: %v, Error: %v, Content: %+v", 
		result != nil && result.Success, err, 
		func() interface{} { if result != nil { return result.Content } else { return nil } }())
	if err != nil {
		return &models.MCPResponse{
			ID:      request.ID,
			JSONRPC: "2.0",
			Error: &models.MCPError{
				Code:    -32603,
				Message: "Tool execution failed",
				Data:    err.Error(),
			},
		}
	}

	// 构建响应
	var mcpResult interface{}
	if result.Success {
		mcpResult = map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": formatToolResult(result),
				},
			},
		}
	} else {
		return &models.MCPResponse{
			ID:      request.ID,
			JSONRPC: "2.0",
			Error: &models.MCPError{
				Code:    -32603,
				Message: "Tool execution failed",
				Data:    result.Error,
			},
		}
	}

	return &models.MCPResponse{
		ID:      request.ID,
		JSONRPC: "2.0",
		Result:  mcpResult,
	}
}

// formatToolResult 格式化工具结果
func formatToolResult(result *models.ToolResult) string {
	if result.Type == "json" {
		if jsonData, err := json.MarshalIndent(result.Content, "", "  "); err == nil {
			return string(jsonData)
		}
	}

	return fmt.Sprintf("%v", result.Content)
}

// sendError 发送错误响应
func sendError(id interface{}, code int, message, data string) {
	response := &models.MCPResponse{
		ID:      models.MCPID{Value: id},
		JSONRPC: "2.0",
		Error: &models.MCPError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}

	responseJSON, _ := json.Marshal(response)
	fmt.Println(string(responseJSON))
}