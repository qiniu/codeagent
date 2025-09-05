package modes

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/qiniu/codeagent/internal/code"
	"github.com/qiniu/codeagent/internal/command"
	"github.com/qiniu/codeagent/internal/config"
	githubcontext "github.com/qiniu/codeagent/internal/context"
	"github.com/qiniu/codeagent/internal/mcp"
	"github.com/qiniu/codeagent/internal/workspace"
	"github.com/qiniu/codeagent/pkg/models"

	"github.com/google/go-github/v58/github"
	ghclient "github.com/qiniu/codeagent/internal/github"
	"github.com/qiniu/x/xlog"
)

// CustomCommandHandler handles CodeAgent custom commands and subagents
type CustomCommandHandler struct {
	*BaseHandler
	clientManager    ghclient.ClientManagerInterface
	workspace        *workspace.Manager
	sessionManager   *code.SessionManager
	mcpClient        mcp.MCPClient
	contextInjector  *githubcontext.GitHubContextInjector
	globalConfigPath string
	defaultAIModel   string // 添加默认 AI 模型字段
	mentionConfig    models.MentionConfig
}

// NewCustomCommandHandler creates a new custom command handler
func NewCustomCommandHandler(clientManager ghclient.ClientManagerInterface, workspace *workspace.Manager, sessionManager *code.SessionManager, mcpClient mcp.MCPClient, globalConfigPath string, codeProvider string, cfg *config.Config) *CustomCommandHandler {
	baseHandler := NewBaseHandler(CustomCommandMode, 1, "CodeAgent custom commands and subagents")

	// Create mention config adapter
	mentionConfig := &models.ConfigMentionAdapter{
		Triggers:       cfg.Mention.Triggers,
		DefaultTrigger: cfg.Mention.DefaultTrigger,
	}

	return &CustomCommandHandler{
		BaseHandler:      baseHandler,
		clientManager:    clientManager,
		workspace:        workspace,
		sessionManager:   sessionManager,
		mcpClient:        mcpClient,
		contextInjector:  githubcontext.NewGitHubContextInjector(),
		globalConfigPath: globalConfigPath,
		defaultAIModel:   codeProvider, // 使用配置中的 CodeProvider
		mentionConfig:    mentionConfig,
	}
}

// CanHandle determines if this handler can process the GitHub context
func (h *CustomCommandHandler) CanHandle(ctx context.Context, githubCtx models.GitHubContext) bool {
	xl := xlog.NewWith(ctx)

	// Check if global config path is available
	if h.globalConfigPath == "" {
		xl.Infof("custom command handler disabled - no global config path")
		return false
	}

	// Extract command from the event using models.HasCommandWithConfig
	cmdInfo, hasCmd := models.HasCommandWithConfig(githubCtx, h.mentionConfig)
	if !hasCmd {
		xl.Infof("No slash command found in event")
		return false
	}

	xl.Infof("custom command handler can process command: %s", cmdInfo.Command)
	return true
}

// Execute processes the GitHub context using custom command system
func (h *CustomCommandHandler) Execute(ctx context.Context, githubCtx models.GitHubContext) error {
	xl := xlog.NewWith(ctx)

	// Extract command and instruction using models.HasCommandWithConfig
	cmdInfo, hasCmd := models.HasCommandWithConfig(githubCtx, h.mentionConfig)
	if !hasCmd {
		return fmt.Errorf("no command found in event")
	}

	// If user didn't specify AI model, use system default configuration
	if strings.TrimSpace(cmdInfo.AIModel) == "" {
		cmdInfo.AIModel = h.defaultAIModel
	}

	xl.Infof("Processing custom command: %s with instruction: %s", cmdInfo.Command, cmdInfo.Args)

	// 1. Create or get workspace based on event type
	workspace, err := h.createWorkspaceForEvent(ctx, githubCtx, cmdInfo.AIModel)
	if err != nil {
		return fmt.Errorf("failed to create workspace: %w", err)
	}

	xl.Infof("Created workspace: %s", workspace.Path)

	// 2. Build GitHub event data for context injection
	githubEvent, err := h.buildGitHubEvent(ctx, githubCtx, cmdInfo.Args)
	if err != nil {
		return fmt.Errorf("failed to build GitHub event: %w", err)
	}

	// 3. Process .codeagent directories with GitHub context
	repositoryConfigPath := filepath.Join(workspace.Path, ".codeagent")
	repoName := strings.ReplaceAll(githubCtx.GetRepository().GetFullName(), "/", "-")

	processor := command.NewContextAwareDirectoryProcessor(h.globalConfigPath, repositoryConfigPath, repoName, h.workspace.GetBaseDir())
	// defer processor.Cleanup()

	if err := processor.ProcessDirectories(githubEvent); err != nil {
		return fmt.Errorf("failed to process .codeagent directories: %w", err)
	}

	// Store processed .codeagent path in workspace for Docker integration
	workspace.ProcessedCodeAgentPath = processor.GetProcessedPath()
	xl.Infof("Processed .codeagent directories with GitHub context: %s", workspace.ProcessedCodeAgentPath)

	// 4. Load command definition from processed directory
	cmdDef, err := processor.LoadCommand(cmdInfo.Command)
	if err != nil {
		return fmt.Errorf("failed to load command '%s' for repo '%s': %w",
			cmdInfo.Command, githubCtx.GetRepository().GetFullName(), err)
	}

	xl.Infof("Loaded command '%s' from %s source", cmdInfo.Command, cmdDef.Source)

	// 5. Apply final context injection to command content
	processedContent, err := h.contextInjector.InjectContextWithLogging(ctx, cmdDef.Content, githubEvent, xl)
	if err != nil {
		xl.Errorf("Context injection failed, falling back to basic injection: %v", err)
		// processedContent = h.contextInjector.InjectContext(cmdDef.Content, githubEvent)
		return fmt.Errorf("context injection failed: %w", err)
	}

	xl.Infof("Processed command content length: %d", len(processedContent))

	// 8. Get code session for the workspace
	codeSession, err := h.sessionManager.GetSession(workspace)
	if err != nil {
		return fmt.Errorf("failed to get code session: %w", err)
	}

	xl.Infof("Got code session for AI model: %s", workspace.AIModel)

	// 9. Execute command with real Claude Code/Gemini integration
	resp, err := code.PromptWithRetry(ctx, codeSession, processedContent, 3)
	if err != nil {
		xl.Errorf("Command execution failed: %v", err)
		return fmt.Errorf("command execution failed: %w", err)
	}

	output, err := io.ReadAll(resp.Out)
	if err != nil {
		return fmt.Errorf("failed to read output for PR %s: %w", cmdInfo.Command, err)
	}
	xl.Infof("Command executed successfully, result length: %d", len(output))
	xl.Infof("Command result: %s", string(output))

	// 10. Post-process results (commit, PR updates, etc.)
	if err := h.postProcessResults(ctx, githubCtx, workspace, string(output), codeSession); err != nil {
		xl.Warnf("Post-processing failed: %v", err)
		// Don't return error here, as the main command succeeded
	}

	return nil
}

// buildGitHubEvent converts GitHub context to GitHub event format for context injection
func (h *CustomCommandHandler) buildGitHubEvent(ctx context.Context, githubCtx models.GitHubContext, instruction string) (*githubcontext.GitHubEvent, error) {
	xl := xlog.NewWith(ctx)

	githubEvent := &githubcontext.GitHubEvent{
		Type:           string(githubCtx.GetEventType()),
		Repository:     githubCtx.GetRepository().GetFullName(),
		TriggerUser:    githubCtx.GetSender().GetLogin(),
		Action:         githubCtx.GetEventAction(),
		TriggerComment: instruction, // The instruction is typically the trigger comment
	}

	switch ctx := githubCtx.(type) {
	case *models.IssueCommentContext:
		githubEvent.Issue = ctx.Issue
		githubEvent.IssueComment = ctx.Comment

		// Collect comment history for issues
		issueComments, err := h.collectIssueCommentHistory(context.Background(), ctx)
		if err != nil {
			xl.Warnf("Failed to collect issue comment history: %v", err)
			githubEvent.IssueComments = []string{} // Empty list as fallback
		} else {
			githubEvent.IssueComments = issueComments
		}

	case *models.PullRequestContext:
		githubEvent.PullRequest = ctx.PullRequest

		// Collect changed files from PR
		changedFiles, err := h.collectPRChangedFiles(context.Background(), ctx.PullRequest)
		if err != nil {
			xl.Warnf("Failed to collect PR changed files: %v", err)
			githubEvent.ChangedFiles = []string{} // Empty list as fallback
		} else {
			githubEvent.ChangedFiles = changedFiles
		}

		// Collect PR comment history
		prComments, reviewComments, err := h.collectPRCommentHistory(context.Background(), ctx)
		if err != nil {
			xl.Warnf("Failed to collect PR comment history: %v", err)
			githubEvent.PRComments = []string{}
			githubEvent.ReviewComments = []string{}
		} else {
			githubEvent.PRComments = prComments
			githubEvent.ReviewComments = reviewComments
		}

	case *models.PullRequestReviewCommentContext:
		githubEvent.Comment = ctx.Comment
		githubEvent.PullRequest = ctx.PullRequest

		// Collect changed files from PR
		changedFiles, err := h.collectPRChangedFiles(context.Background(), ctx.PullRequest)
		if err != nil {
			xl.Warnf("Failed to collect PR changed files: %v", err)
			githubEvent.ChangedFiles = []string{}
		} else {
			githubEvent.ChangedFiles = changedFiles
		}

		// Collect PR comment history
		prComments, reviewComments, err := h.collectPRCommentHistory(context.Background(), ctx)
		if err != nil {
			xl.Warnf("Failed to collect PR comment history: %v", err)
			githubEvent.PRComments = []string{}
			githubEvent.ReviewComments = []string{}
		} else {
			githubEvent.PRComments = prComments
			githubEvent.ReviewComments = reviewComments
		}
	}

	xl.Infof("Built GitHub event for %s with %d issue comments, %d PR comments, %d review comments, %d changed files",
		githubEvent.Repository,
		len(githubEvent.IssueComments), len(githubEvent.PRComments), len(githubEvent.ReviewComments), len(githubEvent.ChangedFiles))

	return githubEvent, nil
}

// createWorkspaceForEvent creates appropriate workspace based on GitHub event type
func (h *CustomCommandHandler) createWorkspaceForEvent(ctx context.Context, githubCtx models.GitHubContext, aiModel string) (*models.Workspace, error) {
	xl := xlog.NewWith(ctx)

	// Use the AI model passed from command parsing
	xl.Infof("Creating workspace with AI model: %s", aiModel)

	switch ctx := githubCtx.(type) {
	case *models.IssueCommentContext:
		if ctx.IsPRComment {
			// This is a PR comment - need to fetch actual PR details from GitHub API
			xl.Infof("Processing PR comment for PR #%d", ctx.Issue.GetNumber())

			// Get full PR information from GitHub API
			repo := ctx.Repository
			if repo == nil {
				return nil, fmt.Errorf("repository information missing")
			}

			// Convert github.Repository to models.Repository
			repoInfo := &models.Repository{
				Owner: repo.GetOwner().GetLogin(),
				Name:  repo.GetName(),
			}

			client, err := h.clientManager.GetClient(context.Background(), repoInfo)
			if err != nil {
				xl.Errorf("Failed to get GitHub client: %v", err)
				return nil, fmt.Errorf("failed to get GitHub client: %v", err)
			}

			pr, err := client.GetPullRequest(repo.GetOwner().GetLogin(), repo.GetName(), ctx.Issue.GetNumber())
			if err != nil {
				xl.Errorf("Failed to fetch PR #%d: %v", ctx.Issue.GetNumber(), err)
				return nil, fmt.Errorf("failed to fetch PR details: %v", err)
			}

			workspace := h.workspace.GetOrCreateWorkspaceForPR(pr, aiModel)
			if workspace != nil {
				return workspace, nil
			}
			return nil, fmt.Errorf("failed to create PR workspace")
		} else {
			// This is an Issue comment
			xl.Infof("Processing Issue comment for Issue #%d", ctx.Issue.GetNumber())
			workspace := h.workspace.GetOrCreateWorkspaceForIssue(ctx.Issue, aiModel)
			if workspace != nil {
				return workspace, nil
			}
			return nil, fmt.Errorf("failed to create Issue workspace")
		}

	case *models.PullRequestContext:
		xl.Infof("Processing PR context for PR #%d", ctx.PullRequest.GetNumber())
		workspace := h.workspace.GetOrCreateWorkspaceForPR(ctx.PullRequest, aiModel)
		if workspace != nil {
			return workspace, nil
		}
		return nil, fmt.Errorf("failed to create PR workspace")

	case *models.PullRequestReviewCommentContext:
		xl.Infof("Processing PR review comment for PR #%d", ctx.PullRequest.GetNumber())
		workspace := h.workspace.GetOrCreateWorkspaceForPR(ctx.PullRequest, aiModel)
		if workspace != nil {
			return workspace, nil
		}
		return nil, fmt.Errorf("failed to create PR review workspace")

	default:
		xl.Warnf("Unsupported GitHub context type: %T", githubCtx)
		return nil, fmt.Errorf("unsupported GitHub context type: %T", githubCtx)
	}
}

// postProcessResults handles post-execution tasks like commits and PR updates
func (h *CustomCommandHandler) postProcessResults(ctx context.Context, githubCtx models.GitHubContext, workspace *models.Workspace, result string, codeSession code.Code) error {
	xl := xlog.NewWith(ctx)

	xl.Infof("Post-processing results for workspace: %s", workspace.Path)

	// TODO: Implement post-processing logic
	// This should include:
	// 1. Commit changes if any
	// 2. Push to remote branch
	// 3. Update PR/Issue with results
	// 4. Handle any errors or feedback

	// For now, just log that we would do post-processing
	xl.Infof("Would commit and push changes, update GitHub with results")

	return nil
}

// Helper methods for GitHub API data collection

// collectPRChangedFiles collects changed files from a Pull Request via GitHub API
func (h *CustomCommandHandler) collectPRChangedFiles(ctx context.Context, pr *github.PullRequest) ([]string, error) {
	xl := xlog.NewWith(ctx)

	if pr == nil {
		return []string{}, fmt.Errorf("pull request is nil")
	}

	if pr.GetBase() == nil || pr.GetBase().GetRepo() == nil {
		return []string{}, fmt.Errorf("PR base repository information is missing")
	}

	repo := pr.GetBase().GetRepo()
	owner := repo.GetOwner().GetLogin()
	repoName := repo.GetName()
	prNumber := pr.GetNumber()

	xl.Debugf("Collecting changed files for PR #%d in %s/%s", prNumber, owner, repoName)

	// Convert github.Repository to models.Repository
	repoInfo := &models.Repository{
		Owner: repo.GetOwner().GetLogin(),
		Name:  repo.GetName(),
	}

	// Get GitHub client from clientManager
	client, err := h.clientManager.GetClient(ctx, repoInfo)
	if err != nil {
		xl.Errorf("Failed to get GitHub client: %v", err)
		return []string{}, fmt.Errorf("failed to get GitHub client: %w", err)
	}

	// Use GitHub API to get changed files
	files, _, err := client.GetClient().PullRequests.ListFiles(ctx, owner, repoName, prNumber, &github.ListOptions{
		PerPage: 100, // Get up to 100 files per page
	})
	if err != nil {
		xl.Errorf("Failed to fetch PR files from GitHub API: %v", err)
		return []string{}, fmt.Errorf("failed to fetch PR changed files: %w", err)
	}

	// Extract filenames from the GitHub API response
	changedFiles := make([]string, len(files))
	for i, file := range files {
		changedFiles[i] = file.GetFilename()
	}

	xl.Infof("Collected %d changed files for PR #%d", len(changedFiles), prNumber)
	return changedFiles, nil
}

// collectIssueCommentHistory collects comment history from an Issue
func (h *CustomCommandHandler) collectIssueCommentHistory(ctx context.Context, issueCtx *models.IssueCommentContext) ([]string, error) {
	xl := xlog.NewWith(ctx)

	if issueCtx == nil || issueCtx.Issue == nil {
		return []string{}, fmt.Errorf("issue context or issue is nil")
	}

	repo := issueCtx.GetRepository()
	if repo == nil {
		return []string{}, fmt.Errorf("repository information is missing")
	}

	owner := repo.GetOwner().GetLogin()
	repoName := repo.GetName()
	issueNumber := issueCtx.Issue.GetNumber()

	xl.Debugf("Collecting comment history for Issue #%d in %s/%s", issueNumber, owner, repoName)

	// Convert github.Repository to models.Repository
	repoInfo := &models.Repository{
		Owner: repo.GetOwner().GetLogin(),
		Name:  repo.GetName(),
	}

	// Get GitHub client from clientManager
	client, err := h.clientManager.GetClient(ctx, repoInfo)
	if err != nil {
		xl.Errorf("Failed to get GitHub client: %v", err)
		return []string{}, fmt.Errorf("failed to get GitHub client: %w", err)
	}

	// Use GitHub API to get issue comments
	issueComments, _, err := client.GetClient().Issues.ListComments(ctx, owner, repoName, issueNumber, &github.IssueListCommentsOptions{
		ListOptions: github.ListOptions{
			PerPage: 100, // Get up to 100 comments per page
		},
	})
	if err != nil {
		xl.Errorf("Failed to fetch issue comments from GitHub API: %v", err)
		return []string{}, fmt.Errorf("failed to fetch issue comments: %w", err)
	}

	// Extract comment bodies
	comments := make([]string, len(issueComments))
	for i, comment := range issueComments {
		comments[i] = comment.GetBody()
	}

	return comments, nil
}

// collectPRCommentHistory collects PR and review comment history
func (h *CustomCommandHandler) collectPRCommentHistory(ctx context.Context, prCtx interface{}) (prComments, reviewComments []string, err error) {
	xl := xlog.NewWith(ctx)

	var pr *github.PullRequest
	var repo *github.Repository

	switch ctx := prCtx.(type) {
	case *models.PullRequestContext:
		pr = ctx.PullRequest
		repo = ctx.GetRepository()
	case *models.PullRequestReviewCommentContext:
		pr = ctx.PullRequest
		repo = ctx.GetRepository()
	default:
		return []string{}, []string{}, fmt.Errorf("unsupported PR context type: %T", prCtx)
	}

	if pr == nil {
		return []string{}, []string{}, fmt.Errorf("pull request is nil")
	}

	if repo == nil {
		return []string{}, []string{}, fmt.Errorf("repository information is missing")
	}

	owner := repo.GetOwner().GetLogin()
	repoName := repo.GetName()
	prNumber := pr.GetNumber()

	xl.Debugf("Collecting comment history for PR #%d in %s/%s", prNumber, owner, repoName)

	// Convert github.Repository to models.Repository
	repoInfo := &models.Repository{
		Owner: repo.GetOwner().GetLogin(),
		Name:  repo.GetName(),
	}

	// Get GitHub client from clientManager
	client, err := h.clientManager.GetClient(ctx, repoInfo)
	if err != nil {
		xl.Errorf("Failed to get GitHub client: %v", err)
		return []string{}, []string{}, fmt.Errorf("failed to get GitHub client: %w", err)
	}

	// Collect PR issue comments (general PR comments)
	prIssueComments, _, err := client.GetClient().Issues.ListComments(ctx, owner, repoName, prNumber, &github.IssueListCommentsOptions{
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	})
	if err != nil {
		xl.Warnf("Failed to fetch PR issue comments: %v", err)
		prIssueComments = []*github.IssueComment{}
	}

	// Collect PR review comments (line-specific comments)
	prReviewComments, _, err := client.GetClient().PullRequests.ListComments(ctx, owner, repoName, prNumber, &github.PullRequestListCommentsOptions{
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	})
	if err != nil {
		xl.Warnf("Failed to fetch PR review comments: %v", err)
		prReviewComments = []*github.PullRequestComment{}
	}

	// Extract PR comment bodies
	prComments = make([]string, len(prIssueComments))
	for i, comment := range prIssueComments {
		prComments[i] = comment.GetBody()
	}

	// Extract review comment bodies
	reviewComments = make([]string, len(prReviewComments))
	for i, comment := range prReviewComments {
		reviewComments[i] = comment.GetBody()
	}

	xl.Infof("Collected %d PR comments and %d review comments for PR #%d", len(prComments), len(reviewComments), prNumber)
	return prComments, reviewComments, nil
}
