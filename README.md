# CodeAgent

[![Go Report Card](https://goreportcard.com/badge/github.com/qiniu/codeagent)](https://goreportcard.com/report/github.com/qiniu/codeagent)
[![Go Version](https://img.shields.io/github/go-mod/go-version/qiniu/codeagent)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![CI](https://github.com/qiniu/codeagent/workflows/CI/badge.svg)](https://github.com/qiniu/codeagent/actions)

🤖 **AI-powered automated code generation and collaboration system** that processes GitHub Issues and Pull Requests through webhooks, automatically generating, modifying, and reviewing code using Claude or Gemini AI models.

## ✨ Key Features

- 🤖 **Multi-AI Support**: Claude and Gemini integration with Docker/CLI execution modes
- 🔄 **Automated Workflows**: Process GitHub Issues and PRs with AI-powered code generation
- 📁 **Smart Workspace Management**: Git worktree-based isolated development environments
- 🐳 **Flexible Deployment**: Docker containerized or local CLI execution
- 🛡️ **Secure Integration**: GitHub webhook signature verification and proper token management
- ⚡ **Real-time Processing**: Instant response to GitHub events with progress tracking

## 🚀 Quick Start

### Prerequisites

- **Go 1.21+** (for building from source)
- **Docker** (if using Docker mode)
- **Git** (for repository management)
- **GitHub Personal Access Token** with repository permissions
- **AI API Key**: Either Claude API key or Google API key for Gemini

### 1. Installation

```bash
git clone https://github.com/qiniu/codeagent.git
cd codeagent
go mod download
```

### 2. Configuration

Create your configuration file:

```bash
cp config.example.yaml config.yaml
```

Edit `config.yaml` with your settings:

```yaml
# Basic server configuration
server:
  port: 8888
  webhook_secret: "your-secure-webhook-secret"

# AI provider selection
code_provider: claude  # Options: claude, gemini
use_docker: false      # true for Docker, false for local CLI

# Workspace settings
workspace:
  base_dir: "./workspace"
  cleanup_after: "24h"
```

### 3. Environment Setup

```bash
# Required: GitHub access
export GITHUB_TOKEN="your-github-token"

# Required: AI provider API key
export CLAUDE_API_KEY="your-claude-key"    # For Claude
# OR
export GOOGLE_API_KEY="your-gemini-key"    # For Gemini

# Optional: Webhook security (recommended for production)
export WEBHOOK_SECRET="your-secure-secret"
```

### 4. Start the Server

```bash
# Option 1: Using Go directly
go run ./cmd/server --config config.yaml

# Option 2: Using Makefile
make build
./bin/codeagent --config config.yaml

# Option 3: Using convenience script (if available)
./scripts/start.sh -p claude  # Claude with local CLI
./scripts/start.sh -p gemini -d  # Gemini with Docker
```

### 5. GitHub Webhook Configuration

1. Go to your GitHub repository settings
2. Navigate to "Webhooks" → "Add webhook"
3. Configure:
   - **URL**: `https://your-domain.com/hook`
   - **Content type**: `application/json`
   - **Secret**: Same as your `WEBHOOK_SECRET`
   - **Events**: Select "Issue comments", "Pull requests", "Pull request reviews"
4. Save the webhook

### 6. Usage

Once configured, interact with CodeAgent through GitHub comments:

**In Issues:**
```
/code Implement user authentication with JWT tokens and password hashing
```

**In Pull Requests:**
```
/continue Add comprehensive unit tests for the authentication module
/fix Resolve the null pointer exception in login validation
```

## 🎯 Usage Examples

### Issue Code Generation
Create a new GitHub Issue and comment:
```
/code Create a REST API endpoint for user profile management with CRUD operations
```
CodeAgent will analyze the request and create a new Pull Request with the implemented code.

### PR Collaboration
In an existing Pull Request, guide development:
```
/continue Add error handling for database connection failures
/continue Implement rate limiting middleware
/fix Update the API documentation to reflect recent changes
```

### Advanced Instructions
```
/code Implement a microservice for order processing with:
- Event-driven architecture using message queues
- Database integration with connection pooling
- Comprehensive error handling and logging
- Unit and integration tests
```

## 📊 Architecture

```
GitHub Events → Webhook → CodeAgent → Workspace → AI Provider → Code Generation → PR Updates
```

### Core Components

- **Webhook Handler**: Processes GitHub events and validates signatures
- **Agent**: Orchestrates the entire workflow and manages AI interactions
- **Workspace Manager**: Creates isolated Git worktrees for safe code manipulation
- **AI Providers**: Interfaces with Claude or Gemini APIs (Docker/CLI modes)
- **GitHub Client**: Manages repository interactions and PR updates

## 📋 Configuration Guide

### Complete Configuration Options

```yaml
# Server settings
server:
  port: 8888
  webhook_secret: "set-via-environment-variable"

# GitHub integration
github:
  token: "set-via-environment-variable"
  webhook_url: "https://your-domain.com/hook"

# Workspace management
workspace:
  base_dir: "./workspace"  # Supports relative paths
  cleanup_after: "24h"

# Claude configuration
claude:
  api_key: "set-via-environment-variable"
  base_url: "https://api.anthropic.com"  # Optional
  container_image: "anthropic/claude-code:latest"
  timeout: "30m"
  interactive: true

# Gemini configuration
gemini:
  api_key: "set-via-environment-variable"
  container_image: "google-gemini/gemini-cli:latest"
  timeout: "30m"

# Docker settings (when use_docker: true)
docker:
  socket: "unix:///var/run/docker.sock"
  network: "bridge"

# Provider selection
code_provider: claude  # Options: claude, gemini
use_docker: false      # true for containers, false for local CLI
```

### Environment Variables

| Variable | Description | Required |
|----------|-------------|----------|
| `GITHUB_TOKEN` | GitHub Personal Access Token | ✅ |
| `CLAUDE_API_KEY` | Anthropic Claude API key | ✅ (if using Claude) |
| `GOOGLE_API_KEY` | Google Gemini API key | ✅ (if using Gemini) |
| `WEBHOOK_SECRET` | GitHub webhook signature secret | ⚠️ (recommended) |
| `CODE_PROVIDER` | AI provider selection (claude/gemini) | ❌ |
| `USE_DOCKER` | Docker mode toggle (true/false) | ❌ |
| `LOG_LEVEL` | Logging verbosity (debug/info/warn/error) | ❌ |

## 🔧 Development

### Build Commands

```bash
# Build binary
make build

# Run tests
make test

# Clean build artifacts
make clean

# Run with development settings
make run
```

### Testing

```bash
# Run all tests
go test ./...

# Test with verbose output
go test -v ./...

# Integration testing
curl http://localhost:8888/health

# Test webhook endpoint
curl -X POST http://localhost:8888/hook \
  -H "Content-Type: application/json" \
  -H "X-GitHub-Event: issue_comment" \
  -d @test-data/issue-comment.json
```

### Project Structure

```
codeagent/
├── cmd/server/          # Application entry point
├── internal/            # Core business logic
│   ├── agent/          # Main orchestration
│   ├── webhook/        # GitHub webhook handling
│   ├── workspace/      # Git worktree management
│   ├── code/           # AI provider implementations
│   └── github/         # GitHub API client
├── pkg/models/         # Shared data structures
├── docs/               # Documentation
└── config.example.yaml # Configuration template
```

## 🛠️ Troubleshooting

### Common Issues

**"Webhook signature verification failed"**
- Ensure `WEBHOOK_SECRET` matches GitHub webhook configuration
- Verify webhook is sending POST requests with proper headers

**"Failed to create workspace"**
- Check directory permissions for `workspace.base_dir`
- Ensure sufficient disk space available
- Verify Git is installed and accessible

**"AI provider connection timeout"**
- Check API keys are valid and have sufficient credits
- Verify network connectivity to AI provider APIs
- Increase timeout values in configuration if needed

**"Docker container failed to start"**
- Ensure Docker daemon is running
- Check container images are available
- Verify Docker socket permissions

### Debug Mode

```bash
export LOG_LEVEL=debug
go run ./cmd/server --config config.yaml
```

### Health Monitoring

```bash
# Check service health
curl http://localhost:8888/health

# Monitor logs in real-time
tail -f /var/log/codeagent.log
```

## 🛡️ Security

- **Token Security**: Never commit API keys or secrets to repositories
- **Webhook Verification**: Always configure webhook secrets in production
- **Network Security**: Use HTTPS for webhook endpoints
- **Access Control**: Limit GitHub token permissions to minimum required scope
- **Regular Rotation**: Rotate API keys and webhook secrets periodically

## 🤝 Contributing

We welcome contributions! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

- 🐛 [Report Bugs](https://github.com/qiniu/codeagent/issues/new?template=bug_report.md)
- 💡 [Request Features](https://github.com/qiniu/codeagent/issues/new?template=feature_request.md)
- 📝 [Improve Documentation](https://github.com/qiniu/codeagent/issues/new?template=documentation.md)

## 📄 License

This project is licensed under the [MIT License](LICENSE).

## 🙏 Acknowledgments

Thanks to all contributors and the open-source community for making this project possible!

---

**Need Help?** 
- 📖 Check our [detailed documentation](docs/)
- 💬 Open an [issue](https://github.com/qiniu/codeagent/issues) for questions
- 🔧 Review [troubleshooting guide](#troubleshooting) above