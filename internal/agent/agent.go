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
	"github.com/qbox/codeagent/internal/prompt"
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
	promptBuilder  *prompt.Builder   // 新增
	validator      *prompt.Validator // 新增
}

func New(cfg *config.Config, workspaceManager *workspace.Manager) *Agent {
	// 初始化 GitHub 客户端
	githubClient, err := ghclient.NewClient(cfg)
	if err != nil {
		log.Errorf("Failed to create GitHub client: %v", err)
		return nil
	}

	// 初始化 Prompt 系统
	promptManager := prompt.NewManager(workspaceManager)
	customConfigDetector := prompt.NewDetector()
	promptConfig := prompt.PromptConfig{
		MaxTotalLength: 8000,
	}
	promptBuilder := prompt.NewBuilder(promptManager, customConfigDetector, promptConfig)

	a := &Agent{
		config:         cfg,
		github:         githubClient,
		workspace:      workspaceManager,
		sessionManager: code.NewSessionManager(cfg),
		promptBuilder:  promptBuilder,
		validator:      nil, // 延迟初始化，需要 code client
	}

	go a.StartCleanupRoutine()

	return a
}

// startCleanupRoutine 启动定期清理协程
func (a *Agent) StartCleanupRoutine() {
	ticker := time.NewTicker(1 * time.Hour) // 每小时检查一次
	defer ticker.Stop()

	for range ticker.C {
		a.cleanupExpiredResources()
	}
}

// cleanupExpiredResources 清理过期的工作空间
func (a *Agent) cleanupExpiredResources() {
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
		log.Infof("Cleaning up expired workspace: %s (AI model: %s, PR: %d)", ws.Path, ws.AIModel, ws.PRNumber)

		// 关闭 code session
		err := a.sessionManager.CloseSession(ws)
		if err != nil {
			log.Errorf("Failed to close session for workspace: %s (AI model: %s)", ws.Path, ws.AIModel)
		} else {
			log.Infof("Closed session for workspace: %s (AI model: %s)", ws.Path, ws.AIModel)
		}

		// 清理工作空间
		b := m.CleanupWorkspace(ws)
		if !b {
			log.Errorf("Failed to clean up expired workspace: %s (AI model: %s)", ws.Path, ws.AIModel)
			continue
		}
		log.Infof("Cleaned up expired workspace: %s (AI model: %s)", ws.Path, ws.AIModel)
	}

}

// ProcessIssueComment 处理 Issue 评论事件，包含完整的仓库信息
func (a *Agent) ProcessIssueComment(ctx context.Context, event *github.IssueCommentEvent) error {
	return a.ProcessIssueCommentWithAI(ctx, event, "", "")
}

// ProcessIssueCommentWithAI 处理 Issue 评论事件，支持指定AI模型
func (a *Agent) ProcessIssueCommentWithAI(ctx context.Context, event *github.IssueCommentEvent, aiModel, args string) error {
	log := xlog.NewWith(ctx)

	issueNumber := event.Issue.GetNumber()
	issueTitle := event.Issue.GetTitle()

	log.Infof("Starting issue comment processing: issue=#%d, title=%s, AI model=%s", issueNumber, issueTitle, aiModel)

	// 1. 创建 Issue 工作空间，包含AI模型信息
	ws := a.workspace.CreateWorkspaceFromIssueWithAI(event.Issue, aiModel)
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
	// 从PR目录名中提取suffix
	prDirName := filepath.Base(ws.Path)
	suffix := a.workspace.ExtractSuffixFromPRDir(ws.AIModel, ws.Repo, pr.GetNumber(), prDirName)

	sessionPath, err := a.workspace.CreateSessionPath(filepath.Dir(ws.Path), ws.AIModel, ws.Repo, pr.GetNumber(), suffix)
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
	code, err := a.sessionManager.GetSession(ws)
	if err != nil {
		log.Errorf("Failed to get code client: %v", err)
		return err
	}
	log.Infof("Code client initialized successfully")

	// 8. 执行代码修改
	// 构建 Prompt 请求
	req := &prompt.PromptRequest{
		TemplateID: "issue_based_code_generation",
		TemplateVars: map[string]interface{}{
			"issue_title":        event.Issue.GetTitle(),
			"issue_body":         event.Issue.GetBody(),
			"historical_context": "",
			"include_tests":      true,
			"include_docs":       true,
		},
		Workspace: ws,
	}

	prompt, err := a.promptBuilder.BuildPrompt(ctx, req)
	if err != nil {
		log.Errorf("Failed to build prompt: %v", err)
		return err
	}

	log.Infof("Executing code modification with AI")
	codeResp, err := a.promptWithRetry(ctx, code, prompt.Content, 3)
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

	log.Infof("Updating PR body")
	if err = a.github.UpdatePullRequest(pr, string(codeOutput)); err != nil {
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

	// 8. 构建包含历史上下文的 prompt
	var prompt string
	var currentCommentID int64
	if event.Comment != nil {
		currentCommentID = event.Comment.GetID()
	}
	historicalContext := a.formatHistoricalComments(allComments, currentCommentID)

	// 根据模式生成不同的 prompt
	prompt = a.buildPrompt(mode, args, historicalContext)

	log.Infof("Using %s prompt with args and historical context", strings.ToLower(mode))

	// 9. 执行 AI 处理
	log.Infof("Executing AI processing for PR %s", strings.ToLower(mode))
	resp, err := a.promptWithRetry(ctx, codeClient, prompt, 3)
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
	if err := a.github.CommitAndPush(ws, result, codeClient); err != nil {
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

// buildPrompt 构建不同模式的 prompt
func (a *Agent) buildPrompt(mode string, args string, historicalContext string) string {
	// 根据模式选择正确的模板
	var templateID string
	switch mode {
	case "Continue":
		templateID = "issue_based_code_generation"
	case "Fix":
		templateID = "review_based_code_modification"
	default:
		templateID = "issue_based_code_generation"
	}

	// 准备模板变量
	templateVars := map[string]interface{}{
		"issue_title":        "PR 处理请求",
		"issue_body":         args,
		"historical_context": historicalContext,
		"include_tests":      true,
		"include_docs":       true,
	}

	// 如果是修复模式，使用不同的变量结构
	if mode == "Fix" {
		templateVars = map[string]interface{}{
			"review_comments":    args,
			"historical_context": historicalContext,
		}
	}

	// 构建 Prompt 请求
	req := &prompt.PromptRequest{
		TemplateID:   templateID,
		TemplateVars: templateVars,
		Workspace:    nil, // 这里暂时传 nil，实际使用时应该传入工作空间
	}

	// 构建 Prompt
	result, err := a.promptBuilder.BuildPrompt(context.Background(), req)
	if err != nil {
		// 如果新系统失败，使用简化的回退模板
		log.Errorf("Failed to build prompt: %v", err)
		return a.buildFallbackPrompt(templateID, templateVars)
	}

	return result.Content
}

// buildSingleReviewPrompt 构建单个 Review Comment 的 Prompt
func (a *Agent) buildSingleReviewPrompt(templateID string, commentBody, filePath, lineRangeInfo, additionalInstructions string, ws *models.Workspace) (string, error) {
	templateVars := map[string]interface{}{
		"comment_body":            commentBody,
		"file_path":               filePath,
		"line_range_info":         lineRangeInfo,
		"additional_instructions": additionalInstructions,
	}

	req := &prompt.PromptRequest{
		TemplateID:   templateID,
		TemplateVars: templateVars,
		Workspace:    ws,
	}

	result, err := a.promptBuilder.BuildPrompt(context.Background(), req)
	if err != nil {
		return "", err
	}

	return result.Content, nil
}

// buildBatchReviewPrompt 构建批量 Review Comments 的 Prompt
func (a *Agent) buildBatchReviewPrompt(reviewBody, batchComments, additionalInstructions, processingMode string, ws *models.Workspace) (string, error) {
	templateVars := map[string]interface{}{
		"review_body":             reviewBody,
		"batch_comments":          batchComments,
		"additional_instructions": additionalInstructions,
		"processing_mode":         processingMode,
	}

	req := &prompt.PromptRequest{
		TemplateID:   "batch_review_processing",
		TemplateVars: templateVars,
		Workspace:    ws,
	}

	result, err := a.promptBuilder.BuildPrompt(context.Background(), req)
	if err != nil {
		return "", err
	}

	return result.Content, nil
}

// buildFallbackPrompt 构建回退 Prompt（当新系统失败时使用）
func (a *Agent) buildFallbackPrompt(templateID string, vars map[string]interface{}) string {
	// 使用简化的内联模板，保持与 prompt 包一致的设计理念
	switch templateID {
	case "single_review_continue":
		commentBody := vars["comment_body"].(string)
		filePath := vars["file_path"].(string)
		lineRangeInfo := vars["line_range_info"].(string)
		additionalInstructions := vars["additional_instructions"].(string)

		// 使用结构化的模板格式
		template := `根据以下代码行评论继续处理代码：

## 代码行评论
%s

## 文件信息
文件：%s
%s

%s

请根据评论要求继续处理代码，确保：
1. 理解评论的意图和要求
2. 进行相应的代码修改或改进
3. 保持代码质量和一致性
4. 遵循项目的编码规范`

		instructions := ""
		if additionalInstructions != "" {
			instructions = fmt.Sprintf("## 额外指令\n%s", additionalInstructions)
		}

		return fmt.Sprintf(template, commentBody, filePath, lineRangeInfo, instructions)

	case "single_review_fix":
		commentBody := vars["comment_body"].(string)
		filePath := vars["file_path"].(string)
		lineRangeInfo := vars["line_range_info"].(string)
		additionalInstructions := vars["additional_instructions"].(string)

		// 使用结构化的模板格式
		template := `根据以下代码行评论修复代码：

## 代码行评论
%s

## 文件信息
文件：%s
%s

%s

请根据评论要求修复代码，确保：
1. 解决评论中提到的问题
2. 保持代码质量和一致性
3. 遵循项目的编码规范
4. 进行必要的测试验证`

		instructions := ""
		if additionalInstructions != "" {
			instructions = fmt.Sprintf("## 额外指令\n%s", additionalInstructions)
		}

		return fmt.Sprintf(template, commentBody, filePath, lineRangeInfo, instructions)

	case "batch_review_processing":
		batchComments := vars["batch_comments"].(string)
		additionalInstructions := vars["additional_instructions"].(string)
		processingMode := vars["processing_mode"].(string)

		// 使用结构化的模板格式
		template := `根据以下 PR Review 的批量评论%s：

## 批量评论
%s

%s

请一次性处理所有评论中提到的问题，确保：
1. 理解每个评论的意图和要求
2. 进行相应的代码修改或改进
3. 保持代码质量和一致性
4. 遵循项目的编码规范
5. 回复要简洁明了`

		action := "处理代码"
		if processingMode == "修复问题" {
			action = "修复代码问题"
		}

		instructions := ""
		if additionalInstructions != "" {
			instructions = fmt.Sprintf("## 额外指令\n%s", additionalInstructions)
		}

		return fmt.Sprintf(template, action, batchComments, instructions)

	case "issue_based_code_generation":
		issueBody := vars["issue_body"].(string)
		historicalContext := vars["historical_context"].(string)

		template := `根据以下 Issue 需求生成高质量的代码：

## Issue 信息
描述：%s

%s

请根据需求生成代码，确保：
1. 代码质量高，符合最佳实践
2. 包含必要的测试和文档
3. 遵循项目的编码规范
4. 考虑性能和安全性`

		context := ""
		if historicalContext != "" {
			context = fmt.Sprintf("## 历史讨论\n%s", historicalContext)
		}

		return fmt.Sprintf(template, issueBody, context)

	case "review_based_code_modification":
		reviewComments := vars["review_comments"].(string)
		historicalContext := vars["historical_context"].(string)

		template := `根据以下 Code Review Comments 修改代码：

## Review Comments
%s

%s

请根据评论要求修改代码，确保：
1. 解决评论中提到的问题
2. 保持代码质量和一致性
3. 遵循项目的编码规范`

		context := ""
		if historicalContext != "" {
			context = fmt.Sprintf("## 历史讨论\n%s", historicalContext)
		}

		return fmt.Sprintf(template, reviewComments, context)

	default:
		return "请根据要求处理代码。"
	}
}

// ContinuePRWithArgs 继续处理 PR 中的任务，支持命令参数
func (a *Agent) ContinuePRWithArgs(ctx context.Context, event *github.IssueCommentEvent, args string) error {
	return a.processPRWithArgs(ctx, event, args, "Continue")
}

// ContinuePRWithArgsAndAI 继续处理 PR 中的任务，支持命令参数和AI模型
func (a *Agent) ContinuePRWithArgsAndAI(ctx context.Context, event *github.IssueCommentEvent, aiModel, args string) error {
	return a.processPRWithArgsAndAI(ctx, event, aiModel, args, "Continue")
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
	return a.processPRWithArgs(ctx, event, args, "Fix")
}

// FixPRWithArgsAndAI 修复 PR 中的问题，支持命令参数和AI模型
func (a *Agent) FixPRWithArgsAndAI(ctx context.Context, event *github.IssueCommentEvent, aiModel, args string) error {
	return a.processPRWithArgsAndAI(ctx, event, aiModel, args, "Fix")
}

// ContinuePRFromReviewComment 从 PR 代码行评论继续处理任务
func (a *Agent) ContinuePRFromReviewComment(ctx context.Context, event *github.PullRequestReviewCommentEvent, args string) error {
	return a.ContinuePRFromReviewCommentWithAI(ctx, event, "", args)
}

// ContinuePRFromReviewCommentWithAI 从 PR 代码行评论继续处理任务，支持AI模型
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
	code, err := a.sessionManager.GetSession(ws)
	if err != nil {
		log.Errorf("failed to get code client for PR continue from review comment: %v", err)
		return err
	}

	// 4. 构建 prompt，使用新的 Prompt 系统
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

	// 使用新的 Prompt 系统
	promptContent, err := a.buildSingleReviewPrompt(
		"single_review_continue",
		event.Comment.GetBody(),
		event.Comment.GetPath(),
		lineRangeInfo,
		args,
		ws,
	)
	if err != nil {
		log.Errorf("Failed to build prompt using new system: %v", err)
		// 使用简化的回退模板
		prompt = a.buildFallbackPrompt("single_review_continue", map[string]interface{}{
			"comment_body":            event.Comment.GetBody(),
			"file_path":               event.Comment.GetPath(),
			"line_range_info":         lineRangeInfo,
			"additional_instructions": args,
		})
	} else {
		prompt = promptContent
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

	// 使用新的 Prompt 系统
	promptContent, err := a.buildSingleReviewPrompt(
		"single_review_fix",
		event.Comment.GetBody(),
		event.Comment.GetPath(),
		lineRangeInfo,
		args,
		ws,
	)
	if err != nil {
		log.Errorf("Failed to build prompt using new system: %v", err)
		// 使用简化的回退模板
		prompt = a.buildFallbackPrompt("single_review_fix", map[string]interface{}{
			"comment_body":            event.Comment.GetBody(),
			"file_path":               event.Comment.GetPath(),
			"line_range_info":         lineRangeInfo,
			"additional_instructions": args,
		})
	} else {
		prompt = promptContent
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
	code, err := a.sessionManager.GetSession(ws)
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

	// 使用新的 Prompt 系统
	var prompt string
	var processingMode string
	if command == "/continue" {
		processingMode = "继续处理"
	} else {
		processingMode = "修复问题"
	}

	promptContent, err := a.buildBatchReviewPrompt(
		event.Review.GetBody(),
		allComments,
		args,
		processingMode,
		ws,
	)
	if err != nil {
		log.Errorf("Failed to build prompt using new system: %v", err)
		// 使用简化的回退模板
		prompt = a.buildFallbackPrompt("batch_review_processing", map[string]interface{}{
			"review_body":             event.Review.GetBody(),
			"batch_comments":          allComments,
			"additional_instructions": args,
			"processing_mode":         processingMode,
		})
	} else {
		prompt = promptContent
	}

	resp, err := a.promptWithRetry(ctx, code, prompt, 3)
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
	if err := a.github.CommitAndPush(ws, result, code); err != nil {
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

// CleanupAfterPRMerged PR 合并后清理工作区、映射和执行的code session
func (a *Agent) CleanupAfterPRMerged(ctx context.Context, pr *github.PullRequest) error {
	log := xlog.NewWith(ctx)

	prNumber := pr.GetNumber()
	log.Infof("Starting cleanup after PR #%d merged", prNumber)

	// 获取所有与该PR相关的工作空间（可能有多个不同AI模型的工作空间）
	workspaces := a.workspace.GetAllWorkspacesByPR(pr)
	if len(workspaces) == 0 {
		log.Infof("No workspaces found for PR: %s, skip cleanup", pr.GetHTMLURL())
		return nil
	}
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

	log.Infof("Cleanup after PR merged completed: PR #%d, cleaned %d workspaces", prNumber, len(workspaces))
	return nil
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
