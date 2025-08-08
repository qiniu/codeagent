# CodeAgent v0.3 - Local CLI Mode and Architecture Optimization

## Overview

CodeAgent v0.3 is a significant architecture optimization release with main improvements including:

1. **Local CLI Mode Support**: Support for directly using locally installed Claude CLI or Gemini CLI
2. **Gemini Implementation Refactor**: Simplified Gemini implementation, removed complex prompt construction logic
3. **Prompt Optimization**: Fixed the issue of Gemini CLI actively accessing GitHub API
4. **GitHub API Compatibility**: Fixed API compatibility issues with PR comment creation
5. **Error Handling Improvements**: Added retry mechanism and better error handling

## Key Features

### 1. Dual Mode Support

CodeAgent now supports two operating modes:

- **Docker Mode** (default): Use Docker containers to run Claude Code or Gemini CLI
- **Local CLI Mode**: Directly use locally installed Claude CLI or Gemini CLI

### 2. Gemini Implementation Supports Dual Modes

#### Architecture Improvements

- **Docker Mode Maintains Interactive**: Uses stdin/stdout pipes to interact with continuously running containers
- **Local CLI Mode Uses Single Calls**: Uses `gemini --prompt` command, starting a new process each time
- **Relies on Native File Context**: Gemini CLI automatically reads files in the working directory as context
- **Avoids Broken Pipe Issues**: Local single-call mode avoids connection drops caused by continuous interaction
- **Claude Remains Simple**: Only supports Docker mode, avoiding untested features

#### Code Structure

```
internal/code/
├── code.go           # Factory function, selects implementation based on configuration
├── claude_docker.go  # Claude Docker implementation
├── claude_local.go   # Claude local CLI implementation
├── gemini_docker.go  # Gemini Docker implementation
├── gemini_local.go   # Gemini local CLI implementation
└── session.go        # Session management
```

#### Implementation Comparison

**Claude Implementation (Interactive Mode):**

```go
type claudeCode struct {
    cmd    *exec.Cmd
    stdin  io.WriteCloser
    stdout io.ReadCloser
}

func (c *claudeCode) Prompt(message string) (*Response, error) {
    if _, err := c.stdin.Write([]byte(message + "\n")); err != nil {
        return nil, err
    }
    return &Response{Out: c.stdout}, nil
}
```

**Gemini Local Implementation (Single Call Mode):**

```go
type geminiLocal struct {
    workspace *models.Workspace
    config    *config.Config
}

func (g *geminiLocal) Prompt(message string) (*Response, error) {
    output, err := g.executeGeminiLocal(message)
    if err != nil {
        return nil, fmt.Errorf("failed to execute gemini prompt: %w", err)
    }
    return &Response{Out: bytes.NewReader(output)}, nil
}
```

### 3. Prompt Optimization

#### Issue Fixes

- **Avoid Network Requests**: No longer include GitHub API URLs in prompts
- **Direct Content Passing**: Pass Issue title and description directly to AI
- **Improve Reliability**: Not dependent on external network connections and API permissions

#### Before and After Comparison

```go
// Before - Contains URL, causing Gemini CLI to actively access
prompt := fmt.Sprintf("This is Issue content %s, organize modification plan based on Issue content", issue.GetURL())

// Now - Direct content passing
prompt := fmt.Sprintf(`This is Issue content:

Title: %s
Description: %s

Please organize a modification plan based on the above Issue content.`, issue.GetTitle(), issue.GetBody())
```

### 4. GitHub API Compatibility Fix

#### PR Comment API Fix

- **Issue**: GitHub API's PR comment interface changed, `PullRequests.CreateComment` requires additional positioning parameters
- **Solution**: Use `Issues.CreateComment` API, as PRs are actually a type of Issue

```go
// Before
comment := &github.PullRequestComment{Body: &commentBody}
_, _, err := c.client.PullRequests.CreateComment(ctx, repoOwner, repoName, pr.GetNumber(), comment)

// Now
comment := &github.IssueComment{Body: &commentBody}
_, _, err := c.client.Issues.CreateComment(ctx, repoOwner, repoName, pr.GetNumber(), comment)
```

### 5. Error Handling Improvements

#### Retry Mechanism

Added `promptWithRetry` method supporting automatic retry:

```go
func (a *Agent) promptWithRetry(code code.Code, prompt string, maxRetries int) (*code.Response, error) {
    for attempt := 1; attempt <= maxRetries; attempt++ {
        resp, err := code.Prompt(prompt)
        if err == nil {
            return resp, nil
        }

        // Special handling for broken pipe errors
        if strings.Contains(err.Error(), "broken pipe") {
            log.Infof("Detected broken pipe, will retry...")
        }

        if attempt < maxRetries {
            time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
        }
    }
    return nil, fmt.Errorf("failed after %d attempts", maxRetries)
}
```

## Configuration Options

### New Configuration

```yaml
# config.yaml
code_provider: gemini # claude or gemini
use_docker: false # true: Docker mode, false: local CLI mode
```

```bash
# Environment variables
export USE_DOCKER=false      # Enable local CLI mode
export CODE_PROVIDER=gemini  # or claude
```

### Configuration Loading Logic

```go
type Config struct {
    // ... other configurations
    CodeProvider string `yaml:"code_provider"`
    UseDocker    bool   `yaml:"use_docker"`
}

func (c *Config) loadFromEnv() {
    // Load configuration from environment variables
    if useDockerStr := os.Getenv("USE_DOCKER"); useDockerStr != "" {
        if useDocker, err := strconv.ParseBool(useDockerStr); err == nil {
            c.UseDocker = useDocker
        }
    }
    // Default value: UseDocker = true
}
```

## Code Architecture

### Factory Pattern Refactor

```go
func New(workspace *models.Workspace, cfg *config.Config) (Code, error) {
    // Create appropriate code provider based on code provider and use_docker configuration
    switch cfg.CodeProvider {
    case ProviderClaude:
        if cfg.UseDocker {
            return NewClaudeDocker(workspace, cfg)
        }
        return NewClaudeLocal(workspace, cfg)
    case ProviderGemini:
        if cfg.UseDocker {
            return NewGeminiDocker(workspace, cfg)
        }
        return NewGeminiLocal(workspace, cfg)
    default:
        return nil, fmt.Errorf("unsupported code provider: %s", cfg.CodeProvider)
    }
}
```

### Execution Mode Description

| Provider | Docker Mode | Local CLI Mode | Execution Method                                          |
| -------- | ----------- | -------------- | --------------------------------------------------------- |
| Claude   | Interactive | Not Supported  | stdin/stdout pipe                                         |
| Gemini   | Interactive | Single Call    | Docker: stdin/stdout pipe<br>Local: `gemini --prompt` command |

### Gemini Implementation Simplification

#### Local CLI Implementation

```go
type geminiLocal struct {
    workspace *models.Workspace
    config    *config.Config
}

func (g *geminiLocal) Prompt(message string) (*Response, error) {
    // Direct execution of local gemini CLI call
    output, err := g.executeGeminiLocal(message)
    if err != nil {
        return nil, fmt.Errorf("failed to execute gemini prompt: %w", err)
    }

    return &Response{Out: bytes.NewReader(output)}, nil
}

func (g *geminiLocal) executeGeminiLocal(prompt string) ([]byte, error) {
    args := []string{"--prompt", prompt}
    cmd := exec.CommandContext(ctx, "gemini", args...)
    cmd.Dir = g.workspace.Path // Set working directory, Gemini CLI will automatically read files in this directory as context

    // Execute command and get output
    output, err := cmd.CombinedOutput()
    // ... error handling
    return output, nil
}
```

#### Docker Implementation

```go
type geminiDocker struct {
    workspace *models.Workspace
    config    *config.Config
}

func (g *geminiDocker) executeGeminiDocker(prompt string) ([]byte, error) {
    args := []string{
        "run", "--rm",
        "-v", fmt.Sprintf("%s:/workspace", g.workspace.Path),
        "-w", "/workspace", // Set working directory, Gemini CLI will automatically read files in this directory as context
        g.config.Gemini.ContainerImage,
        "gemini", "--prompt", prompt,
    }

    cmd := exec.CommandContext(ctx, "docker", args...)
    // ... execute command
}
```

## Usage

### 1. Install Local CLI Tools

#### Gemini CLI

```bash
# Install Gemini CLI
# Reference: https://github.com/google-gemini/gemini-cli
```

#### Claude CLI

```bash
# Install Claude CLI
# Reference: https://github.com/anthropics/anthropic-claude-code
```

### 2. Configure Environment Variables

```bash
# Set local mode
export USE_DOCKER=false
export CODE_PROVIDER=gemini  # or claude

# Set necessary authentication information
export GITHUB_TOKEN="your-github-token"
export GEMINI_API_KEY="your-gemini-api-key"  # or CLAUDE_API_KEY
export WEBHOOK_SECRET="your-webhook-secret"
```

### 3. Start Service

```bash
# Using environment variables
go run ./cmd/server

# Or using configuration file
# Set use_docker: false in config.yaml
go run ./cmd/server --config config.yaml
```

### 4. Test Local Mode

Use the provided test script:

```bash
./scripts/test-local-mode.sh
```

## Advantages

### Advantages of Local CLI Mode

1. **Faster startup speed**: No need to start Docker containers
2. **Less resource consumption**: No Docker runtime required
3. **Better debugging experience**: Can directly debug local CLI tools
4. **Simpler deployment**: No Docker environment required
5. **Avoid broken pipe**: Single prompt mode avoids continuous interaction issues

### Advantages of Architecture Optimization

1. **Multiple execution modes**: Supports interactive mode (Claude, Gemini Docker) and single-call mode (Gemini local)
2. **Avoid broken pipe issues**: Gemini local single-call mode avoids connection drops caused by continuous interaction
3. **More reliable**: Gemini local always uses fresh processes, avoiding issues caused by state accumulation
4. **More efficient**: Gemini CLI natively supports file context, better performance
5. **Better error handling**: Added retry mechanism and special error handling
6. **Keep it simple**: Claude only supports Docker mode, avoiding untested features

### Advantages of Docker Mode

1. **Environment isolation**: Completely isolated execution environment
2. **Consistency**: Ensures running the same environment on different machines
3. **Easy to manage**: Unified container management

## Configuration Examples

### Environment Variable Configuration

```bash
# .env file
USE_DOCKER=false
CODE_PROVIDER=gemini
GITHUB_TOKEN=your-github-token
GEMINI_API_KEY=your-gemini-api-key
WEBHOOK_SECRET=your-webhook-secret
PORT=8888
```

### Configuration File

```yaml
# config.yaml
server:
  port: 8888

github:
  webhook_url: "http://localhost:8888/hook"

workspace:
  base_dir: "/tmp/codeagent"
  cleanup_after: "24h"

claude:
  container_image: "anthropic/claude-code:latest"
  timeout: "30m"

gemini:
  container_image: "google-gemini/gemini-cli:latest"
  timeout: "30m"

docker:
  socket: "unix:///var/run/docker.sock"
  network: "bridge"

code_provider: gemini
use_docker: false # Local CLI mode
```

## Troubleshooting

### Common Issues

1. **CLI tools not found**

   ```bash
   # Check if installed
   which gemini
   which claude
   ```

2. **Permission issues**

   ```bash
   # Ensure CLI tools have execute permissions
   chmod +x $(which gemini)
   chmod +x $(which claude)
   ```

3. **Workspace access issues**

   ```bash
   # Ensure workspace directory exists and has write permissions
   mkdir -p /tmp/codeagent
   chmod 755 /tmp/codeagent
   ```

4. **API key issues**
   ```bash
   # Ensure correct API keys are set
   export GEMINI_API_KEY="your-api-key"
   export GOOGLE_API_KEY="your-api-key"  # Needed in some cases
   ```

### Debug Mode

```bash
# Enable verbose logging
export LOG_LEVEL=debug
go run ./cmd/server --config config.yaml
```

## Version History

### v0.3 Major Improvements

1. **Gemini supports dual modes**: Docker mode maintains interactive, local CLI mode uses single calls
2. **Local CLI support**: Added local CLI mode, supports direct use of local tools
3. **Prompt optimization**: Fixed the issue of Gemini CLI actively accessing GitHub API
4. **API compatibility**: Fixed GitHub API compatibility issues with PR comment creation
5. **Error handling**: Added retry mechanism and better error handling
6. **Configuration optimization**: Added `use_docker` configuration option

### Backward Compatibility

- Maintains compatibility with existing configurations
- Docker mode is still the default mode
- Existing API interfaces remain unchanged