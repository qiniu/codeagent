package prompt

import (
	"strings"
	"testing"

	"github.com/qbox/codeagent/internal/workspace"
	"github.com/qbox/codeagent/pkg/models"
)

func TestPromptManager(t *testing.T) {
	// 创建模拟的工作空间管理器
	workspaceManager := &workspace.Manager{}

	// 创建 Prompt Manager
	pm := NewManager(workspaceManager)

	// 测试获取默认模板
	t.Run("GetDefaultTemplate", func(t *testing.T) {
		// 测试获取 Issue 模板
		template, err := pm.GetTemplate("issue_based_code_generation", nil)
		if err != nil {
			t.Fatalf("Failed to get issue template: %v", err)
		}

		if template.ID != "issue_based_code_generation" {
			t.Errorf("Expected template ID 'issue_based_code_generation', got '%s'", template.ID)
		}

		if template.Source != "default" {
			t.Errorf("Expected template source 'default', got '%s'", template.Source)
		}

		// 测试获取 Review 模板
		template, err = pm.GetTemplate("review_based_code_modification", nil)
		if err != nil {
			t.Fatalf("Failed to get review template: %v", err)
		}

		if template.ID != "review_based_code_modification" {
			t.Errorf("Expected template ID 'review_based_code_modification', got '%s'", template.ID)
		}
	})

	// 测试获取不存在的模板
	t.Run("GetNonExistentTemplate", func(t *testing.T) {
		_, err := pm.GetTemplate("non_existent_template", nil)
		if err == nil {
			t.Error("Expected error for non-existent template")
		}
	})

	// 测试列出所有模板
	t.Run("ListTemplates", func(t *testing.T) {
		templates := pm.ListTemplates()
		if len(templates) < 2 {
			t.Errorf("Expected at least 2 templates, got %d", len(templates))
		}

		// 检查是否包含预期的模板
		foundIssue := false
		foundReview := false
		for _, template := range templates {
			if template.ID == "issue_based_code_generation" {
				foundIssue = true
			}
			if template.ID == "review_based_code_modification" {
				foundReview = true
			}
		}

		if !foundIssue {
			t.Error("Expected to find issue_based_code_generation template")
		}
		if !foundReview {
			t.Error("Expected to find review_based_code_modification template")
		}
	})
}

func TestCustomConfigDetector(t *testing.T) {
	// 创建自定义配置检测器
	detector := NewDetector()

	// 测试空工作空间
	t.Run("NilWorkspace", func(t *testing.T) {
		info, err := detector.GetCODEAGENTFile(nil, nil)
		if err != nil {
			t.Fatalf("Failed to get CODEAGENT file info: %v", err)
		}

		if info.Exists {
			t.Error("Expected CODEAGENT file to not exist for nil workspace")
		}
	})

	// 测试模拟工作空间
	t.Run("MockWorkspace", func(t *testing.T) {
		workspace := &models.Workspace{
			Org:  "testorg",
			Repo: "testrepo",
			Path: "/tmp/test",
		}

		info, err := detector.GetCODEAGENTFile(nil, workspace)
		if err != nil {
			t.Fatalf("Failed to get CODEAGENT file info: %v", err)
		}

		// 由于路径不存在，应该返回 false
		if info.Exists {
			t.Error("Expected CODEAGENT file to not exist for non-existent path")
		}
	})
}

func TestPromptBuilder(t *testing.T) {
	// 创建模拟的工作空间管理器
	workspaceManager := &workspace.Manager{}

	// 创建 Prompt Manager
	pm := NewManager(workspaceManager)

	// 创建自定义配置检测器
	detector := NewDetector()

	// 创建 Prompt 配置
	config := PromptConfig{
		MaxTotalLength: 8000,
	}

	// 创建 Prompt Builder
	pb := NewBuilder(pm, detector, config)

	// 测试构建 Issue 模板
	t.Run("BuildIssueTemplate", func(t *testing.T) {
		req := &PromptRequest{
			TemplateID: "issue_based_code_generation",
			TemplateVars: map[string]interface{}{
				"issue_title":       "测试 Issue",
				"issue_body":        "这是一个测试 Issue",
				"include_tests":     true,
				"include_docs":      true,
				"has_custom_config": false,
			},
			Workspace: nil,
		}

		result, err := pb.BuildPrompt(nil, req)
		if err != nil {
			t.Fatalf("Failed to build prompt: %v", err)
		}

		if result.TemplateID != "issue_based_code_generation" {
			t.Errorf("Expected template ID 'issue_based_code_generation', got '%s'", result.TemplateID)
		}

		if result.TemplateType != "default" {
			t.Errorf("Expected template type 'default', got '%s'", result.TemplateType)
		}

		if len(result.Content) == 0 {
			t.Error("Expected non-empty content")
		}

		// 检查内容是否包含预期的变量
		if !strings.Contains(result.Content, "测试 Issue") {
			t.Error("Expected content to contain issue title")
		}

		if !strings.Contains(result.Content, "这是一个测试 Issue") {
			t.Error("Expected content to contain issue body")
		}
	})

	// 测试构建 Review 模板
	t.Run("BuildReviewTemplate", func(t *testing.T) {
		req := &PromptRequest{
			TemplateID: "review_based_code_modification",
			TemplateVars: map[string]interface{}{
				"review_comments":    "请修复这个 bug",
				"historical_context": "之前的讨论内容",
				"has_custom_config":  false,
			},
			Workspace: nil,
		}

		result, err := pb.BuildPrompt(nil, req)
		if err != nil {
			t.Fatalf("Failed to build prompt: %v", err)
		}

		if result.TemplateID != "review_based_code_modification" {
			t.Errorf("Expected template ID 'review_based_code_modification', got '%s'", result.TemplateID)
		}

		if len(result.Content) == 0 {
			t.Error("Expected non-empty content")
		}

		// 检查内容是否包含预期的变量
		if !strings.Contains(result.Content, "请修复这个 bug") {
			t.Error("Expected content to contain review comments")
		}

		if !strings.Contains(result.Content, "之前的讨论内容") {
			t.Error("Expected content to contain historical context")
		}
	})

	// 测试构建不存在的模板
	t.Run("BuildNonExistentTemplate", func(t *testing.T) {
		req := &PromptRequest{
			TemplateID:   "non_existent_template",
			TemplateVars: map[string]interface{}{},
			Workspace:    nil,
		}

		_, err := pb.BuildPrompt(nil, req)
		if err == nil {
			t.Error("Expected error for non-existent template")
		}
	})
}
