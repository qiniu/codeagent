# Workspace Management

This module is responsible for managing code agent workspaces, including the creation, movement, and cleanup of Issue, PR, and Session directories.

## Directory Format Specifications

All directories follow a unified naming format that contains AI model information to distinguish different AI processing sessions.

### Issue Directory Format

- **Format**: `{aiModel}-{repo}-issue-{issueNumber}-{timestamp}`
- **Example**: `gemini-codeagent-issue-123-1752829201`

### PR Directory Format

- **Format**: `{aiModel}-{repo}-pr-{prNumber}-{timestamp}`
- **Example**: `gemini-codeagent-pr-161-1752829201`

### Session Directory Format

- **Format**: `{aiModel}-{repo}-session-{prNumber}-{timestamp}`
- **Example**: `gemini-codeagent-session-161-1752829201`

## Core Features

### 1. Directory Format Management (`format.go`)

Provides unified directory format generation and parsing functionality as an internal component of `Manager`:

- `generateIssueDirName()` - Generate Issue directory name
- `generatePRDirName()` - Generate PR directory name
- `generateSessionDirName()` - Generate Session directory name
- `parsePRDirName()` - Parse PR directory name
- `extractSuffixFromPRDir()` - Extract suffix from PR directory name

### 2. Workspace Management (`manager.go`)

Responsible for complete workspace lifecycle management and provides public interfaces for directory formatting:

#### Directory Format Public Methods

- `GenerateIssueDirName()` - Generate Issue directory name
- `GeneratePRDirName()` - Generate PR directory name
- `GenerateSessionDirName()` - Generate Session directory name
- `ParsePRDirName()` - Parse PR directory name
- `ExtractSuffixFromPRDir()` - Extract suffix from PR directory name
- `ExtractSuffixFromIssueDir()` - Extract suffix from Issue directory name

#### Workspace Lifecycle Management

- **Creation**: Create workspace from Issue or PR
- **Movement**: Move Issue workspace to PR workspace
- **Cleanup**: Clean up expired workspaces and resources
- **Session Management**: Create and manage AI session directories

#### Main Methods

##### Workspace Creation

- `CreateWorkspaceFromIssueWithAI()` - Create workspace from Issue
- `GetOrCreateWorkspaceForPRWithAI()` - Get or create PR workspace

##### Workspace Operations

- `MoveIssueToPR()` - Move Issue workspace to PR
- `CreateSessionPath()` - Create Session directory
- `CleanupWorkspace()` - Cleanup workspace

##### Workspace Queries

- `GetAllWorkspacesByPR()` - Get all workspaces for PR
- `GetExpiredWorkspaces()` - Get expired workspaces

## Usage Examples

```go
// Create workspace manager
manager := NewManager(config)

// Call directory format functionality through Manager
prDirName := manager.GeneratePRDirName("gemini", "codeagent", 161, 1752829201)
// Result: "gemini-codeagent-pr-161-1752829201"

// Parse PR directory name
prInfo, err := manager.ParsePRDirName("gemini-codeagent-pr-161-1752829201")
if err == nil {
    fmt.Printf("AI Model: %s, Repo: %s, PR: %d\n",
        prInfo.AIModel, prInfo.Repo, prInfo.PRNumber)
}

// Create workspace from Issue
ws := manager.CreateWorkspaceFromIssueWithAI(issue, "gemini")

// Move to PR
err = manager.MoveIssueToPR(ws, prNumber)

// Create Session directory
sessionPath, err := manager.CreateSessionPath(ws.Path, "gemini", "codeagent", prNumber, "1752829201")
```

## Design Principles

1. **Encapsulation**: `dirFormatter` serves as an internal component of `Manager`, not directly exposed to external code
2. **Unified Interface**: All directory format functionality is accessed through `Manager`'s public methods
3. **Unified Format**: All directories follow the same naming conventions
4. **AI Model Distinction**: Use AI model information to distinguish different processing sessions
5. **Timestamp Identification**: Use timestamps to ensure directory name uniqueness
6. **Lifecycle Management**: Complete workspace creation, movement, and cleanup process
7. **Error Handling**: Comprehensive error handling and logging

## Testing

Run tests to ensure functionality is correct:

```bash
go test ./internal/workspace -v
```

Tests cover the following functionality:

- Directory name generation
- Directory name parsing (including error handling)
- Suffix extraction
- Workspace creation and movement
- Session directory management
