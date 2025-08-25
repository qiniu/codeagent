package workspace

import (
	"errors"
	"fmt"
)

// Predefined error types for workspace operations
var (
	// Repository errors
	ErrWorkspaceNotFound     = errors.New("workspace not found")
	ErrWorkspaceAlreadyExists = errors.New("workspace already exists")
	ErrInvalidWorkspace      = errors.New("invalid workspace")

	// Git errors
	ErrGitCloneFailed        = errors.New("git clone failed")
	ErrGitBranchNotFound     = errors.New("git branch not found")
	ErrGitRemoteURLNotFound  = errors.New("git remote URL not found")
	ErrGitOperationFailed    = errors.New("git operation failed")

	// Container errors
	ErrContainerNotFound     = errors.New("container not found")
	ErrContainerOperationFailed = errors.New("container operation failed")

	// Directory errors
	ErrDirectoryCreationFailed = errors.New("directory creation failed")
	ErrDirectoryNotFound      = errors.New("directory not found")
	ErrDirectoryFormatInvalid = errors.New("directory format invalid")

	// Filesystem errors
	ErrFileSystemOperationFailed = errors.New("filesystem operation failed")
	ErrPathNotFound              = errors.New("path not found")
)

// WorkspaceError represents a workspace-related error with context
type WorkspaceError struct {
	Op       string // Operation that failed
	Path     string // Workspace path (if applicable)
	Err      error  // Underlying error
	Context  string // Additional context
}

func (e *WorkspaceError) Error() string {
	if e.Path != "" && e.Context != "" {
		return fmt.Sprintf("workspace %s failed at %s: %v (context: %s)", e.Op, e.Path, e.Err, e.Context)
	}
	if e.Path != "" {
		return fmt.Sprintf("workspace %s failed at %s: %v", e.Op, e.Path, e.Err)
	}
	if e.Context != "" {
		return fmt.Sprintf("workspace %s failed: %v (context: %s)", e.Op, e.Err, e.Context)
	}
	return fmt.Sprintf("workspace %s failed: %v", e.Op, e.Err)
}

func (e *WorkspaceError) Unwrap() error {
	return e.Err
}

// NewWorkspaceError creates a new WorkspaceError
func NewWorkspaceError(op, path string, err error, context string) *WorkspaceError {
	return &WorkspaceError{
		Op:      op,
		Path:    path,
		Err:     err,
		Context: context,
	}
}

// Helper functions for creating specific errors

// GitError creates a git-related error
func GitError(op, path string, err error) error {
	return NewWorkspaceError(op, path, fmt.Errorf("git: %w", err), "")
}

// ContainerError creates a container-related error
func ContainerError(op string, containerName string, err error) error {
	return NewWorkspaceError(op, "", fmt.Errorf("container: %w", err), containerName)
}

// DirectoryError creates a directory-related error
func DirectoryError(op, path string, err error) error {
	return NewWorkspaceError(op, path, fmt.Errorf("directory: %w", err), "")
}

// FileSystemError creates a filesystem-related error
func FileSystemError(op, path string, err error) error {
	return NewWorkspaceError(op, path, fmt.Errorf("filesystem: %w", err), "")
}

// RepositoryError creates a repository-related error
func RepositoryError(op string, err error, context string) error {
	return NewWorkspaceError(op, "", fmt.Errorf("repository: %w", err), context)
}

// IsTemporaryError checks if an error might be temporary/retryable
func IsTemporaryError(err error) bool {
	if err == nil {
		return false
	}

	// Check for specific temporary error conditions
	// This can be expanded based on actual error patterns observed
	errorStr := err.Error()
	
	// Network-related errors that might be temporary
	temporaryPatterns := []string{
		"connection refused",
		"timeout",
		"network is unreachable",
		"temporary failure",
		"resource temporarily unavailable",
		"no such host", // Sometimes DNS issues are temporary
	}

	for _, pattern := range temporaryPatterns {
		if contains(errorStr, pattern) {
			return true
		}
	}

	return false
}

// contains checks if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && 
		   (s == substr || 
		    (len(s) > len(substr) && 
		     (s[:len(substr)] == substr || 
		      s[len(s)-len(substr):] == substr || 
		      indexOf(s, substr) >= 0)))
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}