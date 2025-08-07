# Code Agent System Design v0.1

## Project Overview

Code Agent is an automated code generation system developed in Go that receives `/code` commands through GitHub Webhooks, automatically generates code for Issues, and creates Pull Requests.

## System Architecture

```
GitHub Issue (/code) → Webhook → Code Agent → Temporary Repository → Claude Code Container → PR
```

## Core Workflow

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
- Pass Issue description and requirements as prompts
- Execute automated code generation and modification
- Implement functionality based on Issue requirements

### 4. Result Processing

- Parse Claude Code output and generated code
- Commit changes with descriptive messages
- Push to remote branch
- Update PR description with implementation details

## Technical Implementation

### Directory Structure

```
/tmp/codeagent/
├── repos/
│   └── owner-repo-issue-123/
│       ├── .git/
│       └── [repository files]
└── sessions/
    └── claude-session-123/
        └── [Claude execution context]
```

### Key Components

#### 1. Webhook Handler
- **File**: `internal/webhook/handler.go`
- **Functionality**: Process GitHub webhook events
- **Trigger Detection**: Parse comments for `/code` commands

#### 2. Workspace Manager
- **File**: `internal/workspace/manager.go`
- **Functionality**: Manage temporary Git repositories
- **Operations**: Clone, branch creation, cleanup

#### 3. GitHub Client
- **File**: `internal/github/client.go`
- **Functionality**: GitHub API interactions
- **Operations**: PR creation, commit pushing, status updates

#### 4. Code Execution Engine
- **File**: `internal/code/claude.go`
- **Functionality**: Claude Code container management
- **Operations**: Container creation, prompt execution, result parsing

## Configuration

### Environment Variables

```bash
# GitHub Configuration
GITHUB_TOKEN=your_github_token
WEBHOOK_SECRET=your_webhook_secret

# Claude Configuration  
CLAUDE_API_KEY=your_claude_api_key

# System Configuration
WORKSPACE_PATH=/tmp/codeagent
```

### Config File (config.yaml)

```yaml
github:
  token: "${GITHUB_TOKEN}"
  webhook_secret: "${WEBHOOK_SECRET}"

code:
  provider: "claude"
  claude:
    api_key: "${CLAUDE_API_KEY}"
    model: "claude-3-sonnet-20240229"

workspace:
  base_path: "/tmp/codeagent"
  cleanup_after_hours: 24

server:
  port: 8888
  host: "0.0.0.0"
```

## Key Features

### 1. Automated Issue Processing
- Parse Issue requirements automatically
- Generate appropriate code solutions
- Create PR with implementation

### 2. Git Repository Management
- Temporary workspace isolation
- Automatic branch management
- Clean commit history

### 3. Claude Code Integration
- Container-based execution
- Context-aware code generation
- Error handling and retry logic

### 4. GitHub Integration
- Real-time webhook processing
- PR status updates
- Automatic cleanup

## Error Handling

### Common Scenarios

1. **GitHub API Rate Limits**
   - Implement exponential backoff
   - Queue requests when limit exceeded
   - Graceful degradation

2. **Claude Code Failures**
   - Retry mechanism with different prompts
   - Fallback to simpler implementations
   - Error reporting in PR comments

3. **Git Operation Failures**
   - Repository state validation
   - Conflict resolution strategies
   - Workspace cleanup on failure

4. **Resource Management**
   - Disk space monitoring
   - Process timeout handling
   - Memory usage optimization

## Security Considerations

### 1. Webhook Security
- Signature verification for all incoming requests
- IP allowlisting for GitHub webhooks
- Rate limiting to prevent abuse

### 2. Code Execution Security
- Containerized execution environment
- Resource limits (CPU, memory, disk)
- Network isolation

### 3. Repository Access
- Token scope limitation
- Read-only access where possible
- Audit logging for all operations

## Deployment

### Docker Deployment

```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o codeagent ./cmd/codeagent

FROM alpine:latest
RUN apk --no-cache add ca-certificates git
WORKDIR /root/
COPY --from=builder /app/codeagent .
CMD ["./codeagent"]
```

### Kubernetes Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: codeagent
spec:
  replicas: 2
  selector:
    matchLabels:
      app: codeagent
  template:
    metadata:
      labels:
        app: codeagent
    spec:
      containers:
      - name: codeagent
        image: codeagent:latest
        ports:
        - containerPort: 8888
        env:
        - name: GITHUB_TOKEN
          valueFrom:
            secretKeyRef:
              name: github-secrets
              key: token
        volumeMounts:
        - name: workspace
          mountPath: /tmp/codeagent
      volumes:
      - name: workspace
        emptyDir: {}
```

## Future Enhancements

### v0.2 Planned Features
- Multi-language support (Python, JavaScript, etc.)
- Advanced prompt engineering
- Code quality validation
- Integration testing automation

### v0.3 Roadmap
- Multiple AI provider support (OpenAI, Gemini)
- Custom code templates
- Advanced context understanding
- Performance optimizations

## Monitoring and Logging

### Metrics Collection
- Request processing time
- Success/failure rates
- Resource usage statistics
- GitHub API usage tracking

### Log Levels
- **DEBUG**: Detailed execution flow
- **INFO**: Key operation milestones
- **WARN**: Recoverable errors
- **ERROR**: Critical failures requiring attention

### Health Checks
- `/health` endpoint for load balancer
- Database connectivity checks
- External service availability
- Resource utilization monitoring