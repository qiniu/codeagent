package prompt

import (
	"bytes"
	"context"
	"fmt"
	"text/template"

	"github.com/qbox/codeagent/pkg/models"
)

// PromptBuilder Prompt 构建器
type PromptBuilder struct {
	promptManager        *PromptManager
	customConfigDetector *CustomConfigDetector
	config               PromptConfig
}

// PromptConfig Prompt 配置
type PromptConfig struct {
	MaxTotalLength int `yaml:"max_total_length"`
}

// PromptRequest Prompt 请求
type PromptRequest struct {
	TemplateID   string                 `json:"template_id"`
	TemplateVars map[string]interface{} `json:"template_vars"`
	Workspace    *models.Workspace      `json:"workspace"`
}

// PromptResult Prompt 结果
type PromptResult struct {
	Content      string                 `json:"content"`
	TemplateID   string                 `json:"template_id"`
	TemplateType string                 `json:"template_type"` // "default" 或 "default_with_custom"
	Length       int                    `json:"length"`
	Metadata     map[string]interface{} `json:"metadata"`
}

// NewPromptBuilder 创建新的 Prompt 构建器
func NewPromptBuilder(promptManager *PromptManager, customConfigDetector *CustomConfigDetector, config PromptConfig) *PromptBuilder {
	return &PromptBuilder{
		promptManager:        promptManager,
		customConfigDetector: customConfigDetector,
		config:               config,
	}
}

// BuildPrompt 构建 Prompt
func (pb *PromptBuilder) BuildPrompt(ctx context.Context, req *PromptRequest) (*PromptResult, error) {
	// 1. 获取模板（优先使用自定义模板，回退到默认模板）
	template, err := pb.promptManager.GetTemplate(req.TemplateID, req.Workspace)
	if err != nil {
		return nil, fmt.Errorf("failed to get template: %w", err)
	}

	// 2. 检查是否存在自定义 CODEAGENT.md 文件
	var customConfigInfo *CustomConfigInfo
	if req.Workspace != nil {
		customConfigInfo, _ = pb.customConfigDetector.GetCODEAGENTFile(ctx, req.Workspace)
	}

	// 3. 准备模板变量
	templateVars := make(map[string]interface{})
	for k, v := range req.TemplateVars {
		templateVars[k] = v
	}

	// 4. 如果有自定义配置，添加到模板变量中
	if customConfigInfo != nil && customConfigInfo.Exists {
		templateVars["has_custom_config"] = true
	} else {
		templateVars["has_custom_config"] = false
	}

	// 5. 渲染模板
	content, err := pb.renderTemplate(template, templateVars)
	if err != nil {
		return nil, fmt.Errorf("failed to render template: %w", err)
	}

	return &PromptResult{
		Content:      content,
		TemplateID:   template.ID,
		TemplateType: getTemplateType(template, customConfigInfo),
		Length:       len(content),
		Metadata:     make(map[string]interface{}),
	}, nil
}

// renderTemplate 渲染模板
func (pb *PromptBuilder) renderTemplate(tmpl *Template, vars map[string]interface{}) (string, error) {
	// 创建模板
	t, err := template.New(tmpl.ID).Parse(tmpl.Content)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	// 渲染模板
	var buf bytes.Buffer
	err = t.Execute(&buf, vars)
	if err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// getTemplateType 获取模板类型
func getTemplateType(template *Template, customConfig *CustomConfigInfo) string {
	if customConfig != nil && customConfig.Exists {
		return "default_with_custom"
	}
	return "default"
}

// 模板变量结构体定义
type IssueTemplateVars struct {
	IssueTitle        string `json:"issue_title"`
	IssueBody         string `json:"issue_body"`
	IssueLabels       string `json:"issue_labels,omitempty"`
	HistoricalContext string `json:"historical_context,omitempty"`
	IncludeTests      bool   `json:"include_tests"`
	IncludeDocs       bool   `json:"include_docs"`
	HasCustomConfig   bool   `json:"has_custom_config"`
}

type ReviewTemplateVars struct {
	ReviewComments    string `json:"review_comments"`
	CodeContext       string `json:"code_context,omitempty"`
	HistoricalContext string `json:"historical_context,omitempty"`
	HasCustomConfig   bool   `json:"has_custom_config"`
}
