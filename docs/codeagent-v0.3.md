# CodeAgent v0.3 - 本地 CLI 模式与架构优化

## 概述

CodeAgent v0.3 是一个重要的架构优化版本，主要改进包括：

1. **本地 CLI 模式支持**：支持直接使用本地安装的 Claude CLI 或 Gemini CLI
2. **Gemini 实现重构**：简化了 Gemini 实现，移除了复杂的 prompt 构建逻辑
3. **Prompt 优化**：修复了 Gemini CLI 主动访问 GitHub API 的问题
4. **GitHub API 兼容性**：修复了 PR 评论创建的 API 兼容性问题
5. **错误处理改进**：添加了重试机制和更好的错误处理

## 主要特性

### 1. 双模式支持

CodeAgent 现在支持两种运行模式：

- **Docker 模式**（默认）：使用 Docker 容器运行 Claude Code 或 Gemini CLI
- **本地 CLI 模式**：直接使用本地安装的 Claude CLI 或 Gemini CLI

### 2. Gemini 实现支持双模式

#### 架构改进

- **Docker 模式保持交互式**：使用 stdin/stdout 管道与持续运行的容器交互
- **本地 CLI 模式使用单次调用**：使用 `gemini --prompt` 命令，每次启动新进程
- **依赖原生文件上下文**：Gemini CLI 自动读取工作目录中的文件作为上下文
- **避免 broken pipe 问题**：本地单次调用模式避免了持续交互导致的连接断开
- **Claude 保持简单**：只支持 Docker 模式，避免未测试的功能

#### 代码结构

```
internal/code/
├── code.go           # 工厂函数，根据配置选择实现
├── claude.go         # Claude 本地 CLI 实现
├── gemini.go         # Gemini 本地 CLI 实现
└── gemini_docker.go  # Gemini Docker 实现
```

#### 实现方式对比

**Claude 实现（交互式模式）：**

```go
type claudeCode struct {
    cmd    *exec.Cmd
    stdin  io.WriteCloser
    stdout io.ReadCloser
}

func (c *claudeCode) Prompt(message string) (*Response, error) {
    if _, err := c.stdin.Write([]byte(message + "\n")); err != nil {
        return nil, err
    }
    return &Response{Out: c.stdout}, nil
}
```

**Gemini 本地实现（单次调用模式）：**

```go
type geminiLocal struct {
    workspace *models.Workspace
    config    *config.Config
}

func (g *geminiLocal) Prompt(message string) (*Response, error) {
    output, err := g.executeGeminiLocal(message)
    if err != nil {
        return nil, fmt.Errorf("failed to execute gemini prompt: %w", err)
    }
    return &Response{Out: bytes.NewReader(output)}, nil
}
```

### 3. Prompt 优化

#### 问题修复

- **避免网络请求**：不再在 prompt 中包含 GitHub API URL
- **直接传递内容**：将 Issue 标题和描述直接传递给 AI
- **提高可靠性**：不依赖外部网络连接和 API 权限

#### 改进前后对比

```go
// 之前 - 包含 URL，导致 Gemini CLI 主动访问
prompt := fmt.Sprintf("这是 Issue 内容 %s ，根据 Issue 内容，整理出修改计划", issue.GetURL())

// 现在 - 直接传递内容
prompt := fmt.Sprintf(`这是 Issue 内容：

标题：%s
描述：%s

请根据以上 Issue 内容，整理出修改计划。`, issue.GetTitle(), issue.GetBody())
```

### 4. GitHub API 兼容性修复

#### PR 评论 API 修复

- **问题**：GitHub API 的 PR 评论接口发生变化，`PullRequests.CreateComment` 需要额外的定位参数
- **解决方案**：使用 `Issues.CreateComment` API，因为 PR 实际上也是一种 Issue

```go
// 之前
comment := &github.PullRequestComment{Body: &commentBody}
_, _, err := c.client.PullRequests.CreateComment(ctx, repoOwner, repoName, pr.GetNumber(), comment)

// 现在
comment := &github.IssueComment{Body: &commentBody}
_, _, err := c.client.Issues.CreateComment(ctx, repoOwner, repoName, pr.GetNumber(), comment)
```

### 5. 错误处理改进

#### 重试机制

添加了 `promptWithRetry` 方法，支持自动重试：

```go
func (a *Agent) promptWithRetry(code code.Code, prompt string, maxRetries int) (*code.Response, error) {
    for attempt := 1; attempt <= maxRetries; attempt++ {
        resp, err := code.Prompt(prompt)
        if err == nil {
            return resp, nil
        }

        // 特殊处理 broken pipe 错误
        if strings.Contains(err.Error(), "broken pipe") {
            log.Infof("Detected broken pipe, will retry...")
        }

        if attempt < maxRetries {
            time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
        }
    }
    return nil, fmt.Errorf("failed after %d attempts", maxRetries)
}
```

## 配置选项

### 新增配置

```yaml
# config.yaml
code_provider: gemini # claude 或 gemini
use_docker: false # true: Docker 模式, false: 本地 CLI 模式
```

```bash
# 环境变量
export USE_DOCKER=false      # 启用本地 CLI 模式
export CODE_PROVIDER=gemini  # 或 claude
```

### 配置加载逻辑

```go
type Config struct {
    // ... 其他配置
    CodeProvider string `yaml:"code_provider"`
    UseDocker    bool   `yaml:"use_docker"`
}

func (c *Config) loadFromEnv() {
    // 从环境变量加载配置
    if useDockerStr := os.Getenv("USE_DOCKER"); useDockerStr != "" {
        if useDocker, err := strconv.ParseBool(useDockerStr); err == nil {
            c.UseDocker = useDocker
        }
    }
    // 默认值：UseDocker = true
}
```

## 代码架构

### 工厂模式重构

```go
func New(workspace *models.Workspace, cfg *config.Config) (Code, error) {
    // 根据 code provider 和 use_docker 配置创建相应的代码提供者
    switch cfg.CodeProvider {
    case ProviderClaude:
        if cfg.UseDocker {
            return NewClaudeDocker(workspace, cfg)
        }
        return NewClaudeLocal(workspace, cfg)
    case ProviderGemini:
        if cfg.UseDocker {
            return NewGeminiDocker(workspace, cfg)
        }
        return NewGeminiLocal(workspace, cfg)
    default:
        return nil, fmt.Errorf("unsupported code provider: %s", cfg.CodeProvider)
    }
}
```

### 执行模式说明

| 提供者 | Docker 模式 | 本地 CLI 模式 | 执行方式                                                  |
| ------ | ----------- | ------------- | --------------------------------------------------------- |
| Claude | 交互式      | 不支持        | stdin/stdout 管道                                         |
| Gemini | 交互式      | 单次调用      | Docker: stdin/stdout 管道<br>本地: `gemini --prompt` 命令 |

### Gemini 实现简化

#### 本地 CLI 实现

```go
type geminiLocal struct {
    workspace *models.Workspace
    config    *config.Config
}

func (g *geminiLocal) Prompt(message string) (*Response, error) {
    // 直接执行本地 gemini CLI 调用
    output, err := g.executeGeminiLocal(message)
    if err != nil {
        return nil, fmt.Errorf("failed to execute gemini prompt: %w", err)
    }

    return &Response{Out: bytes.NewReader(output)}, nil
}

func (g *geminiLocal) executeGeminiLocal(prompt string) ([]byte, error) {
    args := []string{"--prompt", prompt}
    cmd := exec.CommandContext(ctx, "gemini", args...)
    cmd.Dir = g.workspace.Path // 设置工作目录，Gemini CLI 会自动读取该目录的文件作为上下文

    // 执行命令并获取输出
    output, err := cmd.CombinedOutput()
    // ... 错误处理
    return output, nil
}
```

#### Docker 实现

```go
type geminiDocker struct {
    workspace *models.Workspace
    config    *config.Config
}

func (g *geminiDocker) executeGeminiDocker(prompt string) ([]byte, error) {
    args := []string{
        "run", "--rm",
        "-v", fmt.Sprintf("%s:/workspace", g.workspace.Path),
        "-w", "/workspace", // 设置工作目录，Gemini CLI 会自动读取该目录的文件作为上下文
        g.config.Gemini.ContainerImage,
        "gemini", "--prompt", prompt,
    }

    cmd := exec.CommandContext(ctx, "docker", args...)
    // ... 执行命令
}
```

## 使用方法

### 1. 安装本地 CLI 工具

#### Gemini CLI

```bash
# 安装 Gemini CLI
# 参考: https://github.com/google-gemini/gemini-cli
```

#### Claude CLI

```bash
# 安装 Claude CLI
# 参考: https://github.com/anthropics/anthropic-claude-code
```

### 2. 配置环境变量

```bash
# 设置本地模式
export USE_DOCKER=false
export CODE_PROVIDER=gemini  # 或 claude

# 设置必要的认证信息
export GITHUB_TOKEN="your-github-token"
export GEMINI_API_KEY="your-gemini-api-key"  # 或 CLAUDE_API_KEY
export WEBHOOK_SECRET="your-webhook-secret"
```

### 3. 启动服务

```bash
# 使用环境变量
go run ./cmd/server

# 或使用配置文件
# 在 config.yaml 中设置 use_docker: false
go run ./cmd/server --config config.yaml
```

### 4. 测试本地模式

使用提供的测试脚本：

```bash
./scripts/test-local-mode.sh
```

## 优势

### 本地 CLI 模式的优势

1. **更快的启动速度**：无需启动 Docker 容器
2. **更少的资源消耗**：不需要 Docker 运行时
3. **更好的调试体验**：可以直接调试本地 CLI 工具
4. **更简单的部署**：不需要 Docker 环境
5. **避免 broken pipe**：单次 prompt 模式避免了持续交互的问题

### 架构优化的优势

1. **多种执行模式**：支持交互式模式（Claude、Gemini Docker）和单次调用模式（Gemini 本地）
2. **避免 broken pipe 问题**：Gemini 本地单次调用模式避免了持续交互导致的连接断开
3. **更可靠**：Gemini 本地每次都是全新的进程，避免了状态累积导致的问题
4. **更高效**：Gemini CLI 原生支持文件上下文，性能更好
5. **更好的错误处理**：添加了重试机制和特殊错误处理
6. **保持简单**：Claude 只支持 Docker 模式，避免未测试的功能

### Docker 模式的优势

1. **环境隔离**：完全隔离的执行环境
2. **一致性**：确保在不同机器上运行相同的环境
3. **易于管理**：统一的容器管理

## 配置示例

### 环境变量配置

```bash
# .env 文件
USE_DOCKER=false
CODE_PROVIDER=gemini
GITHUB_TOKEN=your-github-token
GEMINI_API_KEY=your-gemini-api-key
WEBHOOK_SECRET=your-webhook-secret
PORT=8888
```

### 配置文件

```yaml
# config.yaml
server:
  port: 8888

github:
  webhook_url: "http://localhost:8888/hook"

workspace:
  base_dir: "/tmp/codeagent"
  cleanup_after: "24h"

claude:
  container_image: "anthropic/claude-code:latest"
  timeout: "30m"

gemini:
  container_image: "google-gemini/gemini-cli:latest"
  timeout: "30m"

docker:
  socket: "unix:///var/run/docker.sock"
  network: "bridge"

code_provider: gemini
use_docker: false # 本地 CLI 模式
```

## 故障排除

### 常见问题

1. **CLI 工具未找到**

   ```bash
   # 检查是否安装
   which gemini
   which claude
   ```

2. **权限问题**

   ```bash
   # 确保 CLI 工具有执行权限
   chmod +x $(which gemini)
   chmod +x $(which claude)
   ```

3. **工作空间访问问题**

   ```bash
   # 确保工作空间目录存在且有写权限
   mkdir -p /tmp/codeagent
   chmod 755 /tmp/codeagent
   ```

4. **API 密钥问题**
   ```bash
   # 确保设置了正确的 API 密钥
   export GEMINI_API_KEY="your-api-key"
   export GOOGLE_API_KEY="your-api-key"  # 某些情况下需要
   ```

### 调试模式

```bash
# 启用详细日志
export LOG_LEVEL=debug
go run ./cmd/server --config config.yaml
```

## 版本历史

### v0.3 主要改进

1. **Gemini 支持双模式**：Docker 模式保持交互式，本地 CLI 模式使用单次调用
2. **本地 CLI 支持**：添加了本地 CLI 模式，支持直接使用本地工具
3. **Prompt 优化**：修复了 Gemini CLI 主动访问 GitHub API 的问题
4. **API 兼容性**：修复了 PR 评论创建的 GitHub API 兼容性问题
5. **错误处理**：添加了重试机制和更好的错误处理
6. **配置优化**：添加了 `use_docker` 配置选项

### 向后兼容性

- 保持了与现有配置的兼容性
- Docker 模式仍然是默认模式
- 现有的 API 接口保持不变
