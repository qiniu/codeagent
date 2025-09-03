# PR Review Comment Mention 功能增强设计文档

## 📋 概述

本文档描述如何在现有 TagHandler 中增强 PR Review Comment 的 mention 处理能力，通过复用现有的`processPRComment`方法来快速支持 PR Code Review 场景中的`@qiniu-ci`等 mention 功能。

## 🎯 目标

在 PR Files Changed 页面的 Review Comment 中支持 mention 功能，使用户能够：

- 在代码行级别与 AI 助手互动
- 获得针对特定代码行的回答和建议
- 保持与 PR Conversation 页面 mention 功能的一致性

## 🔍 现状分析

### 当前实现状态

```go
// 在 handlePRReviewComment 中（line 231-242）
case models.CommandMention:
    // TODO(CarlJi): TO BE IMPLEMENTED
    return fmt.Errorf("unsupported command for PR review comment: %s", cmdInfo.Command)
```

### 现有能力

- ✅ PR Conversation 中的 mention (`processPRComment`)
- ✅ Issue 中的 mention (`processIssueComment`)
- ✅ PR Review Comment 中的`/continue`命令
- ❌ PR Review Comment 中的 mention（待实现）

## 🏗️ 设计方案

### 核心思路：专门方法实现

创建一个专门的 `processPRCodeReviewComment` 方法来处理 PR Review Comment 中的 mention，避免复杂的适配器模式，提供更精确的代码行级别上下文。

### 架构图

```
PR Review Comment Event
        ↓
handlePRReviewComment
        ↓
CommandMention case
        ↓
processPRCodeReviewComment (专门方法)
        ↓
buildPRReviewCommentPrompt (包含代码行上下文)
        ↓
AI处理 (结果回复到特定thread)
```

### 关键组件

#### 1. 专门的处理方法

```go
// processPRCodeReviewComment 处理PR Review Comment中的@qiniu-ci命令（代码行级别）
func (th *TagHandler) processPRCodeReviewComment(
    ctx context.Context,
    event *models.PullRequestReviewCommentContext,
    cmdInfo *models.CommandInfo,
) error {
    // 1. 获取PR信息和创建工作空间
    // 2. 构建包含代码行上下文的prompt
    // 3. 调用AI处理
    // 4. AI通过prompt直接回复到comment thread
}
```

#### 2. 增强的 Prompt 构建

```go
// buildPRReviewCommentPrompt 构建PR Review Comment的提示词，包含代码行上下文
func (th *TagHandler) buildPRReviewCommentPrompt(
    ctx context.Context,
    event *models.PullRequestReviewCommentContext,
    cmdInfo *models.CommandInfo,
) (string, error) {
    // 1. 收集代码行位置信息（文件路径、行号、diff hunk）
    // 2. 收集PR的所有评论上下文
    // 3. 构建专门的Review Comment指令
}
```

#### 3. AI 直接回复机制

AI 通过 MCP 上下文获得 GitHub 工具访问权限，可以直接使用 `mcp__codeagent__github-comments__create_review_comment` 工具来回复到 comment thread，无需额外的代码回复逻辑。

**关键点：**

- AI 本身就有 MCP 工具能力
- 在 prompt 中告诉 AI 使用正确的工具和参数
- AI 自己调用工具回复到相同的代码行位置
- 形成完整的 comment thread

## 📐 详细实现设计

### 实现步骤

#### Step 1: 实现专门的处理方法

```go
func (th *TagHandler) processPRCodeReviewComment(
    ctx context.Context,
    event *models.PullRequestReviewCommentContext,
    cmdInfo *models.CommandInfo,
) error {
    xl := xlog.NewWith(ctx)

    prNumber := event.PullRequest.GetNumber()
    prTitle := event.PullRequest.GetTitle()
    commentPath := event.Comment.GetPath()
    commentLine := event.Comment.GetLine()

    xl.Infof("Processing @qiniu-ci command in PR review comment: PR=#%d, title=%s, file=%s, line=%d, AI model=%s, instruction=%s",
        prNumber, prTitle, commentPath, commentLine, cmdInfo.AIModel, cmdInfo.Args)

    // 1. 获取完整的PR信息和工作空间
    // 2. 构建包含代码行上下文的prompt
    prompt, err := th.buildPRReviewCommentPrompt(ctx, event, cmdInfo)
    if err != nil {
        return fmt.Errorf("failed to build PR review comment prompt: %w", err)
    }

    // 3. 调用AI处理
    resp, err := th.promptWithRetry(ctx, codeClient, prompt, 3)
    if err != nil {
        return fmt.Errorf("failed to get AI response: %w", err)
    }

    // 4. AI已经通过prompt直接回复了，不需要额外的回复逻辑
    return nil
}
```

#### Step 2: 实现增强的 Prompt 构建

```go
func (th *TagHandler) buildPRReviewCommentPrompt(
    ctx context.Context,
    event *models.PullRequestReviewCommentContext,
    cmdInfo *models.CommandInfo,
) (string, error) {
    // 创建增强上下文，专门针对Review Comment
    enhancedCtx := &ctxsys.EnhancedContext{
        Type:      ctxsys.ContextTypePR,
        Priority:  ctxsys.PriorityHigh,
        Timestamp: time.Now(),
        Subject:   event,
        Metadata: map[string]interface{}{
            "pr_number":       pr.GetNumber(),
            "pr_title":        pr.GetTitle(),
            "comment_type":    "review_comment",
            "file_path":       comment.GetPath(),
            "line_number":     comment.GetLine(),
            "start_line":      comment.GetStartLine(),
            "diff_hunk":       comment.GetDiffHunk(),
            "commit_id":       comment.GetCommitID(),
        },
    }

    // 收集PR的所有评论（包括issue comments和review comments）
    // 构建专门的Review Comment指令
    reviewCommentInstruction := fmt.Sprintf(
        "用户在第%d行（文件：%s）的代码评论中提到了你。请针对这个具体的代码行和上下文提供帮助。\n\n原始评论：%s\n\n%s",
        comment.GetLine(),
        comment.GetPath(),
        comment.GetBody(),
        cmdInfo.Args,
    )

    return th.contextManager.Generator.GeneratePrompt(enhancedCtx, "default", reviewCommentInstruction)
}
```

#### Step 3: 在 prompt 中告诉 AI 使用 MCP 工具

```go
// 在 buildPRReviewCommentPrompt 中构建指令
reviewCommentInstruction := fmt.Sprintf(
    "用户在第%d行（文件：%s）的代码评论中提到了你。请针对这个具体的代码行和上下文提供帮助。\n\n原始评论：%s\n\n%s\n\n"+
    "重要：请使用 create_review_comment 工具来回复到相同的代码行位置，形成comment thread。\n"+
    "工具参数：\n"+
    "- pull_number: %d\n"+
    "- body: 你的回复内容\n"+
    "- commit_id: %s\n"+
    "- path: %s\n"+
    "- line: %d",
    comment.GetLine(),
    comment.GetPath(),
    comment.GetBody(),
    cmdInfo.Args,
    pr.GetNumber(),
    comment.GetCommitID(),
    comment.GetPath(),
    comment.GetLine(),
)
```

#### Step 4: 修改 handlePRReviewComment

```go
case models.CommandMention:
    // 实现PR Review Comment中的mention处理
    xl.Infof("Processing mention command in PR review comment")
    return th.processPRCodeReviewComment(ctx, event, cmdInfo)
```

### 回复机制增强

#### 实现方案：AI 直接使用 MCP 工具回复

- AI 通过 MCP 上下文自动获得 GitHub 工具访问权限
- 在 prompt 中告诉 AI 使用 `create_review_comment` 工具
- AI 自己调用工具回复到相同的代码行位置，形成 comment thread
- 优点：回复精确，用户体验好，形成完整的讨论线程，AI 自动处理
- 缺点：需要确保 prompt 中提供正确的工具参数

#### Prompt 中的工具指令

```go
// 在 prompt 中告诉 AI 使用正确的工具和参数
reviewCommentInstruction := fmt.Sprintf(
    "重要：请使用 create_review_comment 工具来回复到相同的代码行位置，形成comment thread。\n"+
    "工具参数：\n"+
    "- pull_number: %d\n"+
    "- body: 你的回复内容\n"+
    "- commit_id: %s\n"+
    "- path: %s\n"+
    "- line: %d",
    pr.GetNumber(),
    comment.GetCommitID(),
    comment.GetPath(),
    comment.GetLine(),
)
```

#### AI 自动处理

- AI 接收到 prompt 后，会自动调用指定的 MCP 工具
- 工具参数已经在 prompt 中提供，AI 直接使用
- 无需额外的代码回复逻辑

## 🎨 用户体验设计

### 使用场景示例

#### 场景 1：代码行级问题咨询

```
用户在Files Changed页面的某行代码评论：
"@qiniu-ci 这里的错误处理是否充分？"

AI回复：
> 关于 `src/handler.go` 第42行的提问：

根据代码分析，当前错误处理可以优化：
1. 缺少对空指针的检查
2. 建议添加日志记录
3. 可以考虑自定义错误类型...
```

#### 场景 2：代码优化建议

```
用户评论：
"@qiniu-ci -claude 这个函数性能如何？有优化建议吗？"

AI回复：
> 关于 `src/utils.go` 第15行的提问：

使用Claude分析，该函数性能存在以下问题：
1. 循环中的字符串拼接效率低
2. 建议使用 strings.Builder
3. 预估性能可提升60%...
```

## 🚀 实现优势

### 1. **精确的代码行级别支持**

- 专门处理 PR Review Comment 场景
- 包含完整的代码行上下文信息（文件路径、行号、diff hunk）
- 回复到相同的代码行位置，形成完整的讨论线程

### 2. **更好的用户体验**

- 与 PR Conversation 中的 mention 行为一致
- 用户学习成本低
- 维护统一的 mention 语法
- 回复内容包含代码行上下文，更易理解

### 3. **清晰的架构设计**

- 专门的方法处理专门场景，避免复杂的适配器
- 代码逻辑清晰，易于维护和扩展
- 可以独立测试和调试

### 4. **风险可控**

- 不影响现有功能
- 专门方法易于调试和修改
- 可以独立回退
- 编译和测试都通过

## 🔧 技术细节

### 兼容性考虑

#### GitHub API 兼容性

- PR 在 GitHub API 中本质是特殊的 Issue
- PullRequestComment 和 IssueComment 结构相似
- 通过 PullRequestLinks 字段区分 PR 和 Issue

#### 现有代码兼容性

- `processPRComment`已经处理`IsPRComment=true`的情况
- 工作空间管理逻辑已支持 PR 场景
- MCP 工具调用无需修改

### 错误处理

```go
// 适配过程中的错误处理
if reviewCommentEvent.PullRequest == nil {
    return nil, fmt.Errorf("missing pull request information")
}
if reviewCommentEvent.Comment == nil {
    return nil, fmt.Errorf("missing review comment information")
}

// 确保必要字段不为空
if reviewCommentEvent.PullRequest.Number == nil {
    return nil, fmt.Errorf("missing PR number")
}
```

## 📊 测试策略

### 测试场景

1. **基础 mention 测试**

   - `@qiniu-ci` 简单问答
   - `@qiniu-ci -claude` 指定 AI 模型
   - `@qiniu-ci -gemini` 切换 AI 模型

2. **代码行上下文测试**

   - 单行评论的 mention
   - 多行选择的 mention
   - 不同文件类型的 mention

3. **错误场景测试**
   - 无效的 mention 格式
   - AI 调用失败的降级处理
   - 网络异常的错误处理

### 测试数据

```json
{
  "action": "created",
  "pull_request": { "number": 123, "title": "Test PR" },
  "comment": {
    "body": "@qiniu-ci 这段代码有什么问题？",
    "path": "src/test.go",
    "line": 42
  }
}
```

## ⏱️ 实现计划

### 阶段 1：核心功能（已完成）

- [x] 实现专门方法 `processPRCodeReviewComment`
- [x] 实现增强的 Prompt 构建 `buildPRReviewCommentPrompt`
- [x] 实现 AI 直接使用 MCP 工具回复的机制
- [x] 修改 `handlePRReviewComment` 中的 TODO
- [x] 基础测试验证和编译检查

### 阶段 2：优化增强（后续版本）

- [ ] 添加更多的代码行上下文信息（如代码内容、语法高亮）
- [ ] 支持多行选择的 Review Comment
- [ ] 添加 Review Comment 的历史上下文
- [ ] 性能优化和缓存

### 阶段 3：高级功能（未来扩展）

- [ ] 批量 Review Comment 处理
- [ ] 代码建议的直接应用
- [ ] 与 Review Summary 的整合
- [ ] 支持代码片段的直接修改建议

---
