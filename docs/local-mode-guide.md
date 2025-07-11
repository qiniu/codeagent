# 本地模式使用指南

本指南详细说明如何在本地环境中运行 CodeAgent，无需 Docker 依赖。

## 前置条件

### 1. 系统要求

- Go 1.21+
- Git
- Node.js 18+
- GitHub Personal Access Token

### 2. 安装 CLI 工具

根据你选择的代码提供者，安装相应的 CLI 工具：

#### Claude CLI

```bash
npm install -g @anthropic-ai/claude-code
```

#### Gemini CLI

```bash
npm install -g @google/gemini-cli
```

## 配置

### 1. 环境变量配置

创建环境变量文件或直接在终端中设置：

```bash
# 必需的环境变量
export GITHUB_TOKEN="your-github-token"
export WEBHOOK_SECRET="your-webhook-secret"

# 根据代码提供者选择其一
export CLAUDE_API_KEY="your-claude-api-key"    # 使用 Claude
export GOOGLE_API_KEY="your-google-api-key"    # 使用 Gemini

# 可选的环境变量
export CODE_PROVIDER="claude"   # 或 "gemini"
export USE_DOCKER="false"       # 使用本地 CLI
export PORT="8888"              # 服务器端口
```

### 2. 配置文件

创建 `config.yaml` 文件：

```yaml
server:
  port: 8888

github:
  webhook_url: "http://localhost:8888/hook"

workspace:
  base_dir: "/tmp/codeagent"
  cleanup_after: "24h"

claude:
  timeout: "30m"

gemini:
  timeout: "30m"

# 代码提供者配置
code_provider: claude # 可选值: claude, gemini
use_docker: false     # 使用本地 CLI
```

## 启动方式

### 方式一：使用启动脚本（推荐）

```bash
# 设置环境变量
export GITHUB_TOKEN="your-github-token"
export CLAUDE_API_KEY="your-claude-api-key"
export WEBHOOK_SECRET="your-webhook-secret"

# 启动服务
./scripts/start.sh                    # Claude + 本地 CLI 模式
./scripts/start.sh -p gemini          # Gemini + 本地 CLI 模式
./scripts/start.sh -p claude -d       # Claude + Docker 模式
./scripts/start.sh -p gemini -d       # Gemini + Docker 模式
```

### 方式二：直接运行

```bash
# 使用环境变量
go run ./cmd/server

# 使用命令行参数
go run ./cmd/server \
  --github-token "your-github-token" \
  --claude-api-key "your-claude-api-key" \
  --webhook-secret "your-webhook-secret"
```

### 方式三：使用二进制文件

```bash
# 构建二进制文件
make build

# 运行
./bin/codeagent --github-token "your-github-token" \
                --claude-api-key "your-claude-api-key" \
                --webhook-secret "your-webhook-secret"
```

## 测试

### 1. 健康检查

```bash
curl http://localhost:8888/health
```

### 2. 使用测试脚本

```bash
# 设置环境变量
export GITHUB_TOKEN="your-github-token"
export CLAUDE_API_KEY="your-claude-api-key"
export WEBHOOK_SECRET="your-webhook-secret"

# 运行测试
./scripts/test-local-mode.sh
```

## 配置组合

| 代码提供者 | 执行方式 | 环境变量 | 说明 |
|-----------|----------|----------|------|
| Claude | 本地 CLI | `CODE_PROVIDER=claude`, `USE_DOCKER=false` | 推荐开发环境 |
| Claude | Docker | `CODE_PROVIDER=claude`, `USE_DOCKER=true` | 生产环境 |
| Gemini | 本地 CLI | `CODE_PROVIDER=gemini`, `USE_DOCKER=false` | 推荐开发环境 |
| Gemini | Docker | `CODE_PROVIDER=gemini`, `USE_DOCKER=true` | 生产环境 |

## 优势

### 本地 CLI 模式优势

1. **更快的启动时间** - 无需拉取和启动 Docker 容器
2. **更低的资源消耗** - 不需要额外的 Docker 守护进程
3. **更简单的调试** - 直接在宿主机上运行，便于调试
4. **更好的文件访问** - 避免了 Docker 挂载的权限问题

### Docker 模式优势

1. **环境一致性** - 完全隔离的运行环境
2. **依赖管理** - 所有依赖预装在镜像中
3. **安全性** - 沙盒化执行环境
4. **生产就绪** - 适合生产环境部署

## 故障排除

### 1. CLI 工具未找到

```bash
# 检查 Claude CLI
claude --version

# 检查 Gemini CLI
gemini --version
```

### 2. API 密钥错误

确保设置了正确的 API 密钥：

- Claude: `CLAUDE_API_KEY`
- Gemini: `GOOGLE_API_KEY` 或 `GEMINI_API_KEY`

### 3. 网络问题

本地模式依赖网络连接访问 AI 服务，确保：

- 网络连接正常
- 防火墙允许 HTTPS 出站连接
- 代理配置正确（如果需要）

### 4. 权限问题

确保：

- 工作目录有写权限
- CLI 工具有执行权限
- 环境变量正确设置

## 性能优化

### 1. 工作目录选择

- 避免使用 `/tmp` 目录（在 macOS 上可能有问题）
- 使用 SSD 存储提高 I/O 性能
- 定期清理临时文件

### 2. 超时设置

根据项目复杂度调整超时时间：

```yaml
claude:
  timeout: "30m"  # 大型项目可能需要更长时间

gemini:
  timeout: "30m"
```

### 3. 并发控制

本地模式支持并发处理多个请求，但要注意：

- 系统资源限制
- API 调用频率限制
- 网络带宽限制