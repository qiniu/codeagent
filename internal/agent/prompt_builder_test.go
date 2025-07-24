package agent

import (
	"testing"

	"github.com/qbox/codeagent/internal/prompt"
	"github.com/qbox/codeagent/internal/workspace"
	"github.com/qbox/codeagent/pkg/models"
)

// TestPromptBuilderMethods 测试 Prompt 构建方法
func TestPromptBuilderMethods(t *testing.T) {
	// 创建模拟的工作空间管理器
	workspaceManager := &workspace.Manager{}

	// 创建 Prompt 系统组件
	pm := prompt.NewManager(workspaceManager)
	detector := prompt.NewDetector()
	config := prompt.PromptConfig{
		MaxTotalLength: 8000,
	}
	pb := prompt.NewBuilder(pm, detector, config)

	// 创建模拟的 Agent（只包含 promptBuilder）
	agent := &Agent{
		promptBuilder: pb,
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
}
