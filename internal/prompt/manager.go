package prompt

import (
	"fmt"
	"sync"
	"time"

	"github.com/qbox/codeagent/internal/workspace"
	"github.com/qbox/codeagent/pkg/models"
)

// PromptManager 统一管理内置和自定义模板
type PromptManager struct {
	defaultTemplates map[string]*Template
	customTemplates  map[string]*Template
	workspaceManager *workspace.Manager
	cache            *TemplateCache
	mu               sync.RWMutex
}

// Template 表示一个 Prompt 模板
type Template struct {
	ID          string                 `yaml:"id"`
	Name        string                 `yaml:"name"`
	Description string                 `yaml:"description"`
	Content     string                 `yaml:"content"`
	Variables   []TemplateVariable     `yaml:"variables"`
	Source      string                 `yaml:"source"`   // "default" 或 "custom"
	Priority    int                    `yaml:"priority"` // 自定义模板优先级更高
	Metadata    map[string]interface{} `yaml:"metadata"`
}

// TemplateVariable 表示模板变量
type TemplateVariable struct {
	Name        string `yaml:"name"`
	Type        string `yaml:"type"` // string, int, bool, array
	Required    bool   `yaml:"required"`
	Default     string `yaml:"default"`
	Description string `yaml:"description"`
}

// TemplateCache 模板缓存
type TemplateCache struct {
	cache map[string]*Template
	mu    sync.RWMutex
	ttl   time.Duration
}

// NewPromptManager 创建新的 Prompt Manager
func NewPromptManager(workspaceManager *workspace.Manager) *PromptManager {
	pm := &PromptManager{
		defaultTemplates: make(map[string]*Template),
		customTemplates:  make(map[string]*Template),
		workspaceManager: workspaceManager,
		cache: &TemplateCache{
			cache: make(map[string]*Template),
			ttl:   1 * time.Hour,
		},
	}

	// 加载内置模板
	pm.loadDefaultTemplates()

	return pm
}

// loadDefaultTemplates 加载内置模板
func (pm *PromptManager) loadDefaultTemplates() {
	// 基于 Issue 的代码生成模板
	issueTemplate := &Template{
		ID:          "issue_based_code_generation",
		Name:        "基于 Issue 的代码生成",
		Description: "根据 GitHub Issue 生成相应的代码实现",
		Source:      "default",
		Priority:    1,
		Content: `根据以下 Issue 需求生成高质量的代码：

## Issue 信息
标题：{{.issue_title}}
描述：{{.issue_body}}
{{if .issue_labels}}
标签：{{.issue_labels}}
{{end}}

{{if .historical_context}}
## 历史讨论
{{.historical_context}}
{{end}}

{{if .include_tests}}
请同时生成对应的单元测试。
{{end}}

{{if .include_docs}}
请同时生成相关的文档说明。
{{end}}

{{if .has_custom_config}}
## 项目自定义配置参考
当前项目包含一个 CODEAGENT.md 文件，其中定义了项目的特定要求和配置。
请在完成上述任务时，同步参考该文件中的内容，确保生成的代码符合项目的
技术栈、编码规范和架构要求。

请确保生成的代码：
1. 遵循项目中定义的技术栈和框架
2. 符合项目的编码规范和架构模式
3. 满足项目的特殊要求和约束
4. 保持与现有代码风格的一致性
{{end}}`,
		Variables: []TemplateVariable{
			{Name: "issue_title", Type: "string", Required: true, Description: "Issue 标题"},
			{Name: "issue_body", Type: "string", Required: true, Description: "Issue 描述"},
			{Name: "issue_labels", Type: "string", Required: false, Description: "Issue 标签"},
			{Name: "historical_context", Type: "string", Required: false, Description: "历史讨论内容"},
			{Name: "include_tests", Type: "bool", Required: false, Default: "true", Description: "是否包含测试代码"},
			{Name: "include_docs", Type: "bool", Required: false, Default: "true", Description: "是否包含文档"},
			{Name: "has_custom_config", Type: "bool", Required: false, Default: "false", Description: "是否存在自定义配置"},
		},
	}

	// 基于 Code Review Comments 的代码修改模板
	reviewTemplate := &Template{
		ID:          "review_based_code_modification",
		Name:        "基于 Code Review Comments 的代码修改",
		Description: "根据 Code Review Comments 修改代码",
		Source:      "default",
		Priority:    1,
		Content: `根据以下 Code Review Comments 修改代码：

## Review Comments
{{.review_comments}}

{{if .code_context}}
## 相关代码上下文
{{.code_context}}
{{end}}

{{if .historical_context}}
## 历史讨论
{{.historical_context}}
{{end}}

请根据评论要求修改代码，确保：
1. 解决评论中提到的问题
2. 保持代码质量和一致性
3. 遵循项目的编码规范

{{if .has_custom_config}}
## 项目自定义配置参考
当前项目包含一个 CODEAGENT.md 文件，其中定义了项目的特定要求和配置。
请在完成上述任务时，同步参考该文件中的内容，确保生成的代码符合项目的
技术栈、编码规范和架构要求。

请确保生成的代码：
1. 遵循项目中定义的技术栈和框架
2. 符合项目的编码规范和架构模式
3. 满足项目的特殊要求和约束
4. 保持与现有代码风格的一致性
{{end}}`,
		Variables: []TemplateVariable{
			{Name: "review_comments", Type: "string", Required: true, Description: "Code Review Comments 内容"},
			{Name: "code_context", Type: "string", Required: false, Description: "相关代码上下文"},
			{Name: "historical_context", Type: "string", Required: false, Description: "历史讨论内容"},
			{Name: "has_custom_config", Type: "bool", Required: false, Default: "false", Description: "是否存在自定义配置"},
		},
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.defaultTemplates[issueTemplate.ID] = issueTemplate
	pm.defaultTemplates[reviewTemplate.ID] = reviewTemplate
}

// GetTemplate 获取模板（优先使用自定义模板，回退到默认模板）
func (pm *PromptManager) GetTemplate(templateID string, workspace *models.Workspace) (*Template, error) {
	// 先检查缓存
	if cached := pm.cache.Get(templateID); cached != nil {
		return cached, nil
	}

	pm.mu.RLock()
	defer pm.mu.RUnlock()

	// 优先查找自定义模板
	if customTemplate, exists := pm.customTemplates[templateID]; exists {
		pm.cache.Set(templateID, customTemplate)
		return customTemplate, nil
	}

	// 回退到默认模板
	if defaultTemplate, exists := pm.defaultTemplates[templateID]; exists {
		pm.cache.Set(templateID, defaultTemplate)
		return defaultTemplate, nil
	}

	return nil, fmt.Errorf("template not found: %s", templateID)
}

// LoadCustomTemplate 加载自定义模板
func (pm *PromptManager) LoadCustomTemplate(template *Template) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	template.Source = "custom"
	template.Priority = 10 // 自定义模板优先级更高
	pm.customTemplates[template.ID] = template

	// 清除缓存
	pm.cache.Delete(template.ID)

	return nil
}

// ListTemplates 列出所有模板
func (pm *PromptManager) ListTemplates() []*Template {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var templates []*Template

	// 添加默认模板
	for _, template := range pm.defaultTemplates {
		templates = append(templates, template)
	}

	// 添加自定义模板
	for _, template := range pm.customTemplates {
		templates = append(templates, template)
	}

	return templates
}

// TemplateCache 方法实现
func (tc *TemplateCache) Get(key string) *Template {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	return tc.cache[key]
}

func (tc *TemplateCache) Set(key string, template *Template) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.cache[key] = template
}

func (tc *TemplateCache) Delete(key string) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	delete(tc.cache, key)
}
