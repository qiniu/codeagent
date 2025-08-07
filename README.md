# CodeAgent

[![Go Report Card](https://goreportcard.com/badge/github.com/qiniu/codeagent)](https://goreportcard.com/report/github.com/qiniu/codeagent)
[![Go Version](https://img.shields.io/github/go-mod/go-version/qiniu/codeagent)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
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
  - [AI Provider Setup](#ai-provider-setup)
- [Usage](#usage)
  - [GitHub Integration](#github-integration)
  - [Command Reference](#command-reference)
  - [Examples](#examples)
- [Development](#development)
  - [Project Structure](#project-structure)
  - [Building](#building)
  - [Testing](#testing)
  - [Debugging](#debugging)
- [Deployment](#deployment)
- [Security](#security)
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

**Optional (for Docker mode):**
- Docker Engine
- Docker CLI tools

**Optional (for CLI mode):**
- Claude CLI (`claude`)
- Gemini CLI (`gemini`)

### Installation

```bash
# Clone the repository
git clone https://github.com/qiniu/codeagent.git
cd codeagent

# Download dependencies
go mod download
```

### Quick Setup

**Using the Start Script (Recommended)**

```bash
# Set your credentials
export GITHUB_TOKEN="your-github-token"
export GOOGLE_API_KEY="your-google-api-key"  # or CLAUDE_API_KEY
export WEBHOOK_SECRET="your-webhook-secret"

# Start with default settings (Gemini + CLI mode)
./scripts/start.sh

# Or choose your preferred configuration
./scripts/start.sh -p claude -d    # Claude + Docker mode
./scripts/start.sh -p gemini -d    # Gemini + Docker mode
./scripts/start.sh -p claude       # Claude + CLI mode
```

**Manual Setup**

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
| `CLAUDE_API_KEY` | Anthropic Claude API Key | Yes* | `sk-xxxxxxxxxxxx` |
| `GOOGLE_API_KEY` | Google Gemini API Key | Yes* | `AIzaxxxxxxxxxxxx` |
| `CODE_PROVIDER` | AI provider (claude/gemini) | No | `claude` |
| `USE_DOCKER` | Use Docker containers | No | `true` |
| `PORT` | Server port | No | `8888` |
| `LOG_LEVEL` | Logging level | No | `debug` |

*Either Claude or Gemini API key is required

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
  container_image: "anthropic/claude-code:latest"
  timeout: "30m"

# Gemini configuration  
gemini:
  container_image: "google-gemini/gemini-cli:latest"
  timeout: "30m"

# Docker settings
docker:
  socket: "unix:///var/run/docker.sock"
  network: "bridge"
```

### AI Provider Setup

**Claude Setup**
```bash
# For CLI mode: Install Claude CLI
npm install -g @anthropic/claude

# Set API key
export CLAUDE_API_KEY="sk-your-claude-api-key"
```

**Gemini Setup**  
```bash
# For CLI mode: Install Gemini CLI
npm install -g @google/gemini-cli

# Set API key
export GOOGLE_API_KEY="AIza-your-gemini-api-key"
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
| `/code <description>` | Generate code for an Issue | `/code Implement user authentication with JWT` |
| `/continue <instruction>` | Continue development in PR | `/continue Add unit tests for the login function` |
| `/fix <description>` | Fix code issues in PR | `/fix Handle edge case for empty username` |

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

**3. Bug Fixes**
```
# In a PR comment:
/fix Fix the null pointer exception in user authentication
```

## 🛠️ Development

### Project Structure

```
codeagent/
├── cmd/
│   └── server/                 # Application entry point
├── internal/
│   ├── agent/                  # Core orchestration logic
│   ├── webhook/                # GitHub webhook handling
│   ├── workspace/              # Git workspace management
│   ├── code/                   # AI provider implementations
│   ├── github/                 # GitHub API client
│   └── config/                 # Configuration management
├── pkg/
│   └── models/                 # Shared data structures
├── scripts/
│   └── start.sh                # Development startup script
├── docs/                       # Documentation
├── config.yaml                 # Configuration file
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

### Debugging

```bash
# Enable debug logging
export LOG_LEVEL=debug
go run ./cmd/server --config config.yaml

# Monitor workspace activity
ls -la /tmp/codeagent/  # Default workspace directory
```

## 🚀 Deployment

**Production Environment Variables**
```bash
export GITHUB_TOKEN="your-production-token"
export CLAUDE_API_KEY="your-production-key"
export WEBHOOK_SECRET="your-strong-production-secret"
export CODE_PROVIDER="claude"
export USE_DOCKER="true"
export PORT="8888"
```

**Docker Deployment**
```bash
# Build Docker image
docker build -t codeagent .

# Run container
docker run -d \
  -p 8888:8888 \
  -e GITHUB_TOKEN="$GITHUB_TOKEN" \
  -e CLAUDE_API_KEY="$CLAUDE_API_KEY" \
  -e WEBHOOK_SECRET="$WEBHOOK_SECRET" \
  -v /var/run/docker.sock:/var/run/docker.sock \
  codeagent
```

## 🔐 Security

**Security Best Practices**
- Use strong webhook secrets (32+ characters)
- Always configure webhook secrets in production
- Use HTTPS endpoints for webhooks
- Regularly rotate API keys and secrets
- Limit GitHub token permissions to minimum required scope

**Token Permissions**

Your GitHub token needs these permissions:
- `repo` (for private repositories)
- `public_repo` (for public repositories)
- `pull_requests:write`
- `issues:write`

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

This project is licensed under the [MIT License](LICENSE).

## 🙏 Acknowledgments

Thank you to all developers and users who have contributed to making CodeAgent better!

---

**Need help?** Check our [documentation](docs/) or [open an issue](https://github.com/qiniu/codeagent/issues/new).