package agent

import (
	"testing"
)

// TestBuildFallbackPrompt 测试回退 Prompt 构建
func TestBuildFallbackPrompt(t *testing.T) {
	// 创建模拟的 Agent（只包含基本结构）
	agent := &Agent{}

	// 测试单个 Review Comment 继续处理回退
	t.Run("SingleReviewContinue", func(t *testing.T) {
		vars := map[string]interface{}{
			"comment_body":            "请添加错误处理",
			"file_path":               "main.go",
			"line_range_info":         "行号：42",
			"additional_instructions": "使用 try-catch 模式",
		}

		prompt := agent.buildFallbackPrompt("single_review_continue", vars)
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

		if !contains(prompt, "根据以下代码行评论继续处理代码") {
			t.Error("Expected prompt to contain correct action")
		}
	})

	// 测试单个 Review Comment 修复处理回退
	t.Run("SingleReviewFix", func(t *testing.T) {
		vars := map[string]interface{}{
			"comment_body":            "修复内存泄漏问题",
			"file_path":               "memory.go",
			"line_range_info":         "行号范围：100-120",
			"additional_instructions": "使用 defer 语句",
		}

		prompt := agent.buildFallbackPrompt("single_review_fix", vars)
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

		if !contains(prompt, "根据以下代码行评论修复代码") {
			t.Error("Expected prompt to contain correct action")
		}
	})

	// 测试批量 Review Comments 继续处理回退
	t.Run("BatchReviewProcessingContinue", func(t *testing.T) {
		vars := map[string]interface{}{
			"batch_comments":          "评论1：添加注释\n评论2：优化性能\n评论3：修复bug",
			"additional_instructions": "优先处理高优先级问题",
			"processing_mode":         "继续处理",
		}

		prompt := agent.buildFallbackPrompt("batch_review_processing", vars)
		if len(prompt) == 0 {
			t.Error("Expected non-empty prompt content")
		}

		// 检查内容是否包含预期的变量
		if !contains(prompt, "评论1：添加注释") {
			t.Error("Expected prompt to contain batch comments")
		}

		if !contains(prompt, "优先处理高优先级问题") {
			t.Error("Expected prompt to contain additional instructions")
		}

		if !contains(prompt, "处理代码") {
			t.Error("Expected prompt to contain correct action")
		}
	})

	// 测试批量 Review Comments 修复处理回退
	t.Run("BatchReviewProcessingFix", func(t *testing.T) {
		vars := map[string]interface{}{
			"batch_comments":          "评论1：修复bug\n评论2：优化性能\n评论3：添加测试",
			"additional_instructions": "优先修复严重bug",
			"processing_mode":         "修复问题",
		}

		prompt := agent.buildFallbackPrompt("batch_review_processing", vars)
		if len(prompt) == 0 {
			t.Error("Expected non-empty prompt content")
		}

		// 检查内容是否包含预期的变量
		if !contains(prompt, "评论1：修复bug") {
			t.Error("Expected prompt to contain batch comments")
		}

		if !contains(prompt, "优先修复严重bug") {
			t.Error("Expected prompt to contain additional instructions")
		}

		if !contains(prompt, "修复代码问题") {
			t.Error("Expected prompt to contain correct action")
		}
	})

	// 测试未知模板回退
	t.Run("UnknownTemplate", func(t *testing.T) {
		vars := map[string]interface{}{
			"test": "value",
		}

		prompt := agent.buildFallbackPrompt("unknown_template", vars)
		if len(prompt) == 0 {
			t.Error("Expected non-empty prompt content")
		}

		if !contains(prompt, "请根据要求处理代码") {
			t.Error("Expected prompt to contain default message")
		}
	})
}
