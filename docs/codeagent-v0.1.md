# Code Agent System Design v0.1

## Project Overview

Code Agent is an automated code generation system developed in Go, which receives `/code` commands through GitHub Webhooks and automatically generates code for Issues and creates Pull Requests.

## System Architecture

```
GitHub Issue (/code) → Webhook → Code Agent → Temporary Repository → Claude Code Container → PR
```

## Core Process

### 1. Webhook Reception and Parsing

- Listen for GitHub Issue comment events
- Detect `/code` command triggers
- Parse Issue content and context information

### 2. Temporary Repository Preparation

- Clone target repository to temporary directory
- Create new branch (e.g., `Code-agent/issue-123`)
- Generate initial commit: "Code-agent has received information, preparing to implement target issue"
- Push branch and create PR

### 3. Claude Code Execution

- Mount temporary directory to Claude Code container
- Pass Issue information as prompt
- Wait for code generation completion

### 4. Result Processing

- Detect code changes in temporary directory
- Generate new commit records
- Push to created PR

## System Components

### Webhook Handler

```go
type WebhookHandler struct {
    config     *Config
    agent      *Agent
    validator  *SignatureValidator
}

func (h *WebhookHandler) HandleIssueComment(w http.ResponseWriter, r *http.Request) {
    // 1. Validate Webhook signature
    // 2. Parse Issue comment event
    // 3. Check if contains /code command
    // 4. Create Agent task
    // 5. Execute code generation asynchronously
}
```

### Agent Core

```go
type Agent struct {
    config     *Config
    github     *GitHubClient
    workspace  *WorkspaceManager
    claude     *ClaudeExecutor
}

func (a *Agent) ProcessIssue(issue *github.Issue) error {
    // 1. Prepare temporary workspace
    workspace := a.workspace.Prepare(issue)

    // 2. Clone repository and create branch
    branch := a.github.CreateBranch(workspace, issue)

    // 3. Create initial PR
    pr := a.github.CreatePullRequest(branch, issue)

    // 4. Execute Claude Code
    result := a.claude.Execute(workspace, issue)

    // 5. Commit changes and update PR
    a.github.CommitAndPush(workspace, result)

    return nil
}
```

### Workspace Manager

```go
type WorkspaceManager struct {
    baseDir    string
    tempDir    string
}

func (w *WorkspaceManager) Prepare(issue *github.Issue) *Workspace {
    // 1. Create temporary directory
    // 2. Clone target repository
    // 3. Create new branch
    // 4. Return workspace information
}

func (w *WorkspaceManager) Cleanup(workspace *Workspace) {
    // Clean up temporary files
}
```

### Claude Code Executor

```go
type ClaudeExecutor struct {
    config     *Config
    docker     *DockerClient
}

func (c *ClaudeExecutor) Execute(workspace *Workspace, issue *github.Issue) *ExecutionResult {
    // 1. Build Claude Code container
    // 2. Mount workspace directory
    // 3. Pass Issue information as prompt
    // 4. Wait for execution completion
    // 5. Return execution result
}
```

### GitHub Client

```go
type GitHubClient struct {
    client     *github.Client
    config     *Config
}

func (g *GitHubClient) CreateBranch(workspace *Workspace, issue *github.Issue) *github.Reference {
    // 1. Clone repository
    // 2. Create new branch
    // 3. Generate initial commit
    // 4. Push branch
}

func (g *GitHubClient) CreatePullRequest(branch *github.Reference, issue *github.Issue) *github.PullRequest {
    // Create PR and associate with Issue
}

func (g *GitHubClient) CommitAndPush(workspace *Workspace, result *ExecutionResult) error {
    // 1. Detect file changes
    // 2. Generate commit message
    // 3. Commit and push
}
```

## Core Data Structures

### Workspace

```go
type Workspace struct {
    ID          string           `json:"id"`
    Path        string           `json:"path"`
    Repository  string           `json:"repository"`
    Branch      string           `json:"branch"`
    Issue       *github.Issue    `json:"issue"`
    CreatedAt   time.Time        `json:"created_at"`
}
```

### Execution Result

```go
type ExecutionResult struct {
    Success     bool             `json:"success"`
    Output      string           `json:"output"`
    Error       string           `json:"error,omitempty"`
    FilesChanged []string         `json:"files_changed"`
    Duration    time.Duration    `json:"duration"`
}
```

## Configuration Structure

```yaml
# config.yaml
server:
  port: 8080
  webhook_secret: "your-webhook-secret"

github:
  token: "ghp_your-github-token"
  webhook_url: "https://your-domain.com/webhook"

workspace:
  base_dir: "/tmp/Code-agent"
  cleanup_after: "24h"

claude:
  api_key: "sk-ant-your-claude-key"
  container_image: "anthropic/claude-code:latest"
  timeout: "30m"

docker:
  socket: "unix:///var/run/docker.sock"
  network: "bridge"
```

## Project Structure

```
Code-agent/
├── cmd/
│   └── server/
│       └── main.go              # Main program entry point
├── internal/
│   ├── webhook/
│   │   ├── handler.go           # Webhook handler
│   │   └── validator.go         # Signature validator
│   ├── agent/
│   │   ├── agent.go             # Agent core logic
│   │   └── processor.go         # Task processor
│   ├── workspace/
│   │   ├── manager.go           # Workspace management
│   │   └── git.go               # Git operations
│   ├── claude/
│   │   ├── executor.go          # Claude Code executor
│   │   └── docker.go            # Docker client
│   ├── github/
│   │   ├── client.go            # GitHub API client
│   │   └── pr.go                # PR management
│   └── config/
│       └── config.go            # Configuration management
├── pkg/
│   └── models/
│       ├── workspace.go         # Workspace model
│       └── result.go            # Execution result model
├── configs/
│   └── config.yaml              # Configuration file
├── Dockerfile                   # Agent container
├── docker-compose.yml           # Development environment
└── README.md                    # Project documentation
```

## Deployment Configuration

### Dockerfile

```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o Code-agent ./cmd/server

FROM alpine:latest
RUN apk --no-cache add ca-certificates git docker-cli
WORKDIR /root/
COPY --from=builder /app/xgo-agent .
COPY --from=builder /app/configs ./configs
EXPOSE 8080
CMD ["./xgo-agent"]
```

### docker-compose.yml

```yaml
version: "3.8"
services:
  xgo-agent:
    build: .
    ports:
      - "8080:8080"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - ./workspaces:/tmp/xgo-agent
      - ./configs:/root/configs
    environment:
      - GITHUB_TOKEN=${GITHUB_TOKEN}
      - CLAUDE_API_KEY=${CLAUDE_API_KEY}
    restart: unless-stopped
```

## Usage Process

### 1. Configure GitHub Webhook

- URL: `https://your-domain.com/webhook`
- Event: `Issue comments`
- Secret: Consistent with configuration file

### 2. Trigger Code Generation

- Write in GitHub Issue comment: `/code implement user login functionality`
- System automatically processes and creates PR

### 3. Monitor Execution Status

- View commit records in PR
- Check code generation results
- Adjust and optimize as needed

## Key Features

- **Isolated Execution**: Temporary workspaces avoid polluting main repository
- **Containerization**: Claude Code executes in isolated containers
- **Asynchronous Processing**: Webhook responds quickly, processes tasks in background
- **Status Tracking**: Complete execution status and logging
- **Automatic Cleanup**: Regular cleanup of temporary files and containers
- **Error Handling**: Comprehensive error handling and retry mechanisms

## Extended Features

- Support multiple trigger commands (`/code`, `/fix`, `/refactor`)
- Custom code generation templates
- Multi-repository support
- Execution history
- Web management interface
- Monitoring and alerting

This design provides a complete automated code generation solution that can effectively handle GitHub Issues and generate corresponding code implementations.
