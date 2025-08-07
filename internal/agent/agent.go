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
	workspace      *workspace.Manager
	sessionManager *code.SessionManager
}

func New(cfg *config.Config, workspaceManager *workspace.Manager) *Agent {
	// Initialize GitHub client
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

// StartCleanupRoutine starts periodic cleanup routine
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

	// First collect expired workspaces to avoid calling methods that may acquire locks while holding locks
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

// ProcessIssueComment processes Issue comment events with complete repository information
func (a *Agent) ProcessIssueComment(ctx context.Context, event *github.IssueCommentEvent) error {
	return a.ProcessIssueCommentWithAI(ctx, event, "", "")
}

// ProcessIssueCommentWithAI processes Issue comment events with AI model support
func (a *Agent) ProcessIssueCommentWithAI(ctx context.Context, event *github.IssueCommentEvent, aiModel, args string) error {
	log := xlog.NewWith(ctx)

	issueNumber := event.Issue.GetNumber()
	issueTitle := event.Issue.GetTitle()

	log.Infof("Starting issue comment processing: issue=#%d, title=%s, AI model=%s", issueNumber, issueTitle, aiModel)

	// 1. Create Issue workspace with AI model information
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

	// 3. Create initial PR
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
	code, err := a.sessionManager.GetSession(ws)
	if err != nil {
		log.Errorf("Failed to get code client: %v", err)
		return err
	}
	log.Infof("Code client initialized successfully")

	// 8. Execute code modification
	codePrompt := fmt.Sprintf(`Modify code based on Issue:

Title: %s
Description: %s

Output format:
%s
Brief description of changes

%s
- List modified files and specific changes`, event.Issue.GetTitle(), event.Issue.GetBody(), models.SectionSummary, models.SectionChanges)

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
	if err = a.github.CommitAndPush(ws, result, code); err != nil {
		log.Errorf("Failed to commit and push: %v", err)
		return err
	}
	log.Infof("Changes committed and pushed successfully")

	log.Infof("Issue processing completed successfully: issue=#%d, PR=%s", issueNumber, pr.GetHTMLURL())
	return nil
}

// parseStructuredOutput parses AI's three-section output
func parseStructuredOutput(output string) (summary, changes, testPlan string) {
	lines := strings.Split(output, "\n")

	var currentSection string
	var summaryLines, changesLines, testPlanLines []string

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)

		// Detect section headers
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

		// Collect content based on current section
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

// extractErrorInfo extracts error information
func extractErrorInfo(output string) string {
	lines := strings.Split(output, "\n")

	// Search for error information
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

// processPRWithArgs generic function for processing PRs with different operation modes
func (a *Agent) processPRWithArgs(ctx context.Context, event *github.IssueCommentEvent, args string, mode string) error {
	return a.processPRWithArgsAndAI(ctx, event, "", args, mode)
}

// processPRWithArgsAndAI generic function for processing PRs with different operation modes and AI models
func (a *Agent) processPRWithArgsAndAI(ctx context.Context, event *github.IssueCommentEvent, aiModel, args string, mode string) error {
	log := xlog.NewWith(ctx)

	prNumber := event.Issue.GetNumber()
	log.Infof("%s PR #%d with AI model %s and args: %s", mode, prNumber, aiModel, args)

	// 1. Verify this is a PR comment (only for continue operation)
	if mode == "Continue" && event.Issue.PullRequestLinks == nil {
		log.Errorf("This is not a PR comment, cannot continue")
		return fmt.Errorf("this is not a PR comment, cannot continue")
	}

	// 2. Extract repository information from IssueCommentEvent
	repoURL := ""
	repoOwner := ""
	repoName := ""

	// Prioritize using repository field (if exists)
	if event.Repo != nil {
		repoOwner = event.Repo.GetOwner().GetLogin()
		repoName = event.Repo.GetName()
		repoURL = event.Repo.GetCloneURL()
	}

	// If repository field doesn't exist, extract from Issue's HTML URL
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

	// 3. Get complete PR information from GitHub API
	log.Infof("Fetching PR information from GitHub API")
	pr, err := a.github.GetPullRequest(repoOwner, repoName, event.Issue.GetNumber())
	if err != nil {
		log.Errorf("Failed to get PR #%d: %v", prNumber, err)
		return fmt.Errorf("failed to get PR information: %w", err)
	}
	log.Infof("PR information fetched successfully")

	// 4. If no AI model specified, extract from PR branch
	if aiModel == "" {
		branchName := pr.GetHead().GetRef()
		aiModel = a.workspace.ExtractAIModelFromBranch(branchName)
		if aiModel == "" {
			// If cannot extract from branch, use default configuration
			aiModel = a.config.CodeProvider
		}
		log.Infof("Extracted AI model from branch: %s", aiModel)
	}

	// 5. Get or create PR workspace with AI model information
	log.Infof("Getting or creating workspace for PR with AI model: %s", aiModel)
	ws := a.workspace.GetOrCreateWorkspaceForPRWithAI(pr, aiModel)
	if ws == nil {
		log.Errorf("Failed to get or create workspace for PR %s", strings.ToLower(mode))
		return fmt.Errorf("failed to get or create workspace for PR %s", strings.ToLower(mode))
	}
	log.Infof("Workspace ready: %s", ws.Path)

	// 5. Pull latest changes from remote
	log.Infof("Pulling latest changes from remote")
	if err := a.github.PullLatestChanges(ws, pr); err != nil {
		log.Warnf("Failed to pull latest changes: %v", err)
		// Don't return error, continue execution as it might be a network issue
	} else {
		log.Infof("Latest changes pulled successfully")
	}

	// 6. Initialize code client
	log.Infof("Initializing code client")
	codeClient, err := a.sessionManager.GetSession(ws)
	if err != nil {
		log.Errorf("Failed to create code session: %v", err)
		return fmt.Errorf("failed to create code session: %w", err)
	}
	log.Infof("Code client initialized successfully")

	// 7. Get all PR comment history for building context
	log.Infof("Fetching all PR comments for historical context")
	allComments, err := a.github.GetAllPRComments(pr)
	if err != nil {
		log.Warnf("Failed to get PR comments for context: %v", err)
		// Don't return error, use simple prompt
		allComments = &models.PRAllComments{}
	}

	// 8. Build prompt with historical context
	var prompt string
	var currentCommentID int64
	if event.Comment != nil {
		currentCommentID = event.Comment.GetID()
	}
	historicalContext := a.formatHistoricalComments(allComments, currentCommentID)

	// Generate different prompts based on mode
	prompt = a.buildPrompt(mode, args, historicalContext)

	log.Infof("Using %s prompt with args and historical context", strings.ToLower(mode))

	// 9. Execute AI processing
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

	// 10. Commit changes and update PR
	result := &models.ExecutionResult{
		Output: string(output),
		Error:  "",
	}

	log.Infof("Committing and pushing changes for PR %s", strings.ToLower(mode))
	if err := a.github.CommitAndPush(ws, result, codeClient); err != nil {
		log.Errorf("Failed to commit and push changes: %v", err)
		// Decide whether to return error based on mode
		if mode == "Fix" {
			return err
		}
		// Continue mode doesn't return error, continue with comment
	} else {
		log.Infof("Changes committed and pushed successfully")
	}

	// 11. Comment on PR
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

// buildPrompt builds prompt for different modes
func (a *Agent) buildPrompt(mode string, args string, historicalContext string) string {
	var prompt string
	var taskDescription string
	var defaultTask string

	switch mode {
	case "Continue":
		taskDescription = "Please modify the code according to the above PR description, historical discussions, and current instructions."
		defaultTask = "Continue processing PR, analyze code changes and improve"
	case "Fix":
		taskDescription = "Please fix the code according to the above PR description, historical discussions, and current instructions."
		defaultTask = "Analyze and fix code issues"
	default:
		taskDescription = "Please process the code according to the above PR description, historical discussions, and current instructions."
		defaultTask = "Process code tasks"
	}

	if args != "" {
		if historicalContext != "" {
			prompt = fmt.Sprintf(`As a PR code review assistant, please %s based on the following complete context:

%s

## Current Instructions
%s

%sNote:
1. Current instruction is the main task, historical information is only for context reference
2. Please ensure modifications align with the PR's overall goals and existing discussion consensus
3. If conflicts with historical discussions are found, prioritize current instruction and explain in the response`,
				strings.ToLower(mode), historicalContext, args, taskDescription)
		} else {
			prompt = fmt.Sprintf("According to instruction %s:\n\n%s", strings.ToLower(mode), args)
		}
	} else {
		if historicalContext != "" {
			prompt = fmt.Sprintf(`As a PR code review assistant, please %s based on the following complete context:

%s

## Task
%s

Please make corresponding code modifications and improvements based on the above PR description and historical discussions.`,
				strings.ToLower(mode), historicalContext, defaultTask)
		} else {
			prompt = defaultTask
		}
	}

	return prompt
}

// ContinuePRWithArgs continues processing tasks in PR with command arguments
func (a *Agent) ContinuePRWithArgs(ctx context.Context, event *github.IssueCommentEvent, args string) error {
	return a.processPRWithArgs(ctx, event, args, "Continue")
}

// ContinuePRWithArgsAndAI continues processing tasks in PR with command arguments and AI model
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

// FixPRWithArgs fixes issues in PR with command arguments
func (a *Agent) FixPRWithArgs(ctx context.Context, event *github.IssueCommentEvent, args string) error {
	return a.processPRWithArgs(ctx, event, args, "Fix")
}

// FixPRWithArgsAndAI fixes issues in PR with command arguments and AI model
func (a *Agent) FixPRWithArgsAndAI(ctx context.Context, event *github.IssueCommentEvent, aiModel, args string) error {
	return a.processPRWithArgsAndAI(ctx, event, aiModel, args, "Fix")
}

// ContinuePRFromReviewComment continues processing tasks from PR code line comments
func (a *Agent) ContinuePRFromReviewComment(ctx context.Context, event *github.PullRequestReviewCommentEvent, args string) error {
	return a.ContinuePRFromReviewCommentWithAI(ctx, event, "", args)
}

// ContinuePRFromReviewCommentWithAI continues processing tasks from PR code line comments with AI model support
func (a *Agent) ContinuePRFromReviewCommentWithAI(ctx context.Context, event *github.PullRequestReviewCommentEvent, aiModel, args string) error {
	log := xlog.NewWith(ctx)

	prNumber := event.PullRequest.GetNumber()
	log.Infof("Continue PR #%d from review comment with AI model %s and args: %s", prNumber, aiModel, args)

	// 1. Get PR information from workspace manager
	pr := event.PullRequest

	// 2. If no AI model specified, extract from PR branch
	if aiModel == "" {
		branchName := pr.GetHead().GetRef()
		aiModel = a.workspace.ExtractAIModelFromBranch(branchName)
		if aiModel == "" {
			// If cannot extract from branch, use default configuration
			aiModel = a.config.CodeProvider
		}
		log.Infof("Extracted AI model from branch: %s", aiModel)
	}

	// 3. Get or create PR workspace with AI model information
	ws := a.workspace.GetOrCreateWorkspaceForPRWithAI(pr, aiModel)
	if ws == nil {
		return fmt.Errorf("failed to get or create workspace for PR continue from review comment")
	}

	// 3. Pull latest changes from remote
	if err := a.github.PullLatestChanges(ws, pr); err != nil {
		log.Errorf("Failed to pull latest changes: %v", err)
		// Don't return error, continue execution as it might be a network issue
	}

	// 4. Initialize code client
	code, err := a.sessionManager.GetSession(ws)
	if err != nil {
		log.Errorf("failed to get code client for PR continue from review comment: %v", err)
		return err
	}

	// 4. Build prompt with comment context and command arguments
	var prompt string

	// Get line range information
	startLine := event.Comment.GetStartLine()
	endLine := event.Comment.GetLine()

	var lineRangeInfo string
	if startLine != 0 && endLine != 0 && startLine != endLine {
		// Multi-line selection
		lineRangeInfo = fmt.Sprintf("Line range: %d-%d", startLine, endLine)
	} else {
		// Single line
		lineRangeInfo = fmt.Sprintf("Line: %d", endLine)
	}

	commentContext := fmt.Sprintf("Code line comment: %s\nFile: %s\n%s",
		event.Comment.GetBody(),
		event.Comment.GetPath(),
		lineRangeInfo)

	if args != "" {
		prompt = fmt.Sprintf("Process based on code line comment and instruction:\n\n%s\n\nInstruction: %s", commentContext, args)
	} else {
		prompt = fmt.Sprintf("Process based on code line comment:\n\n%s", commentContext)
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

	// 5. Commit changes and update PR
	result := &models.ExecutionResult{
		Output: string(output),
	}
	if err := a.github.CommitAndPush(ws, result, code); err != nil {
		log.Errorf("Failed to commit and push for PR continue from review comment: %v", err)
		return err
	}

	// 6. Reply to original comment
	commentBody := string(output)
	if err = a.github.ReplyToReviewComment(pr, event.Comment.GetID(), commentBody); err != nil {
		log.Errorf("failed to reply to review comment for continue: %v", err)
		return err
	}

	log.Infof("Successfully continue PR #%d from review comment", pr.GetNumber())
	return nil
}

// FixPRFromReviewComment fixes issues from PR code line comments
func (a *Agent) FixPRFromReviewComment(ctx context.Context, event *github.PullRequestReviewCommentEvent, args string) error {
	return a.FixPRFromReviewCommentWithAI(ctx, event, "", args)
}

// FixPRFromReviewCommentWithAI fixes issues from PR code line comments with AI model support
func (a *Agent) FixPRFromReviewCommentWithAI(ctx context.Context, event *github.PullRequestReviewCommentEvent, aiModel, args string) error {
	log := xlog.NewWith(ctx)

	prNumber := event.PullRequest.GetNumber()
	log.Infof("Fix PR #%d from review comment with AI model %s and args: %s", prNumber, aiModel, args)

	// 1. Get PR information from workspace manager
	pr := event.PullRequest

	// 2. If no AI model specified, extract from PR branch
	if aiModel == "" {
		branchName := pr.GetHead().GetRef()
		aiModel = a.workspace.ExtractAIModelFromBranch(branchName)
		if aiModel == "" {
			// If cannot extract from branch, use default configuration
			aiModel = a.config.CodeProvider
		}
		log.Infof("Extracted AI model from branch: %s", aiModel)
	}

	// 3. Get or create PR workspace with AI model information
	ws := a.workspace.GetOrCreateWorkspaceForPRWithAI(pr, aiModel)
	if ws == nil {
		return fmt.Errorf("failed to get or create workspace for PR fix from review comment")
	}

	// 3. Pull latest changes from remote
	if err := a.github.PullLatestChanges(ws, pr); err != nil {
		log.Errorf("Failed to pull latest changes: %v", err)
		// Don't return error, continue execution as it might be a network issue
	}

	// 4. Initialize code client
	code, err := a.sessionManager.GetSession(ws)
	if err != nil {
		log.Errorf("failed to get code client for PR fix from review comment: %v", err)
		return err
	}

	// 4. Build prompt with comment context and command arguments
	var prompt string

	// Get line range information
	startLine := event.Comment.GetStartLine()
	endLine := event.Comment.GetLine()

	var lineRangeInfo string
	if startLine != 0 && endLine != 0 && startLine != endLine {
		// Multi-line selection
		lineRangeInfo = fmt.Sprintf("Line range: %d-%d", startLine, endLine)
	} else {
		// Single line
		lineRangeInfo = fmt.Sprintf("Line: %d", endLine)
	}

	commentContext := fmt.Sprintf("Code line comment: %s\nFile: %s\n%s",
		event.Comment.GetBody(),
		event.Comment.GetPath(),
		lineRangeInfo)

	if args != "" {
		prompt = fmt.Sprintf("Fix based on code line comment and instruction:\n\n%s\n\nInstruction: %s", commentContext, args)
	} else {
		prompt = fmt.Sprintf("Fix based on code line comment:\n\n%s", commentContext)
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

	// 5. Commit changes and update PR
	result := &models.ExecutionResult{
		Output: string(output),
	}
	if err := a.github.CommitAndPush(ws, result, code); err != nil {
		log.Errorf("Failed to commit and push for PR fix from review comment: %v", err)
		return err
	}

	// 6. Reply to original comment
	commentBody := string(output)
	if err = a.github.ReplyToReviewComment(pr, event.Comment.GetID(), commentBody); err != nil {
		log.Errorf("failed to reply to review comment for fix: %v", err)
		return err
	}

	log.Infof("Successfully fixed PR #%d from review comment", pr.GetNumber())
	return nil
}

// ProcessPRFromReviewWithTriggerUser processes multiple review comments from PR review in batch and mentions user in feedback
func (a *Agent) ProcessPRFromReviewWithTriggerUser(ctx context.Context, event *github.PullRequestReviewEvent, command string, args string, triggerUser string) error {
	return a.ProcessPRFromReviewWithTriggerUserAndAI(ctx, event, command, "", args, triggerUser)
}

// ProcessPRFromReviewWithTriggerUserAndAI processes multiple review comments from PR review in batch and mentions user in feedback with AI model support
func (a *Agent) ProcessPRFromReviewWithTriggerUserAndAI(ctx context.Context, event *github.PullRequestReviewEvent, command string, aiModel, args string, triggerUser string) error {
	log := xlog.NewWith(ctx)

	prNumber := event.PullRequest.GetNumber()
	reviewID := event.Review.GetID()
	log.Infof("Processing PR #%d from review %d with command: %s, AI model: %s, args: %s, triggerUser: %s", prNumber, reviewID, command, aiModel, args, triggerUser)

	// 1. Get PR information from workspace manager
	pr := event.PullRequest

	// 2. If no AI model specified, extract from PR branch
	if aiModel == "" {
		branchName := pr.GetHead().GetRef()
		aiModel = a.workspace.ExtractAIModelFromBranch(branchName)
		if aiModel == "" {
			// If cannot extract from branch, use default configuration
			aiModel = a.config.CodeProvider
		}
		log.Infof("Extracted AI model from branch: %s", aiModel)
	}

	// 3. Get all comments for the specified review
	reviewComments, err := a.github.GetReviewComments(pr, reviewID)
	if err != nil {
		log.Errorf("Failed to get review comments: %v", err)
		return err
	}

	log.Infof("Found %d review comments for review %d", len(reviewComments), reviewID)

	// 4. Get or create PR workspace with AI model information
	ws := a.workspace.GetOrCreateWorkspaceForPRWithAI(pr, aiModel)
	if ws == nil {
		return fmt.Errorf("failed to get or create workspace for PR batch processing from review")
	}

	// 4. Pull latest changes from remote
	if err := a.github.PullLatestChanges(ws, pr); err != nil {
		log.Errorf("Failed to pull latest changes: %v", err)
		// Don't return error, continue execution as it might be a network issue
	}

	// 5. Initialize code client
	code, err := a.sessionManager.GetSession(ws)
	if err != nil {
		log.Errorf("failed to get code client for PR batch processing from review: %v", err)
		return err
	}

	// 6. Build batch processing prompt with all review comments and position information
	var commentContexts []string

	// Add review body as overall context
	if event.Review.GetBody() != "" {
		commentContexts = append(commentContexts, fmt.Sprintf("Review overall description: %s", event.Review.GetBody()))
	}

	// Build detailed context for each comment
	for i, comment := range reviewComments {
		startLine := comment.GetStartLine()
		endLine := comment.GetLine()
		filePath := comment.GetPath()
		commentBody := comment.GetBody()

		var lineRangeInfo string
		if startLine != 0 && endLine != 0 && startLine != endLine {
			// Multi-line selection
			lineRangeInfo = fmt.Sprintf("Line range: %d-%d", startLine, endLine)
		} else {
			// Single line
			lineRangeInfo = fmt.Sprintf("Line: %d", endLine)
		}

		commentContext := fmt.Sprintf("Comment %d:\nFile: %s\n%s\nContent: %s",
			i+1, filePath, lineRangeInfo, commentBody)
		commentContexts = append(commentContexts, commentContext)
	}

	// Combine all contexts
	allComments := strings.Join(commentContexts, "\n\n")

	var prompt string
	if command == "/continue" {
		if args != "" {
			prompt = fmt.Sprintf("Please continue processing code based on the following batch PR Review comments and instructions:\n\n%s\n\nInstructions: %s\n\nPlease handle all issues mentioned in the comments at once, response should be concise and clear.", allComments, args)
		} else {
			prompt = fmt.Sprintf("Please continue processing code based on the following batch PR Review comments:\n\n%s\n\nPlease handle all issues mentioned in the comments at once, response should be concise and clear.", allComments)
		}
	} else { // /fix
		if args != "" {
			prompt = fmt.Sprintf("Please fix code issues based on the following batch PR Review comments and instructions:\n\n%s\n\nInstructions: %s\n\nPlease fix all issues mentioned in the comments at once, response should be concise and clear.", allComments, args)
		} else {
			prompt = fmt.Sprintf("Please fix code issues based on the following batch PR Review comments:\n\n%s\n\nPlease fix all issues mentioned in the comments at once, response should be concise and clear.", allComments)
		}
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

	// 7. Commit changes and update PR
	result := &models.ExecutionResult{
		Output: string(output),
	}
	if err := a.github.CommitAndPush(ws, result, code); err != nil {
		log.Errorf("Failed to commit and push for PR batch processing from review: %v", err)
		return err
	}

	// 8. Create comment with user mention
	var responseBody string
	if triggerUser != "" {
		if len(reviewComments) == 0 {
			responseBody = fmt.Sprintf("@%s Processed according to review instructions:\n\n%s", triggerUser, string(output))
		} else {
			responseBody = fmt.Sprintf("@%s Batch processed %d comments from this review:\n\n%s", triggerUser, len(reviewComments), string(output))
		}
	} else {
		if len(reviewComments) == 0 {
			responseBody = fmt.Sprintf("Processed according to review instructions:\n\n%s", string(output))
		} else {
			responseBody = fmt.Sprintf("Batch processed %d comments from this review:\n\n%s", len(reviewComments), string(output))
		}
	}

	if err = a.github.CreatePullRequestComment(pr, responseBody); err != nil {
		log.Errorf("failed to create PR comment for batch processing result: %v", err)
		return err
	}

	log.Infof("Successfully processed PR #%d from review %d with %d comments", pr.GetNumber(), reviewID, len(reviewComments))
	return nil
}

// ReviewPR reviews PR
func (a *Agent) ReviewPR(ctx context.Context, pr *github.PullRequest) error {
	log := xlog.NewWith(ctx)

	log.Infof("Starting PR review for PR #%d", pr.GetNumber())
	// TODO: Implement PR review logic
	log.Infof("PR review completed for PR #%d", pr.GetNumber())
	return nil
}

// CleanupAfterPRClosed cleans up workspace, mappings, executed code sessions and deletes CodeAgent created branches after PR closed
func (a *Agent) CleanupAfterPRClosed(ctx context.Context, pr *github.PullRequest) error {
	log := xlog.NewWith(ctx)

	prNumber := pr.GetNumber()
	prBranch := pr.GetHead().GetRef()
	log.Infof("Starting cleanup after PR #%d closed, branch: %s", prNumber, prBranch)

	// Get all workspaces related to this PR (may have multiple workspaces with different AI models)
	workspaces := a.workspace.GetAllWorkspacesByPR(pr)
	if len(workspaces) == 0 {
		log.Infof("No workspaces found for PR: %s", pr.GetHTMLURL())
	} else {
		log.Infof("Found %d workspaces for cleanup", len(workspaces))

		// Clean up all workspaces
		for _, ws := range workspaces {
			log.Infof("Cleaning up workspace: %s (AI model: %s)", ws.Path, ws.AIModel)

			// Clean up executed code session
			log.Infof("Closing code session for AI model: %s", ws.AIModel)
			err := a.sessionManager.CloseSession(ws)
			if err != nil {
				log.Errorf("Failed to close code session for PR #%d with AI model %s: %v", prNumber, ws.AIModel, err)
				// Don't return error, continue cleaning other workspaces
			} else {
				log.Infof("Code session closed successfully for AI model: %s", ws.AIModel)
			}

			// Clean up worktree, session directory and corresponding memory mappings
			log.Infof("Cleaning up workspace for AI model: %s", ws.AIModel)
			b := a.workspace.CleanupWorkspace(ws)
			if !b {
				log.Errorf("Failed to cleanup workspace for PR #%d with AI model %s", prNumber, ws.AIModel)
				// Don't return error, continue cleaning other workspaces
			} else {
				log.Infof("Workspace cleaned up successfully for AI model: %s", ws.AIModel)
			}
		}
	}

	// Delete branches created by CodeAgent
	if prBranch != "" && strings.HasPrefix(prBranch, "codeagent") {
		owner := pr.GetBase().GetRepo().GetOwner().GetLogin()
		repoName := pr.GetBase().GetRepo().GetName()

		log.Infof("Deleting CodeAgent branch: %s from repo %s/%s", prBranch, owner, repoName)
		err := a.github.DeleteCodeAgentBranch(ctx, owner, repoName, prBranch)
		if err != nil {
			log.Errorf("Failed to delete branch %s: %v", prBranch, err)
			// Don't return error, continue completing other cleanup work
		} else {
			log.Infof("Successfully deleted CodeAgent branch: %s", prBranch)
		}
	} else {
		log.Infof("Branch %s is not a CodeAgent branch, skipping deletion", prBranch)
	}

	log.Infof("Cleanup after PR closed completed: PR #%d, cleaned %d workspaces", prNumber, len(workspaces))
	return nil
}

// promptWithRetry prompt call with retry mechanism
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

		// If it's a broken pipe error, try to recreate session
		if strings.Contains(err.Error(), "broken pipe") ||
			strings.Contains(err.Error(), "process has already exited") {
			log.Infof("Detected broken pipe or process exit, will retry...")
		}

		if attempt < maxRetries {
			// Wait for a while before retrying
			sleepDuration := time.Duration(attempt) * 500 * time.Millisecond
			log.Infof("Waiting %v before retry", sleepDuration)
			time.Sleep(sleepDuration)
		}
	}

	log.Errorf("All prompt attempts failed after %d attempts", maxRetries)
	return nil, fmt.Errorf("failed after %d attempts, last error: %w", maxRetries, lastErr)
}

// formatHistoricalComments formats historical comments for building context
func (a *Agent) formatHistoricalComments(allComments *models.PRAllComments, currentCommentID int64) string {
	var contextParts []string

	// Add PR description
	if allComments.PRBody != "" {
		contextParts = append(contextParts, fmt.Sprintf("## PR Description\n%s", allComments.PRBody))
	}

	// Add historical general comments (excluding current comment)
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
			contextParts = append(contextParts, fmt.Sprintf("## Historical Comments\n%s", strings.Join(historyComments, "\n\n")))
		}
	}

	// Add code line comments
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
			contextParts = append(contextParts, fmt.Sprintf("## Code Line Comments\n%s", strings.Join(reviewComments, "\n\n")))
		}
	}

	// Add Review comments
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
			contextParts = append(contextParts, fmt.Sprintf("## Review Comments\n%s", strings.Join(reviews, "\n\n")))
		}
	}

	return strings.Join(contextParts, "\n\n")
}
