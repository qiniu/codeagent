# CodeAgent Prompt 系统

CodeAgent v0.5 的 Prompt 系统重新设计，提供统一模板管理、智能配置注入和 AI 驱动验证功能。

## 架构概述

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

## 核心组件

### 1. Prompt Manager

统一管理内置和自定义模板，支持模板缓存和优先级控制。

**功能特性：**

- 内置模板管理（Issue 处理、Code Review）
- 自定义模板加载
- 模板缓存机制
- 优先级控制（自定义模板优先级更高）

**内置模板：**

- `issue_based_code_generation`: 基于 Issue 的代码生成
- `review_based_code_modification`: 基于 Code Review Comments 的代码修改

### 2. Custom Config Detector

自动检测仓库中的 `CODEAGENT.md` 文件，实现智能配置注入。

**功能特性：**

- 文件存在性检查
- 缓存机制
- 自动注入配置信息到模板变量

### 3. Prompt Builder

构建和渲染 Prompt，处理模板变量替换和条件渲染。

**功能特性：**

- 模板变量替换
- 条件渲染（基于 `has_custom_config` 等变量）
- 自动注入自定义配置信息
- 错误处理和回退机制

### 4. Output Validator

使用 AI 进行代码质量验证和输出格式检查。

**功能特性：**

- AI 驱动代码质量验证
- 自动修复代码错误
- 输出格式验证
- 质量分数计算

## 使用示例

### 基本使用

```go
// 创建 Prompt 系统组件
workspaceManager := &workspace.Manager{}
promptManager := prompt.NewPromptManager(workspaceManager)
customConfigDetector := prompt.NewCustomConfigDetector()
promptConfig := prompt.PromptConfig{MaxTotalLength: 8000}
promptBuilder := prompt.NewPromptBuilder(promptManager, customConfigDetector, promptConfig)

// 构建 Issue 处理 Prompt
req := &prompt.PromptRequest{
    TemplateID: "issue_based_code_generation",
    TemplateVars: map[string]interface{}{
        "issue_title":        "实现用户登录功能",
        "issue_body":         "需要实现用户登录接口，支持用户名密码登录",
        "include_tests":      true,
        "include_docs":       true,
    },
    Workspace: workspace,
}

result, err := promptBuilder.BuildPrompt(ctx, req)
if err != nil {
    log.Errorf("Failed to build prompt: %v", err)
    return
}

// 使用生成的 Prompt
fmt.Println(result.Content)
```

### 自定义配置检测

```go
// 检测仓库中是否存在 CODEAGENT.md 文件
configInfo, err := customConfigDetector.GetCODEAGENTFile(ctx, workspace)
if err != nil {
    log.Errorf("Failed to detect custom config: %v", err)
    return
}

if configInfo.Exists {
    // 存在自定义配置，会自动注入到模板变量中
    log.Info("Custom CODEAGENT.md file detected")
}
```

### 代码验证

```go
// 创建验证器
codeClient := code.New(workspace, config)
validator := prompt.NewOutputValidator(codeClient)

// 验证代码质量
validationResult, err := validator.ValidateAndFixCode(ctx, codeContent, "go")
if err != nil {
    log.Errorf("Failed to validate code: %v", err)
    return
}

if !validationResult.IsValid {
    log.Warnf("Code validation failed: %v", validationResult.Issues)
    // 使用修复后的代码
    fixedCode := validationResult.FixedContent
}
```

## 配置说明

### Prompt 配置

```yaml
prompt:
  templates_dir: "./config/templates" # 自定义模板目录
  validators_dir: "./config/validators" # 验证器配置目录
  max_length: 8000 # 最大 Prompt 长度
  enable_cache: true # 启用缓存
```

### CODEAGENT.md 文件格式

参考官方文档：

- [Claude.md 官方介绍](https://docs.anthropic.com/claude/docs/claude-md)
- [Gemini.md 官方介绍](https://ai.google.dev/docs/gemini_md)

`CODEAGENT.md` 文件格式与 `claude.md` 类似，用于定义项目的特定要求和配置。

## 模板变量

### Issue 模板变量

```go
type IssueTemplateVars struct {
    IssueTitle        string `json:"issue_title"`        // Issue 标题
    IssueBody         string `json:"issue_body"`         // Issue 描述
    IssueLabels       string `json:"issue_labels"`       // Issue 标签
    HistoricalContext string `json:"historical_context"` // 历史讨论内容
    IncludeTests      bool   `json:"include_tests"`      // 是否包含测试代码
    IncludeDocs       bool   `json:"include_docs"`       // 是否包含文档
    HasCustomConfig   bool   `json:"has_custom_config"`  // 是否存在自定义配置
}
```

### Review 模板变量

```go
type ReviewTemplateVars struct {
    ReviewComments    string `json:"review_comments"`    // Code Review Comments 内容
    CodeContext       string `json:"code_context"`       // 相关代码上下文
    HistoricalContext string `json:"historical_context"` // 历史讨论内容
    HasCustomConfig   bool   `json:"has_custom_config"`  // 是否存在自定义配置
}
```

## 测试

运行测试：

```bash
cd internal/prompt
go test -v
```

## 迁移指南

### 从旧版本迁移

1. **保持 API 兼容性**：现有的 `buildPrompt` 方法仍然可用，内部使用新的 Prompt 系统
2. **渐进式迁移**：可以逐步替换内部实现，不影响现有功能
3. **回退机制**：如果新系统失败，会自动回退到旧的 Prompt 构建方式

### 新功能启用

1. **自定义模板**：在 `config/templates` 目录下添加自定义模板文件
2. **项目配置**：在仓库根目录创建 `CODEAGENT.md` 文件
3. **验证功能**：启用 AI 驱动的代码质量验证

## 设计优势

1. **统一管理**：内置和自定义模板统一管理，简化架构
2. **智能注入**：自动检测项目配置并将信息注入到模板变量中
3. **简化设计**：移除复杂的上下文收集机制，直接使用模板变量
4. **AI 驱动验证**：完全依赖 AI 进行代码质量验证，无需手动实现复杂规则
5. **向后兼容**：保持现有 API 接口不变，渐进式迁移
