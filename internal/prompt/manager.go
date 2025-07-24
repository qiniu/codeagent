package prompt

import (
	"fmt"
	"sync"
	"time"

	"github.com/qbox/codeagent/internal/workspace"
	"github.com/qbox/codeagent/pkg/models"
)

// Template 表示一个 Prompt 模板
type Template struct {
	ID          string                 `yaml:"id" json:"id"`
	Name        string                 `yaml:"name" json:"name"`
	Description string                 `yaml:"description" json:"description"`
	Content     string                 `yaml:"content" json:"content"`
	Variables   []TemplateVariable     `yaml:"variables" json:"variables"`
	Source      string                 `yaml:"source" json:"source"`     // "default" 或 "custom"
	Priority    int                    `yaml:"priority" json:"priority"` // 自定义模板优先级更高
	Metadata    map[string]interface{} `yaml:"metadata" json:"metadata"`
}

// TemplateVariable 表示模板变量
type TemplateVariable struct {
	Name        string `yaml:"name" json:"name"`
	Type        string `yaml:"type" json:"type"` // string, int, bool, array
	Required    bool   `yaml:"required" json:"required"`
	Default     string `yaml:"default" json:"default"`
	Description string `yaml:"description" json:"description"`
}

// TemplateCache 模板缓存
type TemplateCache struct {
	cache map[string]*Template
	mu    sync.RWMutex
	ttl   time.Duration
}

// Manager 统一管理内置和自定义模板
type Manager struct {
	defaultTemplates map[string]*Template
	customTemplates  map[string]*Template
	workspaceManager *workspace.Manager
	cache            *TemplateCache
	mu               sync.RWMutex
}

// NewManager 创建新的 Prompt Manager
func NewManager(workspaceManager *workspace.Manager) *Manager {
	pm := &Manager{
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
func (pm *Manager) loadDefaultTemplates() {
	// 基于 Issue 的代码生成模板
	issueTemplate := &Template{
		ID:          "issue_based_code_generation",
		Name:        "基于 Issue 的代码生成",
		Description: "根据 GitHub Issue 生成相应的代码实现",
		Source:      "default",
		Priority:    1,
		Content: `你是一个专业的程序员。请根据以下 Issue 需求生成代码实现，并提供一个简洁的 PR 描述。

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

{{if .has_custom_config}}
## 项目自定义配置参考
当前项目包含一个 AGENT.md 文件，其中定义了项目的特定要求和配置。
请在完成上述任务时，同步参考该文件中的内容，确保生成的代码符合项目的
技术栈、编码规范和架构要求。
{{end}}

## 请按照以下格式输出，保持简洁直接

## 改动摘要
简要说明改动内容

## 具体改动
- 列出修改的文件和具体变动

---

<details><summary>思考过程</summary>

[在这里记录你的分析思路、文件查找过程、设计决策等]

</details>
`,
		Variables: []TemplateVariable{
			{Name: "issue_title", Type: "string", Required: true, Description: "Issue 标题"},
			{Name: "issue_body", Type: "string", Required: true, Description: "Issue 描述"},
			{Name: "issue_labels", Type: "string", Required: false, Description: "Issue 标签"},
			{Name: "historical_context", Type: "string", Required: false, Description: "历史讨论内容"},
			{Name: "has_custom_config", Type: "bool", Required: false, Default: "false", Description: "是否存在 AGENT.md 自定义配置"},
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

	// 基于单个 Review Comment 的代码继续处理模板
	singleReviewContinueTemplate := &Template{
		ID:          "single_review_continue",
		Name:        "基于单个 Review Comment 的代码继续处理",
		Description: "根据单个 Review Comment 继续处理代码",
		Source:      "default",
		Priority:    1,
		Content: `根据以下代码行评论继续处理代码：

## 代码行评论
{{.comment_body}}

## 文件信息
文件：{{.file_path}}
{{.line_range_info}}

{{if .additional_instructions}}
## 额外指令
{{.additional_instructions}}
{{end}}

请根据评论要求继续处理代码，确保：
1. 理解评论的意图和要求
2. 进行相应的代码修改或改进
3. 保持代码质量和一致性
4. 遵循项目的编码规范

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
			{Name: "comment_body", Type: "string", Required: true, Description: "评论内容"},
			{Name: "file_path", Type: "string", Required: true, Description: "文件路径"},
			{Name: "line_range_info", Type: "string", Required: true, Description: "行号范围信息"},
			{Name: "additional_instructions", Type: "string", Required: false, Description: "额外指令"},
			{Name: "has_custom_config", Type: "bool", Required: false, Default: "false", Description: "是否存在自定义配置"},
		},
	}

	// 基于单个 Review Comment 的代码修复模板
	singleReviewFixTemplate := &Template{
		ID:          "single_review_fix",
		Name:        "基于单个 Review Comment 的代码修复",
		Description: "根据单个 Review Comment 修复代码问题",
		Source:      "default",
		Priority:    1,
		Content: `根据以下代码行评论修复代码问题：

## 代码行评论
{{.comment_body}}

## 文件信息
文件：{{.file_path}}
{{.line_range_info}}

{{if .additional_instructions}}
## 额外指令
{{.additional_instructions}}
{{end}}

请根据评论要求修复代码问题，确保：
1. 识别并解决评论中提到的问题
2. 修复代码错误或改进代码质量
3. 保持代码质量和一致性
4. 遵循项目的编码规范

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
			{Name: "comment_body", Type: "string", Required: true, Description: "评论内容"},
			{Name: "file_path", Type: "string", Required: true, Description: "文件路径"},
			{Name: "line_range_info", Type: "string", Required: true, Description: "行号范围信息"},
			{Name: "additional_instructions", Type: "string", Required: false, Description: "额外指令"},
			{Name: "has_custom_config", Type: "bool", Required: false, Default: "false", Description: "是否存在自定义配置"},
		},
	}

	// 基于批量 Review Comments 的代码处理模板
	batchReviewTemplate := &Template{
		ID:          "batch_review_processing",
		Name:        "基于批量 Review Comments 的代码处理",
		Description: "根据批量 Review Comments 处理代码",
		Source:      "default",
		Priority:    1,
		Content: `根据以下 PR Review 的批量评论处理代码：

{{if .review_body}}
## Review 总体说明
{{.review_body}}
{{end}}

## 批量评论
{{.batch_comments}}

{{if .additional_instructions}}
## 额外指令
{{.additional_instructions}}
{{end}}

{{if .processing_mode}}
## 处理模式
{{.processing_mode}}
{{end}}

请一次性处理所有评论中提到的问题，确保：
1. 理解每个评论的意图和要求
2. 进行相应的代码修改或改进
3. 保持代码质量和一致性
4. 遵循项目的编码规范
5. 回复要简洁明了

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
			{Name: "review_body", Type: "string", Required: false, Description: "Review 总体说明"},
			{Name: "batch_comments", Type: "string", Required: true, Description: "批量评论内容"},
			{Name: "additional_instructions", Type: "string", Required: false, Description: "额外指令"},
			{Name: "processing_mode", Type: "string", Required: false, Description: "处理模式（继续处理/修复）"},
			{Name: "has_custom_config", Type: "bool", Required: false, Default: "false", Description: "是否存在自定义配置"},
		},
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.defaultTemplates[issueTemplate.ID] = issueTemplate
	pm.defaultTemplates[reviewTemplate.ID] = reviewTemplate
	pm.defaultTemplates[singleReviewContinueTemplate.ID] = singleReviewContinueTemplate
	pm.defaultTemplates[singleReviewFixTemplate.ID] = singleReviewFixTemplate
	pm.defaultTemplates[batchReviewTemplate.ID] = batchReviewTemplate
}

// GetTemplate 获取模板（优先使用自定义模板，回退到默认模板）
func (pm *Manager) GetTemplate(templateID string, workspace *models.Workspace) (*Template, error) {
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
func (pm *Manager) LoadCustomTemplate(template *Template) error {
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
func (pm *Manager) ListTemplates() []*Template {
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
