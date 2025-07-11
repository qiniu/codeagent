package agent

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

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

// ProcessIssueComment 处理 Issue 评论事件，包含完整的仓库信息
func (a *Agent) ProcessIssueComment(event *github.IssueCommentEvent) error {
	// 1. 创建 Issue 工作空间
	ws := a.workspace.CreateWorkspaceFromIssue(event.Issue)
	if ws == nil {
		return fmt.Errorf("failed to create workspace from issue")
	}

	// 2. 创建分支并推送
	if err := a.github.CreateBranch(ws); err != nil {
		log.Errorf("Failed to create branch: %v", err)
		return err
	}

	// 3. 创建初始 PR
	pr, err := a.github.CreatePullRequest(ws)
	if err != nil {
		log.Errorf("Failed to create PR: %v", err)
		return err
	}

	// 4. 移动工作空间从 Issue 到 PR
	if err := a.workspace.MoveIssueToPR(ws, pr.GetNumber()); err != nil {
		log.Errorf("Failed to move workspace: %v", err)
	}
	ws.PRNumber = pr.GetNumber()

	// 5. 创建 session 目录
	suffix := strings.TrimPrefix(filepath.Base(ws.Path), fmt.Sprintf("%s-pr-%d-", ws.Repo, pr.GetNumber()))
	sessionPath, err := a.workspace.CreateSessionPath(filepath.Dir(ws.Path), ws.Repo, pr.GetNumber(), suffix)
	if err != nil {
		log.Errorf("Failed to create session directory: %v", err)
		return err
	}
	ws.SessionPath = sessionPath

	// 6. 注册工作空间到 PR 映射
	ws.PullRequest = pr
	a.workspace.RegisterWorkspace(ws, pr)

	log.Infof("process issue #%d, workspace: %s, session: %s", event.Issue.GetNumber(), ws.Path, ws.SessionPath)

	// 7. 初始化 code client
	code, err := a.sessionManager.GetSession(ws)
	if err != nil {
		log.Errorf("failed to get code client: %v", err)
		return err
	}

	// 8. 执行代码修改，规范 prompt，要求 AI 输出结构化摘要
	codePrompt := fmt.Sprintf(`请根据以下 Issue 内容修改代码：

标题：%s
描述：%s

请直接修改代码，并按照以下格式输出你的分析和操作：

%s
请总结本次代码改动的主要内容。

%s
请以简洁的列表形式列出具体改动：
- 变动的文件（每个文件后面列出具体变动，如：xxx/xx.go 添加删除逻辑）

请确保输出格式清晰，便于阅读和理解。`, event.Issue.GetTitle(), event.Issue.GetBody(), models.SectionSummary, models.SectionChanges)

	codeResp, err := a.promptWithRetry(code, codePrompt, 3)
	if err != nil {
		log.Errorf("failed to prompt for code modification: %v", err)
		return err
	}

	codeOutput, err := io.ReadAll(codeResp.Out)
	if err != nil {
		log.Errorf("failed to read code modification output: %v", err)
		return err
	}

	log.Infof("LLM Output: %s", string(codeOutput))

	// 9. 组织结构化 PR Body（解析三段式输出）
	aiStr := string(codeOutput)

	// 解析三段式输出
	summary, changes, testPlan := parseStructuredOutput(aiStr)

	// 构建PR Body
	prBody := ""
	if summary != "" {
		prBody += models.SectionSummary + "\n\n" + summary + "\n\n"
	}

	if changes != "" {
		prBody += models.SectionChanges + "\n\n" + changes + "\n\n"
	}

	if testPlan != "" {
		prBody += models.SectionTestPlan + "\n\n" + testPlan + "\n\n"
	}

	// 添加原始输出和错误信息
	prBody += "---\n\n"
	prBody += "<details><summary>AI 完整输出</summary>\n\n" + aiStr + "\n\n</details>\n\n"

	// 错误信息判断
	errorInfo := extractErrorInfo(aiStr)
	if errorInfo != "" {
		prBody += "## 错误信息\n\n```text\n" + errorInfo + "\n```\n\n"
	}

	prBody += "<details><summary>原始 Prompt</summary>\n\n" + codePrompt + "\n\n</details>"

	if err = a.github.UpdatePullRequest(pr, prBody); err != nil {
		log.Errorf("failed to update PR body with execution result: %v", err)
		return err
	}

	// 10. 提交变更并推送到远程
	result := &models.ExecutionResult{
		Output: string(codeOutput),
	}
	if err = a.github.CommitAndPush(ws, result, code); err != nil {
		log.Errorf("Failed to commit and push: %v", err)
		return err
	}

	log.Infof("Successfully processed Issue #%d, PR: %s", event.Issue.GetNumber(), pr.GetHTMLURL())
	return nil
}

// parseStructuredOutput 解析AI的三段式输出
func parseStructuredOutput(output string) (summary, changes, testPlan string) {
	lines := strings.Split(output, "\n")

	var currentSection string
	var summaryLines, changesLines, testPlanLines []string

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)

		// 检测章节标题
		if strings.HasPrefix(trimmedLine, models.SectionSummary) {
			currentSection = models.SectionSummaryID
			continue
		} else if strings.HasPrefix(trimmedLine, models.SectionChanges) {
			currentSection = models.SectionChangesID
			continue
		} else if strings.HasPrefix(trimmedLine, models.SectionTestPlan) {
			currentSection = models.SectionTestPlanID
			continue
		}

		// 根据当前章节收集内容
		switch currentSection {
		case models.SectionSummaryID:
			if trimmedLine != "" {
				summaryLines = append(summaryLines, line)
			}
		case models.SectionChangesID:
			changesLines = append(changesLines, line)
		case models.SectionTestPlanID:
			testPlanLines = append(testPlanLines, line)
		}
	}

	summary = strings.TrimSpace(strings.Join(summaryLines, "\n"))
	changes = strings.TrimSpace(strings.Join(changesLines, "\n"))
	testPlan = strings.TrimSpace(strings.Join(testPlanLines, "\n"))

	return summary, changes, testPlan
}

// extractErrorInfo 提取错误信息
func extractErrorInfo(output string) string {
	lines := strings.Split(output, "\n")

	// 查找错误信息
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.ToLower(strings.TrimSpace(lines[i]))
		if strings.HasPrefix(line, models.ErrorPrefixError) ||
			strings.HasPrefix(line, models.ErrorPrefixException) ||
			strings.HasPrefix(line, models.ErrorPrefixTraceback) ||
			strings.HasPrefix(line, models.ErrorPrefixPanic) {
			return strings.TrimSpace(lines[i])
		}
	}

	return ""
}

// ContinuePR 继续处理 PR 中的任务
func (a *Agent) ContinuePR(pr *github.PullRequest) error {
	return a.ContinuePRWithArgs(&github.IssueCommentEvent{
		Issue: &github.Issue{
			Number: github.Int(pr.GetNumber()),
			Title:  github.String(pr.GetTitle()),
		},
	}, "")
}

// ContinuePRWithArgs 继续处理 PR 中的任务，支持命令参数
func (a *Agent) ContinuePRWithArgs(event *github.IssueCommentEvent, args string) error {
	log.Infof("Continue PR #%d with args: %s", event.Issue.GetNumber(), args)

	// 1. 从工作空间管理器获取 PR 信息
	// 由于 PR 评论事件中的 Issue 就是 PR，我们可以直接使用
	pr := &github.PullRequest{
		Number:  event.Issue.Number,
		Title:   event.Issue.Title,
		HTMLURL: event.Issue.HTMLURL,
	}

	// 2. 获取或创建 PR 工作空间
	ws := a.workspace.GetOrCreateWorkspaceForPR(pr)
	if ws == nil {
		return fmt.Errorf("failed to get or create workspace for PR continue")
	}

	// 3. 拉取远端最新代码
	if err := a.github.PullLatestChanges(ws); err != nil {
		log.Errorf("Failed to pull latest changes: %v", err)
		// 不返回错误，继续执行，因为可能是网络问题
	}

	// 4. 初始化 code client
	code, err := a.sessionManager.GetSession(ws)
	if err != nil {
		log.Errorf("failed to get code client for PR continue: %v", err)
		return err
	}

	// 4. 构建 prompt，包含命令参数
	var prompt string
	if args != "" {
		prompt = fmt.Sprintf("请根据以下指令继续处理代码：\n\n指令：%s\n\n请直接进行相应的修改，回复要简洁明了。", args)
	} else {
		prompt = "请继续之前的任务，根据上下文进行相应的修改，回复要简洁明了。"
	}

	resp, err := a.promptWithRetry(code, prompt, 3)
	if err != nil {
		log.Errorf("failed to prompt for PR continue: %v", err)
		return err
	}

	output, err := io.ReadAll(resp.Out)
	if err != nil {
		log.Errorf("failed to read output for PR continue: %v", err)
		return err
	}

	log.Infof("PR Continue Output: %s", string(output))

	// 5. 提交变更并更新 PR
	result := &models.ExecutionResult{
		Output: string(output),
	}
	if err := a.github.CommitAndPush(ws, result, code); err != nil {
		log.Errorf("Failed to commit and push for PR continue: %v", err)
		return err
	}

	// 6. 评论到 PR
	commentBody := string(output)
	if err = a.github.CreatePullRequestComment(pr, commentBody); err != nil {
		log.Errorf("failed to create PR comment for continue: %v", err)
		return err
	}

	log.Infof("Successfully continue PR #%d", pr.GetNumber())
	return nil
}

// FixPR 修复 PR 中的问题
func (a *Agent) FixPR(pr *github.PullRequest) error {
	return a.FixPRWithArgs(&github.IssueCommentEvent{
		Issue: &github.Issue{
			Number: github.Int(pr.GetNumber()),
			Title:  github.String(pr.GetTitle()),
		},
	}, "")
}

// FixPRWithArgs 修复 PR 中的问题，支持命令参数
func (a *Agent) FixPRWithArgs(event *github.IssueCommentEvent, args string) error {
	log.Infof("Fix PR #%d with args: %s", event.Issue.GetNumber(), args)

	// 1. 从工作空间管理器获取 PR 信息
	pr := &github.PullRequest{
		Number:  event.Issue.Number,
		Title:   event.Issue.Title,
		HTMLURL: event.Issue.HTMLURL,
	}

	// 2. 获取或创建 PR 工作空间
	ws := a.workspace.GetOrCreateWorkspaceForPR(pr)
	if ws == nil {
		return fmt.Errorf("failed to get or create workspace for PR fix")
	}

	// 3. 拉取远端最新代码
	if err := a.github.PullLatestChanges(ws); err != nil {
		log.Errorf("Failed to pull latest changes: %v", err)
		// 不返回错误，继续执行，因为可能是网络问题
	}

	// 4. 初始化 code client
	code, err := a.sessionManager.GetSession(ws)
	if err != nil {
		log.Errorf("failed to get code client for PR fix: %v", err)
		return err
	}

	// 4. 构建 prompt，包含命令参数
	var prompt string
	if args != "" {
		prompt = fmt.Sprintf("请根据以下指令修复代码问题：\n\n指令：%s\n\n请直接进行修复，回复要简洁明了。", args)
	} else {
		prompt = "请分析当前代码中的问题并进行修复，回复要简洁明了。"
	}

	resp, err := a.promptWithRetry(code, prompt, 3)
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
	if err := a.github.CommitAndPush(ws, result, code); err != nil {
		log.Errorf("Failed to commit and push for PR fix: %v", err)
		return err
	}

	// 6. 评论到 PR
	commentBody := string(output)
	if err = a.github.CreatePullRequestComment(pr, commentBody); err != nil {
		log.Errorf("failed to create PR comment for fix: %v", err)
		return err
	}

	log.Infof("Successfully fixed PR #%d", pr.GetNumber())
	return nil
}

// ContinuePRFromReviewComment 从 PR 代码行评论继续处理任务
func (a *Agent) ContinuePRFromReviewComment(event *github.PullRequestReviewCommentEvent, args string) error {
	log.Infof("Continue PR #%d from review comment with args: %s", event.PullRequest.GetNumber(), args)

	// 1. 从工作空间管理器获取 PR 信息
	pr := event.PullRequest

	// 2. 获取或创建 PR 工作空间
	ws := a.workspace.GetOrCreateWorkspaceForPR(pr)
	if ws == nil {
		return fmt.Errorf("failed to get or create workspace for PR continue from review comment")
	}

	// 3. 拉取远端最新代码
	if err := a.github.PullLatestChanges(ws); err != nil {
		log.Errorf("Failed to pull latest changes: %v", err)
		// 不返回错误，继续执行，因为可能是网络问题
	}

	// 4. 初始化 code client
	code, err := a.sessionManager.GetSession(ws)
	if err != nil {
		log.Errorf("failed to get code client for PR continue from review comment: %v", err)
		return err
	}

	// 4. 构建 prompt，包含评论上下文和命令参数
	var prompt string

	// 获取行范围信息
	startLine := event.Comment.GetStartLine()
	endLine := event.Comment.GetLine()

	var lineRangeInfo string
	if startLine != 0 && endLine != 0 && startLine != endLine {
		// 多行选择
		lineRangeInfo = fmt.Sprintf("行号范围：%d-%d", startLine, endLine)
	} else {
		// 单行
		lineRangeInfo = fmt.Sprintf("行号：%d", endLine)
	}

	commentContext := fmt.Sprintf("代码行评论：%s\n文件：%s\n%s",
		event.Comment.GetBody(),
		event.Comment.GetPath(),
		lineRangeInfo)

	if args != "" {
		prompt = fmt.Sprintf("请根据以下代码行评论和指令继续处理代码：\n\n%s\n\n指令：%s\n\n请直接进行相应的修改，回复要简洁明了。", commentContext, args)
	} else {
		prompt = fmt.Sprintf("请根据以下代码行评论继续处理代码：\n\n%s\n\n请直接进行相应的修改，回复要简洁明了。", commentContext)
	}

	resp, err := a.promptWithRetry(code, prompt, 3)
	if err != nil {
		log.Errorf("failed to prompt for PR continue from review comment: %v", err)
		return err
	}

	output, err := io.ReadAll(resp.Out)
	if err != nil {
		log.Errorf("failed to read output for PR continue from review comment: %v", err)
		return err
	}

	log.Infof("PR Continue from Review Comment Output: %s", string(output))

	// 5. 提交变更并更新 PR
	result := &models.ExecutionResult{
		Output: string(output),
	}
	if err := a.github.CommitAndPush(ws, result, code); err != nil {
		log.Errorf("Failed to commit and push for PR continue from review comment: %v", err)
		return err
	}

	// 6. 回复原始评论
	commentBody := string(output)
	if err = a.github.ReplyToReviewComment(pr, event.Comment.GetID(), commentBody); err != nil {
		log.Errorf("failed to reply to review comment for continue: %v", err)
		return err
	}

	log.Infof("Successfully continue PR #%d from review comment", pr.GetNumber())
	return nil
}

// FixPRFromReviewComment 从 PR 代码行评论修复问题
func (a *Agent) FixPRFromReviewComment(event *github.PullRequestReviewCommentEvent, args string) error {
	log.Infof("Fix PR #%d from review comment with args: %s", event.PullRequest.GetNumber(), args)

	// 1. 从工作空间管理器获取 PR 信息
	pr := event.PullRequest

	// 2. 获取或创建 PR 工作空间
	ws := a.workspace.GetOrCreateWorkspaceForPR(pr)
	if ws == nil {
		return fmt.Errorf("failed to get or create workspace for PR fix from review comment")
	}

	// 3. 拉取远端最新代码
	if err := a.github.PullLatestChanges(ws); err != nil {
		log.Errorf("Failed to pull latest changes: %v", err)
		// 不返回错误，继续执行，因为可能是网络问题
	}

	// 4. 初始化 code client
	code, err := a.sessionManager.GetSession(ws)
	if err != nil {
		log.Errorf("failed to get code client for PR fix from review comment: %v", err)
		return err
	}

	// 4. 构建 prompt，包含评论上下文和命令参数
	var prompt string

	// 获取行范围信息
	startLine := event.Comment.GetStartLine()
	endLine := event.Comment.GetLine()

	var lineRangeInfo string
	if startLine != 0 && endLine != 0 && startLine != endLine {
		// 多行选择
		lineRangeInfo = fmt.Sprintf("行号范围：%d-%d", startLine, endLine)
	} else {
		// 单行
		lineRangeInfo = fmt.Sprintf("行号：%d", endLine)
	}

	commentContext := fmt.Sprintf("代码行评论：%s\n文件：%s\n%s",
		event.Comment.GetBody(),
		event.Comment.GetPath(),
		lineRangeInfo)

	if args != "" {
		prompt = fmt.Sprintf("请根据以下代码行评论和指令修复代码问题：\n\n%s\n\n指令：%s\n\n请直接进行修复，回复要简洁明了。", commentContext, args)
	} else {
		prompt = fmt.Sprintf("请根据以下代码行评论修复代码问题：\n\n%s\n\n请直接进行修复，回复要简洁明了。", commentContext)
	}

	resp, err := a.promptWithRetry(code, prompt, 3)
	if err != nil {
		log.Errorf("failed to prompt for PR fix from review comment: %v", err)
		return err
	}

	output, err := io.ReadAll(resp.Out)
	if err != nil {
		log.Errorf("failed to read output for PR fix from review comment: %v", err)
		return err
	}

	log.Infof("PR Fix from Review Comment Output: %s", string(output))

	// 5. 提交变更并更新 PR
	result := &models.ExecutionResult{
		Output: string(output),
	}
	if err := a.github.CommitAndPush(ws, result, code); err != nil {
		log.Errorf("Failed to commit and push for PR fix from review comment: %v", err)
		return err
	}

	// 6. 回复原始评论
	commentBody := string(output)
	if err = a.github.ReplyToReviewComment(pr, event.Comment.GetID(), commentBody); err != nil {
		log.Errorf("failed to reply to review comment for fix: %v", err)
		return err
	}

	log.Infof("Successfully fixed PR #%d from review comment", pr.GetNumber())
	return nil
}

// ReviewPR 审查 PR
func (a *Agent) ReviewPR(pr *github.PullRequest) error {
	return nil
}

// promptWithRetry 带重试机制的 prompt 调用
func (a *Agent) promptWithRetry(code code.Code, prompt string, maxRetries int) (*code.Response, error) {
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		resp, err := code.Prompt(prompt)
		if err == nil {
			return resp, nil
		}

		lastErr = err
		log.Warnf("Prompt attempt %d failed: %v", attempt, err)

		// 如果是 broken pipe 错误，尝试重新创建 session
		if strings.Contains(err.Error(), "broken pipe") ||
			strings.Contains(err.Error(), "process has already exited") {
			log.Infof("Detected broken pipe or process exit, will retry...")
		}

		if attempt < maxRetries {
			// 等待一段时间后重试
			time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
		}
	}

	return nil, fmt.Errorf("failed after %d attempts, last error: %w", maxRetries, lastErr)
}
