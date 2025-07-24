package prompt

import (
	"context"
	"testing"

	"github.com/qbox/codeagent/internal/workspace"
	"github.com/qbox/codeagent/pkg/models"
)

// TestAgentIntegration 测试 Agent 集成
func TestAgentIntegration(t *testing.T) {
	// 创建模拟的工作空间管理器
	workspaceManager := &workspace.Manager{}

	// 创建 Prompt 系统组件
	pm := NewManager(workspaceManager)
	detector := NewDetector()
	config := PromptConfig{
		MaxTotalLength: 8000,
	}
	pb := NewBuilder(pm, detector, config)

	// 测试单个 Review Comment 继续处理
	t.Run("SingleReviewContinue", func(t *testing.T) {
		req := PromptRequest{
			TemplateID: "single_review_continue",
			TemplateVars: map[string]interface{}{
				"comment_body":            "请添加错误处理",
				"file_path":               "main.go",
				"line_range_info":         "行号：42",
				"additional_instructions": "使用 try-catch 模式",
			},
			Workspace: &models.Workspace{
				Org:  "testorg",
				Repo: "testrepo",
				Path: "/tmp/test",
			},
		}

		result, err := pb.BuildPrompt(context.Background(), &req)
		if err != nil {
			t.Fatalf("Failed to build prompt: %v", err)
		}

		if result.TemplateID != "single_review_continue" {
			t.Errorf("Expected template ID 'single_review_continue', got '%s'", result.TemplateID)
		}

		if len(result.Content) == 0 {
			t.Error("Expected non-empty content")
		}

		// 检查内容是否包含预期的变量
		if !contains(result.Content, "请添加错误处理") {
			t.Error("Expected content to contain comment body")
		}

		if !contains(result.Content, "main.go") {
			t.Error("Expected content to contain file path")
		}

		if !contains(result.Content, "行号：42") {
			t.Error("Expected content to contain line range info")
		}

		if !contains(result.Content, "使用 try-catch 模式") {
			t.Error("Expected content to contain additional instructions")
		}
	})

	// 测试单个 Review Comment 修复处理
	t.Run("SingleReviewFix", func(t *testing.T) {
		req := PromptRequest{
			TemplateID: "single_review_fix",
			TemplateVars: map[string]interface{}{
				"comment_body":            "修复内存泄漏问题",
				"file_path":               "memory.go",
				"line_range_info":         "行号范围：100-120",
				"additional_instructions": "使用 defer 语句",
			},
			Workspace: &models.Workspace{
				Org:  "testorg",
				Repo: "testrepo",
				Path: "/tmp/test",
			},
		}

		result, err := pb.BuildPrompt(context.Background(), &req)
		if err != nil {
			t.Fatalf("Failed to build prompt: %v", err)
		}

		if result.TemplateID != "single_review_fix" {
			t.Errorf("Expected template ID 'single_review_fix', got '%s'", result.TemplateID)
		}

		if len(result.Content) == 0 {
			t.Error("Expected non-empty content")
		}

		// 检查内容是否包含预期的变量
		if !contains(result.Content, "修复内存泄漏问题") {
			t.Error("Expected content to contain comment body")
		}

		if !contains(result.Content, "memory.go") {
			t.Error("Expected content to contain file path")
		}

		if !contains(result.Content, "行号范围：100-120") {
			t.Error("Expected content to contain line range info")
		}

		if !contains(result.Content, "使用 defer 语句") {
			t.Error("Expected content to contain additional instructions")
		}
	})

	// 测试批量 Review Comments 处理
	t.Run("BatchReviewProcessing", func(t *testing.T) {
		req := PromptRequest{
			TemplateID: "batch_review_processing",
			TemplateVars: map[string]interface{}{
				"review_body":             "整体代码质量良好，但需要一些改进",
				"batch_comments":          "评论1：添加注释\n评论2：优化性能\n评论3：修复bug",
				"additional_instructions": "优先处理高优先级问题",
				"processing_mode":         "继续处理",
			},
			Workspace: &models.Workspace{
				Org:  "testorg",
				Repo: "testrepo",
				Path: "/tmp/test",
			},
		}

		result, err := pb.BuildPrompt(context.Background(), &req)
		if err != nil {
			t.Fatalf("Failed to build prompt: %v", err)
		}

		if result.TemplateID != "batch_review_processing" {
			t.Errorf("Expected template ID 'batch_review_processing', got '%s'", result.TemplateID)
		}

		if len(result.Content) == 0 {
			t.Error("Expected non-empty content")
		}

		// 检查内容是否包含预期的变量
		if !contains(result.Content, "整体代码质量良好，但需要一些改进") {
			t.Error("Expected content to contain review body")
		}

		if !contains(result.Content, "评论1：添加注释") {
			t.Error("Expected content to contain batch comments")
		}

		if !contains(result.Content, "优先处理高优先级问题") {
			t.Error("Expected content to contain additional instructions")
		}

		if !contains(result.Content, "继续处理") {
			t.Error("Expected content to contain processing mode")
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
