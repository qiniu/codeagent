# CLAUDE.md

This file provides guidance for Claude Code (claude.ai/code) when processing this codebase.

## Project Overview

**CodeAgent** is a Go-based automated code generation and collaboration system that receives various AI instructions through GitHub Webhooks, automatically handling code generation, modification, and review tasks for Issues and Pull Requests.

## Architecture Design

The system adopts a webhook-driven architecture:

```
GitHub Events (AI Instructions) → Webhook → CodeAgent → Branch Creation → PR Processing → AI Container → Code Generation/Modification → PR Updates
```

### Core Components

- **EnhancedAgent** (`internal/agent/enhanced_agent.go`): Orchestrates the entire workflow with modern architecture
- **Webhook Handler** (`internal/webhook/handler.go`): Handles GitHub webhooks (Issues and PRs)
- **Workspace Manager** (`internal/workspace/manager.go`): Manages temporary Git worktrees
- **Code Providers** (`internal/code/`): Supports Claude (Docker/CLI) and Gemini (Docker/CLI)
- **GitHub Client** (`internal/github/client.go`): Handles GitHub API interactions

## Development Commands

### Build and Run

```bash
# Build binary file
make build

# Run locally with configuration
# No startup scripts - use direct Go commands or build binary

# Direct Go run
export GITHUB_TOKEN="your-token"
export GOOGLE_API_KEY="your-key"      # or CLAUDE_API_KEY
export WEBHOOK_SECRET="your-secret"
go run ./cmd/server --port 8888  # Now always uses EnhancedAgent
```

### Testing

```bash
# Run tests
make test

# Health check
curl http://localhost:8888/health

# Test webhook processing
curl -X POST http://localhost:8888/hook \
  -H "Content-Type: application/json" \
  -H "X-GitHub-Event: issue_comment" \
  -d @test-data/issue-comment.json
```

### Configuration

**Required environment variables:**

- `GITHUB_TOKEN` - GitHub personal access token
- `WEBHOOK_SECRET` - GitHub webhook secret
- `GOOGLE_API_KEY` (for Gemini) or `CLAUDE_API_KEY` (for Claude)

**Configuration file:** `config.yaml`

```yaml
code_provider: gemini # claude or gemini
use_docker: false # true for Docker, false for CLI
server:
  port: 8888
```

### Key Directories

- `cmd/server/` - Main application entry point
- `internal/` - Core business logic
  - `agent/` - Main orchestration logic
  - `webhook/` - GitHub webhook handling
  - `workspace/` - Git worktree management
  - `code/` - AI provider implementations (Claude/Gemini)
  - `github/` - GitHub API client
- `pkg/models/` - Shared data structures
- `scripts/` - Utility scripts directory (currently empty)

### Development Workflow

1. **Local Development**: Use direct Go commands `go run ./cmd/server`
2. **Testing**: Send test webhooks and example GitHub events
3. **Docker Development**: Use Docker mode for containerized testing
4. **Workspace Management**: Temporary worktrees created in `/tmp/codeagent`, automatically cleaned up after 24 hours

### Command Processing

The system supports various AI instructions triggered through GitHub comments and reviews:

**Issue Instructions:**

- `/code <description>` - Generate initial code for the Issue and create a PR

**PR Collaboration Instructions:**

- `/continue <instruction>` - Continue development in the PR, supporting custom instructions

**Supported Scenarios:**

- Instruction handling in Issue comments
- Instruction handling in PR comments
- Instruction handling in PR Review comments
- Batch processing of PR Reviews (supports batch processing of multiple comments)

**Instruction Features:**

- Supports custom parameters and instruction content
- Automatically retrieves historical comment context
- Intelligent code submission and PR updates
- Complete error handling and retry mechanism

The system is designed as an extensible architecture, making it easy to add new instruction types and processing logic in the future.

### Environment Modes

- **Docker Mode**: Use containerized Claude/Gemini, including a complete toolkit
- **CLI Mode**: Use locally installed Claude CLI or Gemini CLI (faster during development)

### Common Issues

- Docker mode requires ensuring Docker is running
- CLI mode requires checking if CLI tools are installed: `claude` or `gemini`
- Verify GitHub webhook configuration matches local port
- Monitor logs to troubleshoot workspace cleanup issues

# Note

- When modifying code, always remember to perform type checking and formatting
