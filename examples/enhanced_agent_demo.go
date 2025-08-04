package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/qiniu/codeagent/internal/agent"
	"github.com/qiniu/codeagent/internal/config"
	"github.com/qiniu/codeagent/internal/workspace"
	"github.com/qiniu/codeagent/pkg/models"

	githubapi "github.com/google/go-github/v58/github"
)

func main() {
	fmt.Println("🤖 CodeAgent Enhanced Demo")
	fmt.Println("==========================")

	// 1. 创建配置
	cfg := &config.Config{
		GitHub: config.GitHubConfig{
			Token: "your-github-token", // 在实际使用中替换为真实token
		},
		CodeProvider: "claude",
		UseDocker:    false,
		Server: config.ServerConfig{
			Port: 8888,
		},
	}

	// 2. 创建工作空间管理器
	workspaceManager := workspace.NewManager(cfg)

	// 3. 创建增强版Agent
	fmt.Println("Initializing Enhanced Agent...")
	enhancedAgent, err := agent.NewEnhancedAgent(cfg, workspaceManager)
	if err != nil {
		log.Fatalf("Failed to create enhanced agent: %v", err)
	}
	defer enhancedAgent.Shutdown(context.Background())

	// 4. 展示Agent能力
	demonstrateAgentCapabilities(enhancedAgent)

	// 5. 模拟事件处理
	demonstrateEventProcessing(enhancedAgent)

	fmt.Println("\n✅ Demo completed successfully!")
}

func demonstrateAgentCapabilities(agent *agent.EnhancedAgent) {
	fmt.Println("\n📊 Agent Capabilities:")

	// MCP服务器信息
	mcpManager := agent.GetMCPManager()
	servers := mcpManager.GetServers()
	fmt.Printf("- MCP Servers: %d registered\n", len(servers))
	for name := range servers {
		fmt.Printf("  • %s\n", name)
	}

	// 模式处理器信息
	modeManager := agent.GetModeManager()
	fmt.Printf("- Mode Handlers: %d registered\n", modeManager.GetHandlerCount())

	// MCP工具信息
	mcpCtx := createDemoMCPContext()
	tools, err := mcpManager.GetAvailableTools(context.Background(), mcpCtx)
	if err == nil {
		fmt.Printf("- Available Tools: %d tools\n", len(tools))
		for _, tool := range tools[:min(5, len(tools))] {
			fmt.Printf("  • %s: %s\n", tool.Name, tool.Description)
		}
		if len(tools) > 5 {
			fmt.Printf("  • ... and %d more tools\n", len(tools)-5)
		}
	}
}

func demonstrateEventProcessing(agent *agent.EnhancedAgent) {
	fmt.Println("\n🎯 Event Processing Demo:")

	// 创建模拟Issue评论事件
	event := createDemoIssueCommentEvent()

	fmt.Printf("Processing Issue Comment Event:\n")
	fmt.Printf("- Issue: #%d - %s\n", event.Issue.GetNumber(), event.Issue.GetTitle())
	fmt.Printf("- Comment: %s\n", event.Comment.GetBody())
	fmt.Printf("- Repository: %s\n", event.Repo.GetFullName())

	// 注意：由于使用demo token，这会失败，但可以展示流程
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := agent.ProcessGitHubEvent(ctx, "issue_comment", event)
	if err != nil {
		fmt.Printf("❌ Processing failed (expected with demo token): %v\n", err)
		fmt.Println("💡 This is expected behavior with demo credentials")
	} else {
		fmt.Println("✅ Event processed successfully!")
	}
}

func createDemoMCPContext() *models.MCPContext {
	return &models.MCPContext{
		Repository: &models.IssueCommentContext{
			BaseContext: models.BaseContext{
				Repository: &githubapi.Repository{
					Name:     githubapi.String("demo-repo"),
					FullName: githubapi.String("demo-owner/demo-repo"),
					Owner: &githubapi.User{
						Login: githubapi.String("demo-owner"),
					},
				},
			},
		},
		Permissions: []string{"github:read", "github:write"},
		Constraints: []string{},
	}
}

func createDemoIssueCommentEvent() *githubapi.IssueCommentEvent {
	return &githubapi.IssueCommentEvent{
		Action: githubapi.String("created"),
		Issue: &githubapi.Issue{
			Number: githubapi.Int(42),
			Title:  githubapi.String("Implement Hello World Feature"),
			Body:   githubapi.String("We need a simple hello world function for our application."),
			State:  githubapi.String("open"),
			User: &githubapi.User{
				Login: githubapi.String("demo-user"),
			},
		},
		Comment: &githubapi.IssueComment{
			Body: githubapi.String("/code Please implement a hello world function in Go"),
			User: &githubapi.User{
				Login: githubapi.String("demo-user"),
			},
			CreatedAt: &githubapi.Timestamp{Time: time.Now()},
		},
		Repo: &githubapi.Repository{
			Name:     githubapi.String("demo-repo"),
			FullName: githubapi.String("demo-owner/demo-repo"),
			Owner: &githubapi.User{
				Login: githubapi.String("demo-owner"),
			},
		},
		Sender: &githubapi.User{
			Login: githubapi.String("demo-user"),
		},
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
