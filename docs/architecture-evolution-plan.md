# CodeAgent Architecture Evolution Plan

## 📋 Overview

This document outlines the specific architectural changes needed to transform CodeAgent from its current state to a sophisticated AI collaboration platform that matches claude-code-action's capabilities while maintaining its unique strengths.

## 🏗️ Current vs. Target Architecture

### Current Architecture (v0.x)
```
GitHub Webhook → Handler → Agent → Workspace → AI Provider → Git Ops
                   ↓        ↓         ↓          ↓            ↓
               Signature  Monolithic  Basic     Claude/      Basic
               Validation  Handler    Context   Gemini       Branching
```

### Target Architecture (v1.0)
```
GitHub Webhook → Event Router → Context Pipeline → Prompt System → MCP Layer → Smart Git
       ↓              ↓             ↓               ↓            ↓           ↓
   Security       Event Type    Rich Context     Dynamic     Tool         Intelligent
   Framework      Detection     Enrichment       Templates   Management   Branching
       ↓              ↓             ↓               ↓            ↓           ↓
   Permission     Specialized   Multi-Modal      Variable     Provider     Progress
   Control        Handlers      Processing       Resolution   Abstraction  Tracking
```

## 🔧 Core Architectural Changes

### 1. Agent Decomposition Strategy

#### Current Problem
```go
// internal/agent/agent.go - 1000+ lines handling everything
type Agent struct {
    github     *github.Client
    workspace  *workspace.Manager
    providers  map[string]code.Provider
    config     *config.Config
}

func (a *Agent) ProcessWebhook(event *GitHubEvent) error {
    // Handles all event types in single method
    // Issue comments, PR comments, reviews, etc.
    // Complex branching logic throughout
}
```

#### Target Solution
```go
// internal/router/event_router.go
type EventRouter struct {
    handlers map[EventType]EventHandler
    security *security.Manager
    context  *context.Pipeline
}

// internal/handlers/issue_handler.go
type IssueHandler struct {
    github     *github.Client
    workspace  *workspace.Manager
    ai         *ai.Manager
    interaction *interaction.Manager
}

// internal/handlers/pr_handler.go
type PRHandler struct {
    github     *github.Client
    workspace  *workspace.Manager
    ai         *ai.Manager
    branch     *branch.SmartManager
}
```

#### Migration Strategy
1. **Week 1**: Create handler interfaces and routing infrastructure
2. **Week 1**: Extract issue handling logic to `IssueHandler`
3. **Week 1**: Extract PR handling logic to `PRHandler`  
4. **Week 1**: Extract review handling logic to `ReviewHandler`
5. **Week 2**: Migrate all routes to use new system
6. **Week 2**: Remove old monolithic agent methods
7. **Week 2**: Add comprehensive tests for each handler

### 2. Context Enrichment Pipeline

#### Current Problem
```go
// Scattered throughout agent methods
func (a *Agent) ProcessIssueComment(event *IssueCommentEvent) error {
    // Basic context gathering mixed with business logic
    issue, _ := a.github.GetIssue(event.Issue.Number)
    comments, _ := a.github.ListComments(event.Issue.Number)
    // No systematic context building
}
```

#### Target Solution
```go
// internal/context/pipeline.go
type ContextPipeline struct {
    enrichers []ContextEnricher
    cache     *ContextCache
}

type EnrichedContext struct {
    Event           *GitHubEvent
    Repository      *RepositoryInfo
    Issue           *EnrichedIssue
    PullRequest     *EnrichedPullRequest
    Comments        []*ProcessedComment
    ReviewComments  []*ProcessedReviewComment
    ChangedFiles    []*ChangedFileInfo
    Images          []*DownloadedImage
    Timeline        []*TimelineEvent
    UserPermissions *UserPermissions
    Metadata        map[string]interface{}
}

// internal/context/enrichers/github_enricher.go
type GitHubDataEnricher struct {
    client   *github.Client
    graphql  *githubv4.Client
    cache    *TTLCache
}

func (g *GitHubDataEnricher) Enrich(ctx context.Context, event *GitHubEvent) (*EnrichedContext, error) {
    // Comprehensive GitHub data fetching
}

// internal/context/enrichers/media_enricher.go
type MediaEnricher struct {
    processor *media.ImageProcessor
}

func (m *MediaEnricher) Enrich(ctx context.Context, enriched *EnrichedContext) error {
    // Extract and process images from comments
}
```

#### Implementation Steps
1. **Week 2 Day 1-2**: Create context pipeline infrastructure
2. **Week 2 Day 3-4**: Implement GitHub data enricher with GraphQL
3. **Week 2 Day 5-6**: Implement basic media enricher
4. **Week 2 Day 7**: Integrate pipeline into handlers
5. **Week 5-6**: Enhance media enricher with full image processing
6. **Week 10**: Add advanced enrichers (code analysis, history)

### 3. Progressive Communication System

#### Current Problem
```go
// Multiple comments create noise
func (a *Agent) ProcessIssueComment(event *IssueCommentEvent) error {
    // Creates new comment for each response
    comment := "Starting code generation..."
    a.github.CreateComment(event.Issue.Number, comment)
    
    // Later creates another comment
    result := "Code generation completed"
    a.github.CreateComment(event.Issue.Number, result)
}
```

#### Target Solution
```go
// internal/interaction/comment_manager.go
type CommentManager struct {
    github        *github.Client
    commentID     *int64
    lastContent   string
    progressTracker *ProgressTracker
}

type ProgressTracker struct {
    Tasks       []*Task
    CurrentTask *Task
    StartTime   time.Time
    Status      ExecutionStatus
}

func (cm *CommentManager) InitializeProgress(tasks []*Task) error {
    content := cm.renderInitialComment(tasks)
    comment, err := cm.github.CreateComment(cm.repo, cm.number, content)
    cm.commentID = &comment.ID
    return err
}

func (cm *CommentManager) UpdateTask(taskID string, status TaskStatus) error {
    cm.progressTracker.UpdateTask(taskID, status)
    content := cm.renderProgressUpdate()
    return cm.github.UpdateComment(cm.commentID, content)
}

func (cm *CommentManager) ShowSpinner(message string) error {
    content := cm.renderSpinnerUpdate(message)
    return cm.github.UpdateComment(cm.commentID, content)
}
```

#### UI Template System
```go
// internal/interaction/templates.go
const ProgressTemplate = `
## 🤖 CodeAgent Working...

{{range .Tasks}}
- [{{if eq .Status "completed"}}x{{else if eq .Status "in_progress"}}○{{else}} {{end}}] {{.Description}}
{{end}}

{{if .CurrentTask}}
### Currently: {{.CurrentTask.Description}}
{{if .ShowSpinner}}{{.Spinner}} {{.SpinnerMessage}}{{end}}
{{end}}

{{if .Error}}
### ❌ Error
{{.Error}}
{{end}}

---
*Started: {{.StartTime.Format "15:04:05"}} | Duration: {{.Duration}}*
`
```

### 4. Multi-Modal Content Processing

#### Target Implementation
```go
// internal/media/image_processor.go
type ImageProcessor struct {
    downloader *ImageDownloader
    storage    *ImageStorage
    converter  *ImageConverter
}

type ImageDownloader struct {
    client      *http.Client
    maxSize     int64
    allowedTypes []string
    concurrent  int
}

type DownloadedImage struct {
    OriginalURL  string
    LocalPath    string
    MimeType     string
    Size         int64
    Width        int
    Height       int
    Hash         string
    DownloadedAt time.Time
}

func (ip *ImageProcessor) ProcessCommentImages(content string) ([]*DownloadedImage, string, error) {
    // 1. Extract image URLs from markdown
    urls := ip.extractImageURLs(content)
    
    // 2. Download images concurrently
    images := make([]*DownloadedImage, 0, len(urls))
    for _, url := range urls {
        img, err := ip.downloader.Download(url)
        if err != nil {
            continue // Skip failed downloads
        }
        images = append(images, img)
    }
    
    // 3. Replace URLs with local paths
    updatedContent := ip.replaceURLsWithLocalPaths(content, images)
    
    return images, updatedContent, nil
}
```

### 5. MCP (Model Context Protocol) Architecture

#### Target Implementation
```go
// internal/mcp/server.go
type MCPServer interface {
    Name() string
    Version() string
    Tools() []Tool
    Handle(ctx context.Context, request *MCPRequest) (*MCPResponse, error)
}

type MCPManager struct {
    servers map[string]MCPServer
    router  *MCPRouter
}

// internal/mcp/servers/github_server.go
type GitHubServer struct {
    client *github.Client
    repo   string
    branch string
}

func (gs *GitHubServer) Tools() []Tool {
    return []Tool{
        {
            Name: "github_read_file",
            Description: "Read file contents from GitHub repository",
            InputSchema: FileReadSchema,
        },
        {
            Name: "github_write_file", 
            Description: "Write file contents to GitHub repository",
            InputSchema: FileWriteSchema,
        },
        {
            Name: "github_create_comment",
            Description: "Create comment on issue or PR",
            InputSchema: CommentCreateSchema,
        },
    }
}

// internal/mcp/servers/filesystem_server.go
type FilesystemServer struct {
    workspace *workspace.Manager
    security  *security.Manager
}
```

### 6. Smart Branch Management

#### Current Implementation
```go
// Basic branch creation in workspace manager
func (w *WorkspaceManager) CreateWorkspace(repo, branch string) (*Workspace, error) {
    // Simple branch creation without context awareness
}
```

#### Target Implementation
```go
// internal/branch/smart_manager.go
type SmartBranchManager struct {
    strategies map[EventType]BranchStrategy
    cleanup    *BranchCleanup
    git        *git.Operations
}

type BranchStrategy interface {
    ShouldCreateNewBranch(ctx *EnrichedContext) bool
    GetTargetBranch(ctx *EnrichedContext) string
    GetBranchName(ctx *EnrichedContext) string
    GetCommitStrategy() CommitStrategy
}

// internal/branch/strategies/issue_strategy.go
type IssueStrategy struct {
    branchPrefix string
    baseBranch   string
}

func (is *IssueStrategy) ShouldCreateNewBranch(ctx *EnrichedContext) bool {
    return true // Issues always get new branches
}

func (is *IssueStrategy) GetBranchName(ctx *EnrichedContext) string {
    slug := strings.ToLower(ctx.Issue.Title)
    slug = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(slug, "-")
    slug = strings.Trim(slug, "-")
    if len(slug) > 30 {
        slug = slug[:30]
    }
    return fmt.Sprintf("codeagent/issue-%d-%s", ctx.Issue.Number, slug)
}

// internal/branch/strategies/pr_strategy.go
type PRStrategy struct{}

func (ps *PRStrategy) ShouldCreateNewBranch(ctx *EnrichedContext) bool {
    // Use existing branch if PR is open
    return ctx.PullRequest.State != "open"
}

func (ps *PRStrategy) GetTargetBranch(ctx *EnrichedContext) string {
    if ctx.PullRequest.State == "open" {
        return ctx.PullRequest.Head.Ref
    }
    return ps.GetBranchName(ctx)
}
```

## 🚀 Migration Strategy

### Phase 1: Foundation (Weeks 1-4)
**Goal**: Refactor existing code without changing external behavior

1. **Handler Decomposition** (Week 1)
   - Create new handler interfaces
   - Extract logic from monolithic agent
   - Maintain backward compatibility
   - Add comprehensive tests

2. **Context Pipeline** (Week 2)
   - Build enrichment pipeline infrastructure
   - Migrate context gathering to centralized system
   - Add basic GitHub data enricher
   - Implement comment management system

3. **Template System** (Week 3)
   - Replace hardcoded prompts with template system
   - Create variable resolution framework
   - Migrate existing prompts to templates
   - Add template testing framework

4. **Security Framework** (Week 4)
   - Implement permission checking system
   - Add security audit logging
   - Create error handling standards
   - Add comprehensive error recovery

### Phase 2: Feature Development (Weeks 5-10)
**Goal**: Add missing capabilities for claude-code-action parity

1. **Multi-Modal Support** (Weeks 5-6)
   - Image processing pipeline
   - AI provider integration with images
   - Enhanced context enrichment

2. **MCP Architecture** (Weeks 7-8)
   - MCP server framework
   - Built-in server implementations
   - Tool permission system

3. **Smart Branch Management** (Week 9)
   - Context-aware branching strategies
   - Automatic cleanup systems
   - Enhanced git operations

4. **Advanced Context** (Week 10)
   - Rich GitHub data collection
   - Code analysis and insights
   - Historical context building

### Phase 3: Polish & Optimization (Weeks 11-14)
**Goal**: Production readiness and unique differentiators

## 📊 Success Metrics

### Technical Metrics
- **Response Time**: Average < 20s (vs. current 45s+)
- **Success Rate**: 98%+ (vs. current 90%)
- **Memory Usage**: < 1GB per concurrent operation
- **Test Coverage**: 90%+ across all modules

### Feature Parity Metrics
- **GitHub Integration**: 100% feature parity with claude-code-action
- **User Experience**: Progressive communication implemented
- **Multi-Modal**: Image processing capability
- **Security**: Fine-grained permission control

### User Experience Metrics
- **User Satisfaction**: Survey rating > 4.5/5
- **Error Recovery**: Clear error messages and suggested fixes
- **Response Quality**: Consistent, context-aware AI responses

This architectural evolution plan provides a clear path from the current monolithic design to a sophisticated, modular system that matches claude-code-action's capabilities while building on CodeAgent's existing strengths.