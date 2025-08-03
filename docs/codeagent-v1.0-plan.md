# CodeAgent 1.0 版本升级计划

## 📋 项目概述

CodeAgent 1.0 是一个重大版本升级，旨在将现有的基础代码生成系统提升到与 [claude-code-action](https://github.com/anthropics/claude-code-action) 同等水平的智能代码协作平台。

### 升级目标

通过全面重构和功能增强，实现：
- **丰富的上下文理解能力** - 深度解析 GitHub 事件和历史信息
- **智能分支管理策略** - 基于事件类型的自动分支处理
- **实时用户交互体验** - 动态进度反馈和评论更新
- **事件驱动的 Prompt 系统** - 灵活的模板化内容生成
- **完善的权限控制机制** - 细粒度的安全管控
- **多模态内容处理** - 支持图片等多媒体内容

## 🎯 核心功能对比

| 功能维度 | 当前版本 (v0.x) | 目标版本 (v1.0) | claude-code-action |
|---------|----------------|-----------------|-------------------|
| 上下文理解 | 基础 Issue/PR 信息 | 完整历史+文件变更+图片 | ✅ 完整上下文 |
| 分支处理 | 简单创建分支 | 智能分支策略 | ✅ 智能处理 |
| 用户交互 | 基础命令响应 | 实时进度+动态评论 | ✅ 丰富交互 |
| Prompt 系统 | 静态模板 | 事件驱动+变量解析 | ✅ 动态生成 |
| 权限控制 | 基本权限 | 细粒度控制 | ✅ 完善控制 |
| 多模态 | 不支持 | 图片处理 | ✅ 多模态 |

## 🏗️ 架构升级规划

### 现有架构分析

```
当前架构 (v0.x):
GitHub Webhook → Agent → Prompt Builder → AI Provider → Git Operations
     ↓              ↓           ↓             ↓            ↓
  基础事件处理   简单编排   静态模板生成   Claude/Gemini   基础分支操作
```

### 目标架构 (v1.0)

```
升级后架构 (v1.0):
GitHub Webhook → Event Analyzer → Context Enricher → Prompt System → AI Provider → Smart Git Manager
     ↓               ↓              ↓               ↓            ↓             ↓
  复杂事件解析    智能事件分析    丰富上下文收集   动态内容生成   多AI支持    智能分支策略
                     ↓              ↓               ↓                          ↓
              Branch Strategy   Media Processor   Variable Resolver        Progress Tracker
              User Interaction  Security Manager                           Comment Manager
```

## 📊 详细改进方案

### 1. 上下文增强系统

#### 目标
将简单的 Issue/PR 信息获取升级为完整的 GitHub 上下文理解系统。

#### 核心组件
```go
// pkg/models/context.go
type GitHubContext struct {
    Event           *GitHubEvent          // 触发事件
    Issue           *EnrichedIssue        // 丰富的 Issue 信息
    PullRequest     *EnrichedPullRequest  // 完整的 PR 信息
    Comments        []*ProcessedComment   // 处理后的评论
    ReviewComments  []*ProcessedReviewComment // Review 评论
    Reviews         []*ProcessedReview    // PR Review
    ChangedFiles    []*ChangedFileInfo    // 文件变更详情
    Images          []*DownloadedImage    // 下载的图片
    TriggerContext  *TriggerInfo          // 触发上下文
    Timeline        []*TimelineEvent      // 时间线事件
}

type EnrichedPullRequest struct {
    *github.PullRequest
    ChangedFiles    []*github.CommitFile
    FileContents    map[string]string     // 文件内容缓存
    Commits         []*github.RepositoryCommit
    ConflictInfo    *ConflictInfo         // 冲突信息
}

type ProcessedComment struct {
    *github.IssueComment
    ProcessedBody   string               // 处理后的内容
    Images         []*DownloadedImage    // 提取的图片
    Mentions       []string              // @提及
    CodeBlocks     []*CodeBlock          // 代码块
    Commands       []*Command            // 识别的命令
}
```

#### 实施步骤
1. **GitHub API 增强** (`internal/github/fetcher.go`)
   - 批量获取评论、Review、文件变更
   - 实现分页和缓存机制
   - 添加错误重试和限流控制

2. **内容处理器** (`internal/context/processor.go`)
   - Markdown 解析和代码块提取
   - @提及和命令识别
   - 时间线事件排序

3. **图片处理模块** (`internal/media/processor.go`)
   - 从评论中提取图片 URL
   - 下载并保存到本地
   - 生成可访问的文件路径

### 2. 智能分支处理机制

#### 目标
基于不同的 GitHub 事件类型，实现智能的分支创建和管理策略。

#### 核心组件
```go
// internal/branch/strategy.go
type BranchStrategy interface {
    ShouldCreateNewBranch(ctx *GitHubContext) bool
    GetTargetBranch(ctx *GitHubContext) string
    GetBaseBranch(ctx *GitHubContext) string
    GetBranchName(ctx *GitHubContext) string
    GetCommitStrategy() CommitStrategy
}

type SmartBranchManager struct {
    strategies map[EventType]BranchStrategy
    gitOps     *GitOperations
}

// 不同事件的分支策略
type IssueBranchStrategy struct{}        // Issue: 创建 feature/issue-{number} 分支
type OpenPRBranchStrategy struct{}       // 开放 PR: 推送到现有分支
type ClosedPRBranchStrategy struct{}     // 已关闭 PR: 创建新分支
type ReviewBranchStrategy struct{}       // Review: 推送到 PR 分支
```

#### 实施步骤
1. **事件类型识别** (`internal/event/detector.go`)
   - 解析 webhook 事件类型和动作
   - 识别触发条件和上下文

2. **分支策略实现** (`internal/branch/strategies/`)
   - 为每种事件类型实现具体策略
   - 处理边缘情况和错误场景

3. **Git 操作优化** (`internal/git/operations.go`)
   - 原子性的分支操作
   - 冲突检测和处理
   - 分支状态跟踪

### 3. 用户交互和进度反馈系统

#### 目标
提供实时的任务执行反馈，让用户了解代码生成的进度和状态。

#### 核心组件
```go
// internal/interaction/manager.go
type InteractionManager struct {
    github          *ghclient.Client
    commentID       int64
    progressTracker *ProgressTracker
    spinnerActive   bool
}

type ProgressTracker struct {
    Tasks       []*Task           // 任务列表
    CurrentTask *Task             // 当前任务
    StartTime   time.Time         // 开始时间
    Status      ExecutionStatus   // 执行状态
}

type Task struct {
    ID          string
    Description string
    Status      TaskStatus        // pending, in_progress, completed, failed
    StartTime   time.Time
    Duration    time.Duration
    SubTasks    []*Task
    Error       error
}

// 核心方法
func (im *InteractionManager) CreateInitialComment() error
func (im *InteractionManager) UpdateProgress(taskID string, status TaskStatus) error
func (im *InteractionManager) AddTask(task *Task) error
func (im *InteractionManager) ShowSpinner(message string) error
func (im *InteractionManager) HideSpinner() error
func (im *InteractionManager) ReportError(err error) error
func (im *InteractionManager) FinalizeComment(summary string) error
```

#### 实施步骤
1. **评论管理器** (`internal/interaction/comment_manager.go`)
   - GitHub 评论的 CRUD 操作
   - Markdown 格式化
   - 实时更新机制

2. **进度追踪器** (`internal/interaction/progress_tracker.go`)
   - 任务状态管理
   - TodoList 格式输出
   - 时间统计和性能监控

3. **用户界面** (`internal/interaction/ui.go`)
   - Spinner 动画显示
   - 错误信息格式化
   - 成功/失败状态展示

### 4. 模板化 Prompt 系统重构

#### 目标
从静态模板系统升级为事件驱动的动态 Prompt 生成系统。

#### 核心组件
```go
// internal/prompt/v2/system.go
type PromptSystem struct {
    templateEngine   *TemplateEngine
    contextProvider  *ContextProvider
    variableResolver *VariableResolver
    enrichers        []ContextEnricher
}

type TemplateEngine struct {
    templates       map[EventType]map[string]*Template
    fallbacks       map[EventType]*Template
    customTemplates map[string]*Template
}

type VariableResolver struct {
    resolvers map[string]VariableResolverFunc
}

// 事件驱动的模板选择
func (ps *PromptSystem) GeneratePrompt(ctx *GitHubContext) (*GeneratedPrompt, error) {
    // 1. 根据事件类型和动作选择模板
    template := ps.selectTemplate(ctx.Event.Type, ctx.Event.Action)
    
    // 2. 收集和丰富上下文
    enrichedCtx := ps.contextProvider.Enrich(ctx)
    
    // 3. 解析模板变量
    variables := ps.variableResolver.Resolve(enrichedCtx)
    
    // 4. 应用上下文丰富器
    for _, enricher := range ps.enrichers {
        enrichedCtx = enricher.Enrich(enrichedCtx)
    }
    
    // 5. 渲染最终 Prompt
    return ps.templateEngine.Render(template, variables, enrichedCtx)
}
```

#### 模板变量系统
```go
// 支持的变量类型
var SupportedVariables = map[string]string{
    "$REPOSITORY":        "GitHub 仓库名称",
    "$PR_NUMBER":         "Pull Request 编号", 
    "$ISSUE_NUMBER":      "Issue 编号",
    "$PR_TITLE":          "Pull Request 标题",
    "$ISSUE_TITLE":       "Issue 标题",
    "$PR_BODY":           "Pull Request 描述",
    "$ISSUE_BODY":        "Issue 描述",
    "$PR_COMMENTS":       "Pull Request 评论",
    "$ISSUE_COMMENTS":    "Issue 评论",
    "$REVIEW_COMMENTS":   "Review 评论",
    "$CHANGED_FILES":     "变更文件列表",
    "$TRIGGER_COMMENT":   "触发评论内容",
    "$TRIGGER_USERNAME":  "触发用户名",
    "$BRANCH_NAME":       "分支名称",
    "$BASE_BRANCH":       "基础分支",
    "$EVENT_TYPE":        "事件类型",
    "$IS_PR":            "是否为 PR",
    "$COMMIT_SHA":       "提交 SHA",
    "$TIMESTAMP":        "时间戳",
}
```

#### 实施步骤
1. **事件类型枚举** (`pkg/models/events.go`)
   - 定义所有支持的 GitHub 事件类型
   - 事件动作和状态映射

2. **模板引擎重构** (`internal/prompt/v2/engine.go`)
   - 支持条件渲染和循环
   - 模板继承和组合
   - 错误处理和调试

3. **变量解析器** (`internal/prompt/v2/variables.go`)
   - 动态变量计算
   - 类型安全的变量替换
   - 自定义函数支持

4. **上下文丰富器** (`internal/prompt/v2/enrichers/`)
   - 代码分析丰富器
   - 历史信息丰富器
   - 项目配置丰富器

### 5. 权限和安全控制增强

#### 目标
实现细粒度的权限控制，确保系统安全运行。

#### 核心组件
```go
// internal/security/manager.go
type SecurityManager struct {
    toolPolicies     map[string]*ToolPolicy
    userPermissions  map[string]*UserPermissions
    auditLogger      *AuditLogger
}

type ToolPolicy struct {
    AllowedTools     []string
    DisallowedTools  []string
    Conditions       []*Condition
    RateLimits       map[string]*RateLimit
}

type UserPermissions struct {
    CanExecuteCode    bool
    CanModifyFiles    bool
    CanAccessSecrets  bool
    MaxExecutionTime  time.Duration
    AllowedPaths      []string
    DeniedPaths       []string
}

type Condition struct {
    Type     ConditionType  // user, repo, time, file_pattern
    Operator string         // equals, contains, matches
    Value    string
}
```

#### 实施步骤
1. **权限模型设计** (`internal/security/permissions.go`)
2. **策略引擎** (`internal/security/policy_engine.go`)
3. **审计日志** (`internal/security/audit.go`)

### 6. 多模态支持

#### 目标
支持处理 GitHub 评论中的图片等多媒体内容。

#### 核心组件
```go
// internal/media/processor.go
type MediaProcessor struct {
    downloader  *ImageDownloader
    converter   *ImageConverter
    storage     *MediaStorage
    cache       *MediaCache
}

type ImageDownloader struct {
    client      *http.Client
    maxSize     int64
    allowedTypes []string
}

type DownloadedImage struct {
    OriginalURL  string
    LocalPath    string
    ContentType  string
    Size         int64
    Width        int
    Height       int
    DownloadedAt time.Time
}

func (mp *MediaProcessor) ProcessComment(comment *github.IssueComment) (*ProcessedComment, error)
func (mp *MediaProcessor) DownloadImages(urls []string) ([]*DownloadedImage, error)
func (mp *MediaProcessor) ConvertToLocalPath(githubURL string) (string, error)
```

#### 实施步骤
1. **图片提取器** (`internal/media/extractor.go`)
2. **下载管理器** (`internal/media/downloader.go`)
3. **存储管理器** (`internal/media/storage.go`)

## 🗺️ 实施路线图

### 第一阶段：核心功能增强 (Week 1-6)

#### Week 1-2: 上下文增强系统
```
□ GitHub API 客户端增强
  ├── 批量数据获取接口
  ├── 分页处理机制
  └── 缓存和限流控制

□ 内容处理器开发
  ├── Markdown 解析器
  ├── 代码块提取器
  └── 命令识别器

□ 基础图片下载功能
  ├── URL 提取
  ├── 文件下载
  └── 本地存储
```

#### Week 3-4: 智能分支处理
```
□ 事件类型系统
  ├── 事件类型枚举
  ├── 事件检测器
  └── 上下文解析器

□ 分支策略实现
  ├── Issue 分支策略
  ├── PR 分支策略
  └── Review 分支策略

□ Git 操作优化
  ├── 原子性操作
  ├── 冲突处理
  └── 状态跟踪
```

#### Week 5-6: 用户交互系统
```
□ 进度追踪器
  ├── 任务模型定义
  ├── 状态管理逻辑
  └── 时间统计功能

□ 评论管理器
  ├── GitHub API 集成
  ├── Markdown 格式化
  └── 实时更新机制

□ 用户界面组件
  ├── Spinner 动画
  ├── 错误展示
  └── 成功状态显示
```

### 第二阶段：高级功能 (Week 7-10)

#### Week 7-8: Prompt 系统重构
```
□ 模板引擎重构
  ├── 事件驱动模板选择
  ├── 条件渲染支持
  └── 模板继承机制

□ 变量解析器
  ├── 动态变量计算
  ├── 类型安全替换
  └── 自定义函数支持

□ 上下文丰富器
  ├── 代码分析丰富器
  ├── 历史信息丰富器
  └── 项目配置丰富器
```

#### Week 9-10: 权限控制和多模态
```
□ 安全管理器
  ├── 权限模型设计
  ├── 策略引擎开发
  └── 审计日志实现

□ 完整图片处理
  ├── 多格式支持
  ├── 压缩和优化
  └── 缓存管理

□ 工具权限控制
  ├── 细粒度权限
  ├── 条件策略
  └── 实时验证
```

### 第三阶段：集成测试和优化 (Week 11-12)

#### Week 11: 集成测试
```
□ 端到端测试
  ├── 完整工作流测试
  ├── 错误场景测试
  └── 性能压力测试

□ 兼容性测试
  ├── 不同 GitHub 事件测试
  ├── 多种 AI 模型测试
  └── 边缘情况处理测试
```

#### Week 12: 优化和发布
```
□ 性能优化
  ├── 内存使用优化
  ├── 并发处理优化
  └── 网络请求优化

□ 文档完善
  ├── API 文档更新
  ├── 部署指南编写
  └── 最佳实践文档

□ 发布准备
  ├── 版本标签管理
  ├── 发布说明编写
  └── 向后兼容性检查
```

## 📈 预期成果

### 功能对标
完成升级后，CodeAgent 1.0 将在以下维度达到或超越 claude-code-action：

| 功能 | CodeAgent 1.0 | claude-code-action | 状态 |
|------|---------------|-------------------|------|
| 丰富上下文理解 | ✅ | ✅ | ✅ 对标 |
| 智能分支管理 | ✅ | ✅ | ✅ 对标 |
| 实时进度反馈 | ✅ | ✅ | ✅ 对标 |
| 事件驱动 Prompt | ✅ | ✅ | ✅ 对标 |
| 细粒度权限控制 | ✅ | ✅ | ✅ 对标 |
| 多模态处理 | ✅ | ✅ | ✅ 对标 |
| 多 AI 模型支持 | ✅ | ❌ | 🚀 超越 |
| 本地化部署 | ✅ | ❌ | 🚀 超越 |

### 性能指标
- **响应时间**: 平均响应时间 < 30s
- **成功率**: 任务执行成功率 > 95%
- **并发能力**: 支持 100+ 并发请求
- **资源使用**: 内存使用 < 2GB，CPU 使用 < 80%

### 用户体验
- **实时反馈**: 用户可以实时查看任务执行进度
- **错误友好**: 清晰的错误信息和建议解决方案
- **智能交互**: 基于上下文的智能代码生成和修改

## 🔄 版本兼容性

### 向前兼容
- 保持现有 API 接口不变
- 支持旧版配置文件格式
- 提供平滑的迁移路径

### 配置迁移
```yaml
# v0.x 配置（仍然支持）
code_provider: claude
use_docker: false

# v1.0 新增配置
context:
  max_comments: 100
  include_images: true
  include_files: true

interaction:
  enable_progress_tracking: true
  update_frequency: 5s

security:
  allowed_tools: ["Edit", "Read", "Write"]
  max_execution_time: 300s
```

## 📋 质量保证

### 测试策略
- **单元测试覆盖率**: > 80%
- **集成测试**: 覆盖所有核心工作流
- **性能测试**: 压力测试和内存泄漏检测
- **安全测试**: 权限控制和输入验证测试

### 发布流程
1. **Alpha 版本**: 内部测试和核心功能验证
2. **Beta 版本**: 社区测试和反馈收集
3. **RC 版本**: 发布候选版本，最终稳定性测试
4. **正式版本**: v1.0.0 正式发布

---

**项目负责人**: CodeAgent 开发团队  
**预计完成时间**: 12 周  
**文档版本**: 1.0  
**最后更新**: 2025-01-26