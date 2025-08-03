# CodeAgent 1.0 架构设计文档

## 📋 文档概述

本文档详细描述了 CodeAgent 1.0 的系统架构设计，包括核心组件、模块交互、数据流和技术决策。

## 🏗️ 整体架构

### 系统架构图

```
┌─────────────────────────────────────────────────────────────────┐
│                        CodeAgent 1.0 架构                       │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌─────────────┐    ┌──────────────┐    ┌─────────────────┐    │
│  │   GitHub    │────│   Webhook    │────│   Event Router  │    │
│  │   Events    │    │   Handler    │    │                 │    │
│  └─────────────┘    └──────────────┘    └─────────────────┘    │
│                                                   │              │
│                                                   ▼              │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │                 Core Engine                             │    │
│  │  ┌─────────────┐ ┌─────────────┐ ┌─────────────────┐  │    │
│  │  │   Context   │ │   Branch    │ │   Interaction   │  │    │
│  │  │  Enricher   │ │  Strategy   │ │    Manager      │  │    │
│  │  └─────────────┘ └─────────────┘ └─────────────────┘  │    │
│  │  ┌─────────────┐ ┌─────────────┐ ┌─────────────────┐  │    │
│  │  │   Prompt    │ │  Security   │ │     Media       │  │    │
│  │  │   System    │ │   Manager   │ │   Processor     │  │    │
│  │  └─────────────┘ └─────────────┘ └─────────────────┘  │    │
│  └─────────────────────────────────────────────────────────┘    │
│                                   │                              │
│                                   ▼                              │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │               AI Provider Layer                         │    │
│  │  ┌─────────────┐ ┌─────────────┐ ┌─────────────────┐  │    │
│  │  │   Claude    │ │   Gemini    │ │    Session      │  │    │
│  │  │  Provider   │ │  Provider   │ │    Manager      │  │    │
│  │  └─────────────┘ └─────────────┘ └─────────────────┘  │    │
│  └─────────────────────────────────────────────────────────┘    │
│                                   │                              │
│                                   ▼                              │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │              Git Operations Layer                       │    │
│  │  ┌─────────────┐ ┌─────────────┐ ┌─────────────────┐  │    │
│  │  │ Workspace   │ │  Git Ops    │ │   Branch        │  │    │
│  │  │  Manager    │ │  Manager    │ │   Manager       │  │    │
│  │  └─────────────┘ └─────────────┘ └─────────────────┘  │    │
│  └─────────────────────────────────────────────────────────┘    │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### 架构特点

- **事件驱动**: 基于 GitHub Webhook 事件的响应式架构
- **模块化设计**: 各功能模块松耦合，易于扩展和维护
- **智能路由**: 根据事件类型智能选择处理策略
- **多层抽象**: 清晰的分层架构，职责分离
- **可扩展性**: 支持新的 AI 提供商和功能模块

## 🔧 核心组件详设

### 1. Event Router（事件路由器）

#### 职责
- 接收和解析 GitHub Webhook 事件
- 根据事件类型路由到相应的处理器
- 事件验证和安全检查

#### 接口设计
```go
// internal/event/router.go
type EventRouter interface {
    Route(ctx context.Context, event *GitHubEvent) (*RouteResult, error)
    RegisterHandler(eventType EventType, handler EventHandler) error
    GetSupportedEvents() []EventType
}

type RouteResult struct {
    Handler     EventHandler
    Context     *EventContext
    Strategy    ProcessingStrategy
}

type EventHandler interface {
    CanHandle(event *GitHubEvent) bool
    Handle(ctx context.Context, event *GitHubEvent) (*HandlerResult, error)
    GetPriority() int
}
```

#### 实现组件
- **WebhookValidator**: Webhook 签名验证
- **EventParser**: 事件类型解析和规范化
- **RouteSelector**: 路由选择算法
- **HandlerRegistry**: 处理器注册管理

### 2. Context Enricher（上下文丰富器）

#### 职责
- 收集完整的 GitHub 上下文信息
- 处理评论、文件、图片等多媒体内容
- 构建结构化的上下文数据

#### 架构设计
```go
// internal/context/enricher.go
type ContextEnricher interface {
    Enrich(ctx context.Context, event *GitHubEvent) (*EnrichedContext, error)
    GetRequiredPermissions() []string
}

type EnrichedContext struct {
    Event           *GitHubEvent
    Repository      *RepositoryInfo
    Issue           *EnrichedIssue
    PullRequest     *EnrichedPullRequest
    Comments        []*ProcessedComment
    ReviewComments  []*ProcessedReviewComment
    Reviews         []*ProcessedReview
    ChangedFiles    []*ChangedFileInfo
    Images          []*DownloadedImage
    Timeline        []*TimelineEvent
    Metadata        map[string]interface{}
}
```

#### 丰富器链
```
GitHubDataFetcher → CommentProcessor → MediaProcessor → FileAnalyzer → TimelineBuilder
       ↓                 ↓               ↓              ↓             ↓
   基础数据获取        评论处理        图片下载       文件分析      时间线构建
```

#### 核心子组件

**GitHubDataFetcher**
```go
type GitHubDataFetcher struct {
    client      *github.Client
    cache       *DataCache
    rateLimiter *RateLimiter
}

// 批量数据获取接口
func (f *GitHubDataFetcher) FetchIssueData(ctx context.Context, issue *github.Issue) (*EnrichedIssue, error)
func (f *GitHubDataFetcher) FetchPRData(ctx context.Context, pr *github.PullRequest) (*EnrichedPullRequest, error)
func (f *GitHubDataFetcher) FetchComments(ctx context.Context, number int) ([]*github.IssueComment, error)
func (f *GitHubDataFetcher) FetchReviewComments(ctx context.Context, number int) ([]*github.PullRequestComment, error)
func (f *GitHubDataFetcher) FetchChangedFiles(ctx context.Context, number int) ([]*github.CommitFile, error)
```

**CommentProcessor**
```go
type CommentProcessor struct {
    markdownParser *MarkdownParser
    codeExtractor  *CodeExtractor
    commandParser  *CommandParser
}

func (cp *CommentProcessor) ProcessComment(comment *github.IssueComment) (*ProcessedComment, error)
func (cp *CommentProcessor) ExtractCodeBlocks(content string) ([]*CodeBlock, error)
func (cp *CommentProcessor) ParseCommands(content string) ([]*Command, error)
func (cp *CommentProcessor) ExtractMentions(content string) ([]string, error)
```

**MediaProcessor**
```go
type MediaProcessor struct {
    downloader    *ImageDownloader
    storage       *LocalStorage
    transformer   *ImageTransformer
}

func (mp *MediaProcessor) ProcessImages(content string) ([]*DownloadedImage, string, error)
func (mp *MediaProcessor) DownloadImage(url string) (*DownloadedImage, error)
func (mp *MediaProcessor) ConvertToLocalPath(githubURL string) (string, error)
```

### 3. Branch Strategy（分支策略）

#### 职责
- 根据事件类型决定分支操作策略
- 管理分支创建、切换和合并逻辑
- 处理冲突和异常情况

#### 策略模式设计
```go
// internal/branch/strategy.go
type BranchStrategy interface {
    ShouldCreateNewBranch(ctx *EnrichedContext) bool
    GetTargetBranch(ctx *EnrichedContext) string
    GetBaseBranch(ctx *EnrichedContext) string
    GenerateBranchName(ctx *EnrichedContext) string
    GetCommitStrategy() CommitStrategy
    HandleConflicts(conflicts []*GitConflict) (*ConflictResolution, error)
}

type BranchManager struct {
    strategies map[EventType]BranchStrategy
    gitOps     *GitOperations
    workspace  *WorkspaceManager
}
```

#### 具体策略实现

**IssueStrategy**
```go
type IssueStrategy struct {
    branchPrefix string
    baseBranch   string
}

func (s *IssueStrategy) ShouldCreateNewBranch(ctx *EnrichedContext) bool {
    return true // Issue 总是创建新分支
}

func (s *IssueStrategy) GenerateBranchName(ctx *EnrichedContext) string {
    return fmt.Sprintf("feature/issue-%d", ctx.Issue.GetNumber())
}
```

**PullRequestStrategy**
```go
type PullRequestStrategy struct {
    allowDirectPush bool
}

func (s *PullRequestStrategy) ShouldCreateNewBranch(ctx *EnrichedContext) bool {
    // 如果 PR 是开放状态，推送到现有分支
    return ctx.PullRequest.GetState() != "open"
}

func (s *PullRequestStrategy) GetTargetBranch(ctx *EnrichedContext) string {
    if ctx.PullRequest.GetState() == "open" {
        return ctx.PullRequest.GetHead().GetRef()
    }
    return s.GenerateBranchName(ctx)
}
```

### 4. Interaction Manager（交互管理器）

#### 职责
- 管理与用户的实时交互
- 提供进度反馈和状态更新
- 处理错误展示和用户通知

#### 核心设计
```go
// internal/interaction/manager.go
type InteractionManager struct {
    github         *github.Client
    commentManager *CommentManager
    progress       *ProgressTracker
    ui             *UIManager
}

type ProgressTracker struct {
    tasks          []*Task
    currentTask    *Task
    startTime      time.Time
    lastUpdate     time.Time
    status         ExecutionStatus
    spinner        *SpinnerState
}

type Task struct {
    ID             string
    Description    string
    Status         TaskStatus
    StartTime      time.Time
    EndTime        time.Time
    Duration       time.Duration
    SubTasks       []*Task
    Progress       float64  // 0.0 - 1.0
    Error          error
    Metadata       map[string]interface{}
}
```

#### 交互流程
```
1. 创建初始评论 → 2. 显示任务列表 → 3. 开始执行 → 4. 实时更新进度 → 5. 完成总结
      ↓                    ↓                ↓             ↓               ↓
   CreateComment      ShowTaskList     ShowSpinner   UpdateProgress   FinalizeComment
```

#### UI 组件

**CommentManager**
```go
type CommentManager struct {
    client    *github.Client
    formatter *MarkdownFormatter
    cache     *CommentCache
}

func (cm *CommentManager) CreateComment(ctx context.Context, content string) (*github.IssueComment, error)
func (cm *CommentManager) UpdateComment(ctx context.Context, commentID int64, content string) error
func (cm *CommentManager) FormatProgress(tracker *ProgressTracker) string
func (cm *CommentManager) FormatError(err error) string
```

**UIManager**
```go
type UIManager struct {
    templates map[string]*template.Template
    spinner   *SpinnerAnimation
}

func (ui *UIManager) RenderTaskList(tasks []*Task) string
func (ui *UIManager) RenderSpinner(message string) string
func (ui *UIManager) RenderError(err error) string
func (ui *UIManager) RenderSummary(result *ExecutionResult) string
```

### 5. Prompt System（提示系统）

#### 职责
- 根据事件类型和上下文生成动态提示
- 管理模板库和变量解析
- 支持自定义模板和条件渲染

#### 系统架构
```go
// internal/prompt/v2/system.go
type PromptSystem struct {
    engine      *TemplateEngine
    resolver    *VariableResolver
    enrichers   []ContextEnricher
    validator   *PromptValidator
}

type TemplateEngine struct {
    templates    map[EventType]map[string]*Template
    fallbacks    map[EventType]*Template
    customLoader *CustomTemplateLoader
    cache        *TemplateCache
}
```

#### 模板层次结构
```
BaseTemplate
    ├── IssueTemplate
    │   ├── IssueCreatedTemplate
    │   ├── IssueAssignedTemplate
    │   └── IssueLabeledTemplate
    ├── PullRequestTemplate
    │   ├── PROpenedTemplate
    │   ├── PRReviewTemplate
    │   └── PRCommentTemplate
    └── CustomTemplate
        ├── ProjectSpecificTemplate
        └── UserDefinedTemplate
```

#### 变量系统
```go
type VariableResolver struct {
    resolvers    map[string]VariableResolverFunc
    functions    map[string]TemplateFunc
    cache        *VariableCache
}

// 支持的变量类型
type Variable struct {
    Name         string
    Type         VariableType
    Value        interface{}
    Computed     bool
    Dependencies []string
}

// 内置变量解析器
var BuiltinResolvers = map[string]VariableResolverFunc{
    "REPOSITORY":       resolveRepository,
    "PR_NUMBER":        resolvePRNumber,
    "ISSUE_NUMBER":     resolveIssueNumber,
    "CHANGED_FILES":    resolveChangedFiles,
    "TRIGGER_COMMENT":  resolveTriggerComment,
    "TIMESTAMP":        resolveTimestamp,
    // ... 更多变量
}
```

#### 模板示例
```go
const IssueCreatedTemplate = `
你是一个专业的程序员。请根据以下 Issue 需求生成代码实现。

## Issue 信息
**仓库**: {{.REPOSITORY}}
**Issue #{{.ISSUE_NUMBER}}**: {{.ISSUE_TITLE}}

**描述**:
{{.ISSUE_BODY}}

{{if .ISSUE_LABELS}}
**标签**: {{range .ISSUE_LABELS}}[{{.}}] {{end}}
{{end}}

{{if .CHANGED_FILES}}
## 相关文件
{{range .CHANGED_FILES}}
- {{.Path}} ({{.Status}})
{{end}}
{{end}}

{{if .HAS_CUSTOM_CONFIG}}
## 项目配置
请参考项目的 CODEAGENT.md 文件中的自定义配置要求。
{{end}}

## 任务要求
1. 分析 Issue 需求，理解要实现的功能
2. 查看相关代码文件，了解项目结构
3. 生成符合项目规范的代码实现
4. 提供清晰的变更说明

请开始分析和实现...
`
```

### 6. Security Manager（安全管理器）

#### 职责
- 执行细粒度的权限控制
- 验证用户操作和工具使用权限
- 记录安全审计日志

#### 安全模型
```go
// internal/security/manager.go
type SecurityManager struct {
    policyEngine  *PolicyEngine
    permissions   *PermissionManager
    auditor       *SecurityAuditor
    rateLimiter   *RateLimiter
}

type PolicyEngine struct {
    policies     map[string]*SecurityPolicy
    evaluator    *PolicyEvaluator
    cache        *PolicyCache
}

type SecurityPolicy struct {
    ID           string
    Name         string
    Rules        []*SecurityRule
    Conditions   []*Condition
    Actions      []SecurityAction
    Priority     int
}

type SecurityRule struct {
    Resource     string           // file, command, api
    Action       string           // read, write, execute
    Effect       EffectType       // allow, deny
    Conditions   []*Condition
    RateLimit    *RateLimit
}
```

#### 权限验证流程
```
Request → PolicyEngine → PermissionCheck → RateLimit → AuditLog → Allow/Deny
   ↓           ↓              ↓              ↓           ↓          ↓
用户请求    策略评估      权限检查        限流控制     审计记录    结果返回
```

### 7. Media Processor（媒体处理器）

#### 职责
- 处理 GitHub 评论中的图片和多媒体内容
- 下载并转换为本地可访问路径
- 管理媒体文件缓存和清理

#### 处理流程
```go
// internal/media/processor.go
type MediaProcessor struct {
    downloader   *ImageDownloader
    storage      *MediaStorage
    transformer  *ContentTransformer
    cache        *MediaCache
}

type ImageDownloader struct {
    client       *http.Client
    maxSize      int64
    allowedTypes []string
    timeout      time.Duration
}

type MediaStorage struct {
    basePath     string
    maxDiskUsage int64
    retention    time.Duration
}
```

#### 支持的媒体类型
- **图片**: PNG, JPEG, GIF, WebP, SVG
- **文档**: PDF (转换为图片)
- **代码**: 语法高亮的代码截图

## 🔄 数据流设计

### 主要数据流

#### 1. 事件处理流
```
GitHub Webhook → Event Router → Context Enricher → Strategy Selector → AI Provider → Git Operations
       ↓              ↓              ↓                  ↓             ↓              ↓
   事件接收        事件解析        上下文收集          策略选择      AI 生成       Git 操作
```

#### 2. 用户交互流
```
Task Creation → Progress Tracking → Status Update → Comment Update → User Notification
      ↓               ↓               ↓              ↓               ↓
   任务创建        进度跟踪        状态更新       评论更新        用户通知
```

#### 3. 安全验证流
```
User Request → Permission Check → Policy Evaluation → Rate Limiting → Audit Logging
      ↓              ↓               ↓                ↓              ↓
   用户请求        权限检查        策略评估          限流控制       审计记录
```

### 数据模型

#### 核心数据结构
```go
// GitHub 事件模型
type GitHubEvent struct {
    ID          string                 `json:"id"`
    Type        EventType              `json:"type"`
    Action      string                 `json:"action"`
    Repository  *github.Repository     `json:"repository"`
    Sender      *github.User          `json:"sender"`
    Payload     map[string]interface{} `json:"payload"`
    ReceivedAt  time.Time             `json:"received_at"`
}

// 工作空间模型
type Workspace struct {
    ID           string                `json:"id"`
    Path         string                `json:"path"`
    Repository   string                `json:"repository"`
    Branch       string                `json:"branch"`
    BaseBranch   string                `json:"base_branch"`
    PRNumber     int                   `json:"pr_number,omitempty"`
    IssueNumber  int                   `json:"issue_number,omitempty"`
    AIModel      string                `json:"ai_model"`
    Status       WorkspaceStatus       `json:"status"`
    CreatedAt    time.Time            `json:"created_at"`
    UpdatedAt    time.Time            `json:"updated_at"`
    ExpiresAt    time.Time            `json:"expires_at"`
    Metadata     map[string]interface{} `json:"metadata"`
}

// 执行结果模型
type ExecutionResult struct {
    Success       bool                  `json:"success"`
    Output        string                `json:"output"`
    Error         string                `json:"error,omitempty"`
    FilesChanged  []string             `json:"files_changed"`
    CommitSHA     string               `json:"commit_sha,omitempty"`
    BranchName    string               `json:"branch_name"`
    Duration      time.Duration        `json:"duration"`
    TaskResults   []*TaskResult        `json:"task_results"`
    Metadata      map[string]interface{} `json:"metadata"`
}
```

## 🔌 接口设计

### 核心接口

#### EventHandler 接口
```go
type EventHandler interface {
    // 检查是否可以处理该事件
    CanHandle(event *GitHubEvent) bool
    
    // 处理事件
    Handle(ctx context.Context, event *GitHubEvent) (*HandlerResult, error)
    
    // 获取处理器优先级
    GetPriority() int
    
    // 获取所需权限
    GetRequiredPermissions() []string
}
```

#### AIProvider 接口
```go
type AIProvider interface {
    // 提供商名称
    Name() string
    
    // 生成代码
    GenerateCode(ctx context.Context, prompt string, workspace *Workspace) (*GenerationResult, error)
    
    // 健康检查
    HealthCheck(ctx context.Context) error
    
    // 获取支持的模型
    GetSupportedModels() []string
    
    // 配置验证
    ValidateConfig(config map[string]interface{}) error
}
```

#### BranchStrategy 接口
```go
type BranchStrategy interface {
    // 是否应该创建新分支
    ShouldCreateNewBranch(ctx *EnrichedContext) bool
    
    // 获取目标分支名
    GetTargetBranch(ctx *EnrichedContext) string
    
    // 获取基础分支名
    GetBaseBranch(ctx *EnrichedContext) string
    
    // 生成分支名
    GenerateBranchName(ctx *EnrichedContext) string
    
    // 获取提交策略
    GetCommitStrategy() CommitStrategy
}
```

## 📦 模块依赖

### 依赖关系图
```
┌─────────────────┐
│   HTTP Server   │
└─────────┬───────┘
          │
┌─────────▼───────┐    ┌─────────────────┐
│ Event Router    │────│ Security Manager│
└─────────┬───────┘    └─────────────────┘
          │
┌─────────▼───────┐    ┌─────────────────┐
│Context Enricher │────│ Media Processor │
└─────────┬───────┘    └─────────────────┘
          │
┌─────────▼───────┐    ┌─────────────────┐
│ Branch Strategy │────│ Git Operations  │
└─────────┬───────┘    └─────────────────┘
          │
┌─────────▼───────┐    ┌─────────────────┐
│ Prompt System   │────│ Template Engine │
└─────────┬───────┘    └─────────────────┘
          │
┌─────────▼───────┐    ┌─────────────────┐
│ AI Provider     │────│Session Manager  │
└─────────┬───────┘    └─────────────────┘
          │
┌─────────▼───────┐    ┌─────────────────┐
│Interaction Mgr  │────│ Progress Tracker│
└─────────────────┘    └─────────────────┘
```

### 模块间通信

#### 同步通信
- **HTTP API**: 外部 Webhook 接入
- **Function Call**: 模块间直接调用
- **Interface**: 基于接口的解耦调用

#### 异步通信
- **Event Bus**: 内部事件广播
- **Message Queue**: 任务队列处理
- **Callback**: 异步结果回调

## 🔧 技术选型

### 编程语言和框架
- **主语言**: Go 1.21+
- **Web 框架**: Gin（HTTP 服务）
- **Git 操作**: go-git + git CLI
- **GitHub API**: go-github v58
- **模板引擎**: text/template + sprig

### 存储和缓存
- **本地存储**: 文件系统（工作空间和媒体文件）
- **内存缓存**: sync.Map + TTL 管理
- **持久化缓存**: BadgerDB（可选）

### 安全和监控
- **权限控制**: RBAC 模型
- **审计日志**: 结构化日志 + 文件轮转
- **限流**: Token Bucket 算法
- **监控**: Prometheus 指标

### AI 集成
- **Claude**: Anthropic API + Claude CLI
- **Gemini**: Google API + Gemini CLI
- **会话管理**: 长连接 + 会话状态管理

## 📊 性能考虑

### 性能目标
- **响应时间**: 平均 < 30s，P99 < 60s
- **并发处理**: 支持 100+ 并发请求
- **内存使用**: < 2GB 常驻内存
- **磁盘使用**: 工作空间自动清理，< 50GB

### 优化策略

#### 1. 缓存策略
```go
type CacheManager struct {
    // GitHub 数据缓存（5分钟 TTL）
    githubCache    *TTLCache
    
    // 模板缓存（1小时 TTL）
    templateCache  *TTLCache
    
    // 媒体文件缓存（24小时 TTL）
    mediaCache     *TTLCache
    
    // 权限缓存（10分钟 TTL）
    permissionCache *TTLCache
}
```

#### 2. 并发控制
```go
type ConcurrencyManager struct {
    // 全局请求限制
    globalSemaphore    *semaphore.Weighted
    
    // AI 提供商限制
    aiProviderLimits   map[string]*semaphore.Weighted
    
    // 工作空间锁
    workspaceLocks     *sync.Map
}
```

#### 3. 内存管理
- **对象池**: 复用高频对象
- **流式处理**: 大文件流式读取
- **垃圾回收**: 定期清理过期数据

## 🔄 扩展性设计

### 插件系统
```go
type Plugin interface {
    Name() string
    Version() string
    Init(config map[string]interface{}) error
    Shutdown() error
}

type EventPlugin interface {
    Plugin
    HandleEvent(ctx context.Context, event *GitHubEvent) error
}

type AIProviderPlugin interface {
    Plugin
    AIProvider
}
```

### 配置系统
```yaml
# config.yaml
plugins:
  - name: "custom-ai-provider"
    path: "./plugins/custom-ai-provider.so"
    config:
      api_key: "${CUSTOM_AI_API_KEY}"
      
  - name: "slack-notifier"
    path: "./plugins/slack-notifier.so"
    config:
      webhook_url: "${SLACK_WEBHOOK_URL}"

extensions:
  event_handlers:
    - plugin: "custom-ai-provider"
      events: ["issues", "pull_request"]
      
  notification_handlers:
    - plugin: "slack-notifier"
      events: ["task_completed", "task_failed"]
```

## 📋 部署架构

### 单机部署
```
┌─────────────────────────────────────┐
│           CodeAgent Server          │
│  ┌─────────────┐ ┌─────────────────┐│
│  │   HTTP      │ │   Git Workspace ││
│  │  Service    │ │     Manager     ││
│  └─────────────┘ └─────────────────┘│
│  ┌─────────────┐ ┌─────────────────┐│
│  │    AI       │ │     Media       ││
│  │ Providers   │ │    Storage      ││
│  └─────────────┘ └─────────────────┘│
└─────────────────────────────────────┘
```

### 集群部署
```
┌─────────────┐    ┌─────────────────┐
│ Load        │────│ CodeAgent       │
│ Balancer    │    │ Instance 1      │
└─────────────┘    └─────────────────┘
       │           ┌─────────────────┐
       └───────────│ CodeAgent       │
                   │ Instance 2      │
                   └─────────────────┘
                   ┌─────────────────┐
                   │ Shared Storage  │
                   │ (NFS/Object)    │
                   └─────────────────┘
```

---

**文档版本**: 1.0  
**最后更新**: 2025-01-26  
**维护人员**: CodeAgent 架构团队