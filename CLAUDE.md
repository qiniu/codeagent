# CLAUDE.md

本文件为 Claude Code (claude.ai/code) 在处理此代码库时提供指导。

## 项目概述

**CodeAgent** 是一个基于 Go 的自动化代码生成和协作系统，通过 GitHub Webhooks 接收各种 AI 指令，自动处理 Issue 和 Pull Request 的代码生成、修改和审查任务。

## 架构设计

系统采用 webhook 驱动的架构：

```
GitHub 事件 (AI 指令) → Webhook → CodeAgent → 分支创建 → PR 处理 → AI 容器 → 代码生成/修改 → PR 更新
```

### 核心组件

- **Agent** (`internal/agent/agent.go`): 编排整个工作流程
- **Webhook Handler** (`internal/webhook/handler.go`): 处理 GitHub webhooks（Issue 和 PR）
- **Workspace Manager** (`internal/workspace/manager.go`): 管理临时 Git worktree
- **Code Providers** (`internal/code/`): 支持 Claude (Docker/CLI) 和 Gemini (Docker/CLI)
- **GitHub Client** (`internal/github/client.go`): 处理 GitHub API 交互

## 开发命令

### 构建和运行

```bash
# 构建二进制文件
make build

# 使用配置本地运行
./scripts/start.sh                    # Gemini + CLI 模式（默认）
./scripts/start.sh -p claude -d       # Claude + Docker 模式
./scripts/start.sh -p gemini -d       # Gemini + Docker 模式
./scripts/start.sh -p claude          # Claude + CLI 模式

# 直接 Go 运行
export GITHUB_TOKEN="your-token"
export GOOGLE_API_KEY="your-key"      # 或 CLAUDE_API_KEY
export WEBHOOK_SECRET="your-secret"
go run ./cmd/server --port 8888
```

### 测试

```bash
# 运行测试
make test

# 健康检查
curl http://localhost:8888/health

# 测试 webhook 处理
curl -X POST http://localhost:8888/hook \
  -H "Content-Type: application/json" \
  -H "X-GitHub-Event: issue_comment" \
  -d @test-data/issue-comment.json
```

### 配置

**必需的环境变量：**

- `GITHUB_TOKEN` - GitHub 个人访问令牌
- `WEBHOOK_SECRET` - GitHub webhook 密钥
- `GOOGLE_API_KEY` (用于 Gemini) 或 `CLAUDE_API_KEY` (用于 Claude)

**配置文件：** `config.yaml`

```yaml
code_provider: gemini # claude 或 gemini
use_docker: false # true 使用 Docker，false 使用 CLI
server:
  port: 8888
```

### 关键目录

- `cmd/server/` - 主应用程序入口点
- `internal/` - 核心业务逻辑
  - `agent/` - 主要编排逻辑
  - `webhook/` - GitHub webhook 处理
  - `workspace/` - Git worktree 管理
  - `code/` - AI 提供者实现 (Claude/Gemini)
  - `github/` - GitHub API 客户端
- `pkg/models/` - 共享数据结构
- `scripts/` - 实用脚本，包括 `start.sh`

### 开发工作流程

1. **本地开发**: 使用 CLI 模式 `./scripts/start.sh -p claude/gemini`
2. **测试**: 发送测试 webhooks 和示例 GitHub 事件
3. **Docker 开发**: 使用 Docker 模式进行容器化测试
4. **工作空间管理**: 临时 worktree 在 `/tmp/codeagent` 中创建，24 小时后自动清理

### 命令处理

系统支持多种 AI 指令，通过 GitHub 评论和 Review 触发：

**Issue 指令：**

- `/code <描述>` - 为 Issue 生成初始代码并创建 PR

**PR 协作指令：**

- `/continue <指令>` - 在 PR 中继续开发，支持自定义指令
- `/fix <描述>` - 修复 PR 中的代码问题

**支持场景：**

- Issue 评论中的指令处理
- PR 评论中的指令处理
- PR Review 评论中的指令处理
- PR Review 批量处理（支持多个评论的批量处理）

**指令特性：**

- 支持自定义参数和指令内容
- 自动获取历史评论上下文
- 智能代码提交和 PR 更新
- 完整的错误处理和重试机制

系统设计为可扩展架构，未来可以轻松添加新的指令类型和处理逻辑。

### 环境模式

- **Docker 模式**: 使用容器化的 Claude/Gemini，包含完整工具包
- **CLI 模式**: 使用本地安装的 Claude CLI 或 Gemini CLI（开发时更快）

### 常见问题

- Docker 模式需要确保 Docker 正在运行
- CLI 模式需要检查 CLI 工具是否已安装：`claude` 或 `gemini`
- 验证 GitHub webhook 配置与本地端口匹配
- 监控日志以排查工作空间清理问题
