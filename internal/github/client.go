package github

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/qbox/codeagent/internal/config"
	"github.com/qbox/codeagent/pkg/models"

	"github.com/google/go-github/v58/github"
	"github.com/qiniu/x/log"
	"golang.org/x/oauth2"
)

type Client struct {
	client *github.Client
	config *config.Config
}

func NewClient(cfg *config.Config) (*Client, error) {
	if cfg.GitHub.Token == "" {
		return nil, fmt.Errorf("GitHub token is required")
	}

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: cfg.GitHub.Token},
	)
	tc := oauth2.NewClient(context.Background(), ts)
	client := github.NewClient(tc)

	return &Client{
		client: client,
		config: cfg,
	}, nil
}

// CreateBranch 在本地创建分支并推送到远程
func (c *Client) CreateBranch(workspace *models.Workspace) error {
	log.Infof("Creating branch for workspace: %s, path: %s", workspace.ID, workspace.Path)

	// 检查 Git 配置
	c.checkGitConfig(workspace.Path)

	// 创建一个空的 "Initial plan" commit，模仿 Copilot 的行为
	// 这样可以立即建立分支和 PR，提供更好的用户体验
	initialCommitMsg := fmt.Sprintf("Initial plan for Issue #%d: %s",
		workspace.Issue.GetNumber(),
		workspace.Issue.GetTitle())

	// 使用 git commit --allow-empty 创建空提交
	cmd := exec.Command("git", "commit", "--allow-empty", "-m", initialCommitMsg)
	cmd.Dir = workspace.Path
	commitOutput, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create initial empty commit: %w\nCommand output: %s", err, string(commitOutput))
	}

	// 检查当前分支状态
	cmd = exec.Command("git", "status")
	cmd.Dir = workspace.Path
	statusOutput, err := cmd.CombinedOutput()
	if err != nil {
		log.Warnf("Failed to check git status: %v", err)
	} else {
		log.Infof("Git status:\n%s", string(statusOutput))
	}

	// 推送分支到远程
	cmd = exec.Command("git", "push", "-u", "origin", workspace.Branch)
	cmd.Dir = workspace.Path

	// 捕获命令的输出和错误
	pushOutput, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to push branch: %w\nCommand output: %s", err, string(pushOutput))
	}

	log.Infof("Created and pushed branch: %s", workspace.Branch)
	return nil
}

// checkGitConfig 检查 Git 配置
func (c *Client) checkGitConfig(workspacePath string) {
	log.Infof("Checking Git configuration for workspace: %s", workspacePath)

	// 检查 Git 用户配置
	cmd := exec.Command("git", "config", "user.name")
	cmd.Dir = workspacePath
	nameOutput, err := cmd.Output()
	if err != nil {
		log.Warnf("Failed to get git user.name: %v", err)
	} else {
		log.Infof("Git user.name: %s", strings.TrimSpace(string(nameOutput)))
	}

	cmd = exec.Command("git", "config", "user.email")
	cmd.Dir = workspacePath
	emailOutput, err := cmd.Output()
	if err != nil {
		log.Warnf("Failed to get git user.email: %v", err)
	} else {
		log.Infof("Git user.email: %s", strings.TrimSpace(string(emailOutput)))
	}

	// 检查远程仓库配置
	cmd = exec.Command("git", "remote", "-v")
	cmd.Dir = workspacePath
	remoteOutput, err := cmd.Output()
	if err != nil {
		log.Warnf("Failed to get remote configuration: %v", err)
	} else {
		log.Infof("Remote configuration:\n%s", string(remoteOutput))
	}
}

// CreatePullRequest 创建 Pull Request
func (c *Client) CreatePullRequest(workspace *models.Workspace) (*github.PullRequest, error) {
	// 解析仓库信息
	repoOwner, repoName := c.parseRepoURL(workspace.Repository)
	if repoOwner == "" || repoName == "" {
		return nil, fmt.Errorf("invalid repository URL: %s", workspace.Repository)
	}

	// 创建 PR
	prTitle := fmt.Sprintf("实现 Issue #%d: %s", workspace.Issue.GetNumber(), workspace.Issue.GetTitle())
	prBody := fmt.Sprintf(`## 实现计划

这是由 XGo Agent 自动生成的 PR，用于实现 Issue #%d。

### Issue 描述
%s

### 实现计划
- [ ] 分析需求并制定实现方案
- [ ] 编写核心代码
- [ ] 添加测试用例
- [ ] 代码审查和优化

---
*此 PR 由 XGo Agent 自动创建，将逐步完善实现*`,
		workspace.Issue.GetNumber(),
		workspace.Issue.GetBody())

	newPR := &github.NewPullRequest{
		Title: &prTitle,
		Body:  &prBody,
		Head:  &workspace.Branch,
		Base:  github.String("main"), // 假设主分支是 main
	}

	pr, _, err := c.client.PullRequests.Create(context.Background(), repoOwner, repoName, newPR)
	if err != nil {
		return nil, fmt.Errorf("failed to create PR: %w", err)
	}

	log.Infof("Created PR: %s", pr.GetHTMLURL())
	return pr, nil
}

// CommitAndPush 检测文件变更并提交推送
func (c *Client) CommitAndPush(workspace *models.Workspace, result *models.ExecutionResult) error {
	// 检查是否有文件变更
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = workspace.Path
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to check git status: %w", err)
	}

	if strings.TrimSpace(string(output)) == "" {
		log.Infof("No changes detected in workspace")
		return nil
	}

	// 添加所有变更
	cmd = exec.Command("git", "add", ".")
	cmd.Dir = workspace.Path
	addOutput, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to add changes: %w\nCommand output: %s", err, string(addOutput))
	}

	// 创建提交
	commitMsg := fmt.Sprintf("实现 Issue #%d: %s\n\n%s",
		workspace.Issue.GetNumber(),
		workspace.Issue.GetTitle(),
		result.Output)

	cmd = exec.Command("git", "commit", "-m", commitMsg)
	cmd.Dir = workspace.Path
	commitOutput, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to commit changes: %w\nCommand output: %s", err, string(commitOutput))
	}

	// 推送到远程
	cmd = exec.Command("git", "push")
	cmd.Dir = workspace.Path
	pushOutput, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to push changes: %w\nCommand output: %s", err, string(pushOutput))
	}

	log.Infof("Committed and pushed changes for Issue #%d", workspace.Issue.GetNumber())
	return nil
}

// CommitAndPush 检测文件变更并提交推送
func (c *Client) Push(workspace *models.Workspace) error {
	// 推送到远程
	cmd := exec.Command("git", "push")
	cmd.Dir = workspace.Path
	pushOutput, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to push changes: %w\nCommand output: %s", err, string(pushOutput))
	}

	log.Infof("Committed and pushed changes for Issue #%d", workspace.Issue.GetNumber())
	return nil
}

// GetPullRequestChanges 获取 PR 的变更内容 (diff)
func (c *Client) GetPullRequestChanges(pr *github.PullRequest) (string, error) {
	repoOwner, repoName := c.parseRepoURL(pr.GetHTMLURL())
	if repoOwner == "" || repoName == "" {
		return "", fmt.Errorf("invalid repository URL: %s", pr.GetHTMLURL())
	}

	diff, _, err := c.client.PullRequests.GetRaw(context.Background(), repoOwner, repoName, pr.GetNumber(), github.RawOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get PR diff: %w", err)
	}

	return diff, nil
}

// CreatePullRequestComment 在 PR 上创建评论
func (c *Client) CreatePullRequestComment(pr *github.PullRequest, commentBody string) error {
	prURL := pr.GetHTMLURL()
	log.Infof("Creating comment for PR URL: %s", prURL)

	repoOwner, repoName := c.parseRepoURL(prURL)
	if repoOwner == "" || repoName == "" {
		return fmt.Errorf("invalid repository URL: %s", prURL)
	}

	log.Infof("Parsed repository: %s/%s, PR number: %d", repoOwner, repoName, pr.GetNumber())

	// 使用 Issue Comments API 来创建 PR 评论
	// PR 实际上也是一种 Issue，所以可以使用 Issue Comments API
	comment := &github.IssueComment{
		Body: &commentBody,
	}

	_, _, err := c.client.Issues.CreateComment(context.Background(), repoOwner, repoName, pr.GetNumber(), comment)
	if err != nil {
		return fmt.Errorf("failed to create PR comment: %w", err)
	}

	log.Infof("Created comment on PR #%d", pr.GetNumber())
	return nil
}

// UpdatePullRequest 更新 PR 的 Body
func (c *Client) UpdatePullRequest(pr *github.PullRequest, newBody string) error {
	prURL := pr.GetHTMLURL()
	log.Infof("Updating PR body for URL: %s", prURL)

	repoOwner, repoName := c.parseRepoURL(prURL)
	if repoOwner == "" || repoName == "" {
		return fmt.Errorf("invalid repository URL: %s", prURL)
	}

	log.Infof("Parsed repository: %s/%s, PR number: %d", repoOwner, repoName, pr.GetNumber())

	prRequest := &github.PullRequest{Body: &newBody}
	_, _, err := c.client.PullRequests.Edit(context.Background(), repoOwner, repoName, pr.GetNumber(), prRequest)
	if err != nil {
		return fmt.Errorf("failed to update PR body: %w", err)
	}

	log.Infof("Updated PR #%d body", pr.GetNumber())
	return nil
}

// GetPullRequestComments 获取 PR 的评论
func (c *Client) GetPullRequestComments(pr *github.PullRequest) ([]*github.PullRequestComment, error) {
	prURL := pr.GetHTMLURL()
	log.Infof("Getting comments for PR URL: %s", prURL)

	repoOwner, repoName := c.parseRepoURL(prURL)
	if repoOwner == "" || repoName == "" {
		return nil, fmt.Errorf("invalid repository URL: %s", prURL)
	}

	log.Infof("Parsed repository: %s/%s, PR number: %d", repoOwner, repoName, pr.GetNumber())

	comments, _, err := c.client.PullRequests.ListComments(context.Background(), repoOwner, repoName, pr.GetNumber(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get PR comments: %w", err)
	}

	return comments, nil
}

// parseRepoURL 解析仓库 URL 获取 owner 和 repo 名称
func (c *Client) parseRepoURL(repoURL string) (owner, repo string) {
	// 处理 HTTPS URL: https://github.com/owner/repo.git
	if strings.Contains(repoURL, "github.com") {
		parts := strings.Split(repoURL, "/")
		if len(parts) >= 2 {
			// 处理 PR URL: https://github.com/owner/repo/pull/11
			if len(parts) >= 4 && parts[len(parts)-2] == "pull" {
				repo = parts[len(parts)-3]
				owner = parts[len(parts)-4]
			} else {
				// 处理仓库 URL: https://github.com/owner/repo.git
				repo = strings.TrimSuffix(parts[len(parts)-1], ".git")
				owner = parts[len(parts)-2]
			}
		}
	}
	return owner, repo
}
