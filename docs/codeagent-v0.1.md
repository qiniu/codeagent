# Code Agent 系统设计 v0.1

## 项目概述

Code Agent 是一个基于 Go 语言开发的自动化代码生成系统，通过 GitHub Webhook 接收 `/code` 命令，自动为 Issue 生成代码并创建 Pull Request。

## 系统架构

```
GitHub Issue (/code) → Webhook → Code Agent → 临时仓库 → Claude Code 容器 → PR
```

## 核心流程

### 1. Webhook 接收与解析

- 监听 GitHub Issue 评论事件
- 检测 `/code` 命令触发
- 解析 Issue 内容和上下文信息

### 2. 临时仓库准备

- 克隆目标仓库到临时目录
- 创建新分支（如 `Code-agent/issue-123`）
- 生成初始提交："Code-agent 已收到信息，准备开始实现目标 issue"
- 推送分支并创建 PR

### 3. Claude Code 执行

- 将临时目录挂载到 Claude Code 容器
- 传递 Issue 信息作为提示词
- 等待代码生成完成

### 4. 结果处理

- 检测临时目录的代码变更
- 生成新的提交记录
- 推送到已创建的 PR

## 系统组件

### Webhook 处理器

```go
type WebhookHandler struct {
    config     *Config
    agent      *Agent
    validator  *SignatureValidator
}

func (h *WebhookHandler) HandleIssueComment(w http.ResponseWriter, r *http.Request) {
    // 1. 验证 Webhook 签名
    // 2. 解析 Issue 评论事件
    // 3. 检查是否包含 /code 命令
    // 4. 创建 Agent 任务
    // 5. 异步执行代码生成
}
```

### Agent 核心

```go
type Agent struct {
    config     *Config
    github     *GitHubClient
    workspace  *WorkspaceManager
    claude     *ClaudeExecutor
}

func (a *Agent) ProcessIssue(issue *github.Issue) error {
    // 1. 准备临时工作空间
    workspace := a.workspace.Prepare(issue)

    // 2. 克隆仓库并创建分支
    branch := a.github.CreateBranch(workspace, issue)

    // 3. 创建初始 PR
    pr := a.github.CreatePullRequest(branch, issue)

    // 4. 执行 Claude Code
    result := a.claude.Execute(workspace, issue)

    // 5. 提交变更并更新 PR
    a.github.CommitAndPush(workspace, result)

    return nil
}
```

### 工作空间管理器

```go
type WorkspaceManager struct {
    baseDir    string
    tempDir    string
}

func (w *WorkspaceManager) Prepare(issue *github.Issue) *Workspace {
    // 1. 创建临时目录
    // 2. 克隆目标仓库
    // 3. 创建新分支
    // 4. 返回工作空间信息
}

func (w *WorkspaceManager) Cleanup(workspace *Workspace) {
    // 清理临时文件
}
```

### Claude Code 执行器

```go
type ClaudeExecutor struct {
    config     *Config
    docker     *DockerClient
}

func (c *ClaudeExecutor) Execute(workspace *Workspace, issue *github.Issue) *ExecutionResult {
    // 1. 构建 Claude Code 容器
    // 2. 挂载工作空间目录
    // 3. 传递 Issue 信息作为提示词
    // 4. 等待执行完成
    // 5. 返回执行结果
}
```

### GitHub 客户端

```go
type GitHubClient struct {
    client     *github.Client
    config     *Config
}

func (g *GitHubClient) CreateBranch(workspace *Workspace, issue *github.Issue) *github.Reference {
    // 1. 克隆仓库
    // 2. 创建新分支
    // 3. 生成初始提交
    // 4. 推送分支
}

func (g *GitHubClient) CreatePullRequest(branch *github.Reference, issue *github.Issue) *github.PullRequest {
    // 创建 PR 并关联 Issue
}

func (g *GitHubClient) CommitAndPush(workspace *Workspace, result *ExecutionResult) error {
    // 1. 检测文件变更
    // 2. 生成提交信息
    // 3. 提交并推送
}
```

## 核心数据结构

### 工作空间

```go
type Workspace struct {
    ID          string           `json:"id"`
    Path        string           `json:"path"`
    Repository  string           `json:"repository"`
    Branch      string           `json:"branch"`
    Issue       *github.Issue    `json:"issue"`
    CreatedAt   time.Time        `json:"created_at"`
}
```

### 执行结果

```go
type ExecutionResult struct {
    Success     bool             `json:"success"`
    Output      string           `json:"output"`
    Error       string           `json:"error,omitempty"`
    FilesChanged []string         `json:"files_changed"`
    Duration    time.Duration    `json:"duration"`
}
```

## 配置结构

```yaml
# config.yaml
server:
  port: 8080
  webhook_secret: "your-webhook-secret"

github:
  token: "ghp_your-github-token"
  webhook_url: "https://your-domain.com/webhook"

workspace:
  base_dir: "/tmp/Code-agent"
  cleanup_after: "24h"

claude:
  api_key: "sk-ant-your-claude-key"
  container_image: "anthropic/claude-code:latest"
  timeout: "30m"

docker:
  socket: "unix:///var/run/docker.sock"
  network: "bridge"
```

## 项目结构

```
Code-agent/
├── cmd/
│   └── server/
│       └── main.go              # 主程序入口
├── internal/
│   ├── webhook/
│   │   ├── handler.go           # Webhook 处理器
│   │   └── validator.go         # 签名验证器
│   ├── agent/
│   │   ├── agent.go             # Agent 核心逻辑
│   │   └── processor.go         # 任务处理器
│   ├── workspace/
│   │   ├── manager.go           # 工作空间管理
│   │   └── git.go               # Git 操作
│   ├── claude/
│   │   ├── executor.go          # Claude Code 执行器
│   │   └── docker.go            # Docker 客户端
│   ├── github/
│   │   ├── client.go            # GitHub API 客户端
│   │   └── pr.go                # PR 管理
│   └── config/
│       └── config.go            # 配置管理
├── pkg/
│   └── models/
│       ├── workspace.go         # 工作空间模型
│       └── result.go            # 执行结果模型
├── configs/
│   └── config.yaml              # 配置文件
├── Dockerfile                   # Agent 容器
├── docker-compose.yml           # 开发环境
└── README.md                    # 项目文档
```

## 部署配置

### Dockerfile

```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o Code-agent ./cmd/server

FROM alpine:latest
RUN apk --no-cache add ca-certificates git docker-cli
WORKDIR /root/
COPY --from=builder /app/xgo-agent .
COPY --from=builder /app/configs ./configs
EXPOSE 8080
CMD ["./xgo-agent"]
```

### docker-compose.yml

```yaml
version: "3.8"
services:
  xgo-agent:
    build: .
    ports:
      - "8080:8080"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - ./workspaces:/tmp/xgo-agent
      - ./configs:/root/configs
    environment:
      - GITHUB_TOKEN=${GITHUB_TOKEN}
      - CLAUDE_API_KEY=${CLAUDE_API_KEY}
    restart: unless-stopped
```

## 使用流程

### 1. 配置 GitHub Webhook

- URL: `https://your-domain.com/webhook`
- 事件: `Issue comments`
- 密钥: 与配置文件一致

### 2. 触发代码生成

- 在 GitHub Issue 评论中写: `/code 实现用户登录功能`
- 系统自动处理并创建 PR

### 3. 监控执行状态

- 查看 PR 中的提交记录
- 检查代码生成结果
- 根据需要调整和优化

## 关键特性

- **隔离执行**: 临时工作空间避免污染主仓库
- **容器化**: Claude Code 在独立容器中执行
- **异步处理**: Webhook 快速响应，后台处理任务
- **状态跟踪**: 完整的执行状态和日志记录
- **自动清理**: 定期清理临时文件和容器
- **错误处理**: 完善的错误处理和重试机制

## 扩展功能

- 支持多种触发命令（`/code`, `/continue`）
- 自定义代码生成模板
- 多仓库支持
- 执行历史记录
- Web 管理界面
- 监控和告警

这个设计提供了一个完整的自动化代码生成解决方案，能够有效地处理 GitHub Issue 并生成相应的代码实现。
