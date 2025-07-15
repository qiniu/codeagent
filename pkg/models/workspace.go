package models

import (
	"time"

	"github.com/google/go-github/v58/github"
)

type Workspace struct {
	// github org name
	Org string `json:"org"`
	// github repo name
	Repo string `json:"repo"`
	// PR number
	PRNumber int `json:"pr_number"`
	// workspace path in local file system
	Path string `json:"path"`
	// session path in local file system
	SessionPath string `json:"session_path"`
	// github repo url
	Repository string `json:"repository"`
	// github branch name
	Branch      string              `json:"branch"`
	Issue       *github.Issue       `json:"issue"`
	PullRequest *github.PullRequest `json:"pull_request"`
	CreatedAt   time.Time           `json:"created_at"`
	// fork repository information for PR from fork
	ForkOwner string `json:"fork_owner,omitempty"`
	ForkRepo  string `json:"fork_repo,omitempty"`
	ForkURL   string `json:"fork_url,omitempty"`
}

type ExecutionResult struct {
	Success      bool          `json:"success"`
	Output       string        `json:"output"`
	Error        string        `json:"error,omitempty"`
	FilesChanged []string      `json:"files_changed"`
	Duration     time.Duration `json:"duration"`
}
