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
	// fork-related fields
	IsFork         bool   `json:"is_fork"`          // 是否是fork仓库的PR
	ForkOrg        string `json:"fork_org"`         // fork仓库的组织名
	ForkRepo       string `json:"fork_repo"`        // fork仓库的仓库名
	ForkRepository string `json:"fork_repository"`  // fork仓库的URL
}

type ExecutionResult struct {
	Success      bool          `json:"success"`
	Output       string        `json:"output"`
	Error        string        `json:"error,omitempty"`
	FilesChanged []string      `json:"files_changed"`
	Duration     time.Duration `json:"duration"`
}
