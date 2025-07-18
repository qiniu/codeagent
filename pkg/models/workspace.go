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
	// fork repository info (for fork PR collaboration)
	ForkInfo *ForkInfo `json:"fork_info,omitempty"`
}

// ForkInfo contains information about the fork repository
type ForkInfo struct {
	// fork repository owner
	Owner string `json:"owner"`
	// fork repository name
	Repo string `json:"repo"`
	// fork repository URL
	URL string `json:"url"`
	// original branch name in fork repo
	Branch string `json:"branch"`
	// collaboration branch name in fork repo (created by CodeAgent)
	CollabBranch string `json:"collab_branch"`
}

type ExecutionResult struct {
	Success      bool          `json:"success"`
	Output       string        `json:"output"`
	Error        string        `json:"error,omitempty"`
	FilesChanged []string      `json:"files_changed"`
	Duration     time.Duration `json:"duration"`
}

// PRAllComments 包含 PR 的所有评论信息
type PRAllComments struct {
	PRBody         string                       `json:"pr_body"`
	IssueComments  []*github.IssueComment       `json:"issue_comments"`
	ReviewComments []*github.PullRequestComment `json:"review_comments"`
	Reviews        []*github.PullRequestReview  `json:"reviews"`
}
