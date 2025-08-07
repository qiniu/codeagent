package modes

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/qiniu/codeagent/internal/code"
	ctxsys "github.com/qiniu/codeagent/internal/context"
	ghclient "github.com/qiniu/codeagent/internal/github"
	"github.com/qiniu/codeagent/internal/interaction"
	"github.com/qiniu/codeagent/internal/mcp"
	"github.com/qiniu/codeagent/internal/workspace"
	"github.com/qiniu/codeagent/pkg/models"

	"github.com/google/go-github/v58/github"
	"github.com/qiniu/x/xlog"
)

// TagHandler Tag mode handler
// Handles GitHub events containing commands (/code, /continue, /fix)
type TagHandler struct {
	*BaseHandler
	defaultAIModel string
	github         *ghclient.Client
	workspace      *workspace.Manager
	mcpClient      mcp.MCPClient
	sessionManager *code.SessionManager
	contextManager *ctxsys.ContextManager
}

// NewTagHandler creates a Tag mode handler
func NewTagHandler(defaultAIModel string, github *ghclient.Client, workspace *workspace.Manager, mcpClient mcp.MCPClient, sessionManager *code.SessionManager) *TagHandler {
	// Create context manager
	collector := ctxsys.NewDefaultContextCollector(github)
	formatter := ctxsys.NewDefaultContextFormatter(50000) // 50k tokens limit
	generator := ctxsys.NewDefaultPromptGenerator(formatter)
	contextManager := &ctxsys.ContextManager{
		Collector: collector,
		Formatter: formatter,
		Generator: generator,
	}

	return &TagHandler{
		BaseHandler: NewBaseHandler(
			TagMode,
			10, // Medium priority
			"Handle @codeagent mentions and commands (/code, /continue, /fix)",
		),
		defaultAIModel: defaultAIModel,
		github:         github,
		workspace:      workspace,
		mcpClient:      mcpClient,
		sessionManager: sessionManager,
		contextManager: contextManager,
	}
}

// CanHandle checks if the given event can be handled
func (th *TagHandler) CanHandle(ctx context.Context, event models.GitHubContext) bool {
	xl := xlog.NewWith(ctx)

	// Check if event contains commands
	cmdInfo, hasCmd := models.HasCommand(event)
	if !hasCmd {
		xl.Debugf("No command found in event type: %s", event.GetEventType())
		return false
	}

	xl.Infof("Found command: %s with AI model: %s in event type: %s", cmdInfo.Command, cmdInfo.AIModel, event.GetEventType())

	// Tag mode only handles events containing commands
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

// Execute executes Tag mode processing logic
func (th *TagHandler) Execute(ctx context.Context, event models.GitHubContext) error {
	xl := xlog.NewWith(ctx)
	xl.Infof("TagHandler executing for event type: %s", event.GetEventType())

	// Extract command information
	cmdInfo, hasCmd := models.HasCommand(event)
	if !hasCmd {
		return fmt.Errorf("no command found in event")
	}

	// If user didn't specify AI model, use system default configuration
	if strings.TrimSpace(cmdInfo.AIModel) == "" {
		cmdInfo.AIModel = th.defaultAIModel
	}

	xl.Infof("Executing command: %s with AI model: %s, args: %s",
		cmdInfo.Command, cmdInfo.AIModel, cmdInfo.Args)

	// Dispatch processing based on event type and command type
	switch event.GetEventType() {
	case models.EventIssueComment:
		return th.handleIssueComment(ctx, event.(*models.IssueCommentContext), cmdInfo)
	case models.EventPullRequestReview:
		return th.handlePRReview(ctx, event.(*models.PullRequestReviewContext), cmdInfo)
	case models.EventPullRequestReviewComment:
		return th.handlePRReviewComment(ctx, event.(*models.PullRequestReviewCommentContext), cmdInfo)
	default:
		return fmt.Errorf("unsupported event type for TagHandler: %s", event.GetEventType())
	}
}

// handleIssueComment handles Issue comment events
func (th *TagHandler) handleIssueComment(
	ctx context.Context,
	event *models.IssueCommentContext,
	cmdInfo *models.CommandInfo,
) error {
	// Convert event to original GitHub event type (compatible with existing agent interface)
	issueCommentEvent := event.RawEvent.(*github.IssueCommentEvent)

	// Handle comments in created and edited status, allowing users to modify commands
	action := issueCommentEvent.GetAction()
	if action != "created" && action != "edited" {
		return nil
	}

	if event.IsPRComment {
		switch cmdInfo.Command {
		case models.CommandContinue:
			return th.processPRCommand(ctx, event, cmdInfo, "Continue")
		case models.CommandFix:
			return th.processPRCommand(ctx, event, cmdInfo, "Fix")
		default:
			return fmt.Errorf("unsupported command for PR comment: %s", cmdInfo.Command)
		}
	} else {
		switch cmdInfo.Command {
		case models.CommandCode:
			return th.processIssueCodeCommand(ctx, event, cmdInfo)
		default:
			return fmt.Errorf("unsupported command for Issue comment: %s", cmdInfo.Command)
		}
	}
}

// handlePRReview handles PR Review events
func (th *TagHandler) handlePRReview(
	ctx context.Context,
	event *models.PullRequestReviewContext,
	cmdInfo *models.CommandInfo,
) error {
	xl := xlog.NewWith(ctx)
	xl.Infof("Processing PR review with command: %s", cmdInfo.Command)

	// Convert event to original GitHub event type
	reviewEvent := event.RawEvent.(*github.PullRequestReviewEvent)

	// Only handle reviews in 'submitted' status, ignore 'edited' and other statuses
	if reviewEvent.GetAction() != "submitted" {
		xl.Infof("Skipping PR review event with action: %s (only processing 'submitted')", reviewEvent.GetAction())
		return nil
	}

	// PR Review supports batch command processing
	switch cmdInfo.Command {
	case models.CommandContinue:
		// Implement PR Review continue logic, integrating original Agent functionality
		xl.Infof("Processing PR review continue with new architecture")
		return th.processPRReviewCommand(ctx, event, cmdInfo, "Continue")
	case models.CommandFix:
		// Implement PR Review fix logic, integrating original Agent functionality
		xl.Infof("Processing PR review fix with new architecture")
		return th.processPRReviewCommand(ctx, event, cmdInfo, "Fix")
	default:
		return fmt.Errorf("unsupported command for PR review: %s", cmdInfo.Command)
	}
}

// handlePRReviewComment handles PR Review comment events
func (th *TagHandler) handlePRReviewComment(
	ctx context.Context,
	event *models.PullRequestReviewCommentContext,
	cmdInfo *models.CommandInfo,
) error {
	xl := xlog.NewWith(ctx)
	xl.Infof("Processing PR review comment with command: %s", cmdInfo.Command)

	// Convert event to original GitHub event type
	reviewCommentEvent := event.RawEvent.(*github.PullRequestReviewCommentEvent)

	// Handle review comments in created and edited status, allowing users to modify commands
	action := reviewCommentEvent.GetAction()
	if action != "created" && action != "edited" {
		xl.Infof("Skipping PR review comment event with action: %s (only processing 'created' and 'edited')", action)
		return nil
	}

	// PR Review评论支持行级命令
	switch cmdInfo.Command {
	case models.CommandContinue:
		// 实现PR Review评论继续逻辑，集成原姻 Agent功能
		xl.Infof("Processing PR review comment continue with new architecture")
		return th.processPRReviewCommentCommand(ctx, event, cmdInfo, "Continue")
	case models.CommandFix:
		// 实现PR Review评论修复逻辑，集成原姻Agent功能
		xl.Infof("Processing PR review comment fix with new architecture")
		return th.processPRReviewCommentCommand(ctx, event, cmdInfo, "Fix")
	default:
		return fmt.Errorf("unsupported command for PR review comment: %s", cmdInfo.Command)
	}
}

// buildEnhancedIssuePrompt 为Issue中的/code命令构建增强提示词
func (th *TagHandler) buildEnhancedIssuePrompt(ctx context.Context, event *models.IssueCommentContext, args string) (string, error) {
	xl := xlog.NewWith(ctx)

	// 收集Issue的完整上下文
	issue := event.Issue
	repo := event.Repository
	repoFullName := repo.GetFullName()

	// 创建增强上下文
	ctxType := ctxsys.ContextTypeIssue
	enhancedCtx := &ctxsys.EnhancedContext{
		Type:      ctxType,
		Priority:  ctxsys.PriorityHigh,
		Timestamp: time.Now(),
		Subject:   event,
		Metadata: map[string]interface{}{
			"issue_number": issue.GetNumber(),
			"issue_title":  issue.GetTitle(),
			"issue_body":   issue.GetBody(),
			"repository":   repoFullName,
			"sender":       event.Sender.GetLogin(),
		},
	}

	// 收集Issue的评论上下文
	issueNumber := issue.GetNumber()
	owner, repoName := th.extractRepoInfo(repoFullName)

	// 获取Issue的所有评论
	comments, _, err := th.github.GetClient().Issues.ListComments(ctx, owner, repoName, issueNumber, &github.IssueListCommentsOptions{
		Sort:      github.String("created"),
		Direction: github.String("asc"),
	})
	if err != nil {
		xl.Warnf("Failed to get issue comments: %v", err)
	} else {
		// 转换评论格式
		for _, comment := range comments {
			if comment.GetID() != event.Comment.GetID() { // 排除当前评论
				enhancedCtx.Comments = append(enhancedCtx.Comments, ctxsys.CommentContext{
					ID:        comment.GetID(),
					Type:      "comment",
					Author:    comment.GetUser().GetLogin(),
					Body:      comment.GetBody(),
					CreatedAt: comment.GetCreatedAt().Time,
					UpdatedAt: comment.GetUpdatedAt().Time,
				})
			}
		}
	}

	// 使用增强的提示词生成器
	prompt, err := th.contextManager.Generator.GeneratePrompt(enhancedCtx, "Code", args)
	if err != nil {
		return "", fmt.Errorf("failed to generate enhanced prompt: %w", err)
	}

	return prompt, nil
}

// processIssueCodeCommand 处理Issue的/code命令，现在使用增强上下文系统
func (th *TagHandler) processIssueCodeCommand(
	ctx context.Context,
	event *models.IssueCommentContext,
	cmdInfo *models.CommandInfo,
) error {
	xl := xlog.NewWith(ctx)

	issueNumber := event.Issue.GetNumber()
	issueTitle := event.Issue.GetTitle()

	xl.Infof("Starting issue code processing: issue=#%d, title=%s, AI model=%s",
		issueNumber, issueTitle, cmdInfo.AIModel)

	// 执行Issue代码处理流程
	return th.executeIssueCodeProcessing(ctx, event, cmdInfo)
}

// executeIssueCodeProcessing 执行Issue代码处理的主要流程
func (th *TagHandler) executeIssueCodeProcessing(
	ctx context.Context,
	event *models.IssueCommentContext,
	cmdInfo *models.CommandInfo,
) error {
	xl := xlog.NewWith(ctx)

	var ws *models.Workspace
	var pr *github.PullRequest
	var pcm *interaction.ProgressCommentManager
	var result *models.ProgressExecutionResult

	// 使用defer确保最终状态更新
	defer func() {
		th.finalizeIssueProcessing(ctx, ws, pr, pcm, result)
	}()

	// 1. 设置工作空间和分支
	ws, err := th.setupWorkspaceAndBranch(ctx, event, cmdInfo.AIModel)
	if err != nil {
		result = &models.ProgressExecutionResult{
			Success: false,
			Error:   fmt.Sprintf("Failed to setup workspace: %v", err),
		}
		return err
	}

	// 2. 创建PR和初始化进度跟踪
	pr, pcm, err = th.createPRAndInitializeProgress(ctx, ws, event)
	if err != nil {
		result = &models.ProgressExecutionResult{
			Success: false,
			Error:   fmt.Sprintf("Failed to create PR: %v", err),
		}
		return err
	}

	// 3. 生成代码实现
	codeOutput, err := th.generateCodeImplementation(ctx, event, cmdInfo, ws, pcm)
	if err != nil {
		result = &models.ProgressExecutionResult{
			Success: false,
			Error:   fmt.Sprintf("Failed to generate code: %v", err),
		}
		return err
	}

	// 4. 提交并推送代码变更
	err = th.commitAndPushChanges(ctx, ws, codeOutput, pcm)
	if err != nil {
		result = &models.ProgressExecutionResult{
			Success: false,
			Error:   fmt.Sprintf("Failed to commit changes: %v", err),
		}
		return err
	}

	// 5. 更新PR描述
	err = th.updatePRDescription(ctx, ws, pr, event, codeOutput, pcm)
	if err != nil {
		xl.Errorf("Failed to update PR description: %v", err)
		// 不返回错误，因为代码已经提交成功
	}

	// 设置成功结果
	summary, _, _ := th.parseStructuredOutput(string(codeOutput))
	result = &models.ProgressExecutionResult{
		Success:        true,
		Summary:        summary,
		BranchName:     ws.Branch,
		PullRequestURL: pr.GetHTMLURL(),
		FilesChanged:   []string{}, // TODO: 从git diff中提取文件列表
	}

	xl.Infof("Issue code processing completed successfully")
	return nil
}

// setupWorkspaceAndBranch 设置工作空间和创建分支
func (th *TagHandler) setupWorkspaceAndBranch(
	ctx context.Context,
	event *models.IssueCommentContext,
	aiModel string,
) (*models.Workspace, error) {
	xl := xlog.NewWith(ctx)

	// 创建Issue工作空间，包含AI模型信息
	ws := th.workspace.CreateWorkspaceFromIssueWithAI(event.Issue, aiModel)
	if ws == nil {
		return nil, fmt.Errorf("failed to create workspace from issue")
	}
	xl.Infof("Created workspace: %s", ws.Path)

	// 创建分支并推送
	xl.Infof("Creating branch: %s", ws.Branch)
	if err := th.github.CreateBranch(ws); err != nil {
		return nil, fmt.Errorf("failed to create branch: %w", err)
	}
	xl.Infof("Branch created successfully")

	return ws, nil
}

// createPRAndInitializeProgress 创建PR并初始化进度跟踪
func (th *TagHandler) createPRAndInitializeProgress(
	ctx context.Context,
	ws *models.Workspace,
	event *models.IssueCommentContext,
) (*github.PullRequest, *interaction.ProgressCommentManager, error) {
	xl := xlog.NewWith(ctx)

	// 创建初始PR（在代码生成之前）
	xl.Infof("Creating initial PR before code generation")
	pr, err := th.github.CreatePullRequest(ws)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create PR: %w", err)
	}
	xl.Infof("PR created successfully: #%d", pr.GetNumber())

	// 移动工作空间从Issue到PR
	if err := th.workspace.MoveIssueToPR(ws, pr.GetNumber()); err != nil {
		xl.Errorf("Failed to move workspace: %v", err)
	}
	ws.PRNumber = pr.GetNumber()

	// 创建session目录
	if err := th.setupSessionDirectory(ctx, ws); err != nil {
		xl.Errorf("Failed to setup session directory: %v", err)
		// 不返回错误，继续执行
	}

	// 注册工作空间到PR映射
	ws.PullRequest = pr
	th.workspace.RegisterWorkspace(ws, pr)

	xl.Infof("Workspace registered: issue=#%d, workspace=%s, session=%s",
		event.Issue.GetNumber(), ws.Path, ws.SessionPath)

	// 初始化进度管理
	pcm, err := th.initializeProgressTracking(ctx, pr, event)
	if err != nil {
		xl.Errorf("Failed to initialize progress tracking: %v", err)
		// 继续执行，不因为评论失败而中断主流程
	}

	return pr, pcm, nil
}

// setupSessionDirectory 设置会话目录
func (th *TagHandler) setupSessionDirectory(ctx context.Context, ws *models.Workspace) error {
	xl := xlog.NewWith(ctx)

	prDirName := filepath.Base(ws.Path)
	suffix := th.workspace.ExtractSuffixFromPRDir(ws.AIModel, ws.Repo, ws.PRNumber, prDirName)

	sessionPath, err := th.workspace.CreateSessionPath(filepath.Dir(ws.Path), ws.AIModel, ws.Repo, ws.PRNumber, suffix)
	if err != nil {
		return fmt.Errorf("failed to create session directory: %w", err)
	}

	ws.SessionPath = sessionPath
	xl.Infof("Session directory created: %s", sessionPath)
	return nil
}

// initializeProgressTracking 初始化进度跟踪
func (th *TagHandler) initializeProgressTracking(
	ctx context.Context,
	pr *github.PullRequest,
	event *models.IssueCommentContext,
) (*interaction.ProgressCommentManager, error) {
	xl := xlog.NewWith(ctx)

	xl.Infof("Initializing progress tracking in PR #%d", pr.GetNumber())

	// 创建PR进度评论管理器
	pcm := interaction.NewProgressCommentManager(th.github, event.GetRepository(), pr.GetNumber())

	// 定义PR中的任务列表
	tasks := []*models.Task{
		{Name: models.TaskNameGenerateCode, Description: "🤖 Generate code implementation", Status: models.TaskStatusPending},
		{Name: models.TaskNameCommitChanges, Description: "💾 Commit and push changes", Status: models.TaskStatusPending},
		{Name: models.TaskNameUpdatePR, Description: "📝 Update PR description", Status: models.TaskStatusPending},
	}

	// 在PR中初始化进度
	if err := pcm.InitializeProgress(ctx, tasks); err != nil {
		return nil, fmt.Errorf("failed to initialize progress in PR: %w", err)
	}

	return pcm, nil
}

// generateCodeImplementation 生成代码实现
func (th *TagHandler) generateCodeImplementation(
	ctx context.Context,
	event *models.IssueCommentContext,
	cmdInfo *models.CommandInfo,
	ws *models.Workspace,
	pcm *interaction.ProgressCommentManager,
) ([]byte, error) {
	xl := xlog.NewWith(ctx)

	// 更新任务状态
	if err := pcm.UpdateTask(ctx, models.TaskNameGenerateCode, models.TaskStatusInProgress, "Calling AI to generate code implementation"); err != nil {
		xl.Errorf("Failed to update task: %v", err)
	}

	// 初始化code client
	xl.Infof("Initializing code client")
	codeClient, err := th.sessionManager.GetSession(ws)
	if err != nil {
		return nil, fmt.Errorf("failed to get code client: %w", err)
	}
	xl.Infof("Code client initialized successfully")

	// 构建提示词
	codePrompt, err := th.buildCodePrompt(ctx, event, cmdInfo)
	if err != nil {
		xl.Warnf("Failed to build enhanced prompt, falling back to simple prompt: %v", err)
		codePrompt = th.buildFallbackPrompt(event)
	}

	// 执行代码生成
	xl.Infof("Executing code modification with AI")
	if err := pcm.ShowSpinner(ctx, "AI is analyzing and generating code..."); err != nil {
		xl.Errorf("Failed to show spinner: %v", err)
	}

	codeResp, err := th.promptWithRetry(ctx, codeClient, codePrompt, 3)
	if err != nil {
		return nil, fmt.Errorf("failed to prompt for code modification: %w", err)
	}

	codeOutput, err := io.ReadAll(codeResp.Out)
	if err != nil {
		return nil, fmt.Errorf("failed to read code modification output: %w", err)
	}

	if err := pcm.HideSpinner(ctx); err != nil {
		xl.Errorf("Failed to hide spinner: %v", err)
	}

	xl.Infof("Code modification completed, output length: %d", len(codeOutput))
	xl.Debugf("LLM Output: %s", string(codeOutput))

	if err := pcm.UpdateTask(ctx, models.TaskNameGenerateCode, models.TaskStatusCompleted); err != nil {
		xl.Errorf("Failed to update task: %v", err)
	}

	return codeOutput, nil
}

// buildCodePrompt 构建代码生成提示词
func (th *TagHandler) buildCodePrompt(
	ctx context.Context,
	event *models.IssueCommentContext,
	cmdInfo *models.CommandInfo,
) (string, error) {
	return th.buildEnhancedIssuePrompt(ctx, event, cmdInfo.Args)
}

// buildFallbackPrompt 构建备用提示词
func (th *TagHandler) buildFallbackPrompt(event *models.IssueCommentContext) string {
	return fmt.Sprintf(`根据Issue修改代码：

标题：%s
描述：%s

输出格式：
%s
简要说明改动内容

%s
- 列出修改的文件和具体变动`,
		event.Issue.GetTitle(), event.Issue.GetBody(), models.SectionSummary, models.SectionChanges)
}

// commitAndPushChanges 提交并推送代码变更
func (th *TagHandler) commitAndPushChanges(
	ctx context.Context,
	ws *models.Workspace,
	codeOutput []byte,
	pcm *interaction.ProgressCommentManager,
) error {
	xl := xlog.NewWith(ctx)

	// 更新任务状态
	if err := pcm.UpdateTask(ctx, models.TaskNameCommitChanges, models.TaskStatusInProgress, "Committing and pushing code changes to repository"); err != nil {
		xl.Errorf("Failed to update task: %v", err)
	}

	// 准备执行结果
	executionResult := &models.ExecutionResult{
		Output: string(codeOutput),
		Error:  "",
	}

	// 初始化code client用于提交
	codeClient, err := th.sessionManager.GetSession(ws)
	if err != nil {
		return fmt.Errorf("failed to get code client for commit: %w", err)
	}

	// 提交并推送
	_, err = th.github.CommitAndPush(ws, executionResult, codeClient)
	if err != nil {
		return fmt.Errorf("failed to commit and push changes: %w", err)
	}
	xl.Infof("Code changes committed and pushed successfully")

	if err := pcm.UpdateTask(ctx, models.TaskNameCommitChanges, models.TaskStatusCompleted); err != nil {
		xl.Errorf("Failed to update task: %v", err)
	}

	return nil
}

// updatePRDescription 更新PR描述
func (th *TagHandler) updatePRDescription(
	ctx context.Context,
	ws *models.Workspace,
	pr *github.PullRequest,
	event *models.IssueCommentContext,
	codeOutput []byte,
	pcm *interaction.ProgressCommentManager,
) error {
	xl := xlog.NewWith(ctx)

	// 更新任务状态
	if err := pcm.UpdateTask(ctx, models.TaskNameUpdatePR, models.TaskStatusInProgress, "Updating PR description with implementation details"); err != nil {
		xl.Errorf("Failed to update task: %v", err)
	}

	// 组织结构化PR Body
	xl.Infof("Formatting PR description with elegant style")
	summary, changes, testPlan := th.parseStructuredOutput(string(codeOutput))

	// 使用新的PR格式化器创建优雅描述
	prFormatter := ctxsys.NewPRFormatter()
	prBody := prFormatter.FormatPRDescription(
		event.Issue.GetTitle(),
		event.Issue.GetBody(),
		summary,
		changes,
		testPlan,
		string(codeOutput),
		event.Issue.GetNumber(),
	)

	// 使用MCP工具更新PR描述
	xl.Infof("Updating PR description with MCP tools")
	err := th.updatePRWithMCP(ctx, ws, pr, prBody, string(codeOutput))
	if err != nil {
		xl.Errorf("Failed to update PR with MCP: %v", err)
		return err
	}

	xl.Infof("Successfully updated PR description via MCP")

	if err := pcm.UpdateTask(ctx, models.TaskNameUpdatePR, models.TaskStatusCompleted); err != nil {
		xl.Errorf("Failed to update task: %v", err)
	}

	return nil
}

// finalizeIssueProcessing 完成Issue处理的状态更新
func (th *TagHandler) finalizeIssueProcessing(
	ctx context.Context,
	ws *models.Workspace,
	pr *github.PullRequest,
	pcm *interaction.ProgressCommentManager,
	result *models.ProgressExecutionResult,
) {
	xl := xlog.NewWith(ctx)

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

	if pcm != nil {
		if err := pcm.FinalizeComment(ctx, result); err != nil {
			xl.Errorf("Failed to finalize progress comment: %v", err)
		}
	}
}

// promptWithRetry 带重试的提示执行
func (th *TagHandler) promptWithRetry(ctx context.Context, codeClient code.Code, prompt string, maxRetries int) (*code.Response, error) {
	return code.PromptWithRetry(ctx, codeClient, prompt, maxRetries)
}

// parseStructuredOutput 解析结构化输出
func (th *TagHandler) parseStructuredOutput(output string) (summary, changes, testPlan string) {
	return code.ParseStructuredOutput(output)
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

// processPRCommand 处理PR的通用命令（continue/fix），简化版本
func (th *TagHandler) processPRCommand(
	ctx context.Context,
	event *models.IssueCommentContext,
	cmdInfo *models.CommandInfo,
	mode string,
) error {
	xl := xlog.NewWith(ctx)

	prNumber := event.Issue.GetNumber()
	xl.Infof("%s PR #%d with AI model %s and args: %s", mode, prNumber, cmdInfo.AIModel, cmdInfo.Args)

	// 1. 验证PR上下文
	if !event.IsPRComment {
		return fmt.Errorf("this is not a PR comment, cannot %s", strings.ToLower(mode))
	}

	// 2. 从事件中提取仓库信息（支持多种事件类型）
	var repoOwner, repoName string

	// 根据事件类型安全地提取仓库信息
	switch event.GetEventType() {
	case models.EventIssueComment:
		if rawEvent, ok := event.RawEvent.(*github.IssueCommentEvent); ok && rawEvent.Repo != nil {
			repoOwner = rawEvent.Repo.GetOwner().GetLogin()
			repoName = rawEvent.Repo.GetName()
		}
	case models.EventPullRequestReview:
		if rawEvent, ok := event.RawEvent.(*github.PullRequestReviewEvent); ok && rawEvent.Repo != nil {
			repoOwner = rawEvent.Repo.GetOwner().GetLogin()
			repoName = rawEvent.Repo.GetName()
		}
	case models.EventPullRequestReviewComment:
		if rawEvent, ok := event.RawEvent.(*github.PullRequestReviewCommentEvent); ok && rawEvent.Repo != nil {
			repoOwner = rawEvent.Repo.GetOwner().GetLogin()
			repoName = rawEvent.Repo.GetName()
		}
	default:
		// 尝试从Repository字段获取信息作为fallback
		if event.Repository != nil {
			if event.Repository.Owner != nil {
				repoOwner = event.Repository.GetOwner().GetLogin()
			}
			repoName = event.Repository.GetName()
		}
	}

	if repoOwner == "" || repoName == "" {
		xl.Errorf("Failed to extract repository info from event type: %s", event.GetEventType())
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

	// 4. 从PR分支中提取AI模型（Review场景不使用config默认值）
	branchName := pr.GetHead().GetRef()
	cmdInfo.AIModel = th.workspace.ExtractAIModelFromBranch(branchName)
	if cmdInfo.AIModel == "" {
		xl.Errorf("Failed to extract AI model from branch: %s", branchName)
		return fmt.Errorf("cannot extract AI model from branch name: %s", branchName)
	}
	xl.Infof("Extracted AI model from branch: %s", cmdInfo.AIModel)

	// 5. 设置工作空间
	xl.Infof("Getting or creating workspace for PR with AI model: %s", cmdInfo.AIModel)
	ws := th.workspace.GetOrCreateWorkspaceForPRWithAI(pr, cmdInfo.AIModel)
	if ws == nil {
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
		return fmt.Errorf("failed to create code session: %w", err)
	}
	xl.Infof("Code client initialized successfully")

	// 8. 使用增强上下文系统构建上下文和prompt
	xl.Infof("Building enhanced context for PR %s", strings.ToLower(mode))
	prompt, err := th.buildEnhancedPrompt(ctx, "issue_comment", event.RawEvent, pr, mode, cmdInfo.Args, ws.Path)
	if err != nil {
		xl.Warnf("Failed to build enhanced prompt, falling back to simple prompt: %v", err)
		// Fallback to original method
		allComments, err := th.github.GetAllPRComments(pr)
		if err != nil {
			xl.Warnf("Failed to get PR comments for context: %v", err)
			allComments = &models.PRAllComments{}
		}
		var currentCommentID int64
		var currentComment string
		if event.Comment != nil {
			currentCommentID = event.Comment.GetID()
			currentComment = event.Comment.GetBody()
		}
		historicalContext := th.formatHistoricalComments(allComments, currentCommentID)
		prompt = th.buildPromptWithCurrentComment(mode, cmdInfo.Args, historicalContext, currentComment)
	} else {
		xl.Infof("Successfully built enhanced prompt for PR %s", strings.ToLower(mode))
	}

	// 10. 执行AI处理
	xl.Infof("Executing AI processing for PR %s", strings.ToLower(mode))
	resp, err := th.promptWithRetry(ctx, codeClient, prompt, 3)
	if err != nil {
		return fmt.Errorf("failed to process PR %s: %w", strings.ToLower(mode), err)
	}

	output, err := io.ReadAll(resp.Out)
	if err != nil {
		return fmt.Errorf("failed to read output for PR %s: %w", strings.ToLower(mode), err)
	}

	xl.Infof("AI processing completed, output length: %d", len(output))
	xl.Debugf("PR %s Output: %s", mode, string(output))

	// 11. 提交变更
	executionResult := &models.ExecutionResult{
		Output: string(output),
		Error:  "",
	}

	xl.Infof("Committing and pushing changes for PR %s", strings.ToLower(mode))
	commitHash, err := th.github.CommitAndPush(ws, executionResult, codeClient)
	if err != nil {
		xl.Errorf("Failed to commit and push changes: %v", err)
		if mode == "Fix" {
			return err
		}
		// Continue模式不返回错误
	} else {
		xl.Infof("Changes committed and pushed successfully, commit hash: %s", commitHash)
	}

	// 12. 更新PR描述并添加完成评论
	xl.Infof("Updating PR description and adding completion comment")

	// 解析结构化输出用于PR描述
	summary, changes, testPlan := th.parseStructuredOutput(string(output))

	// 使用新的PR格式化器创建优雅描述
	prFormatter := ctxsys.NewPRFormatter()
	prBody := prFormatter.FormatPRDescription(
		pr.GetTitle(),
		pr.GetBody(),
		summary,
		changes,
		testPlan,
		string(output),
		pr.GetNumber(),
	)

	// 更新PR描述
	err = th.updatePRWithMCP(ctx, ws, pr, prBody, string(output))
	if err != nil {
		xl.Errorf("Failed to update PR description via MCP: %v", err)
		// 不返回错误，因为这不是致命的
	} else {
		xl.Infof("Successfully updated PR description via MCP")
	}

	// 添加简洁的完成评论
	var commentBody string
	if commitHash != "" {
		// 使用commit URL
		commitURL := fmt.Sprintf("%s/commits/%s", pr.GetHTMLURL(), commitHash)
		if event.Comment != nil && event.Comment.User != nil {
			commentBody = fmt.Sprintf("@%s 已根据指令完成处理 ✅\n\n**查看代码变更**: %s",
				event.Comment.User.GetLogin(), commitURL)
		} else {
			commentBody = fmt.Sprintf("✅ 处理完成！\n\n**查看代码变更**: %s", commitURL)
		}
	} else {
		// 如果没有commit hash，使用PR URL作为fallback
		if event.Comment != nil && event.Comment.User != nil {
			commentBody = fmt.Sprintf("@%s 已根据指令完成处理 ✅\n\n**查看详情**: %s",
				event.Comment.User.GetLogin(), pr.GetHTMLURL())
		} else {
			commentBody = fmt.Sprintf("✅ 处理完成！\n\n**查看详情**: %s", pr.GetHTMLURL())
		}
	}

	err = th.addPRCommentWithMCP(ctx, ws, pr, commentBody)
	if err != nil {
		xl.Errorf("Failed to add completion comment via MCP: %v", err)
		// 不返回错误，因为这不是致命的
	} else {
		xl.Infof("Successfully added completion comment to PR via MCP")
	}

	xl.Infof("PR %s processing completed successfully", strings.ToLower(mode))
	return nil
}

// processPRReviewCommand 处理PR Review命令
// Submit review批量评论场景：需要本次review的所有comments
func (th *TagHandler) processPRReviewCommand(
	ctx context.Context,
	event *models.PullRequestReviewContext,
	cmdInfo *models.CommandInfo,
	mode string,
) error {
	xl := xlog.NewWith(ctx)
	xl.Infof("Processing PR review %s command (Submit review batch comments)", strings.ToLower(mode))

	prNumber := event.PullRequest.GetNumber()
	reviewID := event.Review.GetID()
	xl.Infof("Processing PR #%d from review %d with command: %s, AI model: %s, args: %s", prNumber, reviewID, mode, cmdInfo.AIModel, cmdInfo.Args)

	// 1. 从工作空间管理器获取 PR 信息
	pr := event.PullRequest

	// 2. 从PR分支中提取AI模型（Review场景不使用config默认值）
	branchName := pr.GetHead().GetRef()
	cmdInfo.AIModel = th.workspace.ExtractAIModelFromBranch(branchName)
	if cmdInfo.AIModel == "" {
		xl.Errorf("Failed to extract AI model from branch: %s", branchName)
		return fmt.Errorf("cannot extract AI model from branch name: %s", branchName)
	}
	xl.Infof("Extracted AI model from branch: %s", cmdInfo.AIModel)

	// 3. 获取指定 review 的所有 comments（只获取本次review的评论）
	reviewComments, err := th.github.GetReviewComments(pr, reviewID)
	if err != nil {
		xl.Errorf("Failed to get review comments: %v", err)
		return err
	}

	xl.Infof("Found %d review comments for review %d", len(reviewComments), reviewID)

	// 4. 获取或创建 PR 工作空间，包含AI模型信息
	ws := th.workspace.GetOrCreateWorkspaceForPRWithAI(pr, cmdInfo.AIModel)
	if ws == nil {
		return fmt.Errorf("failed to get or create workspace for PR batch processing from review")
	}

	// 5. 拉取远端最新代码
	if err := th.github.PullLatestChanges(ws, pr); err != nil {
		xl.Errorf("Failed to pull latest changes: %v", err)
		// 不返回错误，继续执行，因为可能是网络问题
	}

	// 6. 初始化 code client
	codeClient, err := th.sessionManager.GetSession(ws)
	if err != nil {
		xl.Errorf("failed to get code client for PR batch processing from review: %v", err)
		return err
	}

	// 7. 构建批量处理的 prompt，包含所有 review comments 和位置信息
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
	if mode == "Continue" {
		if cmdInfo.Args != "" {
			prompt = fmt.Sprintf("请根据以下 PR Review 的批量评论和指令继续处理代码：\n\n%s\n\n指令：%s\n\n请一次性处理所有评论中提到的问题，回复要简洁明了。", allComments, cmdInfo.Args)
		} else {
			prompt = fmt.Sprintf("请根据以下 PR Review 的批量评论继续处理代码：\n\n%s\n\n请一次性处理所有评论中提到的问题，回复要简洁明了。", allComments)
		}
	} else { // Fix
		if cmdInfo.Args != "" {
			prompt = fmt.Sprintf("请根据以下 PR Review 的批量评论和指令修复代码问题：\n\n%s\n\n指令：%s\n\n请一次性修复所有评论中提到的问题，回复要简洁明了。", allComments, cmdInfo.Args)
		} else {
			prompt = fmt.Sprintf("请根据以下 PR Review 的批量评论修复代码问题：\n\n%s\n\n请一次性修复所有评论中提到的问题，回复要简洁明了。", allComments)
		}
	}

	// 8. 执行AI处理
	resp, err := th.promptWithRetry(ctx, codeClient, prompt, 3)
	if err != nil {
		xl.Errorf("Failed to prompt for PR batch processing from review: %v", err)
		return err
	}

	output, err := io.ReadAll(resp.Out)
	if err != nil {
		xl.Errorf("Failed to read output for PR batch processing from review: %v", err)
		return err
	}

	xl.Infof("PR Batch Processing from Review Output length: %d", len(output))
	xl.Debugf("PR Batch Processing from Review Output: %s", string(output))

	// 9. 提交变更并更新 PR
	executionResult := &models.ExecutionResult{
		Output: string(output),
	}
	commitHash, err := th.github.CommitAndPush(ws, executionResult, codeClient)
	if err != nil {
		xl.Errorf("Failed to commit and push for PR batch processing from review: %v", err)
		return err
	}
	xl.Infof("Successfully committed and pushed changes, commit hash: %s", commitHash)

	// 在PR review场景下，只需要添加完成评论，不更新PR描述
	xl.Infof("Processing review batch results - skipping PR description update for review comments")

	// 创建简洁的完成评论
	var triggerUser string
	if event.Review != nil && event.Review.User != nil {
		triggerUser = event.Review.User.GetLogin()
	}

	var commentBody string
	if commitHash != "" {
		// 使用commit URL
		commitURL := fmt.Sprintf("%s/commits/%s", pr.GetHTMLURL(), commitHash)
		if triggerUser != "" {
			if len(reviewComments) == 0 {
				commentBody = fmt.Sprintf("@%s ✅ 已根据review说明完成批量处理\n\n**查看代码变更**: %s", triggerUser, commitURL)
			} else {
				commentBody = fmt.Sprintf("@%s ✅ 已批量处理此次review的%d个评论\n\n**查看代码变更**: %s", triggerUser, len(reviewComments), commitURL)
			}
		} else {
			if len(reviewComments) == 0 {
				commentBody = fmt.Sprintf("✅ 已根据review说明完成批量处理\n\n**查看代码变更**: %s", commitURL)
			} else {
				commentBody = fmt.Sprintf("✅ 已批量处理此次review的%d个评论\n\n**查看代码变更**: %s", len(reviewComments), commitURL)
			}
		}
	} else {
		// 如果没有commit hash，使用PR URL作为fallback
		if triggerUser != "" {
			if len(reviewComments) == 0 {
				commentBody = fmt.Sprintf("@%s ✅ 已根据review说明完成批量处理\n\n**查看详情**: %s", triggerUser, pr.GetHTMLURL())
			} else {
				commentBody = fmt.Sprintf("@%s ✅ 已批量处理此次review的%d个评论\n\n**查看详情**: %s", triggerUser, len(reviewComments), pr.GetHTMLURL())
			}
		} else {
			if len(reviewComments) == 0 {
				commentBody = fmt.Sprintf("✅ 已根据review说明完成批量处理\n\n**查看详情**: %s", pr.GetHTMLURL())
			} else {
				commentBody = fmt.Sprintf("✅ 已批量处理此次review的%d个评论\n\n**查看详情**: %s", len(reviewComments), pr.GetHTMLURL())
			}
		}
	}

	err = th.addPRCommentWithMCP(ctx, ws, pr, commentBody)
	if err != nil {
		xl.Errorf("Failed to create PR comment for batch processing result via MCP: %v", err)
	} else {
		xl.Infof("Successfully created PR comment for batch processing result via MCP")
	}

	xl.Infof("Successfully processed PR #%d from review %d with %d comments", pr.GetNumber(), reviewID, len(reviewComments))
	return nil
}

// processPRReviewCommentCommand 处理PR Review Comment命令
// Files Changed页面的单行评论场景：只需要代码行上下文，不需要历史评论
func (th *TagHandler) processPRReviewCommentCommand(
	ctx context.Context,
	event *models.PullRequestReviewCommentContext,
	cmdInfo *models.CommandInfo,
	mode string,
) error {
	xl := xlog.NewWith(ctx)
	xl.Infof("Processing PR review comment %s command (Files Changed single line comment)", strings.ToLower(mode))

	prNumber := event.PullRequest.GetNumber()
	xl.Infof("%s PR #%d from review comment with AI model %s and args: %s", mode, prNumber, cmdInfo.AIModel, cmdInfo.Args)

	// 1. 从工作空间管理器获取 PR 信息
	pr := event.PullRequest

	// 2. 从PR分支中提取AI模型（Review场景不使用config默认值）
	branchName := pr.GetHead().GetRef()
	cmdInfo.AIModel = th.workspace.ExtractAIModelFromBranch(branchName)
	if cmdInfo.AIModel == "" {
		xl.Errorf("Failed to extract AI model from branch: %s", branchName)
		return fmt.Errorf("cannot extract AI model from branch name: %s", branchName)
	}
	xl.Infof("Extracted AI model from branch: %s", cmdInfo.AIModel)

	// 3. 获取或创建 PR 工作空间，包含AI模型信息
	ws := th.workspace.GetOrCreateWorkspaceForPRWithAI(pr, cmdInfo.AIModel)
	if ws == nil {
		return fmt.Errorf("failed to get or create workspace for PR %s from review comment", strings.ToLower(mode))
	}

	// 4. 拉取远端最新代码
	if err := th.github.PullLatestChanges(ws, pr); err != nil {
		xl.Errorf("Failed to pull latest changes: %v", err)
		// 不返回错误，继续执行，因为可能是网络问题
	}

	// 5. 初始化 code client
	codeClient, err := th.sessionManager.GetSession(ws)
	if err != nil {
		xl.Errorf("failed to get code client for PR %s from review comment: %v", strings.ToLower(mode), err)
		return err
	}

	// 6. 构建 prompt，只包含评论上下文和命令参数（不包含历史评论）
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

	if cmdInfo.Args != "" {
		if mode == "Continue" {
			prompt = fmt.Sprintf("根据代码行评论和指令继续处理：\n\n%s\n\n指令：%s", commentContext, cmdInfo.Args)
		} else {
			prompt = fmt.Sprintf("根据代码行评论和指令修复：\n\n%s\n\n指令：%s", commentContext, cmdInfo.Args)
		}
	} else {
		if mode == "Continue" {
			prompt = fmt.Sprintf("根据代码行评论继续处理：\n\n%s", commentContext)
		} else {
			prompt = fmt.Sprintf("根据代码行评论修复：\n\n%s", commentContext)
		}
	}

	// 7. 执行AI处理
	resp, err := th.promptWithRetry(ctx, codeClient, prompt, 3)
	if err != nil {
		xl.Errorf("Failed to prompt for PR %s from review comment: %v", strings.ToLower(mode), err)
		return err
	}

	output, err := io.ReadAll(resp.Out)
	if err != nil {
		xl.Errorf("Failed to read output for PR %s from review comment: %v", strings.ToLower(mode), err)
		return err
	}

	xl.Infof("PR %s from Review Comment Output length: %d", mode, len(output))
	xl.Debugf("PR %s from Review Comment Output: %s", mode, string(output))

	// 8. 提交变更并更新 PR
	executionResult := &models.ExecutionResult{
		Output: string(output),
	}
	commitHash, err := th.github.CommitAndPush(ws, executionResult, codeClient)
	if err != nil {
		xl.Errorf("Failed to commit and push for PR %s from review comment: %v", strings.ToLower(mode), err)
		return err
	}

	// 9. 回复原始评论
	// 解析结构化输出用于更优雅的回复
	summary, _, _ := th.parseStructuredOutput(string(output))

	// 创建简洁的回复，指向具体的commit
	var replyBody string
	commitURL := fmt.Sprintf("%s/commits/%s", pr.GetHTMLURL(), commitHash)
	if triggerUser := event.Comment.GetUser(); triggerUser != nil {
		replyBody = fmt.Sprintf("@%s ✅ 处理完成！\n\n**变更摘要**: %s\n\n[查看代码变更](%s)",
			triggerUser.GetLogin(),
			th.truncateText(summary, 100),
			commitURL)
	} else {
		replyBody = fmt.Sprintf("✅ 处理完成！\n\n**变更摘要**: %s\n\n[查看代码变更](%s)",
			th.truncateText(summary, 100),
			commitURL)
	}

	if err = th.github.ReplyToReviewComment(pr, event.Comment.GetID(), replyBody); err != nil {
		xl.Errorf("Failed to reply to review comment: %v", err)
		// 不返回错误，因为这不是致命的，代码修改已经提交成功
	} else {
		xl.Infof("Successfully replied to review comment")
	}

	xl.Infof("Successfully %s PR #%d from review comment", strings.ToLower(mode), pr.GetNumber())
	return nil
}

// buildPromptWithCurrentComment 构建不同模式的prompt，包含当前评论信息
func (th *TagHandler) buildPromptWithCurrentComment(mode string, args string, historicalContext string, currentComment string) string {
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

	// 构建当前评论的上下文信息
	var currentCommentContext string
	if currentComment != "" {
		// 从当前评论中提取command和args
		var commentCommand, commentArgs string
		if strings.HasPrefix(currentComment, "/continue") {
			commentCommand = "/continue"
			commentArgs = strings.TrimSpace(strings.TrimPrefix(currentComment, "/continue"))
		} else if strings.HasPrefix(currentComment, "/fix") {
			commentCommand = "/fix"
			commentArgs = strings.TrimSpace(strings.TrimPrefix(currentComment, "/fix"))
		}

		if commentArgs != "" {
			currentCommentContext = fmt.Sprintf("## 当前评论\n用户刚刚发出指令：%s %s", commentCommand, commentArgs)
		} else {
			currentCommentContext = fmt.Sprintf("## 当前评论\n用户刚刚发出指令：%s", commentCommand)
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

			prompt = fmt.Sprintf(`作为PR代码审查助手，请基于以下完整上下文来%s：

%s

## 执行指令
%s

%s注意：
1. 当前指令是主要任务，历史信息仅作为上下文参考
2. 请确保修改符合PR的整体目标和已有的讨论共识
3. 如果发现与历史讨论有冲突，请优先执行当前指令并在回复中说明`,
				strings.ToLower(mode), fullContext, args, taskDescription)
		} else {
			prompt = fmt.Sprintf("根据指令%s：\n\n%s", strings.ToLower(mode), args)
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

			prompt = fmt.Sprintf(`作为PR代码审查助手，请基于以下完整上下文来%s：

%s

## 任务
%s

请根据上述PR描述和历史讨论，进行相应的代码修改和改进。`,
				strings.ToLower(mode), fullContext, defaultTask)
		} else {
			prompt = defaultTask
		}
	}

	return prompt
}

// buildEnhancedPrompt 使用增强上下文系统构建上下文和prompt
func (th *TagHandler) buildEnhancedPrompt(
	ctx context.Context,
	eventType string,
	payload interface{},
	pr *github.PullRequest,
	mode string,
	args string,
	repoPath string,
) (string, error) {
	xl := xlog.NewWith(ctx)

	// 1. 收集基础上下文
	enhancedCtx, err := th.contextManager.Collector.CollectBasicContext(eventType, payload)
	if err != nil {
		return "", fmt.Errorf("failed to collect basic context: %w", err)
	}

	// 2. 收集代码上下文（如果是PR相关）
	if pr != nil {
		codeCtx, err := th.contextManager.Collector.CollectCodeContext(pr)
		if err != nil {
			xl.Warnf("Failed to collect code context: %v", err)
		} else {
			enhancedCtx.Code = codeCtx
		}

		// 收集评论上下文
		var currentCommentID int64
		if eventType == "issue_comment" {
			if issueEvent, ok := payload.(*github.IssueCommentEvent); ok && issueEvent.Comment != nil {
				currentCommentID = issueEvent.Comment.GetID()
			}
		} else if eventType == "pull_request_review_comment" {
			if reviewCommentEvent, ok := payload.(*github.PullRequestReviewCommentEvent); ok && reviewCommentEvent.Comment != nil {
				currentCommentID = reviewCommentEvent.Comment.GetID()
			}
		}

		comments, err := th.contextManager.Collector.CollectCommentContext(pr, currentCommentID)
		if err != nil {
			xl.Warnf("Failed to collect comment context: %v", err)
		} else {
			enhancedCtx.Comments = comments
		}
	}

	// 4. 使用增强的prompt生成器
	prompt, err := th.contextManager.Generator.GeneratePrompt(enhancedCtx, mode, args)
	if err != nil {
		return "", fmt.Errorf("failed to generate prompt: %w", err)
	}

	return prompt, nil
}

// formatHistoricalComments 格式化历史评论
func (th *TagHandler) formatHistoricalComments(allComments *models.PRAllComments, currentCommentID int64) string {
	return code.FormatHistoricalComments(allComments, currentCommentID)
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

// extractRepoInfo 从仓库全名中提取owner和repo名称
func (th *TagHandler) extractRepoInfo(repoFullName string) (owner, repo string) {
	parts := strings.Split(repoFullName, "/")
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

// truncateText 截断文本到指定长度
func (th *TagHandler) truncateText(text string, maxLength int) string {
	if len(text) <= maxLength {
		return text
	}

	// 避免在单词中间截断
	if maxLength > 3 {
		truncated := text[:maxLength-3]
		lastSpace := strings.LastIndex(truncated, " ")
		if lastSpace > 0 {
			truncated = truncated[:lastSpace]
		}
		return truncated + "..."
	}

	return text[:maxLength]
}
