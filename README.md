# CodeAgent

[![Go Report Card](https://goreportcard.com/badge/github.com/qiniu/codeagent)](https://goreportcard.com/report/github.com/qiniu/codeagent)
[![Go Version](https://img.shields.io/github/go-mod/go-version/qiniu/codeagent)](https://go.dev/)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![CI](https://github.com/qiniu/codeagent/workflows/CI/badge.svg)](https://github.com/qiniu/codeagent/actions)

**CodeAgent** is a Go-based AI-powered automated code generation and collaboration system that seamlessly integrates with GitHub. It receives AI instructions through GitHub webhooks and automatically handles code generation, modification, and review tasks for Issues and Pull Requests.

## 🚀 Key Features

- 🤖 **Multiple AI Providers**: Support for Anthropic Claude and Google Gemini
- 🔄 **GitHub Integration**: Automatic processing of Issues and Pull Requests
- 🐳 **Flexible Deployment**: Docker containers or local CLI execution
- 📁 **Smart Workspace Management**: Git worktree-based isolated environments
- 🔐 **Security First**: Webhook signature verification and secure token handling
- ⚡ **Real-time Processing**: Instant response to GitHub events

## 📋 Table of Contents

- [Architecture Overview](#architecture-overview)
- [Getting Started](#getting-started)
  - [Prerequisites](#prerequisites)
  - [Installation](#installation)
  - [Quick Setup](#quick-setup)
- [Configuration](#configuration)
  - [Environment Variables](#environment-variables)
  - [Configuration File](#configuration-file)
- [Usage](#usage)
  - [GitHub Integration](#github-integration)
  - [Command Reference](#command-reference)
  - [Examples](#examples)
- [Development](#development)
  - [Project Structure](#project-structure)
  - [Building](#building)
  - [Testing](#testing)
- [Troubleshooting](#troubleshooting)
- [Contributing](#contributing)
- [License](#license)

## 🏗️ Architecture Overview

CodeAgent uses a webhook-driven architecture for seamless GitHub integration:

```
GitHub Events → Webhook → CodeAgent → Workspace Creation → AI Processing → Code Generation → PR Updates
```

### Core Components

- **Agent** (`internal/agent/`): Orchestrates the entire workflow
- **Webhook Handler** (`internal/webhook/`): Processes GitHub webhooks (Issues and PRs)
- **Workspace Manager** (`internal/workspace/`): Manages temporary Git worktrees
- **AI Providers** (`internal/code/`): Claude and Gemini integration (Docker/CLI modes)
- **GitHub Client** (`internal/github/`): Handles GitHub API interactions

## 🚀 Getting Started

### Prerequisites

**Required:**
- Go 1.21 or higher
- Git
- GitHub Personal Access Token
- AI Provider API Key (Claude or Gemini)

### Installation

```bash
# Clone the repository
git clone https://github.com/qiniu/codeagent.git
cd codeagent

# Download dependencies
go mod download
```

### Quick Setup

```bash
# Set environment variables
export GITHUB_TOKEN="your-github-token"
export CLAUDE_API_KEY="your-claude-api-key"
export WEBHOOK_SECRET="your-webhook-secret"

# Run the server
go run ./cmd/server --port 8888
```

**Health Check**

```bash
curl http://localhost:8888/health
```

## ⚙️ Configuration

### Environment Variables

| Variable | Description | Required | Example |
|----------|-------------|----------|---------|
| `GITHUB_TOKEN` | GitHub Personal Access Token | Yes | `ghp_xxxxxxxxxxxx` |
| `WEBHOOK_SECRET` | GitHub Webhook Secret | Yes | `your-strong-secret` |
| `CODE_PROVIDER` | AI provider (claude/gemini) | No | `claude` |
| `USE_DOCKER` | Use Docker containers | No | `true` |
| `PORT` | Server port | No | `8888` |
| `LOG_LEVEL` | Logging level | No | `debug` |

### Configuration File

Create `config.yaml` in the project root:

```yaml
# Server configuration
server:
  port: 8888

# GitHub integration
github:
  webhook_url: "http://localhost:8888/hook"

# Workspace settings
workspace:
  base_dir: "./workspace"  # Supports relative paths
  cleanup_after: "24h"

# AI provider selection
code_provider: claude  # Options: claude, gemini
use_docker: false      # true = Docker, false = CLI

# Claude configuration
claude:
  container_image: "goplusorg/codeagent:v0.4"
  timeout: "30m"

# Gemini configuration  
gemini:
  container_image: "goplusorg/codeagent:v0.4"
  timeout: "30m"

```


## 📖 Usage

### GitHub Integration

**1. Configure GitHub Webhook**

Go to your repository settings → Webhooks → Add webhook:

- **URL**: `https://your-domain.com/hook`
- **Content type**: `application/json`
- **Secret**: Same as your `WEBHOOK_SECRET`
- **Events**: Select `Issue comments`, `Pull request reviews`, `Pull requests`

**2. Webhook Security**

CodeAgent supports GitHub webhook signature verification:
- SHA-256 verification (recommended)
- SHA-1 backward compatibility
- Constant-time comparison to prevent timing attacks
- Development mode: verification skipped if secret not configured

### Command Reference

Use these commands in GitHub Issue comments or PR discussions:

| Command | Description | Example |
|---------|-------------|---------|
| `/code [description]` | Generate code for an Issue | `/code Implement user authentication with JWT` or `/code` |
| `/continue <instruction>` | Continue development in PR | `/continue Add unit tests for the login function` |

### Examples

**1. Create New Feature**
```
# In a GitHub Issue comment:
/code Implement user login functionality including username/password validation and JWT token generation
```

**2. Enhance Existing Code**
```  
# In a PR comment:
/continue Add comprehensive error handling and input validation
```


## 🛠️ Development

### Project Structure

```
codeagent/
├── cmd/
│   └── server/                 # Application entry point
├── internal/
│   ├── agent/                  # Core orchestration logic
│   ├── code/                   # AI provider implementations
│   ├── config/                 # Configuration management
│   ├── context/                # Context collection and formatting
│   ├── events/                 # Event parsing
│   ├── github/                 # GitHub API client
│   ├── interaction/            # User interaction handling
│   ├── mcp/                    # MCP (Model Context Protocol) support
│   ├── modes/                  # Processing mode handlers
│   ├── webhook/                # GitHub webhook handling
│   └── workspace/              # Git workspace management
├── pkg/
│   ├── models/                 # Shared data structures
│   └── signature/              # Webhook signature verification
├── test/
│   └── integration/            # Integration tests
├── docs/                       # Documentation
├── config.example.yaml         # Example configuration
└── README.md                   # This file
```

### Building

```bash
# Build for current platform
make build

# Or manually
go build -o bin/codeagent ./cmd/server

# Cross-compilation
GOOS=linux GOARCH=amd64 go build -o bin/codeagent-linux ./cmd/server
```

### Testing

```bash
# Run all tests
make test

# Integration testing
go run ./cmd/server --config test-config.yaml

# Test webhook endpoint
curl -X POST http://localhost:8888/hook \
  -H "Content-Type: application/json" \
  -H "X-GitHub-Event: issue_comment" \
  -d @test-data/issue-comment.json
```



## 🔧 Troubleshooting

**Common Issues**

| Issue | Symptom | Solution |
|-------|---------|----------|
| Webhook not received | No response to GitHub commands | Check webhook URL and secret configuration |
| AI provider timeout | Long delays or timeouts | Increase timeout in config, check API key |
| Docker issues | Container startup failures | Ensure Docker daemon is running |
| CLI not found | Command not found errors | Install Claude/Gemini CLI tools |
| Permission denied | Git operations fail | Check GitHub token permissions |
| Workspace cleanup | Disk space issues | Monitor workspace directory, adjust cleanup_after |

**Debug Commands**
```bash
# Check server status
curl http://localhost:8888/health

# View server logs
export LOG_LEVEL=debug
./scripts/start.sh

# Test webhook manually
curl -X POST http://localhost:8888/hook \
  -H "Content-Type: application/json" \
  -H "X-GitHub-Event: ping" \
  -d '{"zen":"GitHub zen message"}'
```

## 🤝 Contributing

We welcome contributions! Here's how you can help:

**Ways to Contribute**
- 🐛 [Report Bugs](https://github.com/qiniu/codeagent/issues/new?template=bug_report.md)
- 💡 [Feature Requests](https://github.com/qiniu/codeagent/issues/new?template=feature_request.md)
- 📝 [Improve Documentation](https://github.com/qiniu/codeagent/issues/new?template=documentation.md)
- 🔧 [Submit Code](CONTRIBUTING.md#code-contributions)

Please read our [Contributing Guide](CONTRIBUTING.md) for detailed information about the development process.

## 📄 License

This project is licensed under the [Apache License 2.0](LICENSE).

## 🙏 Acknowledgments

Thank you to all developers and users who have contributed to making CodeAgent better!

---

**Need help?** Check our [documentation](docs/) or [open an issue](https://github.com/qiniu/codeagent/issues/new).