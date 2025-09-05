package github

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/qiniu/codeagent/internal/code"
	"github.com/qiniu/codeagent/pkg/models"

	"github.com/google/go-github/v58/github"
	"github.com/qiniu/x/log"
	"github.com/qiniu/x/xlog"
	"github.com/shurcooL/githubv4"
	"golang.org/x/oauth2"
)

type Client struct {
	client *github.Client
}

// GraphQLClient GraphQL客户端封装
type GraphQLClient struct {
	client *githubv4.Client
	token  string
}

// NewGraphQLClient 创建GraphQL客户端
func NewGraphQLClient(token string) *GraphQLClient {
	src := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	httpClient := oauth2.NewClient(context.Background(), src)

	return &GraphQLClient{
		client: githubv4.NewClient(httpClient),
		token:  token,
	}
}

// GetClient 获取底层的GraphQL客户端
func (gc *GraphQLClient) GetClient() *githubv4.Client {
	return gc.client
}

// CreateBranch creates branch locally and pushes to remote
func (c *Client) CreateBranch(workspace *models.Workspace) error {
	log.Infof("Creating branch for workspace: %s, path: %s", workspace.Branch, workspace.Path)

	// Check Git configuration
	c.checkGitConfig(workspace.Path)

	// Create an empty "Initial plan" commit, mimicking Copilot's behavior
	// This allows immediate establishment of branch and PR, providing better user experience
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

	// 获取仓库的默认分支
	defaultBranch, err := c.getDefaultBranch(repoOwner, repoName)
	if err != nil {
		log.Errorf("Failed to get default branch for %s/%s, using 'main' as fallback: %v", repoOwner, repoName, err)
		defaultBranch = "main"
	}
	log.Infof("Using default branch '%s' for repository %s/%s", defaultBranch, repoOwner, repoName)

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
*此 PR 由 Code Agent(https://github.com/qiniu/codeagent) 自动创建，将逐步完善实现*`,
		workspace.Issue.GetNumber(),
		workspace.Issue.GetBody())

	newPR := &github.NewPullRequest{
		Title: &prTitle,
		Body:  &prBody,
		Head:  &workspace.Branch,
		Base:  &defaultBranch,
	}

	pr, _, err := c.client.PullRequests.Create(context.Background(), repoOwner, repoName, newPR)
	if err != nil {
		return nil, fmt.Errorf("failed to create PR: %w", err)
	}

	log.Infof("Created PR: %s", pr.GetHTMLURL())
	return pr, nil
}

// getDefaultBranch 获取仓库的默认分支
func (c *Client) getDefaultBranch(owner, repo string) (string, error) {
	repository, _, err := c.client.Repositories.Get(context.Background(), owner, repo)
	if err != nil {
		return "", fmt.Errorf("failed to get repository info: %w", err)
	}

	defaultBranch := repository.GetDefaultBranch()
	if defaultBranch == "" {
		return "", fmt.Errorf("repository has no default branch")
	}

	return defaultBranch, nil
}

// CommitAndPush 检测文件变更并提交推送
func (c *Client) CommitAndPush(workspace *models.Workspace, result *models.ExecutionResult, codeClient code.Code) (string, error) {
	// 检查是否有文件变更
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = workspace.Path
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to check git status: %w", err)
	}

	statusOutput := strings.TrimSpace(string(output))
	log.Infof("Git status output: '%s'", statusOutput)

	if statusOutput == "" {
		log.Infof("No changes detected in workspace %s", workspace.Path)
		return "", nil
	}

	log.Infof("Changes detected in workspace %s: %s", workspace.Path, statusOutput)

	// 添加所有变更
	cmd = exec.Command("git", "add", ".")
	cmd.Dir = workspace.Path
	addOutput, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to add changes: %w\nCommand output: %s", err, string(addOutput))
	}

	// 再次检查是否有变更可以提交
	cmd = exec.Command("git", "diff", "--cached", "--quiet")
	cmd.Dir = workspace.Path
	err = cmd.Run()
	if err != nil {
		// 有变更可以提交
		log.Infof("Changes staged and ready for commit")
	} else {
		// 没有变更可以提交
		log.Infof("No changes staged for commit, skipping commit")
		return "", nil
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
		return "", fmt.Errorf("failed to commit changes: %w\nCommand output: %s", err, string(commitOutput))
	}

	// 获取commit hash
	cmd = exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = workspace.Path
	commitHashBytes, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get commit hash: %w", err)
	}
	commitHash := strings.TrimSpace(string(commitHashBytes))

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
				return "", fmt.Errorf("failed to push changes: %w\nCommand output: %s", err2, string(pushOutput2))
			}

			log.Infof("Successfully pushed changes after pulling remote updates")
			return commitHash, nil
		}

		return "", fmt.Errorf("failed to push changes: %w\nCommand output: %s", err, string(pushOutput))
	}

	log.Infof("Committed and pushed changes for Issue #%d", workspace.Issue.GetNumber())
	return commitHash, nil
}

// PullLatestChanges 拉取远端最新代码（优先使用rebase策略）
func (c *Client) PullLatestChanges(ctx context.Context, workspace *models.Workspace, pr *github.PullRequest) error {
	xl := xlog.NewWith(ctx)
	xl.Infof("Pulling latest changes for workspace: %s (PR #%d)", workspace.Path, pr.GetNumber())

	// 获取 PR 的目标分支（base branch）
	baseBranch := pr.GetBase().GetRef()
	if baseBranch == "" {
		xl.Errorf("PR base branch is empty for PR #%d", pr.GetNumber())
		return fmt.Errorf("PR base branch is empty for PR #%d", pr.GetNumber())
	}

	// 获取 PR 的源分支（head branch）
	headBranch := pr.GetHead().GetRef()
	if headBranch == "" {
		xl.Errorf("PR head branch is empty for PR #%d", pr.GetNumber())
		return fmt.Errorf("PR head branch is empty for PR #%d", pr.GetNumber())
	}

	xl.Infof("PR #%d: %s -> %s", pr.GetNumber(), headBranch, baseBranch)

	// 1. 获取所有远程引用
	cmd := exec.Command("git", "fetch", "--all", "--prune")
	cmd.Dir = workspace.Path
	xl.Infof("Executing git command: %s (workDir: %s)", cmd.String(), workspace.Path)
	fetchOutput, err := cmd.CombinedOutput()
	if err != nil {
		xl.Errorf("failed to fetch latest changes: %v\nCommand output: %s, workDir: %s, cmd: %s", err, string(fetchOutput), workspace.Path, cmd.String())
		return fmt.Errorf("failed to fetch latest changes: %w\nCommand output: %s", err, string(fetchOutput))
	}
	xl.Infof("Fetched all remote references for PR #%d", pr.GetNumber())

	// 2. 检查当前是否有未提交的变更
	cmd = exec.Command("git", "status", "--porcelain")
	cmd.Dir = workspace.Path
	xl.Infof("Executing git command: %s (workDir: %s)", cmd.String(), workspace.Path)
	statusOutput, err := cmd.Output()
	if err != nil {
		xl.Errorf("failed to check git status: %v\nCommand output: %s, workDir: %s, cmd: %s", err, string(statusOutput), workspace.Path, cmd.String())
		return fmt.Errorf("failed to check git status: %w", err)
	}

	hasChanges := strings.TrimSpace(string(statusOutput)) != ""
	if hasChanges {
		// 有未提交的变更，先 stash
		xl.Infof("Found uncommitted changes in worktree, stashing them")
		cmd = exec.Command("git", "stash", "push", "-m", fmt.Sprintf("Auto stash before syncing PR #%d", pr.GetNumber()))
		cmd.Dir = workspace.Path
		xl.Infof("Executing git command: %s (workDir: %s)", cmd.String(), workspace.Path)
		stashOutput, err := cmd.CombinedOutput()
		if err != nil {
			xl.Warnf("Failed to stash changes: %v, output: %s", err, string(stashOutput))
		}
	}

	// 3. 尝试直接获取 PR 内容
	prNumber := pr.GetNumber()
	xl.Infof("Attempting to fetch PR #%d content directly", prNumber)
	cmd = exec.Command("git", "fetch", "origin", fmt.Sprintf("pull/%d/head", prNumber))
	cmd.Dir = workspace.Path
	xl.Infof("Executing git command: %s (workDir: %s)", cmd.String(), workspace.Path)
	fetchOutput, err = cmd.CombinedOutput()
	if err == nil {
		// 直接获取成功，使用rebase合并更新
		xl.Infof("Successfully fetched PR #%d content, attempting rebase", prNumber)
		cmd = exec.Command("git", "rebase", "FETCH_HEAD")
		cmd.Dir = workspace.Path
		xl.Infof("Executing git command: %s (workDir: %s)", cmd.String(), workspace.Path)
		rebaseOutput, err := cmd.CombinedOutput()
		if err != nil {
			xl.Errorf("Rebase failed, trying reset: %v, output: %s", err, string(rebaseOutput))
			// rebase失败，强制切换到PR内容
			cmd = exec.Command("git", "reset", "--hard", "FETCH_HEAD")
			cmd.Dir = workspace.Path
			xl.Infof("Executing git command: %s (workDir: %s)", cmd.String(), workspace.Path)
			resetOutput, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("failed to reset to PR #%d: %w\nCommand output: %s", prNumber, err, string(resetOutput))
			}
			xl.Infof("Hard reset worktree to PR #%d content", prNumber)
		} else {
			xl.Infof("Successfully rebased worktree to PR #%d content", prNumber)
		}
	} else {
		// 直接获取失败，使用传统rebase方式
		xl.Errorf("Failed to fetch PR #%d directly: %v, falling back to traditional rebase, output: %s", prNumber, err, string(fetchOutput))

		// 尝试rebase到目标分支的最新代码
		cmd = exec.Command("git", "rebase", fmt.Sprintf("origin/%s", baseBranch))
		cmd.Dir = workspace.Path
		xl.Infof("Executing git command: %s (workDir: %s)", cmd.String(), workspace.Path)
		rebaseOutput, err := cmd.CombinedOutput()
		if err != nil {
			log.Errorf("Rebase to base branch failed: %v, output: %s", err, string(rebaseOutput))
			// rebase失败，尝试强制同步到基础分支
			cmd = exec.Command("git", "reset", "--hard", fmt.Sprintf("origin/%s", baseBranch))
			cmd.Dir = workspace.Path
			xl.Infof("Executing git command: %s (workDir: %s)", cmd.String(), workspace.Path)
			resetOutput, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("failed to reset to base branch %s: %w\nCommand output: %s", baseBranch, err, string(resetOutput))
			}
			xl.Infof("Hard reset worktree to base branch %s", baseBranch)
		} else {
			xl.Infof("Successfully rebased worktree to base branch %s", baseBranch)
		}
	}

	// 4. 如果之前有stash，尝试恢复
	if hasChanges {
		xl.Infof("Attempting to restore stashed changes for PR #%d", prNumber)
		cmd = exec.Command("git", "stash", "pop")
		cmd.Dir = workspace.Path
		xl.Infof("Executing git command: %s (workDir: %s)", cmd.String(), workspace.Path)
		stashPopOutput, err := cmd.CombinedOutput()
		if err != nil {
			xl.Warnf("Failed to restore stashed changes: %v, output: %s", err, string(stashPopOutput))
			log.Infof("You may need to manually resolve the stashed changes later")
		} else {
			xl.Infof("Successfully restored stashed changes")
		}
	}

	xl.Infof("Successfully pulled latest changes for PR #%d using rebase strategy", pr.GetNumber())
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

// ReplyToReviewComment 回复 PR 代码行评论，返回新创建的回复评论ID
func (c *Client) ReplyToReviewComment(pr *github.PullRequest, commentID int64, commentBody string) (int64, error) {
	prURL := pr.GetHTMLURL()
	log.Infof("Replying to review comment %d for PR URL: %s", commentID, prURL)

	repoOwner, repoName := c.parseRepoURL(prURL)
	if repoOwner == "" || repoName == "" {
		return 0, fmt.Errorf("invalid repository URL: %s", prURL)
	}

	log.Infof("Parsed repository: %s/%s, PR number: %d, comment ID: %d", repoOwner, repoName, pr.GetNumber(), commentID)

	// 使用 Pull Request Review Comments API 来回复评论
	reply, _, err := c.client.PullRequests.CreateCommentInReplyTo(context.Background(), repoOwner, repoName, pr.GetNumber(), commentBody, commentID)
	if err != nil {
		return 0, fmt.Errorf("failed to reply to review comment: %w", err)
	}

	log.Infof("Replied to review comment %d on PR #%d, new reply ID: %d", commentID, pr.GetNumber(), reply.GetID())
	return reply.GetID(), nil
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

// GetReviewComments 获取指定 review 的所有 comments
func (c *Client) GetReviewComments(pr *github.PullRequest, reviewID int64) ([]*github.PullRequestComment, error) {
	prURL := pr.GetHTMLURL()
	repoOwner, repoName := c.parseRepoURL(prURL)
	if repoOwner == "" || repoName == "" {
		return nil, fmt.Errorf("invalid repository URL: %s", prURL)
	}

	comments, _, err := c.client.PullRequests.ListComments(context.Background(), repoOwner, repoName, pr.GetNumber(), &github.PullRequestListCommentsOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get review comments: %w", err)
	}

	// 过滤出属于指定 review 的评论
	var reviewComments []*github.PullRequestComment
	for _, comment := range comments {
		if comment.GetPullRequestReviewID() == reviewID {
			reviewComments = append(reviewComments, comment)
		}
	}

	return reviewComments, nil
}

// GetAllPRComments 获取 PR 的所有评论，包括一般评论和代码行评论
func (c *Client) GetAllPRComments(pr *github.PullRequest) (*models.PRAllComments, error) {
	prURL := pr.GetHTMLURL()
	repoOwner, repoName := c.parseRepoURL(prURL)
	if repoOwner == "" || repoName == "" {
		return nil, fmt.Errorf("invalid repository URL: %s", prURL)
	}

	prNumber := pr.GetNumber()
	log.Infof("Fetching all comments for PR #%d", prNumber)

	// 获取一般 PR 评论 (Issue Comments)
	issueComments, _, err := c.client.Issues.ListComments(context.Background(), repoOwner, repoName, prNumber, &github.IssueListCommentsOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get PR issue comments: %w", err)
	}

	// 获取代码行评论 (Review Comments)
	reviewComments, _, err := c.client.PullRequests.ListComments(context.Background(), repoOwner, repoName, prNumber, &github.PullRequestListCommentsOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get PR review comments: %w", err)
	}

	// 获取 PR 的所有 reviews
	reviews, _, err := c.client.PullRequests.ListReviews(context.Background(), repoOwner, repoName, prNumber, &github.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get PR reviews: %w", err)
	}

	log.Infof("Found %d issue comments, %d review comments, %d reviews for PR #%d",
		len(issueComments), len(reviewComments), len(reviews), prNumber)

	return &models.PRAllComments{
		PRBody:         pr.GetBody(),
		IssueComments:  issueComments,
		ReviewComments: reviewComments,
		Reviews:        reviews,
	}, nil
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

// DeleteCodeAgentBranch 删除CodeAgent创建的分支
func (c *Client) DeleteCodeAgentBranch(ctx context.Context, owner, repo, branchName string) error {
	log.Infof("Attempting to delete CodeAgent branch: %s", branchName)

	// 确保只删除codeagent开头的分支
	if !strings.HasPrefix(branchName, "codeagent") {
		log.Warnf("Branch %s is not a CodeAgent branch, skipping deletion", branchName)
		return nil
	}

	// 使用GitHub API删除分支
	_, err := c.client.Git.DeleteRef(ctx, owner, repo, fmt.Sprintf("heads/%s", branchName))
	if err != nil {
		// 如果分支不存在，这不是错误
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "Reference does not exist") {
			log.Infof("Branch %s does not exist, no deletion needed", branchName)
			return nil
		}
		return fmt.Errorf("failed to delete branch %s: %w", branchName, err)
	}

	log.Infof("Successfully deleted CodeAgent branch: %s", branchName)
	return nil
}

// CreateComment 在Issue或PR上创建评论
func (c *Client) CreateComment(ctx context.Context, owner, repo string, issueNumber int, body string) (*github.IssueComment, error) {
	comment := &github.IssueComment{
		Body: github.String(body),
	}

	createdComment, _, err := c.client.Issues.CreateComment(ctx, owner, repo, issueNumber, comment)
	if err != nil {
		return nil, fmt.Errorf("failed to create comment: %w", err)
	}

	return createdComment, nil
}

// UpdateComment 更新已存在的评论
func (c *Client) UpdateComment(ctx context.Context, owner, repo string, commentID int64, body string) error {
	comment := &github.IssueComment{
		Body: github.String(body),
	}

	_, _, err := c.client.Issues.EditComment(ctx, owner, repo, commentID, comment)
	if err != nil {
		return fmt.Errorf("failed to update comment: %w", err)
	}
	return nil
}

func (c *Client) UpdatePRComment(ctx context.Context, owner, repo string, commentID int64, body string) error {
	prComment := &github.PullRequestComment{
		Body: github.String(body),
	}
	_, _, err := c.client.PullRequests.EditComment(ctx, owner, repo, commentID, prComment)
	if err != nil {
		return fmt.Errorf("failed to update comment: %w", err)
	}
	return nil
}

// GetComment 获取评论内容
func (c *Client) GetComment(ctx context.Context, owner, repo string, commentID int64) (*github.IssueComment, error) {
	comment, _, err := c.client.Issues.GetComment(ctx, owner, repo, commentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get comment: %w", err)
	}

	return comment, nil
}

// GetClient 获取底层的GitHub客户端（用于MCP服务器）
func (c *Client) GetClient() *github.Client {
	return c.client
}

// GraphQL context data structures used by collector
type GraphQLPRContextResponse struct {
	Number       int
	Title        string
	Body         string
	State        string
	Additions    int
	Deletions    int
	Commits      int
	Author       string
	AuthorAvatar string
	BaseRef      string
	HeadRef      string
	Files        []GraphQLFileResponse
	Comments     []GraphQLCommentResponse
	Reviews      []GraphQLReviewResponse
	RateLimit    GraphQLRateLimitResponse
}

type GraphQLIssueContextResponse struct {
	Number       int
	Title        string
	Body         string
	State        string
	Author       string
	AuthorAvatar string
	Labels       []GraphQLLabelResponse
	Comments     []GraphQLCommentResponse
	RateLimit    GraphQLRateLimitResponse
}

type GraphQLFileResponse struct {
	Path       string
	Additions  int
	Deletions  int
	ChangeType string
}

type GraphQLCommentResponse struct {
	ID        string
	Body      string
	Author    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type GraphQLReviewResponse struct {
	ID        string
	Body      string
	State     string
	Author    string
	CreatedAt time.Time
	Comments  []GraphQLReviewCommentResponse
}

type GraphQLReviewCommentResponse struct {
	ID        string
	Body      string
	Path      string
	Line      int
	DiffHunk  string
	Author    string
	CreatedAt time.Time
}

type GraphQLLabelResponse struct {
	Name  string
	Color string
}

type GraphQLRateLimitResponse struct {
	Limit     int
	Cost      int
	Remaining int
	ResetAt   time.Time
}

// GetPullRequestContext 使用GraphQL获取PR完整上下文
func (gc *GraphQLClient) GetPullRequestContext(ctx context.Context, owner, repo string, prNumber int) (*GraphQLPRContextResponse, error) {
	var query struct {
		Repository struct {
			DefaultBranchRef struct {
				Name githubv4.String
			}
			PullRequest struct {
				Number    githubv4.Int
				Title     githubv4.String
				Body      githubv4.String
				State     githubv4.PullRequestState
				Additions githubv4.Int
				Deletions githubv4.Int
				Commits   struct {
					TotalCount githubv4.Int
				}
				Author struct {
					Login     githubv4.String
					AvatarURL githubv4.String `graphql:"avatarUrl"`
				}
				BaseRefName githubv4.String
				HeadRefName githubv4.String

				// PR 文件变更
				Files struct {
					Nodes []struct {
						Path       githubv4.String
						Additions  githubv4.Int
						Deletions  githubv4.Int
						ChangeType githubv4.String
					}
				} `graphql:"files(first: 100)"`

				// Issue 评论 (PR也是一种Issue)
				Comments struct {
					Nodes []struct {
						ID        githubv4.String
						Body      githubv4.String
						CreatedAt githubv4.DateTime
						UpdatedAt githubv4.DateTime
						Author    struct {
							Login githubv4.String
						}
					}
				} `graphql:"comments(first: 50, orderBy: {field: UPDATED_AT, direction: ASC})"`

				// 代码评审
				Reviews struct {
					Nodes []struct {
						ID        githubv4.String
						Body      githubv4.String
						State     githubv4.PullRequestReviewState
						CreatedAt githubv4.DateTime
						Author    struct {
							Login githubv4.String
						}
						// 评审中的行级评论
						Comments struct {
							Nodes []struct {
								ID       githubv4.String
								Body     githubv4.String
								Path     githubv4.String
								Line     githubv4.Int
								DiffHunk githubv4.String
								Author   struct {
									Login githubv4.String
								}
								CreatedAt githubv4.DateTime
							}
						} `graphql:"comments(first: 50)"`
					}
				} `graphql:"reviews(first: 20, states: [APPROVED, CHANGES_REQUESTED, COMMENTED])"`
			} `graphql:"pullRequest(number: $number)"`
		} `graphql:"repository(owner: $owner, name: $name)"`

		// 速率限制监控
		RateLimit struct {
			Limit     githubv4.Int
			Cost      githubv4.Int
			Remaining githubv4.Int
			ResetAt   githubv4.DateTime
		}
	}

	variables := map[string]interface{}{
		"owner":  githubv4.String(owner),
		"name":   githubv4.String(repo),
		"number": githubv4.Int(prNumber),
	}

	if err := gc.client.Query(ctx, &query, variables); err != nil {
		return nil, fmt.Errorf("failed to execute GraphQL PR query: %w", err)
	}

	// 转换响应数据
	response := &GraphQLPRContextResponse{
		Number:       int(query.Repository.PullRequest.Number),
		Title:        string(query.Repository.PullRequest.Title),
		Body:         string(query.Repository.PullRequest.Body),
		State:        string(query.Repository.PullRequest.State),
		Additions:    int(query.Repository.PullRequest.Additions),
		Deletions:    int(query.Repository.PullRequest.Deletions),
		Commits:      int(query.Repository.PullRequest.Commits.TotalCount),
		Author:       string(query.Repository.PullRequest.Author.Login),
		AuthorAvatar: string(query.Repository.PullRequest.Author.AvatarURL),
		BaseRef:      string(query.Repository.PullRequest.BaseRefName),
		HeadRef:      string(query.Repository.PullRequest.HeadRefName),
		RateLimit: GraphQLRateLimitResponse{
			Limit:     int(query.RateLimit.Limit),
			Cost:      int(query.RateLimit.Cost),
			Remaining: int(query.RateLimit.Remaining),
			ResetAt:   query.RateLimit.ResetAt.Time,
		},
	}

	// 转换文件变更
	for _, file := range query.Repository.PullRequest.Files.Nodes {
		response.Files = append(response.Files, GraphQLFileResponse{
			Path:       string(file.Path),
			Additions:  int(file.Additions),
			Deletions:  int(file.Deletions),
			ChangeType: string(file.ChangeType),
		})
	}

	// 转换评论
	for _, comment := range query.Repository.PullRequest.Comments.Nodes {
		response.Comments = append(response.Comments, GraphQLCommentResponse{
			ID:        string(comment.ID),
			Body:      string(comment.Body),
			Author:    string(comment.Author.Login),
			CreatedAt: comment.CreatedAt.Time,
			UpdatedAt: comment.UpdatedAt.Time,
		})
	}

	// 转换评审和评审评论
	for _, review := range query.Repository.PullRequest.Reviews.Nodes {
		reviewResponse := GraphQLReviewResponse{
			ID:        string(review.ID),
			Body:      string(review.Body),
			State:     string(review.State),
			Author:    string(review.Author.Login),
			CreatedAt: review.CreatedAt.Time,
		}

		// 转换评审中的行级评论
		for _, comment := range review.Comments.Nodes {
			reviewComment := GraphQLReviewCommentResponse{
				ID:        string(comment.ID),
				Body:      string(comment.Body),
				Path:      string(comment.Path),
				Line:      int(comment.Line),
				DiffHunk:  string(comment.DiffHunk),
				Author:    string(comment.Author.Login),
				CreatedAt: comment.CreatedAt.Time,
			}
			reviewResponse.Comments = append(reviewResponse.Comments, reviewComment)
		}

		response.Reviews = append(response.Reviews, reviewResponse)
	}

	return response, nil
}

// GetIssueContext 使用GraphQL获取Issue完整上下文
func (gc *GraphQLClient) GetIssueContext(ctx context.Context, owner, repo string, issueNumber int) (*GraphQLIssueContextResponse, error) {
	var query struct {
		Repository struct {
			Issue struct {
				Number githubv4.Int
				Title  githubv4.String
				Body   githubv4.String
				State  githubv4.IssueState
				Author struct {
					Login     githubv4.String
					AvatarURL githubv4.String `graphql:"avatarUrl"`
				}
				Labels struct {
					Nodes []struct {
						Name  githubv4.String
						Color githubv4.String
					}
				} `graphql:"labels(first: 10)"`
				Comments struct {
					Nodes []struct {
						ID        githubv4.String
						Body      githubv4.String
						CreatedAt githubv4.DateTime
						Author    struct {
							Login githubv4.String
						}
					}
				} `graphql:"comments(first: 50, orderBy: {field: UPDATED_AT, direction: ASC})"`
			} `graphql:"issue(number: $number)"`
		} `graphql:"repository(owner: $owner, name: $name)"`

		// 速率限制监控
		RateLimit struct {
			Limit     githubv4.Int
			Cost      githubv4.Int
			Remaining githubv4.Int
			ResetAt   githubv4.DateTime
		}
	}

	variables := map[string]interface{}{
		"owner":  githubv4.String(owner),
		"name":   githubv4.String(repo),
		"number": githubv4.Int(issueNumber),
	}

	if err := gc.client.Query(ctx, &query, variables); err != nil {
		return nil, fmt.Errorf("failed to execute GraphQL Issue query: %w", err)
	}

	// 转换响应数据
	response := &GraphQLIssueContextResponse{
		Number:       int(query.Repository.Issue.Number),
		Title:        string(query.Repository.Issue.Title),
		Body:         string(query.Repository.Issue.Body),
		State:        string(query.Repository.Issue.State),
		Author:       string(query.Repository.Issue.Author.Login),
		AuthorAvatar: string(query.Repository.Issue.Author.AvatarURL),
		RateLimit: GraphQLRateLimitResponse{
			Limit:     int(query.RateLimit.Limit),
			Cost:      int(query.RateLimit.Cost),
			Remaining: int(query.RateLimit.Remaining),
			ResetAt:   query.RateLimit.ResetAt.Time,
		},
	}

	// 转换标签
	for _, label := range query.Repository.Issue.Labels.Nodes {
		response.Labels = append(response.Labels, GraphQLLabelResponse{
			Name:  string(label.Name),
			Color: string(label.Color),
		})
	}

	// 转换评论
	for _, comment := range query.Repository.Issue.Comments.Nodes {
		response.Comments = append(response.Comments, GraphQLCommentResponse{
			ID:        string(comment.ID),
			Body:      string(comment.Body),
			Author:    string(comment.Author.Login),
			CreatedAt: comment.CreatedAt.Time,
		})
	}

	return response, nil
}

// min 返回两个整数中的较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
