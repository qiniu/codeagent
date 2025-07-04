package agent

import (
	"fmt"
	"io"
	"strings"

	"github.com/qbox/codeagent/internal/code"
	"github.com/qbox/codeagent/internal/config"
	ghclient "github.com/qbox/codeagent/internal/github"
	"github.com/qbox/codeagent/internal/workspace"
	"github.com/qbox/codeagent/pkg/models"

	"github.com/google/go-github/v58/github"
	"github.com/qiniu/x/log"
)

type Agent struct {
	config         *config.Config
	github         *ghclient.Client
	workspace      *workspace.Manager
	sessionManager *code.SessionManager
}

func New(cfg *config.Config, workspaceManager *workspace.Manager) *Agent {
	// 初始化 GitHub 客户端
	githubClient, err := ghclient.NewClient(cfg)
	if err != nil {
		log.Errorf("Failed to create GitHub client: %v", err)
		return nil
	}

	return &Agent{
		config:         cfg,
		github:         githubClient,
		workspace:      workspaceManager,
		sessionManager: code.NewSessionManager(cfg),
	}
}

// ProcessIssue 处理 Issue 事件，生成代码（保留向后兼容）
func (a *Agent) ProcessIssue(issue *github.Issue) error {
	// 1. 准备临时工作空间
	ws := a.workspace.Prepare(issue)
	if ws.ID == "" {
		return fmt.Errorf("failed to prepare workspace")
	}

	// 2. 创建分支并推送
	if err := a.github.CreateBranch(&ws); err != nil {
		log.Errorf("Failed to create branch: %v", err)
		return err
	}

	// 3. 创建初始 PR
	pr, err := a.github.CreatePullRequest(&ws)
	if err != nil {
		log.Errorf("Failed to create PR: %v", err)
		return err
	}

	// 4. 初始化 code client
	code, err := a.sessionManager.GetSession(&ws)
	if err != nil {
		log.Errorf("failed to get code client: %v", err)
		return err
	}

	// 5. 执行 code prompt，获取修改计划
	prompt := fmt.Sprintf("这是 Issue 内容 %s ，根据 Issue 内容，整理出修改计划", issue.GetURL())
	resp, err := code.Prompt(prompt)
	if err != nil {
		log.Errorf("failed to prompt for plan: %v", err)
		return err
	}

	planOutput, err := io.ReadAll(resp.Out)
	if err != nil {
		log.Errorf("failed to read plan output: %v", err)
		return err
	}

	log.Infof("Modification Plan: %s", string(planOutput))

	// 5.5. 更新 PR Body 为修改计划
	if err := a.github.UpdatePullRequest(pr, string(planOutput)); err != nil {
		log.Errorf("failed to update PR body with plan: %v", err)
		return err
	}

	// 6. 执行 code prompt，修改代码
	codePrompt := fmt.Sprintf("按 issue 内容修改代码: %s", issue.GetURL())
	codeResp, err := code.Prompt(codePrompt)
	if err != nil {
		log.Errorf("failed to prompt for code modification: %v", err)
		return err
	}

	codeOutput, err := io.ReadAll(codeResp.Out)
	if err != nil {
		log.Errorf("failed to read code modification output: %v", err)
		return err
	}

	log.Infof("Code Modification Output: %s", string(codeOutput))

	// 6.5. 评论到 PR
	commentBody := fmt.Sprintf("<details><summary>Code Modification Session</summary>%s</details>", string(codeOutput))
	if err := a.github.CreatePullRequestComment(pr, commentBody); err != nil {
		log.Errorf("failed to create PR comment for code modification: %v", err)
		return err
	}

	// 7 commit 变更
	prompt = "帮我把当前的变更，使用开源社区标准的英文 commit 一下, 但不 push"
	_, err = code.Prompt(prompt)
	if err != nil {
		log.Errorf("failed to prompt for plan: %v", err)
		return err
	}

	// 8. 提交变更并更新 PR
	if err := a.github.Push(&ws); err != nil {
		log.Errorf("Failed to commit and push: %v", err)
		return err
	}

	log.Infof("Successfully processed Issue #%d, PR: %s", issue.GetNumber(), pr.GetHTMLURL())
	return nil
}

// ProcessIssueComment 处理 Issue 评论事件，包含完整的仓库信息
func (a *Agent) ProcessIssueComment(event *github.IssueCommentEvent) error {
	// 1. 准备临时工作空间，传递完整事件
	ws := a.workspace.PrepareFromEvent(event)
	if ws.ID == "" {
		return fmt.Errorf("failed to prepare workspace")
	}

	// 2. 创建分支并推送
	if err := a.github.CreateBranch(&ws); err != nil {
		log.Errorf("Failed to create branch: %v", err)
		return err
	}

	// 3. 创建初始 PR
	pr, err := a.github.CreatePullRequest(&ws)
	if err != nil {
		log.Errorf("Failed to create PR: %v", err)
		return err
	}
	a.workspace.RegisterWorkspace(&ws, pr)

	// 4. 初始化 code client
	code, err := a.sessionManager.GetSession(&ws)
	if err != nil {
		log.Errorf("failed to get code client: %v", err)
		return err
	}

	// 5. 执行 code prompt，获取修改计划
	prompt := fmt.Sprintf("这是 Issue 内容 %s ，根据 Issue 内容，整理出修改计划", event.Issue.GetURL())
	resp, err := code.Prompt(prompt)
	if err != nil {
		log.Errorf("failed to prompt for plan: %v", err)
		return err
	}

	planOutput, err := io.ReadAll(resp.Out)
	if err != nil {
		log.Errorf("failed to read plan output: %v", err)
		return err
	}

	log.Infof("Modification Plan: %s", string(planOutput))

	// 5.5. 更新 PR Body 为修改计划
	if err := a.github.UpdatePullRequest(pr, string(planOutput)); err != nil {
		log.Errorf("failed to update PR body with plan: %v", err)
		return err
	}

	// 6. 执行 code prompt，修改代码
	codePrompt := fmt.Sprintf("按 issue 内容修改代码: %s", event.Issue.GetURL())
	codeResp, err := code.Prompt(codePrompt)
	if err != nil {
		log.Errorf("failed to prompt for code modification: %v", err)
		return err
	}

	codeOutput, err := io.ReadAll(codeResp.Out)
	if err != nil {
		log.Errorf("failed to read code modification output: %v", err)
		return err
	}

	log.Infof("Code Modification Output: %s", string(codeOutput))

	// 6.5. 评论到 PR
	commentBody := fmt.Sprintf("<details><summary>Code Modification Session</summary>%s</details>", string(codeOutput))
	if err := a.github.CreatePullRequestComment(pr, commentBody); err != nil {
		log.Errorf("failed to create PR comment for code modification: %v", err)
		return err
	}

	// 7 commit 变更
	prompt = "帮我把当前的变更，使用开源社区标准的英文 commit 一下, 但不 push"
	_, err = code.Prompt(prompt)
	if err != nil {
		log.Errorf("failed to prompt for plan: %v", err)
		return err
	}

	// 8. 提交变更并更新 PR
	if err := a.github.Push(&ws); err != nil {
		log.Errorf("Failed to commit and push: %v", err)
		return err
	}

	log.Infof("Successfully processed Issue #%d, PR: %s", event.Issue.GetNumber(), pr.GetHTMLURL())
	return nil
}

// ContinuePR 继续处理 PR 中的任务
func (a *Agent) ContinuePR(pr *github.PullRequest) error {
	log.Infof("Continue PR #%d: %s", pr.GetNumber(), pr.GetHTMLURL())

	// 1. 准备临时工作空间
	ws := a.workspace.Getworkspace(pr)
	if ws == nil {
		return fmt.Errorf("failed to prepare workspace for PR fix")
	}

	// 2. 初始化 code client
	code, err := a.sessionManager.GetSession(ws)
	if err != nil {
		log.Errorf("failed to get code client for PR fix: %v", err)
		return err
	}

	// 3. 获取 PR 评论
	comments, err := a.github.GetPullRequestComments(pr)
	if err != nil {
		log.Errorf("failed to get PR comments: %v", err)
		return err
	}

	// 4. 构建 prompt
	// TODO(wyvern): 这里需要替换为 /continue 命令的评论
	// 暂时不区分 /continue 和 /fix 命令
	commentBodies := []string{}
	for _, comment := range comments {
		commentBodies = append(commentBodies, comment.GetBody())
	}
	prompt := fmt.Sprintf("请根据以下评论修改代码：\n\n%s", strings.Join(commentBodies, "\n---\n"))
	resp, err := code.Prompt(prompt)
	if err != nil {
		log.Errorf("failed to prompt for PR continue: %v", err)
		return err
	}

	output, err := io.ReadAll(resp.Out)
	if err != nil {
		log.Errorf("failed to read output for PR continue: %v", err)
		return err
	}

	log.Infof("PR Fix Output: %s", string(output))

	// 5. 提交变更并更新 PR
	result := &models.ExecutionResult{
		Output: string(output),
	}
	if err := a.github.CommitAndPush(ws, result); err != nil {
		log.Errorf("Failed to commit and push for PR continue: %v", err)
		return err
	}

	// 6. 评论到 PR
	commentBody := fmt.Sprintf("<details><summary>PR Fix Session</summary>%s</details>", string(output))
	if err := a.github.CreatePullRequestComment(pr, commentBody); err != nil {
		log.Errorf("failed to create PR comment for continue: %v", err)
		return err
	}

	log.Infof("Successfully continue PR #%d", pr.GetNumber())
	return nil
}

// FixPR 修复 PR 中的问题
func (a *Agent) FixPR(pr *github.PullRequest) error {
	log.Infof("Fixing PR #%d: %s", pr.GetNumber(), pr.GetHTMLURL())

	// 1. 准备临时工作空间
	ws := a.workspace.Getworkspace(pr)
	if ws == nil {
		return fmt.Errorf("failed to prepare workspace for PR fix")
	}

	// 2. 初始化 code client
	code, err := a.sessionManager.GetSession(ws)
	if err != nil {
		log.Errorf("failed to get code client for PR fix: %v", err)
		return err
	}

	// 3. 获取 PR 评论
	comments, err := a.github.GetPullRequestComments(pr)
	if err != nil {
		log.Errorf("failed to get PR comments: %v", err)
		return err
	}

	// 4. 构建 prompt
	// TODO(wyvern): 这里需要替换为 /fix 命令的评论
	commentBodies := []string{}
	for _, comment := range comments {
		commentBodies = append(commentBodies, comment.GetBody())
	}
	prompt := fmt.Sprintf("请根据以下评论修改代码：\n\n%s", strings.Join(commentBodies, "\n---\n"))
	resp, err := code.Prompt(prompt)
	if err != nil {
		log.Errorf("failed to prompt for PR fix: %v", err)
		return err
	}

	output, err := io.ReadAll(resp.Out)
	if err != nil {
		log.Errorf("failed to read output for PR fix: %v", err)
		return err
	}

	log.Infof("PR Fix Output: %s", string(output))

	// 5. 提交变更并更新 PR
	result := &models.ExecutionResult{
		Output: string(output),
	}
	if err := a.github.CommitAndPush(ws, result); err != nil {
		log.Errorf("Failed to commit and push for PR fix: %v", err)
		return err
	}

	// 6. 评论到 PR
	commentBody := fmt.Sprintf("<details><summary>PR Fix Session</summary>%s</details>", string(output))
	if err := a.github.CreatePullRequestComment(pr, commentBody); err != nil {
		log.Errorf("failed to create PR comment for fix: %v", err)
		return err
	}

	log.Infof("Successfully fixed PR #%d", pr.GetNumber())
	return nil
}

// ReviewPR 审查 PR
func (a *Agent) ReviewPR(pr *github.PullRequest) error {
	return nil
}
