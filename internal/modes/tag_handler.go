package modes

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/qiniu/codeagent/internal/code"
	ghclient "github.com/qiniu/codeagent/internal/github"
	"github.com/qiniu/codeagent/internal/interaction"
	"github.com/qiniu/codeagent/internal/mcp"
	"github.com/qiniu/codeagent/internal/workspace"
	"github.com/qiniu/codeagent/pkg/models"

	"github.com/google/go-github/v58/github"
	"github.com/qiniu/x/xlog"
)

// TagHandler Tag模式处理器
// 对应claude-code-action中的TagMode
// 处理包含命令的GitHub事件（/code, /continue, /fix）
type TagHandler struct {
	*BaseHandler
	github         *ghclient.Client
	workspace      *workspace.Manager
	mcpClient      mcp.MCPClient
	sessionManager *code.SessionManager
}

// NewTagHandler 创建Tag模式处理器
func NewTagHandler(github *ghclient.Client, workspace *workspace.Manager, mcpClient mcp.MCPClient, sessionManager *code.SessionManager) *TagHandler {
	return &TagHandler{
		BaseHandler: NewBaseHandler(
			TagMode,
			10, // 中等优先级
			"Handle @codeagent mentions and commands (/code, /continue, /fix)",
		),
		github:         github,
		workspace:      workspace,
		mcpClient:      mcpClient,
		sessionManager: sessionManager,
	}
}

// CanHandle 检查是否能处理给定的事件
func (th *TagHandler) CanHandle(ctx context.Context, event models.GitHubContext) bool {
	xl := xlog.NewWith(ctx)
	
	// 检查是否包含命令
	cmdInfo, hasCmd := models.HasCommand(event)
	if !hasCmd {
		xl.Debugf("No command found in event")
		return false
	}
	
	xl.Infof("Found command: %s with AI model: %s", cmdInfo.Command, cmdInfo.AIModel)
	
	// Tag模式处理所有包含命令的事件
	switch event.GetEventType() {
	case models.EventIssueComment,
		 models.EventPullRequestReview,
		 models.EventPullRequestReviewComment:
		return true
	default:
		xl.Debugf("Event type %s not supported by TagHandler", event.GetEventType())
		return false
	}
}

// Execute 执行Tag模式处理逻辑
func (th *TagHandler) Execute(ctx context.Context, event models.GitHubContext) error {
	xl := xlog.NewWith(ctx)
	xl.Infof("TagHandler executing for event type: %s", event.GetEventType())
	
	// 提取命令信息
	cmdInfo, hasCmd := models.HasCommand(event)
	if !hasCmd {
		return fmt.Errorf("no command found in event")
	}
	
	// 设置默认AI模型（如果未指定）
	aiModel := cmdInfo.AIModel
	if aiModel == "" {
		aiModel = "claude" // 默认使用claude，实际应该从配置获取
	}
	
	xl.Infof("Executing command: %s with AI model: %s, args: %s", 
		cmdInfo.Command, aiModel, cmdInfo.Args)
	
	// 根据事件类型和命令类型分发处理
	switch event.GetEventType() {
	case models.EventIssueComment:
		return th.handleIssueComment(ctx, event.(*models.IssueCommentContext), cmdInfo, aiModel)
	case models.EventPullRequestReview:
		return th.handlePRReview(ctx, event.(*models.PullRequestReviewContext), cmdInfo, aiModel)
	case models.EventPullRequestReviewComment:
		return th.handlePRReviewComment(ctx, event.(*models.PullRequestReviewCommentContext), cmdInfo, aiModel)
	default:
		return fmt.Errorf("unsupported event type for TagHandler: %s", event.GetEventType())
	}
}

// handleIssueComment 处理Issue评论事件
func (th *TagHandler) handleIssueComment(
	ctx context.Context,
	event *models.IssueCommentContext,
	cmdInfo *models.CommandInfo,
	aiModel string,
) error {
	xl := xlog.NewWith(ctx)
	
	// 将事件转换为原始GitHub事件类型（兼容现有agent接口）
	_ = event.RawEvent.(*github.IssueCommentEvent)
	
	if event.IsPRComment {
		// 这是PR评论
		xl.Infof("Processing PR comment with command: %s", cmdInfo.Command)
		
		switch cmdInfo.Command {
		case models.CommandContinue:
			// 实现PR继续逻辑，集成原始Agent功能
			xl.Infof("Processing /continue command for PR with new architecture")
			return th.processPRCommand(ctx, event, cmdInfo, aiModel, "Continue")
		case models.CommandFix:
			// 实现PR修复逻辑，集成原始Agent功能
			xl.Infof("Processing /fix command for PR with new architecture")
			return th.processPRCommand(ctx, event, cmdInfo, aiModel, "Fix")
		default:
			return fmt.Errorf("unsupported command for PR comment: %s", cmdInfo.Command)
		}
	} else {
		// 这是Issue评论
		xl.Infof("Processing Issue comment with command: %s", cmdInfo.Command)
		
		switch cmdInfo.Command {
		case models.CommandCode:
			// 实现Issue处理逻辑，集成原始Agent功能
			xl.Infof("Processing /code command for issue with new architecture")
			return th.processIssueCodeCommand(ctx, event, cmdInfo, aiModel)
		default:
			return fmt.Errorf("unsupported command for Issue comment: %s", cmdInfo.Command)
		}
	}
}

// handlePRReview 处理PR Review事件
func (th *TagHandler) handlePRReview(
	ctx context.Context,
	event *models.PullRequestReviewContext,
	cmdInfo *models.CommandInfo,
	aiModel string,
) error {
	xl := xlog.NewWith(ctx)
	xl.Infof("Processing PR review with command: %s", cmdInfo.Command)
	
	// 将事件转换为原始GitHub事件类型
	_ = event.RawEvent.(*github.PullRequestReviewEvent)
	
	// PR Review支持批量处理命令
	switch cmdInfo.Command {
	case models.CommandContinue:
		// 实现PR Review继续逻辑，集成原始 Agent功能
		xl.Infof("Processing PR review continue with new architecture")
		return th.processPRReviewCommand(ctx, event, cmdInfo, aiModel, "Continue")
	case models.CommandFix:
		// 实现PR Review修复逻辑，集成原姻 Agent功能
		xl.Infof("Processing PR review fix with new architecture")
		return th.processPRReviewCommand(ctx, event, cmdInfo, aiModel, "Fix")
	default:
		return fmt.Errorf("unsupported command for PR review: %s", cmdInfo.Command)
	}
}

// handlePRReviewComment 处理PR Review评论事件
func (th *TagHandler) handlePRReviewComment(
	ctx context.Context,
	event *models.PullRequestReviewCommentContext,
	cmdInfo *models.CommandInfo,
	aiModel string,
) error {
	xl := xlog.NewWith(ctx)
	xl.Infof("Processing PR review comment with command: %s", cmdInfo.Command)
	
	// 将事件转换为原始GitHub事件类型
	_ = event.RawEvent.(*github.PullRequestReviewCommentEvent)
	
	// PR Review评论支持行级命令
	switch cmdInfo.Command {
	case models.CommandContinue:
		// 实现PR Review评论继续逻辑，集成原姻 Agent功能
		xl.Infof("Processing PR review comment continue with new architecture")
		return th.processPRReviewCommentCommand(ctx, event, cmdInfo, aiModel, "Continue")
	case models.CommandFix:
		// 实现PR Review评论修复逻辑，集成原姻Agent功能
		xl.Infof("Processing PR review comment fix with new architecture")
		return th.processPRReviewCommentCommand(ctx, event, cmdInfo, aiModel, "Fix")
	default:
		return fmt.Errorf("unsupported command for PR review comment: %s", cmdInfo.Command)
	}
}

// processIssueCodeCommand 处理Issue的/code命令，集成渐进式评论功能
func (th *TagHandler) processIssueCodeCommand(
	ctx context.Context,
	event *models.IssueCommentContext,
	cmdInfo *models.CommandInfo,
	aiModel string,
) error {
	xl := xlog.NewWith(ctx)

	issueNumber := event.Issue.GetNumber()
	issueTitle := event.Issue.GetTitle()

	xl.Infof("Starting issue code processing with progress tracking: issue=#%d, title=%s, AI model=%s", 
		issueNumber, issueTitle, aiModel)

	// 0. 初始化渐进式评论管理器
	pcm := interaction.NewProgressCommentManager(th.github, event.GetRepository(), issueNumber)
	
	// 定义任务列表
	tasks := []*models.Task{
		{ID: "analyze-issue", Description: "📋 Analyze issue requirements", Status: models.TaskStatusPending},
		{ID: "create-workspace", Description: "🗂️ Create workspace and branch", Status: models.TaskStatusPending},
		{ID: "generate-code", Description: "🤖 Generate code implementation", Status: models.TaskStatusPending},
		{ID: "commit-changes", Description: "💾 Commit and push changes", Status: models.TaskStatusPending},
		{ID: "create-pr", Description: "🚀 Create pull request", Status: models.TaskStatusPending},
		{ID: "update-pr", Description: "📝 Update PR description", Status: models.TaskStatusPending},
	}

	// 初始化进度评论
	if err := pcm.InitializeProgress(ctx, tasks); err != nil {
		xl.Errorf("Failed to initialize progress comment: %v", err)
		// 继续执行，不因为评论失败而中断主流程
	}

	var ws *models.Workspace
	var pr *github.PullRequest
	var result *models.ProgressExecutionResult

	// 使用defer确保最终状态更新
	defer func() {
		if result == nil {
			result = &models.ProgressExecutionResult{
				Success: false,
				Error:   "Process interrupted or failed",
			}
		}
		
		// 添加工作空间和PR信息
		if ws != nil {
			result.BranchName = ws.Branch
		}
		if pr != nil {
			result.PullRequestURL = pr.GetHTMLURL()
		}
		
		if err := pcm.FinalizeComment(ctx, result); err != nil {
			xl.Errorf("Failed to finalize progress comment: %v", err)
		}
	}()

	// 1. 分析Issue要求
	if err := pcm.UpdateTask(ctx, "analyze-issue", models.TaskStatusInProgress, "Reading issue details and planning implementation"); err != nil {
		xl.Errorf("Failed to update task: %v", err)
	}

	xl.Infof("Analyzing issue requirements")
	
	if err := pcm.UpdateTask(ctx, "analyze-issue", models.TaskStatusCompleted); err != nil {
		xl.Errorf("Failed to update task: %v", err)
	}

	// 2. 创建工作空间和分支
	if err := pcm.UpdateTask(ctx, "create-workspace", models.TaskStatusInProgress, "Setting up workspace and creating branch"); err != nil {
		xl.Errorf("Failed to update task: %v", err)
	}

	// 创建Issue工作空间，包含AI模型信息
	ws = th.workspace.CreateWorkspaceFromIssueWithAI(event.Issue, aiModel)
	if ws == nil {
		result = &models.ProgressExecutionResult{
			Success: false,
			Error:   "Failed to create workspace from issue",
		}
		return fmt.Errorf("failed to create workspace from issue")
	}
	xl.Infof("Created workspace: %s", ws.Path)

	// 创建分支并推送
	xl.Infof("Creating branch: %s", ws.Branch)
	if err := th.github.CreateBranch(ws); err != nil {
		result = &models.ProgressExecutionResult{
			Success: false,
			Error:   fmt.Sprintf("Failed to create branch: %v", err),
		}
		return err
	}
	xl.Infof("Branch created successfully")

	if err := pcm.UpdateTask(ctx, "create-workspace", models.TaskStatusCompleted); err != nil {
		xl.Errorf("Failed to update task: %v", err)
	}

	// 3. 生成代码实现
	if err := pcm.UpdateTask(ctx, "generate-code", models.TaskStatusInProgress, "Calling AI to generate code implementation"); err != nil {
		xl.Errorf("Failed to update task: %v", err)
	}

	// 初始化code client
	xl.Infof("Initializing code client")
	codeClient, err := th.sessionManager.GetSession(ws)
	if err != nil {
		result = &models.ProgressExecutionResult{
			Success: false,
			Error:   fmt.Sprintf("Failed to get code client: %v", err),
		}
		return err
	}
	xl.Infof("Code client initialized successfully")

	// 执行代码修改
	codePrompt := fmt.Sprintf(`根据Issue修改代码：

标题：%s
描述：%s

输出格式：
%s
简要说明改动内容

%s
- 列出修改的文件和具体变动`, event.Issue.GetTitle(), event.Issue.GetBody(), models.SectionSummary, models.SectionChanges)

	xl.Infof("Executing code modification with AI")
	if err := pcm.ShowSpinner(ctx, "AI is analyzing and generating code..."); err != nil {
		xl.Errorf("Failed to show spinner: %v", err)
	}

	codeResp, err := th.promptWithRetry(ctx, codeClient, codePrompt, 3)
	if err != nil {
		result = &models.ProgressExecutionResult{
			Success: false,
			Error:   fmt.Sprintf("Failed to prompt for code modification: %v", err),
		}
		return err
	}

	codeOutput, err := io.ReadAll(codeResp.Out)
	if err != nil {
		result = &models.ProgressExecutionResult{
			Success: false,
			Error:   fmt.Sprintf("Failed to read code modification output: %v", err),
		}
		return err
	}

	if err := pcm.HideSpinner(ctx); err != nil {
		xl.Errorf("Failed to hide spinner: %v", err)
	}

	xl.Infof("Code modification completed, output length: %d", len(codeOutput))
	xl.Debugf("LLM Output: %s", string(codeOutput))

	if err := pcm.UpdateTask(ctx, "generate-code", models.TaskStatusCompleted); err != nil {
		xl.Errorf("Failed to update task: %v", err)
	}

	// 4. 提交并推送代码变更
	if err := pcm.UpdateTask(ctx, "commit-changes", models.TaskStatusInProgress, "Committing and pushing code changes to repository"); err != nil {
		xl.Errorf("Failed to update task: %v", err)
	}

	aiStr := string(codeOutput)
	executionResult := &models.ExecutionResult{
		Output: aiStr,
		Error:  "",
	}

	err = th.github.CommitAndPush(ws, executionResult, codeClient)
	if err != nil {
		result = &models.ProgressExecutionResult{
			Success: false,
			Error:   fmt.Sprintf("Failed to commit and push changes: %v", err),
		}
		return fmt.Errorf("failed to commit and push changes: %w", err)
	}
	xl.Infof("Code changes committed and pushed successfully")

	if err := pcm.UpdateTask(ctx, "commit-changes", models.TaskStatusCompleted); err != nil {
		xl.Errorf("Failed to update task: %v", err)
	}

	// 5. 创建初始PR
	if err := pcm.UpdateTask(ctx, "create-pr", models.TaskStatusInProgress, "Creating pull request"); err != nil {
		xl.Errorf("Failed to update task: %v", err)
	}

	xl.Infof("Creating initial PR")
	pr, err = th.github.CreatePullRequest(ws)
	if err != nil {
		result = &models.ProgressExecutionResult{
			Success: false,
			Error:   fmt.Sprintf("Failed to create PR: %v", err),
		}
		return err
	}
	xl.Infof("PR created successfully: #%d", pr.GetNumber())

	// 移动工作空间从Issue到PR
	if err := th.workspace.MoveIssueToPR(ws, pr.GetNumber()); err != nil {
		xl.Errorf("Failed to move workspace: %v", err)
	}
	ws.PRNumber = pr.GetNumber()

	// 创建session目录
	prDirName := filepath.Base(ws.Path)
	suffix := th.workspace.ExtractSuffixFromPRDir(ws.AIModel, ws.Repo, pr.GetNumber(), prDirName)

	sessionPath, err := th.workspace.CreateSessionPath(filepath.Dir(ws.Path), ws.AIModel, ws.Repo, pr.GetNumber(), suffix)
	if err != nil {
		xl.Errorf("Failed to create session directory: %v", err)
		// 不返回错误，继续执行
	} else {
		ws.SessionPath = sessionPath
		xl.Infof("Session directory created: %s", sessionPath)
	}

	// 注册工作空间到PR映射
	ws.PullRequest = pr
	th.workspace.RegisterWorkspace(ws, pr)

	xl.Infof("Workspace registered: issue=#%d, workspace=%s, session=%s", 
		issueNumber, ws.Path, ws.SessionPath)

	if err := pcm.UpdateTask(ctx, "create-pr", models.TaskStatusCompleted); err != nil {
		xl.Errorf("Failed to update task: %v", err)
	}

	// 6. 更新PR描述
	if err := pcm.UpdateTask(ctx, "update-pr", models.TaskStatusInProgress, "Updating PR description with implementation details"); err != nil {
		xl.Errorf("Failed to update task: %v", err)
	}

	// 组织结构化PR Body（解析三段式输出）
	xl.Infof("Parsing structured output")
	summary, changes, testPlan := th.parseStructuredOutput(aiStr)

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

	// 使用MCP工具更新PR描述
	xl.Infof("Updating PR description with MCP tools")
	err = th.updatePRWithMCP(ctx, ws, pr, prBody, aiStr)
	if err != nil {
		xl.Errorf("Failed to update PR with MCP: %v", err)
		// 不返回错误，因为代码已经提交成功，只是PR描述更新失败
	} else {
		xl.Infof("Successfully updated PR description via MCP")
	}

	if err := pcm.UpdateTask(ctx, "update-pr", models.TaskStatusCompleted); err != nil {
		xl.Errorf("Failed to update task: %v", err)
	}

	// 设置成功结果
	result = &models.ProgressExecutionResult{
		Success:         true,
		Summary:         summary,
		BranchName:      ws.Branch,
		PullRequestURL:  pr.GetHTMLURL(),
		FilesChanged:    []string{}, // TODO: 从git diff中提取文件列表
	}

	xl.Infof("Issue code processing completed successfully")
	return nil
}

// promptWithRetry 带重试的提示执行
func (th *TagHandler) promptWithRetry(ctx context.Context, codeClient code.Code, prompt string, maxRetries int) (*code.Response, error) {
	xl := xlog.NewWith(ctx)
	
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		xl.Infof("Executing prompt (attempt %d/%d)", i+1, maxRetries)
		
		resp, err := codeClient.Prompt(prompt)
		if err == nil {
			xl.Infof("Prompt executed successfully on attempt %d", i+1)
			return resp, nil
		}
		
		lastErr = err
		xl.Warnf("Prompt failed on attempt %d: %v", i+1, err)
		
		if i < maxRetries-1 {
			xl.Infof("Retrying...")
		}
	}
	
	return nil, fmt.Errorf("failed after %d attempts: %w", maxRetries, lastErr)
}

// parseStructuredOutput 解析结构化输出
func (th *TagHandler) parseStructuredOutput(output string) (summary, changes, testPlan string) {
	// 这里实现解析逻辑，提取summary、changes和testPlan
	// 简化版本，实际中应该有更复杂的解析逻辑
	lines := strings.Split(output, "\n")
	
	currentSection := ""
	var summaryLines, changesLines, testPlanLines []string
	
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		
		if strings.Contains(trimmed, models.SectionSummary) {
			currentSection = "summary"
			continue
		} else if strings.Contains(trimmed, models.SectionChanges) {
			currentSection = "changes"
			continue
		} else if strings.Contains(trimmed, models.SectionTestPlan) {
			currentSection = "testplan"
			continue
		}
		
		switch currentSection {
		case "summary":
			if trimmed != "" {
				summaryLines = append(summaryLines, trimmed)
			}
		case "changes":
			if trimmed != "" {
				changesLines = append(changesLines, trimmed)
			}
		case "testplan":
			if trimmed != "" {
				testPlanLines = append(testPlanLines, trimmed)
			}
		}
	}
	
	return strings.Join(summaryLines, "\n"), 
		   strings.Join(changesLines, "\n"), 
		   strings.Join(testPlanLines, "\n")
}

// updatePRWithMCP 使用MCP工具更新PR
func (th *TagHandler) updatePRWithMCP(ctx context.Context, ws *models.Workspace, pr *github.PullRequest, prBody, originalOutput string) error {
	xl := xlog.NewWith(ctx)
	
	// 创建MCP上下文
	mcpCtx := &models.MCPContext{
		Repository: &models.IssueCommentContext{
			BaseContext: models.BaseContext{
				Repository: &github.Repository{
					Name:     github.String(ws.Repo),
					FullName: github.String(ws.Org + "/" + ws.Repo),
					Owner: &github.User{
						Login: github.String(ws.Org),
					},
				},
			},
		},
		Permissions: []string{"github:read", "github:write"},
		Constraints: []string{},
	}
	
	// 使用MCP工具更新PR描述
	updateCall := &models.ToolCall{
		ID: "update_pr_" + fmt.Sprintf("%d", pr.GetNumber()),
		Function: models.ToolFunction{
			Name: "github-comments_update_pr_description",
			Arguments: map[string]interface{}{
				"pr_number": pr.GetNumber(),
				"body":      prBody,
			},
		},
	}
	
	_, err := th.mcpClient.ExecuteToolCalls(ctx, []*models.ToolCall{updateCall}, mcpCtx)
	if err != nil {
		xl.Errorf("Failed to update PR description via MCP: %v", err)
		// 不返回错误，因为这不是致命的
	} else {
		xl.Infof("Successfully updated PR description via MCP")
	}
	
	return nil
}

// processPRCommand 处理PR的通用命令（continue/fix），集成渐进式评论功能
func (th *TagHandler) processPRCommand(
	ctx context.Context,
	event *models.IssueCommentContext,
	cmdInfo *models.CommandInfo,
	aiModel string,
	mode string,
) error {
	xl := xlog.NewWith(ctx)
	
	prNumber := event.Issue.GetNumber()
	xl.Infof("%s PR #%d with AI model %s and args: %s", mode, prNumber, aiModel, cmdInfo.Args)

	// 初始化渐进式评论管理器
	pcm := interaction.NewProgressCommentManager(th.github, event.GetRepository(), prNumber)
	
	// 定义PR处理任务列表
	tasks := []*models.Task{
		{ID: "validate-pr", Description: fmt.Sprintf("🔍 Validate PR #%d context", prNumber), Status: models.TaskStatusPending},
		{ID: "setup-workspace", Description: "🗂️ Setup workspace for PR", Status: models.TaskStatusPending},
		{ID: "process-ai", Description: fmt.Sprintf("🤖 %s code with AI", mode), Status: models.TaskStatusPending},
		{ID: "commit-push", Description: "💾 Commit and push changes", Status: models.TaskStatusPending},
		{ID: "add-comment", Description: "💬 Add completion comment", Status: models.TaskStatusPending},
	}

	// 初始化进度评论
	if err := pcm.InitializeProgress(ctx, tasks); err != nil {
		xl.Errorf("Failed to initialize progress comment: %v", err)
	}

	var result *models.ProgressExecutionResult

	// 确保最终状态更新
	defer func() {
		if result == nil {
			result = &models.ProgressExecutionResult{
				Success: false,
				Error:   "PR processing interrupted or failed",
			}
		}
		
		if err := pcm.FinalizeComment(ctx, result); err != nil {
			xl.Errorf("Failed to finalize progress comment: %v", err)
		}
	}()
	
	// 1. 验证PR上下文
	if err := pcm.UpdateTask(ctx, "validate-pr", models.TaskStatusInProgress, "Validating PR context and permissions"); err != nil {
		xl.Errorf("Failed to update task: %v", err)
	}

	if !event.IsPRComment {
		result = &models.ProgressExecutionResult{
			Success: false,
			Error:   fmt.Sprintf("This is not a PR comment, cannot %s", strings.ToLower(mode)),
		}
		return fmt.Errorf("this is not a PR comment, cannot %s", strings.ToLower(mode))
	}

	if err := pcm.UpdateTask(ctx, "validate-pr", models.TaskStatusCompleted); err != nil {
		xl.Errorf("Failed to update task: %v", err)
	}
	
	// 2. 从IssueCommentEvent中提取仓库信息
	rawEvent := event.RawEvent.(*github.IssueCommentEvent)
	repoOwner := ""
	repoName := ""
	
	if rawEvent.Repo != nil {
		repoOwner = rawEvent.Repo.GetOwner().GetLogin()
		repoName = rawEvent.Repo.GetName()
	}
	
	if repoOwner == "" || repoName == "" {
		xl.Errorf("Failed to extract repository info from event")
		return fmt.Errorf("failed to extract repository info from event")
	}
	
	xl.Infof("Extracted repository info: owner=%s, name=%s", repoOwner, repoName)
	
	// 3. 从GitHub API获取完整的PR信息
	xl.Infof("Fetching PR information from GitHub API")
	pr, err := th.github.GetPullRequest(repoOwner, repoName, prNumber)
	if err != nil {
		xl.Errorf("Failed to get PR #%d: %v", prNumber, err)
		return fmt.Errorf("failed to get PR information: %w", err)
	}
	xl.Infof("PR information fetched successfully")
	
	// 4. 如果没有指定AI模型，从PR分支中提取
	if aiModel == "" {
		branchName := pr.GetHead().GetRef()
		aiModel = th.workspace.ExtractAIModelFromBranch(branchName)
		if aiModel == "" {
			// 使用默认值
			aiModel = "claude"
		}
		xl.Infof("Extracted AI model from branch: %s", aiModel)
	}
	
	// 5. 设置工作空间
	if err := pcm.UpdateTask(ctx, "setup-workspace", models.TaskStatusInProgress, "Setting up workspace and pulling latest code"); err != nil {
		xl.Errorf("Failed to update task: %v", err)
	}

	xl.Infof("Getting or creating workspace for PR with AI model: %s", aiModel)
	ws := th.workspace.GetOrCreateWorkspaceForPRWithAI(pr, aiModel)
	if ws == nil {
		result = &models.ProgressExecutionResult{
			Success: false,
			Error:   fmt.Sprintf("Failed to get or create workspace for PR %s", strings.ToLower(mode)),
		}
		return fmt.Errorf("failed to get or create workspace for PR %s", strings.ToLower(mode))
	}
	xl.Infof("Workspace ready: %s", ws.Path)
	
	// 6. 拉取远端最新代码
	xl.Infof("Pulling latest changes from remote")
	if err := th.github.PullLatestChanges(ws, pr); err != nil {
		xl.Warnf("Failed to pull latest changes: %v", err)
		// 不返回错误，继续执行
	} else {
		xl.Infof("Latest changes pulled successfully")
	}

	// 初始化code client
	xl.Infof("Initializing code client")
	codeClient, err := th.sessionManager.GetSession(ws)
	if err != nil {
		result = &models.ProgressExecutionResult{
			Success: false,
			Error:   fmt.Sprintf("Failed to create code session: %v", err),
		}
		return fmt.Errorf("failed to create code session: %w", err)
	}
	xl.Infof("Code client initialized successfully")

	if err := pcm.UpdateTask(ctx, "setup-workspace", models.TaskStatusCompleted); err != nil {
		xl.Errorf("Failed to update task: %v", err)
	}
	
	// 8. 获取PR评论历史用于构建上下文
	xl.Infof("Fetching all PR comments for historical context")
	allComments, err := th.github.GetAllPRComments(pr)
	if err != nil {
		xl.Warnf("Failed to get PR comments for context: %v", err)
		allComments = &models.PRAllComments{}
	}
	
	// 9. 构建包含历史上下文的prompt
	var currentCommentID int64
	if event.Comment != nil {
		currentCommentID = event.Comment.GetID()
	}
	historicalContext := th.formatHistoricalComments(allComments, currentCommentID)
	prompt := th.buildPrompt(mode, cmdInfo.Args, historicalContext)
	
	xl.Infof("Using %s prompt with args and historical context", strings.ToLower(mode))
	
	// 10. 执行AI处理
	if err := pcm.UpdateTask(ctx, "process-ai", models.TaskStatusInProgress, fmt.Sprintf("AI is processing %s request...", strings.ToLower(mode))); err != nil {
		xl.Errorf("Failed to update task: %v", err)
	}

	if err := pcm.ShowSpinner(ctx, fmt.Sprintf("AI is working on %s...", strings.ToLower(mode))); err != nil {
		xl.Errorf("Failed to show spinner: %v", err)
	}

	xl.Infof("Executing AI processing for PR %s", strings.ToLower(mode))
	resp, err := th.promptWithRetry(ctx, codeClient, prompt, 3)
	if err != nil {
		result = &models.ProgressExecutionResult{
			Success: false,
			Error:   fmt.Sprintf("Failed to process PR %s: %v", strings.ToLower(mode), err),
		}
		return fmt.Errorf("failed to process PR %s: %w", strings.ToLower(mode), err)
	}
	
	output, err := io.ReadAll(resp.Out)
	if err != nil {
		result = &models.ProgressExecutionResult{
			Success: false,
			Error:   fmt.Sprintf("Failed to read output for PR %s: %v", strings.ToLower(mode), err),
		}
		return fmt.Errorf("failed to read output for PR %s: %w", strings.ToLower(mode), err)
	}

	if err := pcm.HideSpinner(ctx); err != nil {
		xl.Errorf("Failed to hide spinner: %v", err)
	}
	
	xl.Infof("AI processing completed, output length: %d", len(output))
	xl.Debugf("PR %s Output: %s", mode, string(output))

	if err := pcm.UpdateTask(ctx, "process-ai", models.TaskStatusCompleted); err != nil {
		xl.Errorf("Failed to update task: %v", err)
	}
	
	// 11. 提交变更
	if err := pcm.UpdateTask(ctx, "commit-push", models.TaskStatusInProgress, "Committing and pushing changes to repository"); err != nil {
		xl.Errorf("Failed to update task: %v", err)
	}

	executionResult := &models.ExecutionResult{
		Output: string(output),
		Error:  "",
	}
	
	xl.Infof("Committing and pushing changes for PR %s", strings.ToLower(mode))
	if err := th.github.CommitAndPush(ws, executionResult, codeClient); err != nil {
		xl.Errorf("Failed to commit and push changes: %v", err)
		if mode == "Fix" {
			result = &models.ProgressExecutionResult{
				Success: false,
				Error:   fmt.Sprintf("Failed to commit and push changes: %v", err),
			}
			return err
		}
		// Continue模式不返回错误
	} else {
		xl.Infof("Changes committed and pushed successfully")
	}

	if err := pcm.UpdateTask(ctx, "commit-push", models.TaskStatusCompleted); err != nil {
		xl.Errorf("Failed to update task: %v", err)
	}
	
	// 12. 添加完成评论
	if err := pcm.UpdateTask(ctx, "add-comment", models.TaskStatusInProgress, "Adding completion comment to PR"); err != nil {
		xl.Errorf("Failed to update task: %v", err)
	}

	xl.Infof("Adding comment to PR using MCP tools")
	err = th.addPRCommentWithMCP(ctx, ws, pr, string(output))
	if err != nil {
		xl.Errorf("Failed to add comment via MCP: %v", err)
		// 不返回错误，因为这不是致命的
	} else {
		xl.Infof("Successfully added comment to PR via MCP")
	}

	if err := pcm.UpdateTask(ctx, "add-comment", models.TaskStatusCompleted); err != nil {
		xl.Errorf("Failed to update task: %v", err)
	}

	// 设置成功结果
	result = &models.ProgressExecutionResult{
		Success:        true,
		Summary:        fmt.Sprintf("Successfully %s PR #%d", strings.ToLower(mode), prNumber),
		BranchName:     ws.Branch,
		PullRequestURL: pr.GetHTMLURL(),
		FilesChanged:   []string{}, // TODO: 从git diff中提取文件列表
	}
	
	xl.Infof("PR %s processing completed successfully", strings.ToLower(mode))
	return nil
}

// processPRReviewCommand 处理PR Review命令
func (th *TagHandler) processPRReviewCommand(
	ctx context.Context,
	event *models.PullRequestReviewContext,
	cmdInfo *models.CommandInfo,
	aiModel string,
	mode string,
) error {
	xl := xlog.NewWith(ctx)
	xl.Infof("Processing PR review %s command - not fully implemented yet", strings.ToLower(mode))
	
	// 这里可以扩展为完整的PR Review处理逻辑
	// 暂时返回成功，避免错误
	return nil
}

// processPRReviewCommentCommand 处理PR Review Comment命令
func (th *TagHandler) processPRReviewCommentCommand(
	ctx context.Context,
	event *models.PullRequestReviewCommentContext,
	cmdInfo *models.CommandInfo,
	aiModel string,
	mode string,
) error {
	xl := xlog.NewWith(ctx)
	xl.Infof("Processing PR review comment %s command - not fully implemented yet", strings.ToLower(mode))
	
	// 这里可以扩展为完整的PR Review Comment处理逻辑
	// 暂时返回成功，避免错误
	return nil
}

// buildPrompt 构建不同模式的prompt
func (th *TagHandler) buildPrompt(mode string, args string, historicalContext string) string {
	var prompt string
	var taskDescription string
	var defaultTask string
	
	switch mode {
	case "Continue":
		taskDescription = "请根据上述PR描述、历史讨论和当前指令，进行相应的代码修改。"
		defaultTask = "继续处理PR，分析代码变更并改进"
	case "Fix":
		taskDescription = "请根据上述PR描述、历史讨论和当前指令，进行相应的代码修复。"
		defaultTask = "分析并修复代码问题"
	default:
		taskDescription = "请根据上述PR描述、历史讨论和当前指令，进行相应的代码处理。"
		defaultTask = "处理代码任务"
	}
	
	if args != "" {
		if historicalContext != "" {
			prompt = fmt.Sprintf(`作为PR代码审查助手，请基于以下完整上下文来%s：

%s

## 当前指令
%s

%s注意：
1. 当前指令是主要任务，历史信息仅作为上下文参考
2. 请确保修改符合PR的整体目标和已有的讨论共识
3. 如果发现与历史讨论有冲突，请优先执行当前指令并在回复中说明`,
				strings.ToLower(mode), historicalContext, args, taskDescription)
		} else {
			prompt = fmt.Sprintf("根据指令%s：\n\n%s", strings.ToLower(mode), args)
		}
	} else {
		if historicalContext != "" {
			prompt = fmt.Sprintf(`作为PR代码审查助手，请基于以下完整上下文来%s：

%s

%s`, strings.ToLower(mode), historicalContext, taskDescription)
		} else {
			prompt = fmt.Sprintf("%s", defaultTask)
		}
	}
	
	return prompt
}

// formatHistoricalComments 格式化历史评论
func (th *TagHandler) formatHistoricalComments(allComments *models.PRAllComments, currentCommentID int64) string {
	if allComments == nil {
		return ""
	}
	
	var contextParts []string
	
	// 添加PR描述
	if allComments.PRBody != "" {
		contextParts = append(contextParts, "## PR描述\n"+allComments.PRBody)
	}
	
	// 添加Issue评论
	if len(allComments.IssueComments) > 0 {
		contextParts = append(contextParts, "## PR讨论")
		for _, comment := range allComments.IssueComments {
			if comment.GetID() != currentCommentID {
				contextParts = append(contextParts, fmt.Sprintf("**%s**: %s", 
					comment.User.GetLogin(), comment.GetBody()))
			}
		}
	}
	
	// 添加Review评论
	if len(allComments.ReviewComments) > 0 {
		contextParts = append(contextParts, "## 代码审查评论")
		for _, comment := range allComments.ReviewComments {
			contextParts = append(contextParts, fmt.Sprintf("**%s** (文件: %s): %s", 
				comment.User.GetLogin(), comment.GetPath(), comment.GetBody()))
		}
	}
	
	return strings.Join(contextParts, "\n\n")
}

// addPRCommentWithMCP 使用MCP工具添加PR评论
func (th *TagHandler) addPRCommentWithMCP(ctx context.Context, ws *models.Workspace, pr *github.PullRequest, comment string) error {
	xl := xlog.NewWith(ctx)
	
	// 创建MCP上下文
	mcpCtx := &models.MCPContext{
		Repository: &models.IssueCommentContext{
			BaseContext: models.BaseContext{
				Repository: &github.Repository{
					Name:     github.String(ws.Repo),
					FullName: github.String(ws.Org + "/" + ws.Repo),
					Owner: &github.User{
						Login: github.String(ws.Org),
					},
				},
			},
		},
		Permissions: []string{"github:read", "github:write"},
		Constraints: []string{},
	}
	
	// 使用MCP工具添加评论
	commentCall := &models.ToolCall{
		ID: "comment_pr_" + fmt.Sprintf("%d", pr.GetNumber()),
		Function: models.ToolFunction{
			Name: "github-comments_create_comment",
			Arguments: map[string]interface{}{
				"issue_number": pr.GetNumber(),
				"body":         comment,
			},
		},
	}
	
	_, err := th.mcpClient.ExecuteToolCalls(ctx, []*models.ToolCall{commentCall}, mcpCtx)
	if err != nil {
		xl.Errorf("Failed to add comment via MCP: %v", err)
		return err
	}
	
	xl.Infof("Successfully added comment to PR via MCP")
	return nil
}