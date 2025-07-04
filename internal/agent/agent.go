package agent

import (
	"fmt"
	"io"

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

	// 2. 初始化 code client
	code, err := a.sessionManager.GetSession(&ws)
	if err != nil {
		log.Errorf("failed to get code client: %v", err)
		return err
	}
	defer a.sessionManager.CloseSession(ws.ID)

	// 确保处理完成后清理工作空间
	defer func() {
		a.workspace.Cleanup(ws)
		log.Infof("Cleaned up workspace: %s", ws.ID)
	}()

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

	// 4. 执行 code prompt
	prompt := fmt.Sprintf("这是 Issue 内容 %s ，根据 Issue 内容，整理出修改计划", issue.GetURL())
	resp, err := code.Prompt(prompt)
	if err != nil {
		log.Errorf("failed to prompt: %v", err)
		return err
	}

	output, err := io.ReadAll(resp.Out)
	if err != nil {
		log.Errorf("failed to read output: %v", err)
		return err
	}

	log.Infof("output: %s", string(output))

	result := &models.ExecutionResult{
		Output: string(output),
	}

	// 5. 提交变更并更新 PR
	if err := a.github.CommitAndPush(&ws, result); err != nil {
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

	// 4. 初始化 code client
	code, err := a.sessionManager.GetSession(&ws)
	if err != nil {
		log.Errorf("failed to get code client: %v", err)
		return err
	}

	// 5. 执行 code prompt
	prompt := fmt.Sprintf("这是 Issue 内容 %s ，根据 Issue 内容，整理出修改计划", event.Issue.GetURL())
	resp, err := code.Prompt(prompt)
	if err != nil {
		log.Errorf("failed to prompt: %v", err)
		return err
	}

	output, err := io.ReadAll(resp.Out)
	if err != nil {
		log.Errorf("failed to read output: %v", err)
		return err
	}

	log.Infof("output: %s", string(output))

	result := &models.ExecutionResult{
		Output: string(output),
	}

	// 5. 提交变更并更新 PR
	if err := a.github.CommitAndPush(&ws, result); err != nil {
		log.Errorf("Failed to commit and push: %v", err)
		return err
	}

	log.Infof("Successfully processed Issue #%d, PR: %s", event.Issue.GetNumber(), pr.GetHTMLURL())
	return nil
}

// ContinuePR 继续处理 PR 中的任务
func (a *Agent) ContinuePR(pr *github.PullRequest) error {
	// TODO: 实现继续处理 PR 的逻辑
	return nil
}

// FixPR 修复 PR 中的问题
func (a *Agent) FixPR(pr *github.PullRequest) error {
	// TODO: 实现修复 PR 的逻辑
	return nil
}

// ReviewPR 审查 PR
func (a *Agent) ReviewPR(pr *github.PullRequest) error {
	log.Infof("Reviewing PR #%d: %s", pr.GetNumber(), pr.GetHTMLURL())

	// 1. 准备临时工作空间
	ws := a.workspace.PrepareFromPR(pr)
	if ws.ID == "" {
		return fmt.Errorf("failed to prepare workspace for PR review")
	}

	// 2. 初始化 code client
	code, err := a.sessionManager.GetSession(&ws)
	if err != nil {
		log.Errorf("failed to get code client for PR review: %v", err)
		return err
	}

	// 3. 获取 PR 变更
	changes, err := a.github.GetPullRequestChanges(pr)
	if err != nil {
		log.Errorf("failed to get PR changes: %v", err)
		return err
	}

	// 4. 构建 prompt
	prompt := fmt.Sprintf("请审查以下 PR 变更，并提供改进建议：\n\n%s", changes)
	resp, err := code.Prompt(prompt)
	if err != nil {
		log.Errorf("failed to prompt for PR review: %v", err)
		return err
	}

	output, err := io.ReadAll(resp.Out)
	if err != nil {
		log.Errorf("failed to read output for PR review: %v", err)
		return err
	}

	log.Infof("PR Review Output: %s", string(output))

	// 5. 评论到 PR
	if err := a.github.CreatePullRequestComment(pr, string(output)); err != nil {
		log.Errorf("failed to create PR comment: %v", err)
		return err
	}

	log.Infof("Successfully reviewed PR #%d", pr.GetNumber())
	return nil
}
