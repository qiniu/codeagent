# CodeAgent

CodeAgent 是一个基于 Go 语言开发的自动化代码生成系统，通过 GitHub Webhook 接收 `/code` 命令，自动为 Issue 生成代码并创建 Pull Request。

## 功能特性

- 🤖 **智能代码生成**: 基于 Issue 描述自动生成代码
- 🔄 **GitHub 集成**: 通过 Webhook 接收命令，自动创建 PR
- ⚡ **即时响应**: 立即创建分支和 PR，提供进度跟踪
- 🐳 **容器化执行**: 使用 Docker 容器隔离执行环境
- 💻 **本地 CLI 模式**: 支持本地 Claude CLI 和 Gemini CLI，无需 Docker
- 🧹 **自动清理**: 智能管理临时工作空间，避免资源泄露
- 📊 **状态监控**: 实时监控系统状态和执行进度
- 🔒 **安全可靠**: 完善的错误处理和重试机制
- 🧠 **上下文感知**: Gemini CLI 模式自动构建完整上下文，提升代码质量

## 系统架构

```
GitHub Issue (/code) → Webhook → CodeAgent → 创建分支和空PR → Claude Code 容器 → 更新PR
```

### 工作流程

1. **接收命令**: 通过 GitHub Webhook 接收 `/code` 命令
2. **创建分支**: 立即创建分支并推送空的 "Initial plan" commit
3. **创建 PR**: 基于空 commit 创建 Pull Request，提供进度跟踪
4. **代码生成**: 在后台执行 Claude Code 生成代码
5. **Mock 测试**: 创建模拟文件用于测试二次提交流程
6. **更新 PR**: 将生成的代码作为新的 commit 推送到 PR

## 快速开始

### 环境要求

- Go 1.21+
- Git
- GitHub Personal Access Token
- **Docker 模式**: Docker（默认）
- **本地 CLI 模式**: Claude CLI 或 Gemini CLI

### 安装

1. **克隆项目**

```bash
git clone <your-repo-url>
cd codeagent
```

2. **安装依赖**

```bash
go mod tidy
```

### 配置

#### 方式一：命令行参数（推荐）

```bash
go run ./cmd/server \
  --github-token "your-github-token" \
  --claude-api-key "your-claude-api-key" \
  --webhook-secret "your-webhook-secret" \
  --port 8888
```

#### 方式二：环境变量

```bash
export GITHUB_TOKEN="your-github-token"
export CLAUDE_API_KEY="your-claude-api-key"
export WEBHOOK_SECRET="your-webhook-secret"
export PORT=8888

go run ./cmd/server
```

#### 方式三：配置文件（不包含敏感信息）

创建配置文件 `config.yaml`：

```yaml
server:
  port: 8888
  # webhook_secret: 通过命令行参数或环境变量设置

github:
  # token: 通过命令行参数或环境变量设置
  webhook_url: "http://localhost:8888/hook"

workspace:
  base_dir: "/tmp/codeagent"
  cleanup_after: "24h"

claude:
  # api_key: 通过命令行参数或环境变量设置
  container_image: "anthropic/claude-code:latest"
  timeout: "30m"

gemini:
  # api_key: 通过命令行参数或环境变量设置
  container_image: "google-gemini/gemini-cli:latest"
  timeout: "30m"

docker:
  socket: "unix:///var/run/docker.sock"
  network: "bridge"

# 代码提供者配置
code_provider: claude # 可选值: claude, gemini
use_docker: false # 是否使用 Docker，true 表示使用 Docker 容器，false 表示使用本地 CLI
```

**配置说明：**

- `code_provider`: 选择代码生成服务
  - `claude`: 使用 Anthropic Claude
  - `gemini`: 使用 Google Gemini
- `use_docker`: 选择执行方式
  - `true`: 使用 Docker 容器（推荐用于生产环境）
  - `false`: 使用本地 CLI（推荐用于开发环境）

**注意**: 敏感信息（如 token、api_key）应该通过命令行参数或环境变量设置，而不是写在配置文件中。

### 本地运行

#### 配置组合示例

**1. Claude + 本地 CLI 模式（默认）**

```bash
# 使用环境变量
export GITHUB_TOKEN="your-github-token"
export CLAUDE_API_KEY="your-claude-api-key"
export WEBHOOK_SECRET="your-webhook-secret"
export CODE_PROVIDER=claude
export USE_DOCKER=false
go run ./cmd/server

# 或使用配置文件
# config.yaml 中设置: code_provider: claude, use_docker: false
go run ./cmd/server --config config.yaml
```

**2. Claude + Docker 模式**

```bash
# 使用环境变量
export GITHUB_TOKEN="your-github-token"
export CLAUDE_API_KEY="your-claude-api-key"
export WEBHOOK_SECRET="your-webhook-secret"
export CODE_PROVIDER=claude
export USE_DOCKER=true
go run ./cmd/server

# 或使用配置文件
# config.yaml 中设置: code_provider: claude, use_docker: true
go run ./cmd/server --config config.yaml
```

**3. Gemini + Docker 模式**

```bash
# 使用环境变量
export GITHUB_TOKEN="your-github-token"
export GOOGLE_API_KEY="your-google-api-key"
export WEBHOOK_SECRET="your-webhook-secret"
export CODE_PROVIDER=gemini
export USE_DOCKER=true
go run ./cmd/server

# 或使用配置文件
# config.yaml 中设置: code_provider: gemini, use_docker: true
go run ./cmd/server --config config.yaml
```

**4. Gemini + 本地 CLI 模式（推荐开发环境）**

```bash
# 使用环境变量
export GITHUB_TOKEN="your-github-token"
export GOOGLE_API_KEY="your-google-api-key"
export WEBHOOK_SECRET="your-webhook-secret"
export CODE_PROVIDER=gemini
export USE_DOCKER=false
go run ./cmd/server

# 或使用配置文件
# config.yaml 中设置: code_provider: gemini, use_docker: false
go run ./cmd/server --config config.yaml
```

#### 使用启动脚本（推荐）

我们提供了一个便捷的启动脚本，支持所有配置组合：

```bash
# 设置环境变量
export GITHUB_TOKEN="your-github-token"
export GOOGLE_API_KEY="your-google-api-key"  # 或 CLAUDE_API_KEY
export WEBHOOK_SECRET="your-webhook-secret"

# 使用启动脚本
./scripts/start.sh                    # Gemini + 本地 CLI 模式（默认）
./scripts/start.sh -p claude          # Claude + 本地 CLI 模式
./scripts/start.sh -p claude -d       # Claude + Docker 模式
./scripts/start.sh -p gemini -d       # Gemini + Docker 模式

# 查看帮助
./scripts/start.sh --help
```

启动脚本会自动检查环境依赖并设置相应的环境变量。

**注意**:

- 本地 CLI 模式需要预先安装 Claude CLI 或 Gemini CLI 工具
- Gemini CLI 模式使用单次 prompt 方式，每次调用都会启动新的进程，避免了 broken pipe 错误
- Gemini CLI 会自动构建包含项目上下文、Issue 信息和对话历史的完整 prompt，提供更好的代码生成质量

2. **测试健康检查**

```bash
curl http://localhost:8888/health
```

3. **配置 GitHub Webhook**
   - URL: `http://your-domain.com/hook`
   - 事件: `Issue comments`, `Pull request reviews`
   - 密钥: 与配置文件中的 `webhook_secret` 一致

### 使用示例

1. **在 GitHub Issue 中触发代码生成**

```
/code 实现用户登录功能，包括用户名密码验证和 JWT token 生成
```

2. **在 PR 评论中继续开发**

```
/continue 添加单元测试
```

3. **修复代码问题**

```
/fix 修复登录验证逻辑中的 bug
```

## 本地开发

### 项目结构

```
codeagent/
├── cmd/
│   └── server/
│       └── main.go              # 主程序入口
├── internal/
│   ├── webhook/
│   │   └── handler.go           # Webhook 处理器
│   ├── agent/
│   │   └── agent.go             # Agent 核心逻辑
│   ├── workspace/
│   │   └── manager.go           # 工作空间管理
│   ├── claude/
│   │   └── executor.go          # Claude Code 执行器
│   ├── github/
│   │   └── client.go            # GitHub API 客户端
│   └── config/
│       └── config.go            # 配置管理
├── pkg/
│   └── models/
│       └── workspace.go         # 数据模型
├── docs/
│   └── xgo-agent.md             # 设计文档
├── config.yaml                  # 配置文件
├── go.mod                       # Go 模块文件
└── README.md                    # 项目文档
```

3. **构建**

```bash
# 构建二进制文件
go build -o bin/codeagent ./cmd/server

# 交叉编译
GOOS=linux GOARCH=amd64 go build -o bin/codeagent-linux ./cmd/server
```

**集成测试**

```bash
# 启动测试服务器
go run ./cmd/server --config test-config.yaml

# 发送测试 Webhook
curl -X POST http://localhost:8888/hook \
  -H "Content-Type: application/json" \
  -H "X-GitHub-Event: issue_comment" \
  -d @test-data/issue-comment.json
```

### 调试

1. **日志级别**

```bash
# 设置详细日志
export LOG_LEVEL=debug
go run ./cmd/server --config config.yaml
```
