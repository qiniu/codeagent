package agent

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/qiniu/codeagent/internal/code"
	"github.com/qiniu/codeagent/internal/config"
	ghclient "github.com/qiniu/codeagent/internal/github"
	"github.com/qiniu/codeagent/internal/workspace"
	"github.com/qiniu/codeagent/pkg/models"

	"github.com/google/go-github/v58/github"
	"github.com/qiniu/x/log"
	"github.com/qiniu/x/xlog"
)

type Agent struct {
	config         *config.Config
	github         *ghclient.Client
	githubManager  *ghclient.GitHubClientManager
	workspace      *workspace.Manager
	sessionManager *code.SessionManager
}

func New(cfg *config.Config, workspaceManager *workspace.Manager) *Agent {
	// Initialize GitHub client manager (new auto-detection approach)
	githubManager, err := ghclient.NewGitHubClientManager(cfg)
	if err != nil {
		log.Errorf("Failed to create GitHub client manager: %v", err)
		return nil
	}

	// Initialize legacy GitHub client for backward compatibility
	githubClient, err := ghclient.NewClient(cfg)
	if err != nil {
		log.Errorf("Failed to create GitHub client: %v", err)
		return nil
	}

	a := &Agent{
		config:         cfg,
		github:         githubClient,
		githubManager:  githubManager,
		workspace:      workspaceManager,
		sessionManager: code.NewSessionManager(cfg),
	}

	// Log authentication info
	authInfo := githubManager.GetAuthInfo()
	log.Infof("GitHub authentication initialized: type=%s, user=%s", authInfo.Type, authInfo.User)

	go a.StartCleanupRoutine()

	return a
}

// StartCleanupRoutine starts the periodic cleanup routine
func (a *Agent) StartCleanupRoutine() {
	ticker := time.NewTicker(1 * time.Hour) // Check every hour
	defer ticker.Stop()

	for range ticker.C {
		a.cleanupExpiredResources()
	}
}

// cleanupExpiredResources cleans up expired workspaces
func (a *Agent) cleanupExpiredResources() {
	m := a.workspace

	// First collect expired workspaces to avoid calling methods that may acquire locks while holding a lock
	expiredWorkspaces := a.workspace.GetExpiredWorkspaces()

	// Return directly if no expired workspaces
	if len(expiredWorkspaces) == 0 {
		return
	}

	log.Infof("Found %d expired workspaces to clean up", len(expiredWorkspaces))

	// Clean up expired workspaces and code sessions
	for _, ws := range expiredWorkspaces {
		log.Infof("Cleaning up expired workspace: %s (AI model: %s, PR: %d)", ws.Path, ws.AIModel, ws.PRNumber)

		// Close code session
		err := a.sessionManager.CloseSession(ws)
		if err != nil {
			log.Errorf("Failed to close session for workspace: %s (AI model: %s)", ws.Path, ws.AIModel)
		} else {
			log.Infof("Closed session for workspace: %s (AI model: %s)", ws.Path, ws.AIModel)
		}

		// Clean up workspace
		b := m.CleanupWorkspace(ws)
		if !b {
			log.Errorf("Failed to clean up expired workspace: %s (AI model: %s)", ws.Path, ws.AIModel)
			continue
		}
		log.Infof("Cleaned up expired workspace: %s (AI model: %s)", ws.Path, ws.AIModel)
	}

}

// GetGitHubClient returns a GitHub client with automatic authentication detection
// This method should be used instead of direct access to a.github for new code
func (a *Agent) GetGitHubClient(ctx context.Context) (*github.Client, error) {
	if a.githubManager == nil {
		return nil, fmt.Errorf("GitHub client manager is not initialized")
	}
	return a.githubManager.GetClient(ctx)
}

// GetGitHubInstallationClient returns a GitHub client for a specific installation
func (a *Agent) GetGitHubInstallationClient(ctx context.Context, installationID int64) (*github.Client, error) {
	if a.githubManager == nil {
		return nil, fmt.Errorf("GitHub client manager is not initialized")
	}
	return a.githubManager.GetInstallationClient(ctx, installationID)
}

// GetAuthInfo returns information about the current GitHub authentication
func (a *Agent) GetAuthInfo() interface{} {
	if a.githubManager == nil {
		return map[string]string{"status": "not_initialized"}
	}
	
	authInfo := a.githubManager.GetAuthInfo()
	return map[string]interface{}{
		"type":         authInfo.Type,
		"user":         authInfo.User,
		"permissions":  authInfo.Permissions,
		"app_id":       authInfo.AppID,
		"configured":   a.githubManager.DetectAuthMode(),
	}
}

// ProcessIssueComment processes Issue comment events, including complete repository information
func (a *Agent) ProcessIssueComment(ctx context.Context, event *github.IssueCommentEvent) error {
	return a.ProcessIssueCommentWithAI(ctx, event, "", "")
}

// ProcessIssueCommentWithAI processes Issue comment events with support for specifying AI models
func (a *Agent) ProcessIssueCommentWithAI(ctx context.Context, event *github.IssueCommentEvent, aiModel, args string) error {
	log := xlog.NewWith(ctx)

	issueNumber := event.Issue.GetNumber()
	issueTitle := event.Issue.GetTitle()

	log.Infof("Starting issue comment processing: issue=#%d, title=%s, AI model=%s", issueNumber, issueTitle, aiModel)

	// 1. Create Issue workspace, including AI model information
	ws := a.workspace.CreateWorkspaceFromIssueWithAI(event.Issue, aiModel)
	if ws == nil {
		log.Errorf("Failed to create workspace from issue")
		return fmt.Errorf("failed to create workspace from issue")
	}
	log.Infof("Created workspace: %s", ws.Path)

	// 2. Create branch and push
	log.Infof("Creating branch: %s", ws.Branch)
	if err := a.github.CreateBranch(ws); err != nil {
		log.Errorf("Failed to create branch: %v", err)
		return err
	}
	log.Infof("Branch created successfully")

	// 3. Create initial PR (before code generation)
	log.Infof("Creating initial PR")
	pr, err := a.github.CreatePullRequest(ws)
	if err != nil {
		log.Errorf("Failed to create PR: %v", err)
		return err
	}
	log.Infof("PR created successfully: #%d", pr.GetNumber())

	// 4. Move workspace from Issue to PR
	if err := a.workspace.MoveIssueToPR(ws, pr.GetNumber()); err != nil {
		log.Errorf("Failed to move workspace: %v", err)
	}
	ws.PRNumber = pr.GetNumber()

	// 5. Create session directory
	// Extract suffix from PR directory name
	prDirName := filepath.Base(ws.Path)
	suffix := a.workspace.ExtractSuffixFromPRDir(ws.AIModel, ws.Repo, pr.GetNumber(), prDirName)

	sessionPath, err := a.workspace.CreateSessionPath(filepath.Dir(ws.Path), ws.AIModel, ws.Repo, pr.GetNumber(), suffix)
	if err != nil {
		log.Errorf("Failed to create session directory: %v", err)
		return err
	}
	ws.SessionPath = sessionPath
	log.Infof("Session directory created: %s", sessionPath)

	// 6. Register workspace to PR mapping
	ws.PullRequest = pr
	a.workspace.RegisterWorkspace(ws, pr)

	log.Infof("Workspace registered: issue=#%d, workspace=%s, session=%s", issueNumber, ws.Path, ws.SessionPath)

	// 7. Initialize code client
	log.Infof("Initializing code client")
	codeClient, err := a.sessionManager.GetSession(ws)
	if err != nil {
		log.Errorf("Failed to get code client: %v", err)
		return err
	}
	log.Infof("Code client initialized successfully")

	// 8. Execute code modification
	codePrompt := fmt.Sprintf(`Modify code according to Issue:

Title: %s
Description: %s

Output format:
%s
Brief description of changes

%s
- List modified files and specific changes`, event.Issue.GetTitle(), event.Issue.GetBody(), models.SectionSummary, models.SectionChanges)

	log.Infof("Executing code modification with AI")
	codeResp, err := code.PromptWithRetry(ctx, codeClient, codePrompt, 3)
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

	// 9. Organize structured PR Body (parse three-section output)
	aiStr := string(codeOutput)

	log.Infof("Parsing structured output")
	// Parse three-section output
	summary, changes, testPlan := parseStructuredOutput(aiStr)

	// Build PR Body
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

	// Add original output and error information
	prBody += "---\n\n"
	prBody += "<details><summary>AI Complete Output</summary>\n\n" + aiStr + "\n\n</details>\n\n"

	// Error information detection
	errorInfo := extractErrorInfo(aiStr)
	if errorInfo != "" {
		prBody += "## Error Information\n\n```text\n" + errorInfo + "\n```\n\n"
		log.Warnf("Error detected in AI output: %s", errorInfo)
	}

	prBody += "<details><summary>Original Prompt</summary>\n\n" + codePrompt + "\n\n</details>"

	log.Infof("Updating PR body")
	if err = a.github.UpdatePullRequest(pr, prBody); err != nil {
		log.Errorf("Failed to update PR body with execution result: %v", err)
		return err
	}
	log.Infof("PR body updated successfully")

	// 10. Commit changes and push to remote
	result := &models.ExecutionResult{
		Output: string(codeOutput),
	}
	log.Infof("Committing and pushing changes")
	if _, err = a.github.CommitAndPush(ws, result, codeClient); err != nil {
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

// processPRWithArgs 处理PR的通用函数，支持不同的操作模式
func (a *Agent) processPRWithArgs(ctx context.Context, event *github.IssueCommentEvent, args string, mode string) error {
	return a.processPRWithArgsAndAI(ctx, event, "", args, mode)
}

// processPRWithArgsAndAI 处理PR的通用函数，支持不同的操作模式和AI模型
func (a *Agent) processPRWithArgsAndAI(ctx context.Context, event *github.IssueCommentEvent, aiModel, args string, mode string) error {
	log := xlog.NewWith(ctx)

	prNumber := event.Issue.GetNumber()
	log.Infof("%s PR #%d with AI model %s and args: %s", mode, prNumber, aiModel, args)

	// 1. 验证这是一个 PR 评论（仅对continue操作）
	if mode == "Continue" && event.Issue.PullRequestLinks == nil {
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

	// 4. 如果没有指定AI模型，从PR分支中提取
	if aiModel == "" {
		branchName := pr.GetHead().GetRef()
		aiModel = a.workspace.ExtractAIModelFromBranch(branchName)
		if aiModel == "" {
			// 如果无法从分支中提取，使用默认配置
			aiModel = a.config.CodeProvider
		}
		log.Infof("Extracted AI model from branch: %s", aiModel)
	}

	// 5. 获取或创建 PR 工作空间，包含AI模型信息
	log.Infof("Getting or creating workspace for PR with AI model: %s", aiModel)
	ws := a.workspace.GetOrCreateWorkspaceForPRWithAI(pr, aiModel)
	if ws == nil {
		log.Errorf("Failed to get or create workspace for PR %s", strings.ToLower(mode))
		return fmt.Errorf("failed to get or create workspace for PR %s", strings.ToLower(mode))
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

	// 7. 获取所有PR评论历史用于构建上下文
	log.Infof("Fetching all PR comments for historical context")
	allComments, err := a.github.GetAllPRComments(pr)
	if err != nil {
		log.Warnf("Failed to get PR comments for context: %v", err)
		// 不返回错误，使用简单的prompt
		allComments = &models.PRAllComments{}
	}

	// 8. 构建包含历史上下文和当前评论的 prompt
	var prompt string
	var currentCommentID int64
	var currentComment string
	if event.Comment != nil {
		currentCommentID = event.Comment.GetID()
		currentComment = event.Comment.GetBody()
	}
	historicalContext := a.formatHistoricalComments(allComments, currentCommentID)

	// 根据模式生成不同的 prompt
	prompt = a.buildPromptWithCurrentComment(mode, args, historicalContext, currentComment)

	log.Infof("Using %s prompt with args and historical context", strings.ToLower(mode))

	// 9. 执行 AI 处理
	log.Infof("Executing AI processing for PR %s", strings.ToLower(mode))
	resp, err := code.PromptWithRetry(ctx, codeClient, prompt, 3)
	if err != nil {
		log.Errorf("Failed to process PR %s: %v", strings.ToLower(mode), err)
		return fmt.Errorf("failed to process PR %s: %w", strings.ToLower(mode), err)
	}

	output, err := io.ReadAll(resp.Out)
	if err != nil {
		log.Errorf("Failed to read output for PR %s: %v", strings.ToLower(mode), err)
		return fmt.Errorf("failed to read output for PR %s: %w", strings.ToLower(mode), err)
	}

	log.Infof("AI processing completed, output length: %d", len(output))
	log.Debugf("PR %s Output: %s", mode, string(output))

	// 10. 提交变更并更新 PR
	result := &models.ExecutionResult{
		Output: string(output),
		Error:  "",
	}

	log.Infof("Committing and pushing changes for PR %s", strings.ToLower(mode))
	if _, err := a.github.CommitAndPush(ws, result, codeClient); err != nil {
		log.Errorf("Failed to commit and push changes: %v", err)
		// 根据模式决定是否返回错误
		if mode == "Fix" {
			return err
		}
		// Continue模式不返回错误，继续执行评论
	} else {
		log.Infof("Changes committed and pushed successfully")
	}

	// 11. 评论到 PR
	commentBody := string(output)
	log.Infof("Creating PR comment")
	if err = a.github.CreatePullRequestComment(pr, commentBody); err != nil {
		log.Errorf("Failed to create PR comment: %v", err)
		return fmt.Errorf("failed to create PR comment: %w", err)
	}
	log.Infof("PR comment created successfully")

	log.Infof("Successfully %s PR #%d", strings.ToLower(mode), prNumber)
	return nil
}

// buildPromptWithCurrentComment builds prompts for different modes, including current comment information
func (a *Agent) buildPromptWithCurrentComment(mode string, args string, historicalContext string, currentComment string) string {
	var prompt string
	var taskDescription string
	var defaultTask string

	switch mode {
	case "Continue":
		taskDescription = "Please make corresponding code modifications based on the above PR description, historical discussions, and current instructions."
		defaultTask = "Continue processing PR, analyze code changes and improve"
	case "Fix":
		taskDescription = "Please make corresponding code fixes based on the above PR description, historical discussions, and current instructions."
		defaultTask = "Analyze and fix code issues"
	default:
		taskDescription = "Please handle the code accordingly based on the above PR description, historical discussions, and current instructions."
		defaultTask = "Handle code tasks"
	}

	// Build context information for current comment
	var currentCommentContext string
	if currentComment != "" {
		// Extract command and args from current comment
		var commentCommand, commentArgs string
		if strings.HasPrefix(currentComment, "/continue") {
			commentCommand = "/continue"
			commentArgs = strings.TrimSpace(strings.TrimPrefix(currentComment, "/continue"))
		} else if strings.HasPrefix(currentComment, "/fix") {
			commentCommand = "/fix"
			commentArgs = strings.TrimSpace(strings.TrimPrefix(currentComment, "/fix"))
		}

		if commentArgs != "" {
			currentCommentContext = fmt.Sprintf("## Current Comment\nUser just issued command: %s %s", commentCommand, commentArgs)
		} else {
			currentCommentContext = fmt.Sprintf("## Current Comment\nUser just issued command: %s", commentCommand)
		}
	}

	if args != "" {
		if historicalContext != "" || currentCommentContext != "" {
			contextParts := []string{}
			if historicalContext != "" {
				contextParts = append(contextParts, historicalContext)
			}
			if currentCommentContext != "" {
				contextParts = append(contextParts, currentCommentContext)
			}
			fullContext := strings.Join(contextParts, "\n\n")

			prompt = fmt.Sprintf(`As a PR code review assistant, please %s based on the following complete context:

%s

## Execution Instructions
%s

%sNote:
1. The current instruction is the main task, historical information is only for context reference
2. Please ensure modifications align with the PR's overall goals and existing discussion consensus
3. If conflicts with historical discussions are found, prioritize executing the current instruction and explain in the response`,
				strings.ToLower(mode), fullContext, args, taskDescription)
		} else {
			prompt = fmt.Sprintf("According to instruction %s:\n\n%s", strings.ToLower(mode), args)
		}
	} else {
		if historicalContext != "" || currentCommentContext != "" {
			contextParts := []string{}
			if historicalContext != "" {
				contextParts = append(contextParts, historicalContext)
			}
			if currentCommentContext != "" {
				contextParts = append(contextParts, currentCommentContext)
			}
			fullContext := strings.Join(contextParts, "\n\n")

			prompt = fmt.Sprintf(`As a PR code review assistant, please %s based on the following complete context:

%s

## Task
%s

Please make corresponding code modifications and improvements based on the above PR description and historical discussions.`,
				strings.ToLower(mode), fullContext, defaultTask)
		} else {
			prompt = defaultTask
		}
	}

	return prompt
}

// ContinuePRWithArgs continues processing tasks in PR, supporting command parameters
func (a *Agent) ContinuePRWithArgs(ctx context.Context, event *github.IssueCommentEvent, args string) error {
	return a.processPRWithArgs(ctx, event, args, "Continue")
}

// ContinuePRWithArgsAndAI continues processing tasks in PR, supporting command parameters and AI models
func (a *Agent) ContinuePRWithArgsAndAI(ctx context.Context, event *github.IssueCommentEvent, aiModel, args string) error {
	return a.processPRWithArgsAndAI(ctx, event, aiModel, args, "Continue")
}

// FixPR fixes issues in PR
func (a *Agent) FixPR(ctx context.Context, pr *github.PullRequest) error {
	return a.FixPRWithArgs(ctx, &github.IssueCommentEvent{
		Issue: &github.Issue{
			Number: github.Int(pr.GetNumber()),
			Title:  github.String(pr.GetTitle()),
		},
	}, "")
}

// FixPRWithArgs fixes issues in PR, supporting command parameters
func (a *Agent) FixPRWithArgs(ctx context.Context, event *github.IssueCommentEvent, args string) error {
	return a.processPRWithArgs(ctx, event, args, "Fix")
}

// FixPRWithArgsAndAI fixes issues in PR, supporting command parameters and AI models
func (a *Agent) FixPRWithArgsAndAI(ctx context.Context, event *github.IssueCommentEvent, aiModel, args string) error {
	return a.processPRWithArgsAndAI(ctx, event, aiModel, args, "Fix")
}

// ContinuePRFromReviewComment continues processing tasks from PR code line comments
func (a *Agent) ContinuePRFromReviewComment(ctx context.Context, event *github.PullRequestReviewCommentEvent, args string) error {
	return a.ContinuePRFromReviewCommentWithAI(ctx, event, "", args)
}

// ContinuePRFromReviewCommentWithAI continues processing tasks from PR code line comments, supporting AI models
func (a *Agent) ContinuePRFromReviewCommentWithAI(ctx context.Context, event *github.PullRequestReviewCommentEvent, aiModel, args string) error {
	log := xlog.NewWith(ctx)

	prNumber := event.PullRequest.GetNumber()
	log.Infof("Continue PR #%d from review comment with AI model %s and args: %s", prNumber, aiModel, args)

	// 1. 从工作空间管理器获取 PR 信息
	pr := event.PullRequest

	// 2. 如果没有指定AI模型，从PR分支中提取
	if aiModel == "" {
		branchName := pr.GetHead().GetRef()
		aiModel = a.workspace.ExtractAIModelFromBranch(branchName)
		if aiModel == "" {
			// 如果无法从分支中提取，使用默认配置
			aiModel = a.config.CodeProvider
		}
		log.Infof("Extracted AI model from branch: %s", aiModel)
	}

	// 3. 获取或创建 PR 工作空间，包含AI模型信息
	ws := a.workspace.GetOrCreateWorkspaceForPRWithAI(pr, aiModel)
	if ws == nil {
		return fmt.Errorf("failed to get or create workspace for PR continue from review comment")
	}

	// 3. 拉取远端最新代码
	if err := a.github.PullLatestChanges(ws, pr); err != nil {
		log.Errorf("Failed to pull latest changes: %v", err)
		// 不返回错误，继续执行，因为可能是网络问题
	}

	// 4. 初始化 code client
	codeClient, err := a.sessionManager.GetSession(ws)
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
		prompt = fmt.Sprintf("根据代码行评论和指令处理：\n\n%s\n\n指令：%s", commentContext, args)
	} else {
		prompt = fmt.Sprintf("根据代码行评论处理：\n\n%s", commentContext)
	}

	resp, err := code.PromptWithRetry(ctx, codeClient, prompt, 3)
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
	if _, err := a.github.CommitAndPush(ws, result, codeClient); err != nil {
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
	return a.FixPRFromReviewCommentWithAI(ctx, event, "", args)
}

// FixPRFromReviewCommentWithAI 从 PR 代码行评论修复问题，支持AI模型
func (a *Agent) FixPRFromReviewCommentWithAI(ctx context.Context, event *github.PullRequestReviewCommentEvent, aiModel, args string) error {
	log := xlog.NewWith(ctx)

	prNumber := event.PullRequest.GetNumber()
	log.Infof("Fix PR #%d from review comment with AI model %s and args: %s", prNumber, aiModel, args)

	// 1. 从工作空间管理器获取 PR 信息
	pr := event.PullRequest

	// 2. 如果没有指定AI模型，从PR分支中提取
	if aiModel == "" {
		branchName := pr.GetHead().GetRef()
		aiModel = a.workspace.ExtractAIModelFromBranch(branchName)
		if aiModel == "" {
			// 如果无法从分支中提取，使用默认配置
			aiModel = a.config.CodeProvider
		}
		log.Infof("Extracted AI model from branch: %s", aiModel)
	}

	// 3. 获取或创建 PR 工作空间，包含AI模型信息
	ws := a.workspace.GetOrCreateWorkspaceForPRWithAI(pr, aiModel)
	if ws == nil {
		return fmt.Errorf("failed to get or create workspace for PR fix from review comment")
	}

	// 3. 拉取远端最新代码
	if err := a.github.PullLatestChanges(ws, pr); err != nil {
		log.Errorf("Failed to pull latest changes: %v", err)
		// 不返回错误，继续执行，因为可能是网络问题
	}

	// 4. 初始化 code client
	codeClient, err := a.sessionManager.GetSession(ws)
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
		prompt = fmt.Sprintf("根据代码行评论和指令修复：\n\n%s\n\n指令：%s", commentContext, args)
	} else {
		prompt = fmt.Sprintf("根据代码行评论修复：\n\n%s", commentContext)
	}

	resp, err := code.PromptWithRetry(ctx, codeClient, prompt, 3)
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
	if _, err := a.github.CommitAndPush(ws, result, codeClient); err != nil {
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

// ProcessPRFromReviewWithTriggerUser 从 PR review 批量处理多个 review comments 并在反馈中@用户
func (a *Agent) ProcessPRFromReviewWithTriggerUser(ctx context.Context, event *github.PullRequestReviewEvent, command string, args string, triggerUser string) error {
	return a.ProcessPRFromReviewWithTriggerUserAndAI(ctx, event, command, "", args, triggerUser)
}

// ProcessPRFromReviewWithTriggerUserAndAI 从 PR review 批量处理多个 review comments 并在反馈中@用户，支持AI模型
func (a *Agent) ProcessPRFromReviewWithTriggerUserAndAI(ctx context.Context, event *github.PullRequestReviewEvent, command string, aiModel, args string, triggerUser string) error {
	log := xlog.NewWith(ctx)

	prNumber := event.PullRequest.GetNumber()
	reviewID := event.Review.GetID()
	log.Infof("Processing PR #%d from review %d with command: %s, AI model: %s, args: %s, triggerUser: %s", prNumber, reviewID, command, aiModel, args, triggerUser)

	// 1. 从工作空间管理器获取 PR 信息
	pr := event.PullRequest

	// 2. 如果没有指定AI模型，从PR分支中提取
	if aiModel == "" {
		branchName := pr.GetHead().GetRef()
		aiModel = a.workspace.ExtractAIModelFromBranch(branchName)
		if aiModel == "" {
			// 如果无法从分支中提取，使用默认配置
			aiModel = a.config.CodeProvider
		}
		log.Infof("Extracted AI model from branch: %s", aiModel)
	}

	// 3. 获取指定 review 的所有 comments
	reviewComments, err := a.github.GetReviewComments(pr, reviewID)
	if err != nil {
		log.Errorf("Failed to get review comments: %v", err)
		return err
	}

	log.Infof("Found %d review comments for review %d", len(reviewComments), reviewID)

	// 4. 获取或创建 PR 工作空间，包含AI模型信息
	ws := a.workspace.GetOrCreateWorkspaceForPRWithAI(pr, aiModel)
	if ws == nil {
		return fmt.Errorf("failed to get or create workspace for PR batch processing from review")
	}

	// 4. 拉取远端最新代码
	if err := a.github.PullLatestChanges(ws, pr); err != nil {
		log.Errorf("Failed to pull latest changes: %v", err)
		// 不返回错误，继续执行，因为可能是网络问题
	}

	// 5. 初始化 code client
	codeClient, err := a.sessionManager.GetSession(ws)
	if err != nil {
		log.Errorf("failed to get code client for PR batch processing from review: %v", err)
		return err
	}

	// 6. 构建批量处理的 prompt，包含所有 review comments 和位置信息
	var commentContexts []string

	// 添加 review body 作为总体上下文
	if event.Review.GetBody() != "" {
		commentContexts = append(commentContexts, fmt.Sprintf("Review 总体说明：%s", event.Review.GetBody()))
	}

	// 为每个 comment 构建详细上下文
	for i, comment := range reviewComments {
		startLine := comment.GetStartLine()
		endLine := comment.GetLine()
		filePath := comment.GetPath()
		commentBody := comment.GetBody()

		var lineRangeInfo string
		if startLine != 0 && endLine != 0 && startLine != endLine {
			// 多行选择
			lineRangeInfo = fmt.Sprintf("行号范围：%d-%d", startLine, endLine)
		} else {
			// 单行
			lineRangeInfo = fmt.Sprintf("行号：%d", endLine)
		}

		commentContext := fmt.Sprintf("评论 %d：\n文件：%s\n%s\n内容：%s",
			i+1, filePath, lineRangeInfo, commentBody)
		commentContexts = append(commentContexts, commentContext)
	}

	// 组合所有上下文
	allComments := strings.Join(commentContexts, "\n\n")

	var prompt string
	if command == "/continue" {
		if args != "" {
			prompt = fmt.Sprintf("请根据以下 PR Review 的批量评论和指令继续处理代码：\n\n%s\n\n指令：%s\n\n请一次性处理所有评论中提到的问题，回复要简洁明了。", allComments, args)
		} else {
			prompt = fmt.Sprintf("请根据以下 PR Review 的批量评论继续处理代码：\n\n%s\n\n请一次性处理所有评论中提到的问题，回复要简洁明了。", allComments)
		}
	} else { // /fix
		if args != "" {
			prompt = fmt.Sprintf("请根据以下 PR Review 的批量评论和指令修复代码问题：\n\n%s\n\n指令：%s\n\n请一次性修复所有评论中提到的问题，回复要简洁明了。", allComments, args)
		} else {
			prompt = fmt.Sprintf("请根据以下 PR Review 的批量评论修复代码问题：\n\n%s\n\n请一次性修复所有评论中提到的问题，回复要简洁明了。", allComments)
		}
	}

	resp, err := code.PromptWithRetry(ctx, codeClient, prompt, 3)
	if err != nil {
		log.Errorf("Failed to prompt for PR batch processing from review: %v", err)
		return err
	}

	output, err := io.ReadAll(resp.Out)
	if err != nil {
		log.Errorf("Failed to read output for PR batch processing from review: %v", err)
		return err
	}

	log.Infof("PR Batch Processing from Review Output length: %d", len(output))
	log.Debugf("PR Batch Processing from Review Output: %s", string(output))

	// 7. 提交变更并更新 PR
	result := &models.ExecutionResult{
		Output: string(output),
	}
	if _, err := a.github.CommitAndPush(ws, result, codeClient); err != nil {
		log.Errorf("Failed to commit and push for PR batch processing from review: %v", err)
		return err
	}

	// 8. 创建评论，包含@用户提及
	var responseBody string
	if triggerUser != "" {
		if len(reviewComments) == 0 {
			responseBody = fmt.Sprintf("@%s 已根据 review 说明处理：\n\n%s", triggerUser, string(output))
		} else {
			responseBody = fmt.Sprintf("@%s 已批量处理此次 review 的 %d 个评论：\n\n%s", triggerUser, len(reviewComments), string(output))
		}
	} else {
		if len(reviewComments) == 0 {
			responseBody = fmt.Sprintf("已根据 review 说明处理：\n\n%s", string(output))
		} else {
			responseBody = fmt.Sprintf("已批量处理此次 review 的 %d 个评论：\n\n%s", len(reviewComments), string(output))
		}
	}

	if err = a.github.CreatePullRequestComment(pr, responseBody); err != nil {
		log.Errorf("failed to create PR comment for batch processing result: %v", err)
		return err
	}

	log.Infof("Successfully processed PR #%d from review %d with %d comments", pr.GetNumber(), reviewID, len(reviewComments))
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

// CleanupAfterPRClosed PR 关闭后清理工作区、映射、执行的code session和删除CodeAgent创建的分支
func (a *Agent) CleanupAfterPRClosed(ctx context.Context, pr *github.PullRequest) error {
	log := xlog.NewWith(ctx)

	prNumber := pr.GetNumber()
	prBranch := pr.GetHead().GetRef()
	log.Infof("Starting cleanup after PR #%d closed, branch: %s", prNumber, prBranch)

	// 获取所有与该PR相关的工作空间（可能有多个不同AI模型的工作空间）
	workspaces := a.workspace.GetAllWorkspacesByPR(pr)
	if len(workspaces) == 0 {
		log.Infof("No workspaces found for PR: %s", pr.GetHTMLURL())
	} else {
		log.Infof("Found %d workspaces for cleanup", len(workspaces))

		// 清理所有工作空间
		for _, ws := range workspaces {
			log.Infof("Cleaning up workspace: %s (AI model: %s)", ws.Path, ws.AIModel)

			// 清理执行的 code session
			log.Infof("Closing code session for AI model: %s", ws.AIModel)
			err := a.sessionManager.CloseSession(ws)
			if err != nil {
				log.Errorf("Failed to close code session for PR #%d with AI model %s: %v", prNumber, ws.AIModel, err)
				// 不返回错误，继续清理其他工作空间
			} else {
				log.Infof("Code session closed successfully for AI model: %s", ws.AIModel)
			}

			// 清理 worktree,session 目录 和 对应的内存映射
			log.Infof("Cleaning up workspace for AI model: %s", ws.AIModel)
			b := a.workspace.CleanupWorkspace(ws)
			if !b {
				log.Errorf("Failed to cleanup workspace for PR #%d with AI model %s", prNumber, ws.AIModel)
				// 不返回错误，继续清理其他工作空间
			} else {
				log.Infof("Workspace cleaned up successfully for AI model: %s", ws.AIModel)
			}
		}
	}

	// 删除CodeAgent创建的分支
	if prBranch != "" && strings.HasPrefix(prBranch, "codeagent") {
		owner := pr.GetBase().GetRepo().GetOwner().GetLogin()
		repoName := pr.GetBase().GetRepo().GetName()

		log.Infof("Deleting CodeAgent branch: %s from repo %s/%s", prBranch, owner, repoName)
		err := a.github.DeleteCodeAgentBranch(ctx, owner, repoName, prBranch)
		if err != nil {
			log.Errorf("Failed to delete branch %s: %v", prBranch, err)
			// 不返回错误，继续完成其他清理工作
		} else {
			log.Infof("Successfully deleted CodeAgent branch: %s", prBranch)
		}
	} else {
		log.Infof("Branch %s is not a CodeAgent branch, skipping deletion", prBranch)
	}

	log.Infof("Cleanup after PR closed completed: PR #%d, cleaned %d workspaces", prNumber, len(workspaces))
	return nil
}

// formatHistoricalComments 格式化历史评论，用于构建上下文
func (a *Agent) formatHistoricalComments(allComments *models.PRAllComments, currentCommentID int64) string {
	var contextParts []string

	// 添加 PR 描述
	if allComments.PRBody != "" {
		contextParts = append(contextParts, fmt.Sprintf("## PR 描述\n%s", allComments.PRBody))
	}

	// 添加历史的一般评论（排除当前评论）
	if len(allComments.IssueComments) > 0 {
		var historyComments []string
		for _, comment := range allComments.IssueComments {
			if comment.GetID() != currentCommentID {
				user := comment.GetUser().GetLogin()
				body := comment.GetBody()
				createdAt := comment.GetCreatedAt().Format("2006-01-02 15:04:05")
				historyComments = append(historyComments, fmt.Sprintf("**%s** (%s):\n%s", user, createdAt, body))
			}
		}
		if len(historyComments) > 0 {
			contextParts = append(contextParts, fmt.Sprintf("## 历史评论\n%s", strings.Join(historyComments, "\n\n")))
		}
	}

	// 添加代码行评论
	if len(allComments.ReviewComments) > 0 {
		var reviewComments []string
		for _, comment := range allComments.ReviewComments {
			if comment.GetID() != currentCommentID {
				user := comment.GetUser().GetLogin()
				body := comment.GetBody()
				path := comment.GetPath()
				line := comment.GetLine()
				createdAt := comment.GetCreatedAt().Format("2006-01-02 15:04:05")
				reviewComments = append(reviewComments, fmt.Sprintf("**%s** (%s) - %s:%d:\n%s", user, createdAt, path, line, body))
			}
		}
		if len(reviewComments) > 0 {
			contextParts = append(contextParts, fmt.Sprintf("## 代码行评论\n%s", strings.Join(reviewComments, "\n\n")))
		}
	}

	// 添加 Review 评论
	if len(allComments.Reviews) > 0 {
		var reviews []string
		for _, review := range allComments.Reviews {
			if review.GetBody() != "" {
				user := review.GetUser().GetLogin()
				body := review.GetBody()
				state := review.GetState()
				createdAt := review.GetSubmittedAt().Format("2006-01-02 15:04:05")
				reviews = append(reviews, fmt.Sprintf("**%s** (%s) - %s:\n%s", user, createdAt, state, body))
			}
		}
		if len(reviews) > 0 {
			contextParts = append(contextParts, fmt.Sprintf("## Review 评论\n%s", strings.Join(reviews, "\n\n")))
		}
	}

	return strings.Join(contextParts, "\n\n")
}
