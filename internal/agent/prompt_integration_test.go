package agent

import (
	"testing"

	"github.com/qbox/codeagent/internal/config"
	"github.com/qbox/codeagent/internal/workspace"
	"github.com/qbox/codeagent/pkg/models"
)

// TestPromptSystemIntegration 测试 Prompt 系统在 Agent 中的集成
func TestPromptSystemIntegration(t *testing.T) {
	// 跳过需要外部服务的测试
	t.Skip("Skipping test that requires external services")

	// 创建测试配置
	cfg := &config.Config{
		CodeProvider: "claude",
		// 其他配置项...
	}

	// 创建工作空间管理器
	workspaceManager := workspace.NewManager(cfg)

	// 创建 Agent
	agent := New(cfg, workspaceManager)
	if agent == nil {
		t.Fatal("Failed to create agent")
	}

	// 测试 buildSingleReviewPrompt 方法
	t.Run("BuildSingleReviewPrompt", func(t *testing.T) {
		// 模拟工作空间
		ws := &models.Workspace{
			Org:  "testorg",
			Repo: "testrepo",
			Path: "/tmp/test",
		}

		// 测试继续处理模板
		prompt, err := agent.buildSingleReviewPrompt(
			"single_review_continue",
			"请添加错误处理",
			"main.go",
			"行号：42",
			"使用 try-catch 模式",
			ws,
		)
		if err != nil {
			t.Fatalf("Failed to build single review continue prompt: %v", err)
		}

		if len(prompt) == 0 {
			t.Error("Expected non-empty prompt content")
		}

		// 检查内容是否包含预期的变量
		if !contains(prompt, "请添加错误处理") {
			t.Error("Expected prompt to contain comment body")
		}

		if !contains(prompt, "main.go") {
			t.Error("Expected prompt to contain file path")
		}

		if !contains(prompt, "行号：42") {
			t.Error("Expected prompt to contain line range info")
		}

		if !contains(prompt, "使用 try-catch 模式") {
			t.Error("Expected prompt to contain additional instructions")
		}

		// 测试修复模板
		prompt, err = agent.buildSingleReviewPrompt(
			"single_review_fix",
			"修复内存泄漏问题",
			"memory.go",
			"行号范围：100-120",
			"使用 defer 语句",
			ws,
		)
		if err != nil {
			t.Fatalf("Failed to build single review fix prompt: %v", err)
		}

		if len(prompt) == 0 {
			t.Error("Expected non-empty prompt content")
		}

		// 检查内容是否包含预期的变量
		if !contains(prompt, "修复内存泄漏问题") {
			t.Error("Expected prompt to contain comment body")
		}

		if !contains(prompt, "memory.go") {
			t.Error("Expected prompt to contain file path")
		}

		if !contains(prompt, "行号范围：100-120") {
			t.Error("Expected prompt to contain line range info")
		}

		if !contains(prompt, "使用 defer 语句") {
			t.Error("Expected prompt to contain additional instructions")
		}
	})

	// 测试 buildBatchReviewPrompt 方法
	t.Run("BuildBatchReviewPrompt", func(t *testing.T) {
		// 模拟工作空间
		ws := &models.Workspace{
			Org:  "testorg",
			Repo: "testrepo",
			Path: "/tmp/test",
		}

		// 测试批量处理模板
		prompt, err := agent.buildBatchReviewPrompt(
			"整体代码质量良好，但需要一些改进",
			"评论1：添加注释\n评论2：优化性能\n评论3：修复bug",
			"优先处理高优先级问题",
			"继续处理",
			ws,
		)
		if err != nil {
			t.Fatalf("Failed to build batch review prompt: %v", err)
		}

		if len(prompt) == 0 {
			t.Error("Expected non-empty prompt content")
		}

		// 检查内容是否包含预期的变量
		if !contains(prompt, "整体代码质量良好，但需要一些改进") {
			t.Error("Expected prompt to contain review body")
		}

		if !contains(prompt, "评论1：添加注释") {
			t.Error("Expected prompt to contain batch comments")
		}

		if !contains(prompt, "优先处理高优先级问题") {
			t.Error("Expected prompt to contain additional instructions")
		}

		if !contains(prompt, "继续处理") {
			t.Error("Expected prompt to contain processing mode")
		}
	})

	// 测试 buildPrompt 方法
	t.Run("BuildPrompt", func(t *testing.T) {
		// 测试继续模式
		prompt := agent.buildPrompt("Continue", "请优化性能", "历史讨论：之前已经优化过一次")
		if len(prompt) == 0 {
			t.Error("Expected non-empty prompt content")
		}

		// 检查内容是否包含预期的变量
		if !contains(prompt, "请优化性能") {
			t.Error("Expected prompt to contain args")
		}

		if !contains(prompt, "历史讨论：之前已经优化过一次") {
			t.Error("Expected prompt to contain historical context")
		}

		// 测试修复模式
		prompt = agent.buildPrompt("Fix", "修复bug", "历史讨论：这个bug之前出现过")
		if len(prompt) == 0 {
			t.Error("Expected non-empty prompt content")
		}

		// 检查内容是否包含预期的变量
		if !contains(prompt, "修复bug") {
			t.Error("Expected prompt to contain args")
		}

		if !contains(prompt, "历史讨论：这个bug之前出现过") {
			t.Error("Expected prompt to contain historical context")
		}
	})
}

// contains 检查字符串是否包含子字符串
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > len(substr) && (s[:len(substr)] == substr ||
			s[len(s)-len(substr):] == substr ||
			func() bool {
				for i := 1; i <= len(s)-len(substr); i++ {
					if s[i:i+len(substr)] == substr {
						return true
					}
				}
				return false
			}())))
}
