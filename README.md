# CodeAgent

CodeAgent 是一个基于 AI 的代码代理，能够自动处理 GitHub Issue 和 Pull Request，生成代码修改建议。

## 功能特性

- 🤖 支持多种 AI 模型（Claude、Gemini）
- 🔄 自动处理 GitHub Issue 和 Pull Request
- 🐳 Docker 容器化执行环境
- 🔒 GitHub Webhook 签名验证
- 📁 基于 Git Worktree 的工作空间管理
- 🛠️ 灵活的配置选项，支持相对路径

## 快速开始

### 安装

```bash
git clone https://github.com/qbox/codeagent.git
cd codeagent
go mod download
```

### 配置

#### 方式一：命令行参数

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

#### 方式三：配置文件（推荐）

创建配置文件 `config.yaml`：

```yaml
server:
  port: 8888
  # webhook_secret: 通过命令行参数或环境变量设置

github:
  # token: 通过命令行参数或环境变量设置
  webhook_url: "http://localhost:8888/hook"

workspace:
  base_dir: "./codeagent" # 支持相对路径！
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
use_docker: true # 是否使用 Docker，false 表示使用本地 CLI
```

**配置说明：**

- `code_provider`: 选择代码生成服务
  - `claude`: 使用 Anthropic Claude
  - `gemini`: 使用 Google Gemini
- `use_docker`: 选择执行方式
  - `true`: 使用 Docker 容器（推荐用于生产环境）
  - `false`: 使用本地 CLI（推荐用于开发环境）

**注意**: 敏感信息（如 token、api_key、webhook_secret）应该通过命令行参数或环境变量设置，而不是写在配置文件中。

### 相对路径支持

CodeAgent 现在支持在配置文件中使用相对路径，提供更灵活的配置选项：

```yaml
workspace:
  base_dir: "./codeagent"     # 相对于配置文件目录
  # 或者
  base_dir: "../workspace"    # 相对于配置文件目录的上级目录
  # 或者
  base_dir: "/tmp/codeagent"  # 绝对路径（保持不变）
```

相对路径会在配置加载时自动转换为绝对路径，详情请参考 [相对路径支持文档](docs/relative-path-support.md)。

### 安全配置

#### Webhook 签名验证

为了防止 webhook 接口被恶意利用，CodeAgent 支持 GitHub Webhook 签名验证功能：

1. **配置 webhook secret**:

   ```bash
   # 方式1: 环境变量（推荐）
   export WEBHOOK_SECRET="your-strong-secret-here"

   # 方式2: 命令行参数
   go run ./cmd/server --webhook-secret "your-strong-secret-here"
   ```

2. **GitHub Webhook 设置**:

   - 在 GitHub 仓库设置中添加 Webhook
   - URL: `https://your-domain.com/hook`
   - Content type: `application/json`
   - Secret: 输入与 `WEBHOOK_SECRET` 相同的值
   - 选择事件: `Issue comments`, `Pull request reviews`, `Pull requests`

3. **签名验证机制**:
   - 支持 SHA-256 签名验证（优先）
   - 向下兼容 SHA-1 签名验证
   - 使用恒定时间比较防止时间攻击
   - 如果未配置 `webhook_secret`，则跳过签名验证（仅用于开发环境）

#### 安全建议

- 使用强密码作为 webhook secret（建议 32 字符以上）
- 在生产环境中务必配置 webhook secret
- 使用 HTTPS 保护 webhook 端点
- 定期轮换 API 密钥和 webhook secret
- 限制 GitHub Token 的权限范围

### 本地运行

#### 配置组合示例

**1. Claude + Docker 模式（默认）**

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

**2. Claude + 本地 CLI 模式**

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
./scripts/start.sh -p claude -d       # Claude + Docker 模式
./scripts/start.sh -p gemini -d       # Gemini + Docker 模式
./scripts/start.sh -p claude          # Claude + 本地 CLI 模式

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
   - 密钥: 与配置文件中的 `webhook_secret` 一致（用于签名验证）
   - 推荐使用 HTTPS 和强密码来保证安全性

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
