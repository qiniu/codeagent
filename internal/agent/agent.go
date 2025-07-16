package agent

import (
	"context"
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
	"github.com/qiniu/x/xlog"
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

	a := &Agent{
		config:         cfg,
		github:         githubClient,
		workspace:      workspaceManager,
		sessionManager: code.NewSessionManager(cfg),
	}

	go a.StartCleanupRoutine()

	return a
}

// startCleanupRoutine 启动定期清理协程
func (a *Agent) StartCleanupRoutine() {
	ticker := time.NewTicker(1 * time.Hour) // 每小时检查一次
	defer ticker.Stop()

	for range ticker.C {
		a.cleanupExpiredResouces()
	}
}

// cleanupExpiredResouces 清理过期的工作空间
func (a *Agent) cleanupExpiredResouces() {
	m := a.workspace

	// 先收集过期的工作空间，避免在持有锁时调用可能获取锁的方法
	expiredWorkspaces := a.workspace.GetExpiredWorkspaces()

	// 如果没有过期的工作空间，直接返回
	if len(expiredWorkspaces) == 0 {
		return
	}

	log.Infof("Found %d expired workspaces to clean up", len(expiredWorkspaces))

	// 清理过期的工作空间 和 code session
	for _, ws := range expiredWorkspaces {
		// 关闭 code session
		err := a.sessionManager.CloseSession(ws)
		if err != nil {
			log.Errorf("Failed to close session for workspace: %s", ws.Path)
		}

		// 清理工作空间
		b := m.CleanupWorkspace(ws)
		if !b {
			log.Errorf("Failed to clean up expired workspace : %s", ws.Path)
			continue
		}
		log.Infof("Cleaned up expired workspace: %s", ws.Path)
	}

}

// ProcessIssueComment 处理 Issue 评论事件，包含完整的仓库信息
func (a *Agent) ProcessIssueComment(ctx context.Context, event *github.IssueCommentEvent) error {
	log := xlog.NewWith(ctx)

	issueNumber := event.Issue.GetNumber()
	issueTitle := event.Issue.GetTitle()

	log.Infof("Starting issue comment processing: issue=#%d, title=%s", issueNumber, issueTitle)

	// 1. 创建 Issue 工作空间
	ws := a.workspace.CreateWorkspaceFromIssue(event.Issue)
	if ws == nil {
		log.Errorf("Failed to create workspace from issue")
		return fmt.Errorf("failed to create workspace from issue")
	}
	log.Infof("Created workspace: %s", ws.Path)

	// 2. 创建分支并推送
	log.Infof("Creating branch: %s", ws.Branch)
	if err := a.github.CreateBranch(ws); err != nil {
		log.Errorf("Failed to create branch: %v", err)
		return err
	}
	log.Infof("Branch created successfully")

	// 3. 创建初始 PR
	log.Infof("Creating initial PR")
	pr, err := a.github.CreatePullRequest(ws)
	if err != nil {
		log.Errorf("Failed to create PR: %v", err)
		return err
	}
	log.Infof("PR created successfully: #%d", pr.GetNumber())

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
	log.Infof("Session directory created: %s", sessionPath)

	// 6. 注册工作空间到 PR 映射
	ws.PullRequest = pr
	a.workspace.RegisterWorkspace(ws, pr)

	log.Infof("Workspace registered: issue=#%d, workspace=%s, session=%s", issueNumber, ws.Path, ws.SessionPath)

	// 7. 初始化 code client
	log.Infof("Initializing code client")
	code, err := a.sessionManager.GetSession(ws)
	if err != nil {
		log.Errorf("Failed to get code client: %v", err)
		return err
	}
	log.Infof("Code client initialized successfully")

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

	log.Infof("Executing code modification with AI")
	codeResp, err := a.promptWithRetry(ctx, code, codePrompt, 3)
	if err != nil {
		log.Errorf("Failed to prompt for code modification: %v", err)
		return err
	}

	codeOutput, err := io.ReadAll(codeResp.Out)
	if err != nil {
		log.Errorf("Failed to read code modification output: %v", err)
		return err
	}

	log.Infof("Code modification completed, output length: %d", len(codeOutput))
	log.Debugf("LLM Output: %s", string(codeOutput))

	// 9. 组织结构化 PR Body（解析三段式输出）
	aiStr := string(codeOutput)

	log.Infof("Parsing structured output")
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
		log.Warnf("Error detected in AI output: %s", errorInfo)
	}

	prBody += "<details><summary>原始 Prompt</summary>\n\n" + codePrompt + "\n\n</details>"

	log.Infof("Updating PR body")
	if err = a.github.UpdatePullRequest(pr, prBody); err != nil {
		log.Errorf("Failed to update PR body with execution result: %v", err)
		return err
	}
	log.Infof("PR body updated successfully")

	// 10. 提交变更并推送到远程
	result := &models.ExecutionResult{
		Output: string(codeOutput),
	}
	log.Infof("Committing and pushing changes")
	if err = a.github.CommitAndPush(ws, result, code); err != nil {
		log.Errorf("Failed to commit and push: %v", err)
		return err
	}
	log.Infof("Changes committed and pushed successfully")

	log.Infof("Issue processing completed successfully: issue=#%d, PR=%s", issueNumber, pr.GetHTMLURL())
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

// ContinuePRWithArgs 继续处理 PR 中的任务，支持命令参数
func (a *Agent) ContinuePRWithArgs(ctx context.Context, event *github.IssueCommentEvent, args string) error {
	log := xlog.NewWith(ctx)

	prNumber := event.Issue.GetNumber()
	log.Infof("Continue PR #%d with args: %s", prNumber, args)

	// 1. 验证这是一个 PR 评论（而不是 Issue 评论）
	if event.Issue.PullRequestLinks == nil {
		log.Errorf("This is not a PR comment, cannot continue")
		return fmt.Errorf("this is not a PR comment, cannot continue")
	}

	// 2. 从 IssueCommentEvent 中提取仓库信息
	repoURL := ""
	repoOwner := ""
	repoName := ""

	// 优先使用 repository 字段（如果存在）
	if event.Repo != nil {
		repoOwner = event.Repo.GetOwner().GetLogin()
		repoName = event.Repo.GetName()
		repoURL = event.Repo.GetCloneURL()
	}

	// 如果 repository 字段不存在，从 Issue 的 HTML URL 中提取
	if repoURL == "" {
		htmlURL := event.Issue.GetHTMLURL()
		if strings.Contains(htmlURL, "github.com") {
			parts := strings.Split(htmlURL, "/")
			if len(parts) >= 5 {
				repoOwner = parts[len(parts)-4] // owner
				repoName = parts[len(parts)-3]  // repo
				repoURL = fmt.Sprintf("https://github.com/%s/%s.git", repoOwner, repoName)
			}
		}
	}

	if repoURL == "" {
		log.Errorf("Failed to extract repository URL from event")
		return fmt.Errorf("failed to extract repository URL from event")
	}

	log.Infof("Extracted repository info: owner=%s, name=%s", repoOwner, repoName)

	// 3. 从 GitHub API 获取完整的 PR 信息
	log.Infof("Fetching PR information from GitHub API")
	pr, err := a.github.GetPullRequest(repoOwner, repoName, event.Issue.GetNumber())
	if err != nil {
		log.Errorf("Failed to get PR #%d: %v", prNumber, err)
		return fmt.Errorf("failed to get PR information: %w", err)
	}
	log.Infof("PR information fetched successfully")

	// 4. 获取或创建 PR 工作空间
	log.Infof("Getting or creating workspace for PR")
	ws := a.workspace.GetOrCreateWorkspaceForPR(pr)
	if ws == nil {
		log.Errorf("Failed to get or create workspace for PR continue")
		return fmt.Errorf("failed to get or create workspace for PR continue")
	}
	log.Infof("Workspace ready: %s", ws.Path)

	// 5. 拉取远端最新代码
	log.Infof("Pulling latest changes from remote")
	if err := a.github.PullLatestChanges(ws, pr); err != nil {
		log.Warnf("Failed to pull latest changes: %v", err)
		// 不返回错误，继续执行，因为可能是网络问题
	} else {
		log.Infof("Latest changes pulled successfully")
	}

	// 6. 初始化 code client
	log.Infof("Initializing code client")
	codeClient, err := a.sessionManager.GetSession(ws)
	if err != nil {
		log.Errorf("Failed to create code session: %v", err)
		return fmt.Errorf("failed to create code session: %w", err)
	}
	log.Infof("Code client initialized successfully")

	// 7. 构建 prompt，包含完整PR上下文和命令参数
	var prompt string

	// 构建包含所有PR上下文的信息
	prContext, err := a.buildPRContextForGeneralComment(ctx, pr, event.Comment.GetBody())
	if err != nil {
		log.Warnf("Failed to build PR context, using simple context: %v", err)
		// 降级到原有的简单上下文
		if args != "" {
			prompt = fmt.Sprintf("请根据以下指令继续处理这个 PR：\n\n%s\n\n请分析当前的代码变更，并根据指令执行相应的操作。", args)
		} else {
			prompt = "请继续处理这个 PR，分析代码变更并提供改进建议。"
		}
	} else {
		if args != "" {
			prompt = fmt.Sprintf("你是一个代码助手，需要根据PR的完整背景信息和当前评论来继续处理代码。\n\n%s\n\n**附加指令**: %s\n\n请根据以上信息和当前评论，分析代码变更并执行相应的操作。注意：当前评论是核心指令，历史评论仅作为上下文参考。", prContext, args)
			log.Infof("Using custom prompt with args and full context")
		} else {
			prompt = fmt.Sprintf("你是一个代码助手，需要根据PR的完整背景信息和当前评论来继续处理代码。\n\n%s\n\n请根据以上信息，分析代码变更并提供改进建议。注意：当前评论是核心指令，历史评论仅作为上下文参考。", prContext)
			log.Infof("Using default prompt with full context")
		}
	}

	// 8. 执行 AI 处理
	log.Infof("Executing AI processing for PR continue")
	resp, err := a.promptWithRetry(ctx, codeClient, prompt, 3)
	if err != nil {
		log.Errorf("Failed to process PR continue: %v", err)
		return fmt.Errorf("failed to process PR continue: %w", err)
	}

	output, err := io.ReadAll(resp.Out)
	if err != nil {
		log.Errorf("Failed to read output for PR continue: %v", err)
		return fmt.Errorf("failed to read output for PR continue: %w", err)
	}

	log.Infof("AI processing completed, output length: %d", len(output))
	log.Debugf("PR Continue Output: %s", string(output))

	// 9. 提交变更并更新 PR
	result := &models.ExecutionResult{
		Output: string(output),
		Error:  "",
	}

	log.Infof("Committing and pushing changes for PR continue")
	if err := a.github.CommitAndPush(ws, result, codeClient); err != nil {
		log.Errorf("Failed to commit and push changes: %v", err)
		// 不返回错误，继续执行评论
	} else {
		log.Infof("Changes committed and pushed successfully")
	}

	// 10. 评论到 PR
	commentBody := string(output)
	log.Infof("Creating PR comment")
	if err = a.github.CreatePullRequestComment(pr, commentBody); err != nil {
		log.Errorf("Failed to create PR comment: %v", err)
		return fmt.Errorf("failed to create PR comment: %w", err)
	}
	log.Infof("PR comment created successfully")

	log.Infof("Successfully continued PR #%d", prNumber)
	return nil
}

// FixPR 修复 PR 中的问题
func (a *Agent) FixPR(ctx context.Context, pr *github.PullRequest) error {
	return a.FixPRWithArgs(ctx, &github.IssueCommentEvent{
		Issue: &github.Issue{
			Number: github.Int(pr.GetNumber()),
			Title:  github.String(pr.GetTitle()),
		},
	}, "")
}

// FixPRWithArgs 修复 PR 中的问题，支持命令参数
func (a *Agent) FixPRWithArgs(ctx context.Context, event *github.IssueCommentEvent, args string) error {
	log := xlog.NewWith(ctx)

	prNumber := event.Issue.GetNumber()
	log.Infof("Fix PR #%d with args: %s", prNumber, args)

	// 1. 从 IssueCommentEvent 中提取仓库信息
	repoURL := ""
	repoOwner := ""
	repoName := ""

	// 优先使用 repository 字段（如果存在）
	if event.Repo != nil {
		repoOwner = event.Repo.GetOwner().GetLogin()
		repoName = event.Repo.GetName()
		repoURL = event.Repo.GetCloneURL()
	}

	// 如果 repository 字段不存在，从 Issue 的 HTML URL 中提取
	if repoURL == "" {
		htmlURL := event.Issue.GetHTMLURL()
		if strings.Contains(htmlURL, "github.com") {
			parts := strings.Split(htmlURL, "/")
			if len(parts) >= 5 {
				repoOwner = parts[len(parts)-4] // owner
				repoName = parts[len(parts)-3]  // repo
				repoURL = fmt.Sprintf("https://github.com/%s/%s.git", repoOwner, repoName)
			}
		}
	}

	if repoURL == "" {
		return fmt.Errorf("failed to extract repository URL from event")
	}

	// 2. 从 GitHub API 获取完整的 PR 信息
	pr, err := a.github.GetPullRequest(repoOwner, repoName, event.Issue.GetNumber())
	if err != nil {
		log.Errorf("Failed to get PR #%d: %v", event.Issue.GetNumber(), err)
		return fmt.Errorf("failed to get PR information: %w", err)
	}

	// 2. 获取或创建 PR 工作空间
	ws := a.workspace.GetOrCreateWorkspaceForPR(pr)
	if ws == nil {
		return fmt.Errorf("failed to get or create workspace for PR fix")
	}

	// 3. 拉取远端最新代码
	if err := a.github.PullLatestChanges(ws, pr); err != nil {
		log.Errorf("Failed to pull latest changes: %v", err)
		// 不返回错误，继续执行，因为可能是网络问题
	}

	// 4. 初始化 code client
	code, err := a.sessionManager.GetSession(ws)
	if err != nil {
		log.Errorf("failed to get code client for PR fix: %v", err)
		return err
	}

	// 4. 构建 prompt，包含完整PR上下文和命令参数
	var prompt string

	// 构建包含所有PR上下文的信息
	prContext, err := a.buildPRContextForGeneralComment(ctx, pr, event.Comment.GetBody())
	if err != nil {
		log.Warnf("Failed to build PR context, using simple context: %v", err)
		// 降级到原有的简单上下文
		if args != "" {
			prompt = fmt.Sprintf("请根据以下指令修复代码问题：\n\n指令：%s\n\n请直接进行修复，回复要简洁明了。", args)
		} else {
			prompt = "请分析当前代码中的问题并进行修复，回复要简洁明了。"
		}
	} else {
		if args != "" {
			prompt = fmt.Sprintf("你是一个代码助手，需要根据PR的完整背景信息和当前评论来修复代码问题。\n\n%s\n\n**附加指令**: %s\n\n请根据以上信息和当前评论，分析并修复代码问题。注意：当前评论是核心指令，历史评论仅作为上下文参考。回复要简洁明了。", prContext, args)
		} else {
			prompt = fmt.Sprintf("你是一个代码助手，需要根据PR的完整背景信息和当前评论来修复代码问题。\n\n%s\n\n请根据以上信息，分析当前代码中的问题并进行修复。注意：当前评论是核心指令，历史评论仅作为上下文参考。回复要简洁明了。", prContext)
		}
	}

	resp, err := a.promptWithRetry(ctx, code, prompt, 3)
	if err != nil {
		log.Errorf("Failed to prompt for PR fix: %v", err)
		return err
	}

	output, err := io.ReadAll(resp.Out)
	if err != nil {
		log.Errorf("Failed to read output for PR fix: %v", err)
		return err
	}

	log.Infof("PR Fix Output length: %d", len(output))
	log.Debugf("PR Fix Output: %s", string(output))

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
func (a *Agent) ContinuePRFromReviewComment(ctx context.Context, event *github.PullRequestReviewCommentEvent, args string) error {
	log := xlog.NewWith(ctx)

	prNumber := event.PullRequest.GetNumber()
	log.Infof("Continue PR #%d from review comment with args: %s", prNumber, args)

	// 1. 从工作空间管理器获取 PR 信息
	pr := event.PullRequest

	// 2. 获取或创建 PR 工作空间
	ws := a.workspace.GetOrCreateWorkspaceForPR(pr)
	if ws == nil {
		return fmt.Errorf("failed to get or create workspace for PR continue from review comment")
	}

	// 3. 拉取远端最新代码
	if err := a.github.PullLatestChanges(ws, pr); err != nil {
		log.Errorf("Failed to pull latest changes: %v", err)
		// 不返回错误，继续执行，因为可能是网络问题
	}

	// 4. 初始化 code client
	code, err := a.sessionManager.GetSession(ws)
	if err != nil {
		log.Errorf("failed to get code client for PR continue from review comment: %v", err)
		return err
	}

	// 4. 构建 prompt，包含完整PR上下文和当前评论
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

	// 构建包含所有PR上下文的信息
	prContext, err := a.buildPRContextForReviewComment(ctx, pr, event.Comment.GetBody(), event.Comment.GetPath(), lineRangeInfo)
	if err != nil {
		log.Warnf("Failed to build PR context, using simple context: %v", err)
		// 降级到原有的简单上下文
		commentContext := fmt.Sprintf("代码行评论：%s\n文件：%s\n%s",
			event.Comment.GetBody(),
			event.Comment.GetPath(),
			lineRangeInfo)
		prContext = commentContext
	}

	if args != "" {
		prompt = fmt.Sprintf("你是一个代码助手，需要根据PR的完整背景信息和当前的代码行评论来继续处理代码。\n\n%s\n\n**附加指令**: %s\n\n请根据以上信息和当前需要处理的评论，直接进行相应的代码修改。注意：当前评论是核心指令，历史评论仅作为上下文参考。回复要简洁明了。", prContext, args)
	} else {
		prompt = fmt.Sprintf("你是一个代码助手，需要根据PR的完整背景信息和当前的代码行评论来继续处理代码。\n\n%s\n\n请根据以上信息和当前需要处理的评论，直接进行相应的代码修改。注意：当前评论是核心指令，历史评论仅作为上下文参考。回复要简洁明了。", prContext)
	}

	resp, err := a.promptWithRetry(ctx, code, prompt, 3)
	if err != nil {
		log.Errorf("Failed to prompt for PR continue from review comment: %v", err)
		return err
	}

	output, err := io.ReadAll(resp.Out)
	if err != nil {
		log.Errorf("Failed to read output for PR continue from review comment: %v", err)
		return err
	}

	log.Infof("PR Continue from Review Comment Output length: %d", len(output))
	log.Debugf("PR Continue from Review Comment Output: %s", string(output))

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
func (a *Agent) FixPRFromReviewComment(ctx context.Context, event *github.PullRequestReviewCommentEvent, args string) error {
	log := xlog.NewWith(ctx)

	prNumber := event.PullRequest.GetNumber()
	log.Infof("Fix PR #%d from review comment with args: %s", prNumber, args)

	// 1. 从工作空间管理器获取 PR 信息
	pr := event.PullRequest

	// 2. 获取或创建 PR 工作空间
	ws := a.workspace.GetOrCreateWorkspaceForPR(pr)
	if ws == nil {
		return fmt.Errorf("failed to get or create workspace for PR fix from review comment")
	}

	// 3. 拉取远端最新代码
	if err := a.github.PullLatestChanges(ws, pr); err != nil {
		log.Errorf("Failed to pull latest changes: %v", err)
		// 不返回错误，继续执行，因为可能是网络问题
	}

	// 4. 初始化 code client
	code, err := a.sessionManager.GetSession(ws)
	if err != nil {
		log.Errorf("failed to get code client for PR fix from review comment: %v", err)
		return err
	}

	// 4. 构建 prompt，包含完整PR上下文和当前评论
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

	// 构建包含所有PR上下文的信息
	prContext, err := a.buildPRContextForReviewComment(ctx, pr, event.Comment.GetBody(), event.Comment.GetPath(), lineRangeInfo)
	if err != nil {
		log.Warnf("Failed to build PR context, using simple context: %v", err)
		// 降级到原有的简单上下文
		commentContext := fmt.Sprintf("代码行评论：%s\n文件：%s\n%s",
			event.Comment.GetBody(),
			event.Comment.GetPath(),
			lineRangeInfo)
		prContext = commentContext
	}

	if args != "" {
		prompt = fmt.Sprintf("你是一个代码助手，需要根据PR的完整背景信息和当前的代码行评论来修复代码问题。\n\n%s\n\n**附加指令**: %s\n\n请根据以上信息和当前需要处理的评论，直接进行代码修复。注意：当前评论是核心指令，历史评论仅作为上下文参考。回复要简洁明了。", prContext, args)
	} else {
		prompt = fmt.Sprintf("你是一个代码助手，需要根据PR的完整背景信息和当前的代码行评论来修复代码问题。\n\n%s\n\n请根据以上信息和当前需要处理的评论，直接进行代码修复。注意：当前评论是核心指令，历史评论仅作为上下文参考。回复要简洁明了。", prContext)
	}

	resp, err := a.promptWithRetry(ctx, code, prompt, 3)
	if err != nil {
		log.Errorf("Failed to prompt for PR fix from review comment: %v", err)
		return err
	}

	output, err := io.ReadAll(resp.Out)
	if err != nil {
		log.Errorf("Failed to read output for PR fix from review comment: %v", err)
		return err
	}

	log.Infof("PR Fix from Review Comment Output length: %d", len(output))
	log.Debugf("PR Fix from Review Comment Output: %s", string(output))

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
func (a *Agent) ReviewPR(ctx context.Context, pr *github.PullRequest) error {
	log := xlog.NewWith(ctx)

	log.Infof("Starting PR review for PR #%d", pr.GetNumber())
	// TODO: 实现 PR 审查逻辑
	log.Infof("PR review completed for PR #%d", pr.GetNumber())
	return nil
}

// CleanupAfterPRMerged PR 合并后清理工作区、映射和执行的code session
func (a *Agent) CleanupAfterPRMerged(ctx context.Context, pr *github.PullRequest) error {
	log := xlog.NewWith(ctx)

	prNumber := pr.GetNumber()
	log.Infof("Starting cleanup after PR #%d merged", prNumber)

	// 获取 workspace
	ws := a.workspace.GetWorkspaceByPR(pr)
	if ws == nil {
		log.Infof("No workspace found for PR: %s, skip cleanup", pr.GetHTMLURL())
		return nil
	}
	log.Infof("Found workspace for cleanup: %s", ws.Path)

	// 清理执行的 code session
	log.Infof("Closing code session")
	err := a.sessionManager.CloseSession(ws)
	if err != nil {
		log.Errorf("Failed to close code session for PR #%d: %v", prNumber, err)
		return fmt.Errorf("failed to close code session for PR #%d: %v", prNumber, err)
	}
	log.Infof("Code session closed successfully")

	// 清理 worktree,session 目录 和 对应的内存映射
	log.Infof("Cleaning up workspace")
	b := a.workspace.CleanupWorkspace(ws)
	if !b {
		log.Errorf("Failed to cleanup workspace for PR #%d", prNumber)
		return fmt.Errorf("failed to cleanup workspace for PR #%d", prNumber)
	}
	log.Infof("Workspace cleaned up successfully")

	log.Infof("Cleanup after PR merged completed: PR #%d, workspace: %s", prNumber, ws.Path)
	return nil
}

// buildPRContextForReviewComment 构建PR的完整上下文信息，包括PR描述和所有评论
func (a *Agent) buildPRContextForReviewComment(ctx context.Context, pr *github.PullRequest, currentCommentBody string, filePath string, lineInfo string) (string, error) {
	log := xlog.NewWith(ctx)

	var contextBuilder strings.Builder

	// 1. PR基本信息和描述
	contextBuilder.WriteString("## PR背景信息\n")
	contextBuilder.WriteString(fmt.Sprintf("**PR标题**: %s\n", pr.GetTitle()))
	contextBuilder.WriteString(fmt.Sprintf("**PR编号**: #%d\n", pr.GetNumber()))

	if pr.GetBody() != "" {
		contextBuilder.WriteString(fmt.Sprintf("**PR描述**:\n%s\n\n", pr.GetBody()))
	} else {
		contextBuilder.WriteString("**PR描述**: 无\n\n")
	}

	// 2. 获取并添加所有Issue评论（一般性PR评论）
	issueComments, err := a.github.GetPullRequestIssueComments(pr)
	if err != nil {
		log.Warnf("Failed to get PR issue comments: %v", err)
	} else if len(issueComments) > 0 {
		contextBuilder.WriteString("## PR讨论历史（按时间顺序）\n")
		for i, comment := range issueComments {
			// 过滤掉机器人评论和命令
			commentBody := comment.GetBody()
			if strings.HasPrefix(commentBody, "/") ||
				(comment.GetUser() != nil && strings.Contains(comment.GetUser().GetLogin(), "bot")) {
				continue
			}

			contextBuilder.WriteString(fmt.Sprintf("**评论 %d** (by %s):\n%s\n\n",
				i+1,
				comment.GetUser().GetLogin(),
				commentBody))
		}
	}

	// 3. 获取并添加所有代码行评论（Review评论），重点关注相关行
	reviewComments, err := a.github.GetPullRequestComments(pr)
	if err != nil {
		log.Warnf("Failed to get PR review comments: %v", err)
	} else if len(reviewComments) > 0 {
		// 分离当前文件和行的评论与其他评论
		var currentFileComments []*github.PullRequestComment
		var otherComments []*github.PullRequestComment

		// 解析当前行号信息
		currentLine := 0
		if strings.Contains(lineInfo, "行号：") {
			fmt.Sscanf(lineInfo, "行号：%d", &currentLine)
		} else if strings.Contains(lineInfo, "行号范围：") {
			fmt.Sscanf(lineInfo, "行号范围：%d-", &currentLine)
		}

		for _, comment := range reviewComments {
			// 过滤掉机器人评论和命令
			commentBody := comment.GetBody()
			if strings.HasPrefix(commentBody, "/") ||
				(comment.GetUser() != nil && strings.Contains(comment.GetUser().GetLogin(), "bot")) {
				continue
			}

			// 检查是否是同一文件的评论
			if comment.GetPath() == filePath {
				// 进一步检查是否在相同或相近的行
				commentLine := comment.GetLine()
				if currentLine > 0 && commentLine > 0 && abs(commentLine-currentLine) <= 10 {
					currentFileComments = append(currentFileComments, comment)
				} else {
					otherComments = append(otherComments, comment)
				}
			} else {
				otherComments = append(otherComments, comment)
			}
		}

		// 优先显示当前文件和相关行的评论
		if len(currentFileComments) > 0 {
			contextBuilder.WriteString("## 当前文件相关行的评论历史（重点关注）\n")
			for i, comment := range currentFileComments {
				commentBody := comment.GetBody()
				startLine := comment.GetStartLine()
				endLine := comment.GetLine()
				var lineRange string
				if startLine != 0 && endLine != 0 && startLine != endLine {
					lineRange = fmt.Sprintf("行号%d-%d", startLine, endLine)
				} else {
					lineRange = fmt.Sprintf("行号%d", endLine)
				}

				contextBuilder.WriteString(fmt.Sprintf("**🔍 相关评论 %d** (by %s, %s):\n%s\n\n",
					i+1,
					comment.GetUser().GetLogin(),
					lineRange,
					commentBody))
			}
		}

		// 显示其他代码评论
		if len(otherComments) > 0 {
			contextBuilder.WriteString("## 其他代码评审历史（按时间顺序）\n")
			for i, comment := range otherComments {
				commentBody := comment.GetBody()
				startLine := comment.GetStartLine()
				endLine := comment.GetLine()
				var lineRange string
				if startLine != 0 && endLine != 0 && startLine != endLine {
					lineRange = fmt.Sprintf("行号%d-%d", startLine, endLine)
				} else {
					lineRange = fmt.Sprintf("行号%d", endLine)
				}

				contextBuilder.WriteString(fmt.Sprintf("**代码评论 %d** (by %s, 文件:%s, %s):\n%s\n\n",
					i+1,
					comment.GetUser().GetLogin(),
					comment.GetPath(),
					lineRange,
					commentBody))
			}
		}
	}

	// 4. 当前需要处理的评论（突出显示）
	contextBuilder.WriteString("## 当前需要处理的评论\n")
	contextBuilder.WriteString(fmt.Sprintf("**文件**: %s\n", filePath))
	contextBuilder.WriteString(fmt.Sprintf("**位置**: %s\n", lineInfo))
	contextBuilder.WriteString(fmt.Sprintf("**评论内容**: %s\n\n", currentCommentBody))

	return contextBuilder.String(), nil
}

// abs 计算两个整数的绝对差值
func abs(a int) int {
	if a < 0 {
		return -a
	}
	return a
}

// buildPRContextForGeneralComment 构建PR的完整上下文信息，用于一般性PR评论（非代码行评论）
func (a *Agent) buildPRContextForGeneralComment(ctx context.Context, pr *github.PullRequest, currentCommentBody string) (string, error) {
	log := xlog.NewWith(ctx)

	var contextBuilder strings.Builder

	// 1. PR基本信息和描述
	contextBuilder.WriteString("## PR背景信息\n")
	contextBuilder.WriteString(fmt.Sprintf("**PR标题**: %s\n", pr.GetTitle()))
	contextBuilder.WriteString(fmt.Sprintf("**PR编号**: #%d\n", pr.GetNumber()))

	if pr.GetBody() != "" {
		contextBuilder.WriteString(fmt.Sprintf("**PR描述**:\n%s\n\n", pr.GetBody()))
	} else {
		contextBuilder.WriteString("**PR描述**: 无\n\n")
	}

	// 2. 获取并添加所有Issue评论（一般性PR评论）
	issueComments, err := a.github.GetPullRequestIssueComments(pr)
	if err != nil {
		log.Warnf("Failed to get PR issue comments: %v", err)
	} else if len(issueComments) > 0 {
		contextBuilder.WriteString("## PR讨论历史（按时间顺序）\n")
		for i, comment := range issueComments {
			// 过滤掉机器人评论、命令和当前评论
			commentBody := comment.GetBody()
			if strings.HasPrefix(commentBody, "/") ||
				(comment.GetUser() != nil && strings.Contains(comment.GetUser().GetLogin(), "bot")) ||
				commentBody == currentCommentBody {
				continue
			}

			contextBuilder.WriteString(fmt.Sprintf("**评论 %d** (by %s):\n%s\n\n",
				i+1,
				comment.GetUser().GetLogin(),
				commentBody))
		}
	}

	// 3. 获取并添加所有代码行评论（Review评论）
	reviewComments, err := a.github.GetPullRequestComments(pr)
	if err != nil {
		log.Warnf("Failed to get PR review comments: %v", err)
	} else if len(reviewComments) > 0 {
		contextBuilder.WriteString("## 代码评审历史（按时间顺序）\n")
		for i, comment := range reviewComments {
			// 过滤掉机器人评论和命令
			commentBody := comment.GetBody()
			if strings.HasPrefix(commentBody, "/") ||
				(comment.GetUser() != nil && strings.Contains(comment.GetUser().GetLogin(), "bot")) {
				continue
			}

			startLine := comment.GetStartLine()
			endLine := comment.GetLine()
			var lineRange string
			if startLine != 0 && endLine != 0 && startLine != endLine {
				lineRange = fmt.Sprintf("行号%d-%d", startLine, endLine)
			} else {
				lineRange = fmt.Sprintf("行号%d", endLine)
			}

			contextBuilder.WriteString(fmt.Sprintf("**代码评论 %d** (by %s, 文件:%s, %s):\n%s\n\n",
				i+1,
				comment.GetUser().GetLogin(),
				comment.GetPath(),
				lineRange,
				commentBody))
		}
	}

	// 4. 当前需要处理的评论（突出显示）
	contextBuilder.WriteString("## 当前需要处理的评论\n")
	contextBuilder.WriteString(fmt.Sprintf("**评论内容**: %s\n\n", currentCommentBody))

	return contextBuilder.String(), nil
}

// promptWithRetry 带重试机制的 prompt 调用
func (a *Agent) promptWithRetry(ctx context.Context, code code.Code, prompt string, maxRetries int) (*code.Response, error) {
	log := xlog.NewWith(ctx)
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		log.Debugf("Prompt attempt %d/%d", attempt, maxRetries)
		resp, err := code.Prompt(prompt)
		if err == nil {
			log.Infof("Prompt succeeded on attempt %d", attempt)
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
			sleepDuration := time.Duration(attempt) * 500 * time.Millisecond
			log.Infof("Waiting %v before retry", sleepDuration)
			time.Sleep(sleepDuration)
		}
	}

	log.Errorf("All prompt attempts failed after %d attempts", maxRetries)
	return nil, fmt.Errorf("failed after %d attempts, last error: %w", maxRetries, lastErr)
}
