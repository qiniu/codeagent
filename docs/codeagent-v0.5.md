# CodeAgent v0.5 - Prompt 系统重新设计

## 概述

CodeAgent v0.5 版本将重新设计 Prompt 构建系统，以"专业程序员"为核心角色定位，专注于代码生成、代码审查和代码优化。新系统将解决当前版本中 Prompt 硬编码、缺乏灵活性、上下文管理简单等问题，提供更专业的代码处理能力。

## 当前问题分析

### 1. 硬编码问题

- Prompt 模板直接写在 Go 代码中，修改需要重新编译
- 缺乏模板复用和组合机制
- 无法动态调整 Prompt 策略

### 2. 上下文管理简单

- 只是简单的字符串拼接
- 缺乏上下文优先级和权重控制
- 无法控制上下文长度，可能导致 token 超限

### 3. 缺乏版本控制

- 没有 Prompt 版本管理机制
- 缺乏基于效果的优化机制

### 4. 输出格式验证不足

- 对 AI 输出格式错误的处理不够完善
- 缺乏自动修复机制
- 错误信息提取不够准确

### 5. 专业程序员角色定位不明确

- 缺乏专业的代码生成策略
- 代码审查标准不够严格
- 代码优化建议不够深入
- 缺乏编程最佳实践指导

## 当前项目实现分析

### 现有架构特点

1. **多 AI 模型支持**：支持 Claude 和 Gemini，包括本地和 Docker 模式
2. **交互式会话管理**：实现了 SessionManager 来管理 AI 会话
3. **工作空间管理**：支持多 PR 的并发处理
4. **简单的 Prompt 构建**：在 `agent.go` 中的 `buildPrompt` 函数
5. **代码处理能力**：具备基本的代码生成、修改和审查功能

### 当前 Prompt 构建方式

```go
// 当前实现（internal/agent/agent.go:482-535）
func (a *Agent) buildPrompt(mode string, args string, historicalContext string) string {
    var prompt string
    var taskDescription string
    var defaultTask string

    switch mode {
    case "Continue":
        taskDescription = "请根据上述PR描述、历史讨论和当前指令，进行相应的代码修改。"
        defaultTask = "继续处理PR，分析代码变更并改进"
    case "Fix":
        taskDescription = "请根据上述PR描述、历史讨论和当前指令，进行相应的代码修复。"
        defaultTask = "分析并修复代码问题"
    default:
        taskDescription = "请根据上述PR描述、历史讨论和当前指令，进行相应的代码处理。"
        defaultTask = "处理代码任务"
    }

    if args != "" {
        if historicalContext != "" {
            prompt = fmt.Sprintf(`作为PR代码审查助手，请基于以下完整上下文来%s：

%s

## 当前指令
%s

%s注意：
1. 当前指令是主要任务，历史信息仅作为上下文参考
2. 请确保修改符合PR的整体目标和已有的讨论共识
3. 如果发现与历史讨论有冲突，请优先执行当前指令并在回复中说明`,
                strings.ToLower(mode), historicalContext, args, taskDescription)
        } else {
            prompt = fmt.Sprintf("根据指令%s：\n\n%s", strings.ToLower(mode), args)
        }
    } else {
        if historicalContext != "" {
            prompt = fmt.Sprintf(`作为PR代码审查助手，请基于以下完整上下文来%s：

%s

## 任务
%s

请根据上述PR描述和历史讨论，进行相应的代码修改和改进。`,
                strings.ToLower(mode), historicalContext, defaultTask)
        } else {
            prompt = defaultTask
        }
    }

    return prompt
}
```

### 当前上下文构建方式

```go
// 当前实现（internal/agent/agent.go:1019-1080）
func (a *Agent) formatHistoricalComments(allComments *models.PRAllComments, currentCommentID int64) string {
    var contextParts []string

    // 添加 PR 描述
    if allComments.PRBody != "" {
        contextParts = append(contextParts, fmt.Sprintf("## PR 描述\n%s", allComments.PRBody))
    }

    // 添加历史的一般评论（排除当前评论）
    if len(allComments.IssueComments) > 0 {
        var historyComments []string
        for _, comment := range allComments.IssueComments {
            if comment.GetID() != currentCommentID {
                user := comment.GetUser().GetLogin()
                body := comment.GetBody()
                createdAt := comment.GetCreatedAt().Format("2006-01-02 15:04:05")
                historyComments = append(historyComments, fmt.Sprintf("**%s** (%s):\n%s", user, createdAt, body))
            }
        }
        if len(historyComments) > 0 {
            contextParts = append(contextParts, fmt.Sprintf("## 历史评论\n%s", strings.Join(historyComments, "\n\n")))
        }
    }

    // 添加代码行评论
    if len(allComments.ReviewComments) > 0 {
        var reviewComments []string
        for _, comment := range allComments.ReviewComments {
            if comment.GetID() != currentCommentID {
                user := comment.GetUser().GetLogin()
                body := comment.GetBody()
                path := comment.GetPath()
                line := comment.GetLine()
                createdAt := comment.GetCreatedAt().Format("2006-01-02 15:04:05")
                reviewComments = append(reviewComments, fmt.Sprintf("**%s** (%s) - %s:%d:\n%s", user, createdAt, path, line, body))
            }
        }
        if len(reviewComments) > 0 {
            contextParts = append(contextParts, fmt.Sprintf("## 代码行评论\n%s", strings.Join(reviewComments, "\n\n")))
        }
    }

    // 添加 Review 评论
    if len(allComments.Reviews) > 0 {
        var reviews []string
        for _, review := range allComments.Reviews {
            if review.GetBody() != "" {
                user := review.GetUser().GetLogin()
                body := review.GetBody()
                state := review.GetState()
                createdAt := review.GetSubmittedAt().Format("2006-01-02 15:04:05")
                reviews = append(reviews, fmt.Sprintf("**%s** (%s) - %s:\n%s", user, createdAt, state, body))
            }
        }
        if len(reviews) > 0 {
            contextParts = append(contextParts, fmt.Sprintf("## Review 评论\n%s", strings.Join(reviews, "\n\n")))
        }
    }

    return strings.Join(contextParts, "\n\n")
}
```

## 新架构设计

### 1. 整体架构

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Default       │    │   Custom        │    │   Prompt        │
│   Prompt        │    │   Prompt        │    │   Builder       │
│   Manager       │    │   Manager       │    │                 │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                       │                       │
         └───────────────────────┼───────────────────────┘
                                 │
         ┌─────────────────┐    │    ┌─────────────────┐
         │   Custom        │    │    │   Output        │
         │   Config        │    │    │   Validator     │
         │   Detector      │    │    │                 │
         └─────────────────┘    │    └─────────────────┘
                                 │
         ┌─────────────────┐    │    ┌─────────────────┐
         │   Error         │    │    │   Handler       │
         │   Handler       │    │    │                 │
         └─────────────────┘    │    └─────────────────┘
                                 │
         ┌─────────────────┐    │    ┌─────────────────┐
         │   Code          │    │    │   Quality       │
         │   Quality       │    │    │   Analyzer      │
         │   Analyzer      │    │    │                 │
         └─────────────────┘    │    └─────────────────┘
                                 ▼
                    ┌─────────────────┐
                    │   AI Provider   │
                    │   Interface     │
                    └─────────────────┘
```

### 2. 专业程序员角色定位

CodeAgent 作为专业程序员，具备以下核心能力：

1. **代码生成专家**

   - 根据需求生成高质量代码
   - 遵循编程最佳实践
   - 支持多种编程语言和框架
   - 生成可测试、可维护的代码

2. **代码审查专家**

   - 严格的代码质量标准
   - 安全性检查
   - 性能优化建议
   - 架构设计指导

3. **代码优化专家**

   - 性能瓶颈识别
   - 重构建议
   - 设计模式应用
   - 代码可读性提升

4. **问题诊断专家**
   - 错误分析和修复
   - 调试指导
   - 测试策略建议
   - 部署优化

### 3. 核心组件设计

#### 3.1 Prompt 管理器 (Prompt Manager)

**功能职责：**

- 管理 Prompt 模板（内置 + 自定义）
- 支持基于 Issue 和 Code Review Comments 的代码生成
- 自动检测和加载 `CODEAGENT.md` 中的自定义模板
- 提供统一的模板管理接口

**设计结构：**

```go
type PromptManager struct {
    defaultTemplates map[string]*Template
    customTemplates  map[string]*Template
    workspaceManager *workspace.Manager
    cache            TemplateCache
}

type Template struct {
    ID          string                 `yaml:"id"`
    Name        string                 `yaml:"name"`
    Description string                 `yaml:"description"`
    Content     string                 `yaml:"content"`
    Variables   []TemplateVariable     `yaml:"variables"`
    Source      string                 `yaml:"source"` // "default" 或 "custom"
    Priority    int                    `yaml:"priority"` // 自定义模板优先级更高
    Metadata    map[string]interface{} `yaml:"metadata"`
}

type TemplateVariable struct {
    Name        string `yaml:"name"`
    Type        string `yaml:"type"` // string, int, bool, array
    Required    bool   `yaml:"required"`
    Default     string `yaml:"default"`
    Description string `yaml:"description"`
}
```

**模板示例：**

```yaml
# 内置模板：基于 Issue 的代码生成
id: issue_based_code_generation
name: "基于 Issue 的代码生成"
description: "根据 GitHub Issue 生成相应的代码实现"
source: "default"
priority: 1
content: |
  根据以下 Issue 需求生成高质量的代码：

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
  {{end}}

variables:
  - name: issue_title
    type: string
    required: true
    description: "Issue 标题"

  - name: issue_body
    type: string
    required: true
    description: "Issue 描述"

  - name: issue_labels
    type: string
    required: false
    description: "Issue 标签"

  - name: historical_context
    type: string
    required: false
    description: "历史讨论内容"

  - name: include_tests
    type: bool
    required: false
    default: "true"
    description: "是否包含测试代码"

  - name: include_docs
    type: bool
    required: false
    default: "true"
    description: "是否包含文档"

  - name: has_custom_config
    type: bool
    required: false
    default: "false"
    description: "是否存在自定义配置"
```

````yaml
# 内置模板：基于 Code Review Comments 的代码修改
id: review_based_code_modification
name: "基于 Code Review Comments 的代码修改"
description: "根据 Code Review Comments 修改代码"
source: "default"
priority: 1
content: |
  根据以下 Code Review Comments 修改代码：

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
  {{end}}

variables:
  - name: review_comments
    type: string
    required: true
    description: "Code Review Comments 内容"

  - name: code_context
    type: string
    required: false
    description: "相关代码上下文"

  - name: historical_context
    type: string
    required: false
    description: "历史讨论内容"

#### 3.2 自定义配置检测器 (Custom Config Detector)

**功能职责：**

- 检测仓库中是否存在 `CODEAGENT.md` 文件
- 读取文件信息（路径、大小、修改时间）
- 提供简单的文件存在性检查和信息获取
- 无需解析文件内容，直接让 AI 工具读取

**设计结构：**

```go
type CustomConfigDetector struct {
    workspaceManager *workspace.Manager
    cache            ConfigCache
}

type CustomConfigInfo struct {
    Exists bool `json:"exists"`
}

type ConfigCache struct {
    cache map[string]*CustomConfigInfo
    mutex sync.RWMutex
    ttl   time.Duration
}
```

**CODEAGENT.md 文件格式：**

参考官方文档：
- [Claude.md 官方介绍](https://docs.anthropic.com/claude/docs/claude-md)
- [Gemini.md 官方介绍](https://ai.google.dev/docs/gemini_md)

`CODEAGENT.md` 文件格式与 `claude.md` 类似，用于定义项目的特定要求和配置。

#### 3.3 简化的上下文处理

**设计理念：**

既然我们已经明确定义了具体的 Prompt 模板（`issue_based_code_generation` 和 `review_based_code_modification`），并且这些模板的变量都是明确的，因此不需要复杂的上下文收集机制。所有必要的上下文信息都通过模板变量直接传入。

**模板变量直接传入：**

```go
// 基于 Issue 的代码生成模板变量
type IssueTemplateVars struct {
    IssueTitle        string `json:"issue_title"`
    IssueBody         string `json:"issue_body"`
    IssueLabels       string `json:"issue_labels,omitempty"`
    HistoricalContext string `json:"historical_context,omitempty"`
    IncludeTests      bool   `json:"include_tests"`
    IncludeDocs       bool   `json:"include_docs"`
    HasCustomConfig   bool   `json:"has_custom_config"`
}

// 基于 Code Review Comments 的代码修改模板变量
type ReviewTemplateVars struct {
    ReviewComments    string `json:"review_comments"`
    CodeContext       string `json:"code_context,omitempty"`
    HistoricalContext string `json:"historical_context,omitempty"`
    HasCustomConfig   bool   `json:"has_custom_config"`
}
````

#### 3.4 Prompt 构建器 (Prompt Builder)

**功能职责：**

- 统一管理 Prompt 模板（内置 + 自定义）
- 处理模板变量替换
- 自动检测和注入自定义配置信息
- 简化构建流程，直接使用模板变量

**设计结构：**

```go
type PromptBuilder struct {
    promptManager        *PromptManager
    customConfigDetector *CustomConfigDetector
    config               PromptConfig
}

type PromptConfig struct {
    MaxTotalLength int `yaml:"max_total_length"`
}

type PromptRequest struct {
    TemplateID   string                 `json:"template_id"`
    TemplateVars map[string]interface{} `json:"template_vars"`
    Workspace    *models.Workspace      `json:"workspace"`
}

type PromptResult struct {
    Content      string                 `json:"content"`
    TemplateID   string                 `json:"template_id"`
    TemplateType string                 `json:"template_type"` // "default" 或 "default_with_custom"
    Length       int                    `json:"length"`
    Metadata     map[string]interface{} `json:"metadata"`
}
```

**简化的构建流程：**

```go
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

// 获取模板类型
func getTemplateType(template *Template, customConfig *CustomConfigInfo) string {
    if customConfig != nil && customConfig.Exists {
        return "default_with_custom"
    }
    return "default"
}
```

#### 3.5 输出验证器 (Output Validator)

**功能职责：**

- 使用 AI 验证代码质量和格式
- 自动修复代码错误和格式问题
- 提取代码结构和关键信息
- 提供代码优化建议和最佳实践指导

**实现策略：**

- **AI 驱动验证**：完全依赖 AI 模型进行代码质量验证，无需手动实现复杂规则
- **智能修复**：AI 自动修复代码错误、格式问题和性能问题
- **专业指导**：提供编程最佳实践和架构设计建议

## 总结

CodeAgent v0.5 的 Prompt 系统重新设计将显著提升系统的灵活性、可维护性和用户体验。通过"统一模板管理 + 智能配置注入"的设计，既保证了开箱即用的便利性，又提供了灵活的自定义能力。

**核心创新**：

- **统一管理**：将内置和自定义模板统一管理，简化架构
- **智能注入**：自动检测 `CODEAGENT.md` 文件并将配置信息注入到模板变量中
- **条件渲染**：通过模板变量控制是否显示自定义配置引用部分
- **简化设计**：移除复杂的上下文收集和 A/B 测试机制，专注于核心功能

**简化后的设计优势**：

1. **更清晰的架构**：移除了复杂的上下文收集机制，直接使用模板变量
2. **更简单的实现**：不需要实现各种 ContextProvider 和 A/B 测试引擎
3. **更直接的调用方式**：模板变量直接传入，逻辑更直观
4. **更快的开发速度**：减少了不必要的开发时间，降低维护成本
5. **保持核心功能**：仍然支持自定义配置和条件渲染

新架构采用模块化设计，各组件职责清晰，便于后续扩展和维护。分阶段实施计划确保系统稳定性和兼容性，同时为团队提供充足的学习和适应时间。

**实现重点**：

- **统一模板管理**：内置和自定义模板统一管理
- **智能配置检测**：自动检测 `CODEAGENT.md` 文件
- **简化变量处理**：直接使用模板变量，无需复杂上下文收集
- **AI 驱动验证**：完全依赖 AI 进行代码质量验证

这个简化设计既保持了核心功能，又大大降低了实现复杂度，是一个非常实用的改进！
