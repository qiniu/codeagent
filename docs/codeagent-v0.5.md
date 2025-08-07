# CodeAgent v0.5 核心特性

## 架构重构

### 1. 新增 EnhancedAgent 架构

- **文件**: `internal/agent/enhanced_agent.go`
- **核心改进**: 引入组件化架构，替代单体 Agent 设计
- **新增组件**:
  - `eventParser`: 类型安全的事件解析
  - `modeManager`: 插件化处理模式管理
  - `mcpManager`: MCP 工具系统管理
  - `taskFactory`: 交互任务工厂

### 2. 模式化处理系统

- **文件**: `internal/modes/`
- **TagHandler**: 处理命令式操作(`/code`, `/continue`, `/fix`)
- **AgentHandler**: 处理@mention 事件
- **ReviewHandler**: 自动 PR 审查
- **BaseHandler**: 统一的处理器接口和优先级管理

## 上下文系统增强

### 3. 增强上下文收集器

- **文件**: `internal/context/`
- **collector.go**: 智能收集 GitHub 事件、PR 状态、评论历史
- **formatter.go**: 上下文格式化，支持 token 限制
- **generator.go**: 智能提示词生成
- **pr_formatter.go**: PR 描述格式化

### 4. 事件解析系统

- **文件**: `internal/events/parser.go`
- **特性**: 类型安全的 GitHub webhook 事件解析
- **支持事件**: IssueComment、PullRequestReview、PullRequestReviewComment

## MCP 工具系统

### 5. MCP (Model Context Protocol) 基础架构

- **文件**: `internal/mcp/`
- **状态**: 基础架构实现，为未来AI工具集成做准备
- **manager.go**: MCP 服务器管理框架
- **client.go**: MCP 客户端接口定义
- **servers/github\_\*.go**: GitHub API MCP 服务器模板
- **interfaces.go**: MCP 工具系统接口定义

### 6. 进度跟踪系统

- **文件**: `internal/interaction/progress_comment.go`
- **特性**: PR 中的实时进度更新
- **支持**: 任务状态跟踪、Spinner 动画、进度条

## 核心功能优化

### 7. AI 模型选择逻辑

- **TagHandler 场景**: `/code -claude` 用户指定 > `config.CodeProvider` 系统默认
- **Review 场景**: 从分支名提取 AI 模型 (`codeagent/<aimodel>/<other>`)
- **配置驱动**: 支持 claude/gemini 动态切换

### 8. 工作空间管理增强

- **文件**: `internal/workspace/manager.go`
- **特性**: AI 模型感知的工作空间创建
- **格式**: 分支命名规范化 (`codeagent/<aimodel>/<task>`)
- **清理**: 自动清理冲突的 worktree

### 9. 会话管理系统

- **文件**: `internal/code/session.go`
- **特性**: AI 会话隔离和复用
- **支持**: Claude CLI/Docker、Gemini CLI/Docker

## 命令处理改进

### 10. 统一命令解析

- **文件**: `pkg/models/events.go`
- **支持**: `/code`, `/continue`, `/fix` 命令
- **AI 模型参数**: `-claude`, `-gemini` 参数支持

### 11. PR Review 批量处理

- **特性**: Review 中的批量评论处理
- **场景支持**:
  - Issue Comment → PR Comment
  - PR Review → 批量处理
  - PR Review Comment → 单行级处理
