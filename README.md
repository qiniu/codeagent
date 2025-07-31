# CodeAgent

[![Go Report Card](https://goreportcard.com/badge/github.com/qiniu/codeagent)](https://goreportcard.com/report/github.com/qiniu/codeagent)
[![Go Version](https://img.shields.io/github/go-mod/go-version/qiniu/codeagent)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![CI](https://github.com/qiniu/codeagent/workflows/CI/badge.svg)](https://github.com/qiniu/codeagent/actions)

CodeAgent is an AI-powered code agent that automatically processes GitHub Issues and Pull Requests, generating code modification suggestions.

## 📋 Table of Contents

- [Features](#features)
- [Quick Start](#quick-start)
- [Documentation](#documentation)
- [Contributing](#contributing)
- [Security](#security)
- [License](#license)

## Features

- 🤖 Support for multiple AI models (Claude, Gemini)
- 🔄 Automatic processing of GitHub Issues and Pull Requests
- 🐳 Docker containerized execution environment
- 📁 Git Worktree-based workspace management

## Quick Start

### Installation

```bash
git clone https://github.com/qiniu/codeagent.git
cd codeagent
go mod download
```

### Configuration

#### Method 1: Command Line Arguments

```bash
go run ./cmd/server \
  --github-token "your-github-token" \
  --claude-api-key "your-claude-api-key" \
  --webhook-secret "your-webhook-secret" \
  --port 8888
```

#### Method 2: Environment Variables

```bash
export GITHUB_TOKEN="your-github-token"
export CLAUDE_API_KEY="your-claude-api-key"
export WEBHOOK_SECRET="your-webhook-secret"
export PORT=8888

go run ./cmd/server
```

#### Method 3: Configuration File (Recommended)

Create a configuration file `config.yaml`:

```yaml
server:
  port: 8888
  # webhook_secret: Set via command line arguments or environment variables

github:
  # token: Set via command line arguments or environment variables
  webhook_url: "http://localhost:8888/hook"

workspace:
  base_dir: "./codeagent" # Supports relative paths!
  cleanup_after: "24h"

claude:
  # api_key: Set via command line arguments or environment variables
  container_image: "anthropic/claude-code:latest"
  timeout: "30m"

gemini:
  # api_key: Set via command line arguments or environment variables
  container_image: "google-gemini/gemini-cli:latest"
  timeout: "30m"

docker:
  socket: "unix:///var/run/docker.sock"
  network: "bridge"

# Code provider configuration
code_provider: claude # Options: claude, gemini
use_docker: true # Whether to use Docker, false means use local CLI
```

**Configuration Notes:**

- `code_provider`: Choose code generation service
  - `claude`: Use Anthropic Claude
  - `gemini`: Use Google Gemini
- `use_docker`: Choose execution method
  - `true`: Use Docker containers (recommended for production)
  - `false`: Use local CLI (recommended for development)

**Note**: Sensitive information (such as tokens, api_keys, webhook_secret) should be set via command line arguments or environment variables, not written in configuration files.

### Relative Path Support

CodeAgent now supports using relative paths in configuration files, providing more flexible configuration options:

```yaml
workspace:
  base_dir: "./codeagent"     # Relative to configuration file directory
  # or
  base_dir: "../workspace"    # Relative to parent directory of configuration file
  # or
  base_dir: "/tmp/codeagent"  # Absolute path (unchanged)
```

Relative paths are automatically converted to absolute paths when configuration is loaded. For details, please refer to the [Relative Path Support Documentation](docs/relative-path-support.md).

### Security Configuration

#### Webhook Signature Verification

To prevent malicious exploitation of webhook interfaces, CodeAgent supports GitHub Webhook signature verification:

1. **Configure webhook secret**:

   ```bash
   # Method 1: Environment variables (recommended)
   export WEBHOOK_SECRET="your-strong-secret-here"

   # Method 2: Command line arguments
   go run ./cmd/server --webhook-secret "your-strong-secret-here"
   ```

2. **GitHub Webhook Settings**:

   - Add Webhook in GitHub repository settings
   - URL: `https://your-domain.com/hook`
   - Content type: `application/json`
   - Secret: Enter the same value as `WEBHOOK_SECRET`
   - Select events: `Issue comments`, `Pull request reviews`, `Pull requests`

3. **Signature Verification Mechanism**:
   - Supports SHA-256 signature verification (priority)
   - Backward compatible with SHA-1 signature verification
   - Uses constant-time comparison to prevent timing attacks
   - If `webhook_secret` is not configured, signature verification is skipped (development environment only)

#### Security Recommendations

- Use strong passwords as webhook secrets (recommended 32+ characters)
- Always configure webhook secrets in production environments
- Use HTTPS to protect webhook endpoints
- Regularly rotate API keys and webhook secrets
- Limit GitHub Token permission scope

### Local Development

#### Configuration Combination Examples

**1. Claude + Docker Mode (Default)**

```bash
# Using environment variables
export GITHUB_TOKEN="your-github-token"
export CLAUDE_API_KEY="your-claude-api-key"
export WEBHOOK_SECRET="your-webhook-secret"
export CODE_PROVIDER=claude
export USE_DOCKER=true
go run ./cmd/server

# Or using configuration file
# Set in config.yaml: code_provider: claude, use_docker: true
go run ./cmd/server --config config.yaml
```

**2. Claude + Local CLI Mode**

```bash
# Using environment variables
export GITHUB_TOKEN="your-github-token"
export CLAUDE_API_KEY="your-claude-api-key"
export WEBHOOK_SECRET="your-webhook-secret"
export CODE_PROVIDER=claude
export USE_DOCKER=false
go run ./cmd/server

# Or using configuration file
# Set in config.yaml: code_provider: claude, use_docker: false
go run ./cmd/server --config config.yaml
```

**3. Gemini + Docker Mode**

```bash
# Using environment variables
export GITHUB_TOKEN="your-github-token"
export GOOGLE_API_KEY="your-google-api-key"
export WEBHOOK_SECRET="your-webhook-secret"
export CODE_PROVIDER=gemini
export USE_DOCKER=true
go run ./cmd/server

# Or using configuration file
# Set in config.yaml: code_provider: gemini, use_docker: true
go run ./cmd/server --config config.yaml
```

**4. Gemini + Local CLI Mode (Recommended for Development)**

```bash
# Using environment variables
export GITHUB_TOKEN="your-github-token"
export GOOGLE_API_KEY="your-google-api-key"
export WEBHOOK_SECRET="your-webhook-secret"
export CODE_PROVIDER=gemini
export USE_DOCKER=false
go run ./cmd/server

# Or using configuration file
# Set in config.yaml: code_provider: gemini, use_docker: false
go run ./cmd/server --config config.yaml
```

#### Using Startup Script (Recommended)

We provide a convenient startup script that supports all configuration combinations:

```bash
# Set environment variables
export GITHUB_TOKEN="your-github-token"
export GOOGLE_API_KEY="your-google-api-key"  # or CLAUDE_API_KEY
export WEBHOOK_SECRET="your-webhook-secret"

# Use startup script
./scripts/start.sh                    # Gemini + Local CLI mode (default)
./scripts/start.sh -p claude -d       # Claude + Docker mode
./scripts/start.sh -p gemini -d       # Gemini + Docker mode
./scripts/start.sh -p claude          # Claude + Local CLI mode

# View help
./scripts/start.sh --help
```

The startup script automatically checks environment dependencies and sets appropriate environment variables.

**Notes**:

- Local CLI mode requires pre-installation of Claude CLI or Gemini CLI tools
- Gemini CLI mode uses single prompt approach, starting new process for each call, avoiding broken pipe errors
- Gemini CLI automatically builds complete prompts including project context, Issue information, and conversation history, providing better code generation quality

2. **Test Health Check**

```bash
curl http://localhost:8888/health
```

3. **Configure GitHub Webhook**
   - URL: `http://your-domain.com/hook`
   - Events: `Issue comments`, `Pull request reviews`
   - Secret: Same as `webhook_secret` in configuration (for signature verification)
   - Recommended to use HTTPS and strong passwords for security

### Usage Examples

1. **Trigger Code Generation in GitHub Issue**

```
/code Implement user login functionality including username/password validation and JWT token generation
```

2. **Continue Development in PR Comments**

```
/continue Add unit tests
```

3. **Fix Code Issues**

```
/fix Fix login validation logic bug
```

## Local Development

### Project Structure

```
codeagent/
├── cmd/
│   └── server/
│       └── main.go              # Main program entry point
├── internal/
│   ├── webhook/
│   │   └── handler.go           # Webhook handler
│   ├── agent/
│   │   └── agent.go             # Agent core logic
│   ├── workspace/
│   │   └── manager.go           # Workspace management
│   ├── claude/
│   │   └── executor.go          # Claude Code executor
│   ├── github/
│   │   └── client.go            # GitHub API client
│   └── config/
│       └── config.go            # Configuration management
├── pkg/
│   └── models/
│       └── workspace.go         # Data models
├── docs/
│   └── xgo-agent.md             # Design documentation
├── config.yaml                  # Configuration file
├── go.mod                       # Go module file
└── README.md                    # Project documentation
```

3. **Build**

```bash
# Build binary file
go build -o bin/codeagent ./cmd/server

# Cross-compilation
GOOS=linux GOARCH=amd64 go build -o bin/codeagent-linux ./cmd/server
```

**Integration Testing**

```bash
# Start test server
go run ./cmd/server --config test-config.yaml

# Send test webhook
curl -X POST http://localhost:8888/hook \
  -H "Content-Type: application/json" \
  -H "X-GitHub-Event: issue_comment" \
  -d @test-data/issue-comment.json
```

### Debugging

1. **Log Level**

```bash
# Set detailed logging
export LOG_LEVEL=debug
go run ./cmd/server --config config.yaml
```

## 🤝 Contributing

We welcome all forms of contributions! Please check the [Contributing Guide](CONTRIBUTING.md) to learn how to participate in project development.

### Ways to Contribute

- 🐛 [Report Bugs](https://github.com/qiniu/codeagent/issues/new?template=bug_report.md)
- 💡 [Feature Requests](https://github.com/qiniu/codeagent/issues/new?template=feature_request.md)
- 📝 [Improve Documentation](https://github.com/qiniu/codeagent/issues/new?template=documentation.md)
- 🔧 [Submit Code](CONTRIBUTING.md#code-contributions)

## 📄 License

This project is licensed under the [MIT License](LICENSE).

## 🙏 Acknowledgments

Thank you to all developers and users who have contributed to this project!
