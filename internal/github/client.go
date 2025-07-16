package github

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/qbox/codeagent/internal/code"
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
	log.Infof("Creating branch for workspace: %s, path: %s", workspace.Branch, workspace.Path)

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

这是由 Code Agent 自动生成的 PR，用于实现 Issue #%d。

### Issue 描述
%s

### 实现计划
- [ ] 分析需求并制定实现方案
- [ ] 编写核心代码
- [ ] 添加测试用例
- [ ] 代码审查和优化

---
*此 PR 由 Code Agent(https://github.com/qbox/codeagent) 自动创建，将逐步完善实现*`,
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
func (c *Client) CommitAndPush(workspace *models.Workspace, result *models.ExecutionResult, codeClient code.Code) error {
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

	// 使用AI生成标准的英文commit message
	commitMsg, err := c.generateCommitMessage(workspace, result, codeClient)
	if err != nil {
		log.Errorf("Failed to generate commit message with AI, using fallback: %v", err)
		// 使用fallback的commit message
		summary := extractSummaryFromOutput(result.Output)
		commitMsg = fmt.Sprintf("feat: implement Issue #%d - %s\n\n%s",
			workspace.Issue.GetNumber(),
			workspace.Issue.GetTitle(),
			summary)
	}

	cmd = exec.Command("git", "commit", "-m", commitMsg)
	cmd.Dir = workspace.Path
	commitOutput, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to commit changes: %w\nCommand output: %s", err, string(commitOutput))
	}

	// 推送到远程（带冲突处理）
	cmd = exec.Command("git", "push")
	cmd.Dir = workspace.Path
	pushOutput, err := cmd.CombinedOutput()
	if err != nil {
		pushOutputStr := string(pushOutput)
		log.Infof("Push failed, output: %s", pushOutputStr)

		// 检查是否是推送冲突（更宽松的检测）
		if strings.Contains(pushOutputStr, "non-fast-forward") {
			log.Infof("Push failed due to remote changes, attempting to resolve conflict")

			// 拉取成功后，再次尝试推送
			cmd = exec.Command("git", "push")
			cmd.Dir = workspace.Path
			pushOutput2, err2 := cmd.CombinedOutput()
			if err2 != nil {
				log.Errorf("Push still failed after pull, attempting force push")
				return fmt.Errorf("failed to push changes: %w\nCommand output: %s", err2, string(pushOutput2))
			}

			log.Infof("Successfully pushed changes after pulling remote updates")
			return nil
		}

		return fmt.Errorf("failed to push changes: %w\nCommand output: %s", err, string(pushOutput))
	}

	log.Infof("Committed and pushed changes for Issue #%d", workspace.Issue.GetNumber())
	return nil
}

// PullLatestChanges 拉取远端最新代码（优先使用rebase策略）
func (c *Client) PullLatestChanges(workspace *models.Workspace, pr *github.PullRequest) error {
	log.Infof("Pulling latest changes for workspace: %s (PR #%d)", workspace.Path, pr.GetNumber())

	// 获取 PR 的目标分支（base branch）
	baseBranch := pr.GetBase().GetRef()
	if baseBranch == "" {
		log.Errorf("PR base branch is empty for PR #%d", pr.GetNumber())
		return fmt.Errorf("PR base branch is empty for PR #%d", pr.GetNumber())
	}

	// 获取 PR 的源分支（head branch）
	headBranch := pr.GetHead().GetRef()
	if headBranch == "" {
		log.Errorf("PR head branch is empty for PR #%d", pr.GetNumber())
		return fmt.Errorf("PR head branch is empty for PR #%d", pr.GetNumber())
	}

	log.Infof("PR #%d: %s -> %s", pr.GetNumber(), headBranch, baseBranch)

	// 1. 获取所有远程引用
	cmd := exec.Command("git", "fetch", "--all", "--prune")
	cmd.Dir = workspace.Path
	fetchOutput, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to fetch latest changes: %w\nCommand output: %s", err, string(fetchOutput))
	}
	log.Infof("Fetched all remote references for PR #%d", pr.GetNumber())

	// 2. 检查当前是否有未提交的变更
	cmd = exec.Command("git", "status", "--porcelain")
	cmd.Dir = workspace.Path
	statusOutput, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to check git status: %w", err)
	}

	hasChanges := strings.TrimSpace(string(statusOutput)) != ""
	if hasChanges {
		// 有未提交的变更，先 stash
		log.Infof("Found uncommitted changes in worktree, stashing them")
		cmd = exec.Command("git", "stash", "push", "-m", fmt.Sprintf("Auto stash before syncing PR #%d", pr.GetNumber()))
		cmd.Dir = workspace.Path
		stashOutput, err := cmd.CombinedOutput()
		if err != nil {
			log.Warnf("Failed to stash changes: %v, output: %s", err, string(stashOutput))
		}
	}

	// 3. 尝试直接获取 PR 内容
	prNumber := pr.GetNumber()
	log.Infof("Attempting to fetch PR #%d content directly", prNumber)
	cmd = exec.Command("git", "fetch", "origin", fmt.Sprintf("pull/%d/head", prNumber))
	cmd.Dir = workspace.Path
	fetchOutput, err = cmd.CombinedOutput()
	if err == nil {
		// 直接获取成功，使用rebase合并更新
		log.Infof("Successfully fetched PR #%d content, attempting rebase", prNumber)
		cmd = exec.Command("git", "rebase", "FETCH_HEAD")
		cmd.Dir = workspace.Path
		rebaseOutput, err := cmd.CombinedOutput()
		if err != nil {
			log.Errorf("Rebase failed, trying reset: %v, output: %s", err, string(rebaseOutput))
			// rebase失败，强制切换到PR内容
			cmd = exec.Command("git", "reset", "--hard", "FETCH_HEAD")
			cmd.Dir = workspace.Path
			resetOutput, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("failed to reset to PR #%d: %w\nCommand output: %s", prNumber, err, string(resetOutput))
			}
			log.Infof("Hard reset worktree to PR #%d content", prNumber)
		} else {
			log.Infof("Successfully rebased worktree to PR #%d content", prNumber)
		}
	} else {
		// 直接获取失败，使用传统rebase方式
		log.Errorf("Failed to fetch PR #%d directly: %v, falling back to traditional rebase, output: %s", prNumber, err, string(fetchOutput))

		// 尝试rebase到目标分支的最新代码
		cmd = exec.Command("git", "rebase", fmt.Sprintf("origin/%s", baseBranch))
		cmd.Dir = workspace.Path
		rebaseOutput, err := cmd.CombinedOutput()
		if err != nil {
			log.Errorf("Rebase to base branch failed: %v, output: %s", err, string(rebaseOutput))
			// rebase失败，尝试强制同步到基础分支
			cmd = exec.Command("git", "reset", "--hard", fmt.Sprintf("origin/%s", baseBranch))
			cmd.Dir = workspace.Path
			resetOutput, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("failed to reset to base branch %s: %w\nCommand output: %s", baseBranch, err, string(resetOutput))
			}
			log.Infof("Hard reset worktree to base branch %s", baseBranch)
		} else {
			log.Infof("Successfully rebased worktree to base branch %s", baseBranch)
		}
	}

	// 4. 如果之前有stash，尝试恢复
	if hasChanges {
		log.Infof("Attempting to restore stashed changes for PR #%d", prNumber)
		cmd = exec.Command("git", "stash", "pop")
		cmd.Dir = workspace.Path
		stashPopOutput, err := cmd.CombinedOutput()
		if err != nil {
			log.Warnf("Failed to restore stashed changes: %v, output: %s", err, string(stashPopOutput))
			log.Infof("You may need to manually resolve the stashed changes later")
		} else {
			log.Infof("Successfully restored stashed changes")
		}
	}

	log.Infof("Successfully pulled latest changes for PR #%d using rebase strategy", pr.GetNumber())
	return nil
}

// Push 推送当前分支到远程
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

// GetPullRequest 获取 PR 的完整信息
func (c *Client) GetPullRequest(owner, repo string, prNumber int) (*github.PullRequest, error) {
	pr, _, err := c.client.PullRequests.Get(context.Background(), owner, repo, prNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to get PR #%d: %w", prNumber, err)
	}
	return pr, nil
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

// ReplyToReviewComment 回复 PR 代码行评论
func (c *Client) ReplyToReviewComment(pr *github.PullRequest, commentID int64, commentBody string) error {
	prURL := pr.GetHTMLURL()
	log.Infof("Replying to review comment %d for PR URL: %s", commentID, prURL)

	repoOwner, repoName := c.parseRepoURL(prURL)
	if repoOwner == "" || repoName == "" {
		return fmt.Errorf("invalid repository URL: %s", prURL)
	}

	log.Infof("Parsed repository: %s/%s, PR number: %d, comment ID: %d", repoOwner, repoName, pr.GetNumber(), commentID)

	// 使用 Pull Request Review Comments API 来回复评论
	_, _, err := c.client.PullRequests.CreateCommentInReplyTo(context.Background(), repoOwner, repoName, pr.GetNumber(), commentBody, commentID)
	if err != nil {
		return fmt.Errorf("failed to reply to review comment: %w", err)
	}

	log.Infof("Replied to review comment %d on PR #%d", commentID, pr.GetNumber())
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

	// 获取所有代码行评论，按时间排序
	opts := &github.PullRequestListCommentsOptions{
		Sort:      "created",
		Direction: "asc",
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}

	var allComments []*github.PullRequestComment
	for {
		comments, resp, err := c.client.PullRequests.ListComments(context.Background(), repoOwner, repoName, pr.GetNumber(), opts)
		if err != nil {
			return nil, fmt.Errorf("failed to get PR comments: %w", err)
		}

		allComments = append(allComments, comments...)

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	log.Infof("Retrieved %d PR review comments", len(allComments))
	return allComments, nil
}

// GetPullRequestIssueComments 获取 PR 的 Issue 评论（一般性评论，不是代码行评论）
func (c *Client) GetPullRequestIssueComments(pr *github.PullRequest) ([]*github.IssueComment, error) {
	prURL := pr.GetHTMLURL()
	log.Infof("Getting issue comments for PR URL: %s", prURL)

	repoOwner, repoName := c.parseRepoURL(prURL)
	if repoOwner == "" || repoName == "" {
		return nil, fmt.Errorf("invalid repository URL: %s", prURL)
	}

	log.Infof("Parsed repository: %s/%s, PR number: %d", repoOwner, repoName, pr.GetNumber())

	// PR的issue comments使用Issues API，因为PR也是一个Issue
	comments, _, err := c.client.Issues.ListComments(context.Background(), repoOwner, repoName, pr.GetNumber(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get PR issue comments: %w", err)
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

// extractSummaryFromOutput 从AI输出中提取摘要信息
func extractSummaryFromOutput(output string) string {
	lines := strings.Split(output, "\n")

	// 查找改动摘要部分
	var summaryLines []string
	var inSummarySection bool

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)

		// 检测改动摘要章节开始
		if strings.HasPrefix(trimmedLine, models.SectionSummary) {
			inSummarySection = true
			continue
		}

		// 检测其他章节开始，结束摘要部分
		if inSummarySection && strings.HasPrefix(trimmedLine, "## ") {
			break
		}

		// 收集摘要内容
		if inSummarySection && trimmedLine != "" {
			summaryLines = append(summaryLines, line)
		}
	}

	summary := strings.TrimSpace(strings.Join(summaryLines, "\n"))

	// 如果没有找到摘要，返回前几行作为fallback
	if summary == "" && len(lines) > 0 {
		// 取前3行非空内容
		var fallbackLines []string
		for _, line := range lines[:min(3, len(lines))] {
			if strings.TrimSpace(line) != "" {
				fallbackLines = append(fallbackLines, strings.TrimSpace(line))
			}
		}
		summary = strings.Join(fallbackLines, "\n")
	}

	return summary
}

// generateCommitMessage 使用AI生成标准的英文commit message
func (c *Client) generateCommitMessage(workspace *models.Workspace, result *models.ExecutionResult, codeClient code.Code) (string, error) {
	// 获取git status和diff信息
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = workspace.Path
	statusOutput, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get git status: %w", err)
	}

	cmd = exec.Command("git", "diff", "--cached")
	cmd.Dir = workspace.Path
	diffOutput, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get git diff: %w", err)
	}

	// 构建更详细的prompt
	prompt := fmt.Sprintf(`Please help me generate a standard English commit message that follows open source community conventions.

Issue Information:
- Title: %s
- Description: %s

AI Execution Result:
%s

Git Status:
%s

Git Changes:
%s

Please follow this format for the commit message:
1. Use conventional commits format (e.g., feat:, fix:, docs:, style:, refactor:, test:, chore:)
2. Keep the title concise and clear, no more than 50 characters
3. If necessary, add detailed description after an empty line
4. Finally add "Closes #%d" to link the Issue

Important: Please return only the plain text commit message content, do not include any formatting marks (such as markdown syntax, etc.), and do not include any explanatory text.`,
		workspace.Issue.GetTitle(),
		workspace.Issue.GetBody(),
		result.Output,
		string(statusOutput),
		string(diffOutput),
		workspace.Issue.GetNumber(),
	)

	// 调用AI生成commit message
	resp, err := codeClient.Prompt(prompt)
	if err != nil {
		return "", fmt.Errorf("failed to generate commit message: %w", err)
	}

	// 读取AI输出
	output, err := io.ReadAll(resp.Out)
	if err != nil {
		return "", fmt.Errorf("failed to read AI output: %w", err)
	}

	commitMsg := strings.TrimSpace(string(output))

	// 确保commit message不为空
	if commitMsg == "" {
		return "", fmt.Errorf("AI generated empty commit message")
	}

	return commitMsg, nil
}

// min 返回两个整数中的较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
