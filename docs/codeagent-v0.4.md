# CodeAgent v0.4 - Git Worktree-Based Workspace Management Design

## Overview

CodeAgent v0.4 adopts a minimalist workspace management solution, completely based on Git worktree mechanisms. All workspaces are uniquely identified by directory names only, requiring no additional mapping files or persistent metadata, allowing the system to automatically recover all states after restarts.

## Design Principles

1. **Directory Uniqueness**: Each workspace directory name is unique, containing key information (such as repo, issue/pr number, timestamp).
2. **No Mapping/No Additional Metadata**: All states are expressed solely through directory names, requiring no mapping files or databases.
3. **Minimal Recovery**: System startup only needs to scan directory names to recover all workspaces.
4. **Directory Isolation**: All worktree directories are at the same level as repository directories, keeping repository internal structure clean.

## Workspace Lifecycle

### 1. Issue Workspace

- Directory name format when created: `{repo}-issue-{issue-number}-{timestamp}`
- Example: `codeagent-issue-123-1703123456789`

### 2. PR Workspace

- After PR creation, directory name format: `{repo}-pr-{pr-number}-{timestamp}`
- Example: `codeagent-pr-91-1703123456789`

Session directory unified as: `{repo}-session-{pr-number}-{timestamp}`

### 3. Directory Structure Example

```
basedir/
├── qbox/
│   ├── codeagent/                  # Repository directory
│   │   ├── .git/                   # Shared Git repository
│   ├── codeagent-issue-124-1703123456790/   # Issue workspace
│   ├── codeagent-pr-91-1703123456789/       # PR workspace
│   ├── codeagent-session-issue-124-1703123456790/  # Issue session directory
│   ├── codeagent-session-pr-91-1703123456789/      # PR session directory
```

## Recovery and Cleanup Mechanisms

- **Recovery**: At system startup, recursively scan all organization/repository directories for worktree directories (`{repo}-issue-*`, `{repo}-pr-*`) and session directories (`{repo}-session-issue-*`, `{repo}-session-pr-*`), parse issue/pr numbers and timestamps from directory names, and automatically recover all workspaces.
- **Cleanup**: Simply judge expiration based on directory names and business logic, then directly delete corresponding worktree directories and session directories.

## Main Advantages

- **Minimalist**: No redundant metadata, directory is state.
- **Robust**: Even after abnormal restarts, directory structure remains unchanged, all workspaces can be recovered.
- **High Performance**: Fully utilize Git worktree's native capabilities, no need for repeated clones.
- **Easy Maintenance**: Clear directory structure, convenient for manual troubleshooting and automated script processing.

## Summary

CodeAgent v0.4's workspace management solution completely abandons complex mechanisms like mapping, move operations, and databases, relying entirely on Git worktree and directory name uniqueness to achieve extremely simple, robust, and recoverable multi-workspace management.
