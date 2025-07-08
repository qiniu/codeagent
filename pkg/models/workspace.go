package models

import (
	"time"

	"github.com/google/go-github/v58/github"
)

type Workspace struct {
	ID          string              `json:"id"`
	Path        string              `json:"path"`
	SessionPath string              `json:"session_path"`
	Repository  string              `json:"repository"`
	Branch      string              `json:"branch"`
	Issue       *github.Issue       `json:"issue"`
	PullRequest *github.PullRequest `json:"pull_request"`
	CreatedAt   time.Time           `json:"created_at"`
}

type ExecutionResult struct {
	Success      bool          `json:"success"`
	Output       string        `json:"output"`
	Error        string        `json:"error,omitempty"`
	FilesChanged []string      `json:"files_changed"`
	Duration     time.Duration `json:"duration"`
}
