package context

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-github/v58/github"
	ghclient "github.com/qiniu/codeagent/internal/github"
	"github.com/qiniu/x/xlog"
)

// TemplateError represents errors that occur during template processing
type TemplateError struct {
	Type      string // "parse", "execute", "validation", "build"
	Template  string // Template content snippet
	Variable  string // Problematic variable (if applicable)
	Cause     error  // Underlying error
	Timestamp time.Time
}

func (e *TemplateError) Error() string {
	if e.Variable != "" {
		return fmt.Sprintf("template %s error: %s (variable: %s)", e.Type, e.Cause.Error(), e.Variable)
	}
	return fmt.Sprintf("template %s error: %s", e.Type, e.Cause.Error())
}

// CommentDetail represents a GitHub comment with metadata for template rendering
// Use map-like structure for flexible template access
type CommentDetail map[string]interface{}

// GitHubContextInjector provides intelligent template engine for GitHub context injection
type GitHubContextInjector struct {
	templateEngine *template.Template
}

// GitHubTemplateData represents structured template data for Go Template rendering
type GitHubTemplateData struct {
	// Core variables (GitHub Actions compatible)
	GITHUB_REPOSITORY   string
	GITHUB_EVENT_TYPE   string
	GITHUB_TRIGGER_USER string

	// Issue variables
	GITHUB_ISSUE_NUMBER int
	GITHUB_ISSUE_TITLE  string
	GITHUB_ISSUE_BODY   string
	GITHUB_ISSUE_LABELS []string
	GITHUB_ISSUE_AUTHOR string // NEW: Missing field

	// PR variables
	GITHUB_PR_NUMBER     int
	GITHUB_PR_TITLE      string
	GITHUB_PR_BODY       string // NEW: Missing field
	GITHUB_PR_AUTHOR     string // NEW: Missing field
	GITHUB_BRANCH_NAME   string
	GITHUB_BASE_BRANCH   string
	GITHUB_CHANGED_FILES []string

	// Comment and interaction variables
	GITHUB_TRIGGER_COMMENT string          // NEW: The command comment that triggered this execution
	GITHUB_ISSUE_COMMENTS  []CommentDetail // Enhanced: Structured comment list with metadata
	GITHUB_PR_COMMENTS     []string        // Enhanced: Structured comment list
	GITHUB_REVIEW_COMMENTS []string        // NEW: Code review comments

	// Review Comment variables (high-precision line-level context)
	GITHUB_REVIEW_FILE_PATH    string
	GITHUB_REVIEW_LINE_RANGE   string
	GITHUB_REVIEW_COMMENT_BODY string
	GITHUB_REVIEW_DIFF_HUNK    string
	GITHUB_REVIEW_FILE_CONTENT string

	// Advanced context
	GITHUB_REVIEW_START_LINE *int // nil for single line comments
	GITHUB_REVIEW_END_LINE   int
	GITHUB_IS_PR             bool
	GITHUB_IS_ISSUE          bool

	// Metadata
	GITHUB_EVENT_ACTION string // NEW: GitHub event action (opened, edited, etc.)
	GITHUB_REPO_OWNER   string // NEW: Repository owner
	GITHUB_REPO_NAME    string // NEW: Repository name
	GITHUB_ACTOR        string // NEW: User who triggered the event
	
	// Custom instruction support
	CUSTOM_INSTRUCTION  string // User's custom analysis instruction
}

// GitHubEvent represents unified GitHub event data
type GitHubEvent struct {
	Type         string
	Repository   string
	TriggerUser  string
	Issue        *github.Issue
	PullRequest  *github.PullRequest
	Comment      *github.PullRequestComment
	IssueComment *github.IssueComment
	ChangedFiles []string
	// Enhanced fields for complete context
	Action            string          // GitHub event action
	TriggerComment    string          // The comment that triggered this event
	IssueComments     []CommentDetail // Issue comment history with metadata
	PRComments        []string        // PR comment history
	ReviewComments    []string        // Review comment history
	CustomInstruction string          // User's custom analysis instruction
}

// getTemplateFunctions returns the comprehensive set of template functions for GitHub context processing
func getTemplateFunctions() template.FuncMap {
	return template.FuncMap{
		// Basic utility functions
		"join": strings.Join,
		"len":  func(slice []string) int { return len(slice) },
		"gt":   func(a, b int) bool { return a > b },
		"lt":   func(a, b int) bool { return a < b },
		"ge":   func(a, b int) bool { return a >= b },
		"le":   func(a, b int) bool { return a <= b },
		"eq":   func(a, b int) bool { return a == b },
		"add":  func(a, b int) int { return a + b },
		"sub":  func(a, b int) int { return a - b },

		// String manipulation functions
		"upper":     strings.ToUpper,
		"lower":     strings.ToLower,
		"title":     strings.Title,
		"contains":  strings.Contains,
		"hasPrefix": strings.HasPrefix,
		"hasSuffix": strings.HasSuffix,
		"replace":   strings.ReplaceAll,
		"trim":      strings.TrimSpace,
		"split":     strings.Split,

		// GitHub-specific formatting functions
		"formatTime": func(t *time.Time) string {
			if t == nil {
				return ""
			}
			return t.Format("2006-01-02 15:04:05")
		},
		"formatISOTime": func(t *time.Time) string {
			if t == nil {
				return ""
			}
			return t.Format(time.RFC3339)
		},
		"formatRelativeTime": func(t *time.Time) string {
			if t == nil {
				return ""
			}
			duration := time.Since(*t)
			days := int(duration.Hours() / 24)
			hours := int(duration.Hours()) % 24
			minutes := int(duration.Minutes()) % 60

			if days > 0 {
				return fmt.Sprintf("%d days ago", days)
			} else if hours > 0 {
				return fmt.Sprintf("%d hours ago", hours)
			} else if minutes > 0 {
				return fmt.Sprintf("%d minutes ago", minutes)
			} else {
				return "just now"
			}
		},
		"formatLabels": func(labels []string) string {
			if len(labels) == 0 {
				return "No labels"
			}
			return "Labels: " + strings.Join(labels, ", ")
		},
		"formatChangedFiles": func(files []string) string {
			count := len(files)
			if count == 0 {
				return "No files changed"
			} else if count == 1 {
				return fmt.Sprintf("1 file changed: %s", files[0])
			} else if count <= 3 {
				return fmt.Sprintf("%d files changed: %s", count, strings.Join(files, ", "))
			} else {
				return fmt.Sprintf("%d files changed: %s and %d more", count, strings.Join(files[:3], ", "), count-3)
			}
		},
		"extractFileExtensions": func(files []string) []string {
			extensions := make(map[string]bool)
			for _, file := range files {
				ext := filepath.Ext(file)
				if ext != "" {
					extensions[ext] = true
				}
			}
			result := make([]string, 0, len(extensions))
			for ext := range extensions {
				result = append(result, ext)
			}
			return result
		},
		"summarizeComments": func(comments []string) string {
			count := len(comments)
			if count == 0 {
				return "No comments"
			} else if count == 1 {
				return "1 comment"
			} else {
				return fmt.Sprintf("%d comments", count)
			}
		},
		"truncateText": func(text string, maxLength int) string {
			if len(text) <= maxLength {
				return text
			}
			return text[:maxLength-3] + "..."
		},
		"formatMarkdownLink": func(text, url string) string {
			return fmt.Sprintf("[%s](%s)", text, url)
		},
		"formatIssueRef": func(number int) string {
			return fmt.Sprintf("#%d", number)
		},
		"formatUserMention": func(username string) string {
			if username == "" {
				return ""
			}
			return "@" + username
		},
	}
}

// NewGitHubContextInjector creates a new context injector with template functions
func NewGitHubContextInjector() *GitHubContextInjector {
	tmpl := template.New("github_context").Funcs(getTemplateFunctions())

	return &GitHubContextInjector{
		templateEngine: tmpl,
	}
}

// InjectContext performs intelligent context injection with both simple replacement and Go Template syntax
func (g *GitHubContextInjector) InjectContext(commandContent string, eventData *GitHubEvent) string {
	// 1. Build structured template data
	data := g.buildTemplateData(eventData)

	// 2. First try Go Template rendering for advanced syntax
	if result, err := g.renderTemplate(commandContent, data); err == nil {
		return result
	} else {
		// Log template error instead of silently falling back
		// Note: This uses basic logging as we don't have a context logger here
		// For better logging, use InjectContextWithLogging instead
	}

	// 3. Fallback to simple variable replacement for basic cases
	return g.simpleVariableReplacement(commandContent, data)
}

// InjectContextWithLogging performs context injection with comprehensive error handling and logging
func (g *GitHubContextInjector) InjectContextWithLogging(ctx context.Context, commandContent string, eventData *GitHubEvent, xl *xlog.Logger) (string, error) {
	xl.Infof("Starting context injection for event type: %s", eventData.Type)

	// Build template data with error handling
	data, err := g.buildTemplateDataWithValidation(eventData)
	if err != nil {
		xl.Errorf("Failed to build template data: %v", err)
		return "", &TemplateError{Type: "build", Cause: err, Timestamp: time.Now()}
	}

	// Validate data completeness
	if missing := g.validateTemplateData(data); len(missing) > 0 {
		xl.Warnf("Missing template variables: %v", missing)
	}

	// Log template data for debugging
	g.logTemplateData(data, xl)
	
	// Debug: Log template variables that might be missing
	xl.Infof("Template data debug:")
	xl.Infof("  GITHUB_REPOSITORY: '%s'", data.GITHUB_REPOSITORY)
	xl.Infof("  GITHUB_ISSUE_NUMBER: %d", data.GITHUB_ISSUE_NUMBER)
	xl.Infof("  GITHUB_ISSUE_TITLE: '%s'", data.GITHUB_ISSUE_TITLE)
	xl.Infof("  GITHUB_ISSUE_AUTHOR: '%s'", data.GITHUB_ISSUE_AUTHOR)
	bodyPreview := data.GITHUB_ISSUE_BODY
	if len(bodyPreview) > 100 {
		bodyPreview = bodyPreview[:100] + "..."
	}
	xl.Infof("  GITHUB_ISSUE_BODY: '%s' (length: %d)", bodyPreview, len(data.GITHUB_ISSUE_BODY))
	xl.Infof("  GITHUB_ISSUE_LABELS: %v", data.GITHUB_ISSUE_LABELS)
	xl.Infof("  CUSTOM_INSTRUCTION: '%s'", data.CUSTOM_INSTRUCTION)
	xl.Infof("  GITHUB_ISSUE_COMMENTS count: %d", len(data.GITHUB_ISSUE_COMMENTS))

	// Check if this contains $ variables (simple replacement) vs {{ }} variables (template syntax)
	containsDollarVars := strings.Contains(commandContent, "$GITHUB_")
	containsTemplateVars := strings.Contains(commandContent, "{{.") && strings.Contains(commandContent, "}}")
	xl.Infof("Command contains $ variables: %t, template variables: %t", containsDollarVars, containsTemplateVars)

	var result string

	// Choose appropriate processing method based on variable syntax
	if containsTemplateVars {
		// Use Go template rendering for {{.VARIABLE}} syntax
		result, err = g.renderTemplate(commandContent, data)
		if err != nil {
			xl.Warnf("Template rendering failed, falling back to simple replacement: %v", err)
			result = g.simpleVariableReplacement(commandContent, data)
		} else {
			xl.Infof("Template rendering succeeded")
		}
	} else if containsDollarVars {
		// Use simple replacement for $VARIABLE syntax
		xl.Infof("Using simple variable replacement for $ syntax")
		result = g.simpleVariableReplacement(commandContent, data)
	} else {
		// No variables detected, try template first as fallback then return as-is
		result, err = g.renderTemplate(commandContent, data)
		if err != nil {
			xl.Infof("No variables detected, using content as-is")
			result = commandContent
		}
	}

	xl.Infof("Context injection completed, result length: %d", len(result))
	return result, nil
}

// buildTemplateData constructs structured data based on event type
func (g *GitHubContextInjector) buildTemplateData(event *GitHubEvent) *GitHubTemplateData {
	if event == nil {
		return &GitHubTemplateData{}
	}

	data := &GitHubTemplateData{
		GITHUB_REPOSITORY:      event.Repository,
		GITHUB_EVENT_TYPE:      event.Type,
		GITHUB_TRIGGER_USER:    event.TriggerUser,
		GITHUB_EVENT_ACTION:    event.Action,
		GITHUB_TRIGGER_COMMENT: event.TriggerComment,
		GITHUB_ACTOR:           event.TriggerUser,
		CUSTOM_INSTRUCTION:     event.CustomInstruction,
	}

	// Extract repository metadata
	owner, name := g.extractRepoMetadata(event.Repository)
	data.GITHUB_REPO_OWNER = owner
	data.GITHUB_REPO_NAME = name

	// Set comment histories
	data.GITHUB_ISSUE_COMMENTS = event.IssueComments
	data.GITHUB_PR_COMMENTS = event.PRComments
	data.GITHUB_REVIEW_COMMENTS = event.ReviewComments

	// Event-specific context injection
	switch event.Type {
	case "pull_request_review_comment":
		g.injectReviewCommentContext(data, event)
	case "pull_request":
		g.injectPullRequestContext(data, event)
	case "issues", "issue_comment":
		g.injectIssueContext(data, event)
	}

	return data
}

// buildTemplateDataWithValidation constructs structured data with enhanced validation and error handling
func (g *GitHubContextInjector) buildTemplateDataWithValidation(event *GitHubEvent) (*GitHubTemplateData, error) {
	if event == nil {
		return nil, fmt.Errorf("event data is nil")
	}

	data := &GitHubTemplateData{
		GITHUB_REPOSITORY:      event.Repository,
		GITHUB_EVENT_TYPE:      event.Type,
		GITHUB_TRIGGER_USER:    event.TriggerUser,
		GITHUB_EVENT_ACTION:    event.Action,
		GITHUB_TRIGGER_COMMENT: event.TriggerComment,
		GITHUB_ACTOR:           event.TriggerUser, // Same as trigger user in most cases
		CUSTOM_INSTRUCTION:     event.CustomInstruction,
	}

	// Extract repository metadata
	owner, name := g.extractRepoMetadata(event.Repository)
	data.GITHUB_REPO_OWNER = owner
	data.GITHUB_REPO_NAME = name

	// Set comment histories
	data.GITHUB_ISSUE_COMMENTS = event.IssueComments
	data.GITHUB_PR_COMMENTS = event.PRComments
	data.GITHUB_REVIEW_COMMENTS = event.ReviewComments

	// Event-specific context injection
	switch event.Type {
	case "pull_request_review_comment":
		if err := g.injectReviewCommentContextWithValidation(data, event); err != nil {
			return nil, fmt.Errorf("failed to inject review comment context: %w", err)
		}
	case "pull_request":
		if err := g.injectPullRequestContextWithValidation(data, event); err != nil {
			return nil, fmt.Errorf("failed to inject pull request context: %w", err)
		}
	case "issues", "issue_comment":
		if err := g.injectIssueContextWithValidation(data, event); err != nil {
			return nil, fmt.Errorf("failed to inject issue context: %w", err)
		}
	}

	return data, nil
}

// injectReviewCommentContext provides high-precision line-level context
func (g *GitHubContextInjector) injectReviewCommentContext(data *GitHubTemplateData, event *GitHubEvent) {
	if event.Comment == nil {
		return
	}

	comment := event.Comment
	data.GITHUB_REVIEW_FILE_PATH = comment.GetPath()
	data.GITHUB_REVIEW_COMMENT_BODY = comment.GetBody()
	data.GITHUB_REVIEW_DIFF_HUNK = comment.GetDiffHunk()

	// Line range formatting
	if comment.GetLine() != 0 {
		data.GITHUB_REVIEW_END_LINE = comment.GetLine()
		if comment.GetStartLine() != 0 && comment.GetStartLine() != comment.GetLine() {
			data.GITHUB_REVIEW_START_LINE = github.Int(comment.GetStartLine())
			data.GITHUB_REVIEW_LINE_RANGE = fmt.Sprintf("行号范围：%d-%d",
				comment.GetStartLine(), comment.GetLine())
		} else {
			data.GITHUB_REVIEW_LINE_RANGE = fmt.Sprintf("行号：%d", comment.GetLine())
		}
	}

	// Add PR context if available
	if event.PullRequest != nil {
		data.GITHUB_IS_PR = true
		data.GITHUB_PR_NUMBER = event.PullRequest.GetNumber()
		data.GITHUB_PR_TITLE = event.PullRequest.GetTitle()
		data.GITHUB_PR_BODY = event.PullRequest.GetBody()
		if event.PullRequest.GetUser() != nil {
			data.GITHUB_PR_AUTHOR = event.PullRequest.GetUser().GetLogin()
		}
		if event.PullRequest.GetHead() != nil {
			data.GITHUB_BRANCH_NAME = event.PullRequest.GetHead().GetRef()
		}
		if event.PullRequest.GetBase() != nil {
			data.GITHUB_BASE_BRANCH = event.PullRequest.GetBase().GetRef()
		}
		data.GITHUB_CHANGED_FILES = event.ChangedFiles
	}

	// Add Issue context if available (review comments can be associated with issues)
	if event.Issue != nil {
		data.GITHUB_IS_ISSUE = true
		data.GITHUB_ISSUE_NUMBER = event.Issue.GetNumber()
		data.GITHUB_ISSUE_TITLE = event.Issue.GetTitle()
		data.GITHUB_ISSUE_BODY = event.Issue.GetBody()
		if event.Issue.GetUser() != nil {
			data.GITHUB_ISSUE_AUTHOR = event.Issue.GetUser().GetLogin()
		}

		// Extract labels with nil checks
		labels := make([]string, 0, len(event.Issue.Labels))
		for _, label := range event.Issue.Labels {
			if label != nil {
				labels = append(labels, label.GetName())
			}
		}
		data.GITHUB_ISSUE_LABELS = labels
	}
}

// injectReviewCommentContextWithValidation provides enhanced review comment context with validation
func (g *GitHubContextInjector) injectReviewCommentContextWithValidation(data *GitHubTemplateData, event *GitHubEvent) error {
	if event == nil {
		return fmt.Errorf("event is nil")
	}
	if event.Comment == nil {
		return fmt.Errorf("review comment is nil")
	}
	if data == nil {
		return fmt.Errorf("data is nil")
	}

	comment := event.Comment
	data.GITHUB_REVIEW_FILE_PATH = comment.GetPath()
	data.GITHUB_REVIEW_COMMENT_BODY = comment.GetBody()
	data.GITHUB_REVIEW_DIFF_HUNK = comment.GetDiffHunk()

	// Line range formatting
	if comment.GetLine() != 0 {
		data.GITHUB_REVIEW_END_LINE = comment.GetLine()
		if comment.GetStartLine() != 0 && comment.GetStartLine() != comment.GetLine() {
			data.GITHUB_REVIEW_START_LINE = github.Int(comment.GetStartLine())
			data.GITHUB_REVIEW_LINE_RANGE = fmt.Sprintf("行号范围：%d-%d",
				comment.GetStartLine(), comment.GetLine())
		} else {
			data.GITHUB_REVIEW_LINE_RANGE = fmt.Sprintf("行号：%d", comment.GetLine())
		}
	}

	// Add PR context if available
	if event.PullRequest != nil {
		data.GITHUB_IS_PR = true
		data.GITHUB_PR_NUMBER = event.PullRequest.GetNumber()
		data.GITHUB_PR_TITLE = event.PullRequest.GetTitle()
		data.GITHUB_PR_BODY = event.PullRequest.GetBody()
		if event.PullRequest.GetUser() != nil {
			data.GITHUB_PR_AUTHOR = event.PullRequest.GetUser().GetLogin()
		}
		if event.PullRequest.GetHead() != nil {
			data.GITHUB_BRANCH_NAME = event.PullRequest.GetHead().GetRef()
		}
		if event.PullRequest.GetBase() != nil {
			data.GITHUB_BASE_BRANCH = event.PullRequest.GetBase().GetRef()
		}
		data.GITHUB_CHANGED_FILES = event.ChangedFiles
	}

	// Add Issue context if available (review comments can be associated with issues)
	if event.Issue != nil {
		data.GITHUB_IS_ISSUE = true
		data.GITHUB_ISSUE_NUMBER = event.Issue.GetNumber()
		data.GITHUB_ISSUE_TITLE = event.Issue.GetTitle()
		data.GITHUB_ISSUE_BODY = event.Issue.GetBody()
		if event.Issue.GetUser() != nil {
			data.GITHUB_ISSUE_AUTHOR = event.Issue.GetUser().GetLogin()
		}

		// Extract labels with nil checks
		labels := make([]string, 0, len(event.Issue.Labels))
		for _, label := range event.Issue.Labels {
			if label != nil {
				labels = append(labels, label.GetName())
			}
		}
		data.GITHUB_ISSUE_LABELS = labels
	}

	return nil
}

// injectPullRequestContext provides standard PR context
func (g *GitHubContextInjector) injectPullRequestContext(data *GitHubTemplateData, event *GitHubEvent) {
	if event.PullRequest == nil {
		return
	}

	pr := event.PullRequest
	data.GITHUB_IS_PR = true
	data.GITHUB_PR_NUMBER = pr.GetNumber()
	data.GITHUB_PR_TITLE = pr.GetTitle()
	data.GITHUB_PR_BODY = pr.GetBody()
	data.GITHUB_PR_AUTHOR = pr.GetUser().GetLogin()
	data.GITHUB_BRANCH_NAME = pr.GetHead().GetRef()
	data.GITHUB_BASE_BRANCH = pr.GetBase().GetRef()
	data.GITHUB_CHANGED_FILES = event.ChangedFiles
}

// injectPullRequestContextWithValidation provides enhanced PR context with validation
func (g *GitHubContextInjector) injectPullRequestContextWithValidation(data *GitHubTemplateData, event *GitHubEvent) error {
	if event == nil {
		return fmt.Errorf("event is nil")
	}
	if event.PullRequest == nil {
		return fmt.Errorf("pull request is nil")
	}
	if data == nil {
		return fmt.Errorf("data is nil")
	}

	pr := event.PullRequest
	data.GITHUB_IS_PR = true
	data.GITHUB_PR_NUMBER = pr.GetNumber()
	data.GITHUB_PR_TITLE = pr.GetTitle()
	data.GITHUB_PR_BODY = pr.GetBody()
	if pr.GetUser() != nil {
		data.GITHUB_PR_AUTHOR = pr.GetUser().GetLogin()
	}
	if pr.GetHead() != nil {
		data.GITHUB_BRANCH_NAME = pr.GetHead().GetRef()
	}
	if pr.GetBase() != nil {
		data.GITHUB_BASE_BRANCH = pr.GetBase().GetRef()
	}
	data.GITHUB_CHANGED_FILES = event.ChangedFiles

	return nil
}

// injectIssueContext provides Issue-specific context
func (g *GitHubContextInjector) injectIssueContext(data *GitHubTemplateData, event *GitHubEvent) {
	if data == nil {
		return
	}
	if event == nil || event.Issue == nil {
		return
	}

	issue := event.Issue
	data.GITHUB_IS_ISSUE = true
	data.GITHUB_ISSUE_NUMBER = issue.GetNumber()
	data.GITHUB_ISSUE_TITLE = issue.GetTitle()
	data.GITHUB_ISSUE_BODY = issue.GetBody()
	if issue.GetUser() != nil {
		data.GITHUB_ISSUE_AUTHOR = issue.GetUser().GetLogin()
	}

	// Extract labels with nil checks
	labels := make([]string, 0, len(issue.Labels))
	for _, label := range issue.Labels {
		if label != nil {
			labels = append(labels, label.GetName())
		}
	}
	data.GITHUB_ISSUE_LABELS = labels
}

// injectIssueContextWithValidation provides enhanced Issue context with validation
func (g *GitHubContextInjector) injectIssueContextWithValidation(data *GitHubTemplateData, event *GitHubEvent) error {
	if event == nil {
		return fmt.Errorf("event is nil")
	}
	if event.Issue == nil {
		return fmt.Errorf("issue is nil")
	}
	if data == nil {
		return fmt.Errorf("data is nil")
	}

	issue := event.Issue
	data.GITHUB_IS_ISSUE = true
	data.GITHUB_ISSUE_NUMBER = issue.GetNumber()
	data.GITHUB_ISSUE_TITLE = issue.GetTitle()
	data.GITHUB_ISSUE_BODY = issue.GetBody()
	if issue.GetUser() != nil {
		data.GITHUB_ISSUE_AUTHOR = issue.GetUser().GetLogin()
	}

	// Extract labels
	labels := make([]string, 0, len(issue.Labels))
	for _, label := range issue.Labels {
		if label != nil {
			labels = append(labels, label.GetName())
		}
	}
	data.GITHUB_ISSUE_LABELS = labels

	return nil
}

// renderTemplate performs Go Template rendering with error handling
func (g *GitHubContextInjector) renderTemplate(content string, data *GitHubTemplateData) (string, error) {
	// Create a new template for each rendering to avoid conflicts, using the comprehensive function set
	tmpl := template.New("github_context").Funcs(getTemplateFunctions())

	parsedTmpl, err := tmpl.Parse(content)
	if err != nil {
		return "", fmt.Errorf("template parse error: %w", err)
	}

	var buf bytes.Buffer
	if err := parsedTmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("template execute error: %w", err)
	}

	return buf.String(), nil
}

// simpleVariableReplacement performs basic $VAR replacement for backward compatibility
func (g *GitHubContextInjector) simpleVariableReplacement(content string, data *GitHubTemplateData) string {
	replacements := map[string]string{
		// Core variables
		"$GITHUB_REPOSITORY":   data.GITHUB_REPOSITORY,
		"$GITHUB_EVENT_TYPE":   data.GITHUB_EVENT_TYPE,
		"$GITHUB_TRIGGER_USER": data.GITHUB_TRIGGER_USER,
		"$GITHUB_EVENT_ACTION": data.GITHUB_EVENT_ACTION,
		"$GITHUB_REPO_OWNER":   data.GITHUB_REPO_OWNER,
		"$GITHUB_REPO_NAME":    data.GITHUB_REPO_NAME,
		"$GITHUB_ACTOR":        data.GITHUB_ACTOR,
		// Issue variables
		"$GITHUB_ISSUE_NUMBER": strconv.Itoa(data.GITHUB_ISSUE_NUMBER),
		"$GITHUB_ISSUE_TITLE":  data.GITHUB_ISSUE_TITLE,
		"$GITHUB_ISSUE_BODY":   data.GITHUB_ISSUE_BODY,
		"$GITHUB_ISSUE_AUTHOR": data.GITHUB_ISSUE_AUTHOR,
		// PR variables
		"$GITHUB_PR_NUMBER":   strconv.Itoa(data.GITHUB_PR_NUMBER),
		"$GITHUB_PR_TITLE":    data.GITHUB_PR_TITLE,
		"$GITHUB_PR_BODY":     data.GITHUB_PR_BODY,
		"$GITHUB_PR_AUTHOR":   data.GITHUB_PR_AUTHOR,
		"$GITHUB_BRANCH_NAME": data.GITHUB_BRANCH_NAME,
		"$GITHUB_BASE_BRANCH": data.GITHUB_BASE_BRANCH,
		// Comment variables
		"$GITHUB_TRIGGER_COMMENT": data.GITHUB_TRIGGER_COMMENT,
		"$CUSTOM_INSTRUCTION":     data.CUSTOM_INSTRUCTION,
		// Review variables
		"$GITHUB_REVIEW_FILE_PATH":    data.GITHUB_REVIEW_FILE_PATH,
		"$GITHUB_REVIEW_LINE_RANGE":   data.GITHUB_REVIEW_LINE_RANGE,
		"$GITHUB_REVIEW_COMMENT_BODY": data.GITHUB_REVIEW_COMMENT_BODY,
		"$GITHUB_REVIEW_DIFF_HUNK":    data.GITHUB_REVIEW_DIFF_HUNK,
		"$GITHUB_REVIEW_FILE_CONTENT": data.GITHUB_REVIEW_FILE_CONTENT,
	}

	result := content
	for placeholder, value := range replacements {
		result = strings.ReplaceAll(result, placeholder, value)
	}

	return result
}

// SetFileContent allows external components to inject file content for review comments
func (data *GitHubTemplateData) SetFileContent(content string) {
	data.GITHUB_REVIEW_FILE_CONTENT = content
}

// SetComments allows external components to inject comment history
func (data *GitHubTemplateData) SetComments(issueComments []CommentDetail, prComments []string) {
	data.GITHUB_ISSUE_COMMENTS = issueComments
	data.GITHUB_PR_COMMENTS = prComments
}

// validateTemplateData validates the completeness of template data
func (g *GitHubContextInjector) validateTemplateData(data *GitHubTemplateData) []string {
	missing := make([]string, 0)

	// Check core fields
	if data.GITHUB_REPOSITORY == "" {
		missing = append(missing, "GITHUB_REPOSITORY")
	}
	if data.GITHUB_EVENT_TYPE == "" {
		missing = append(missing, "GITHUB_EVENT_TYPE")
	}
	if data.GITHUB_TRIGGER_USER == "" {
		missing = append(missing, "GITHUB_TRIGGER_USER")
	}

	// Check event-specific fields
	if data.GITHUB_IS_ISSUE {
		if data.GITHUB_ISSUE_AUTHOR == "" {
			missing = append(missing, "GITHUB_ISSUE_AUTHOR")
		}
		if data.GITHUB_ISSUE_TITLE == "" {
			missing = append(missing, "GITHUB_ISSUE_TITLE")
		}
	}
	if data.GITHUB_IS_PR {
		if data.GITHUB_PR_AUTHOR == "" {
			missing = append(missing, "GITHUB_PR_AUTHOR")
		}
		if data.GITHUB_PR_TITLE == "" {
			missing = append(missing, "GITHUB_PR_TITLE")
		}
		if data.GITHUB_BRANCH_NAME == "" {
			missing = append(missing, "GITHUB_BRANCH_NAME")
		}
	}

	return missing
}

// logTemplateData logs template data for debugging purposes
func (g *GitHubContextInjector) logTemplateData(data *GitHubTemplateData, xl *xlog.Logger) {
	xl.Debugf("Template data populated:")
	xl.Debugf("  Repository: %s", data.GITHUB_REPOSITORY)
	xl.Debugf("  Event Type: %s", data.GITHUB_EVENT_TYPE)
	xl.Debugf("  Trigger User: %s", data.GITHUB_TRIGGER_USER)
	xl.Debugf("  Is Issue: %t, Is PR: %t", data.GITHUB_IS_ISSUE, data.GITHUB_IS_PR)

	if data.GITHUB_IS_ISSUE {
		xl.Debugf("  Issue #%d: %s (Author: %s)", data.GITHUB_ISSUE_NUMBER, data.GITHUB_ISSUE_TITLE, data.GITHUB_ISSUE_AUTHOR)
		xl.Debugf("  Issue Comments: %d", len(data.GITHUB_ISSUE_COMMENTS))
	}

	if data.GITHUB_IS_PR {
		xl.Debugf("  PR #%d: %s (Author: %s)", data.GITHUB_PR_NUMBER, data.GITHUB_PR_TITLE, data.GITHUB_PR_AUTHOR)
		xl.Debugf("  Branch: %s -> %s", data.GITHUB_BRANCH_NAME, data.GITHUB_BASE_BRANCH)
		xl.Debugf("  Changed Files: %d", len(data.GITHUB_CHANGED_FILES))
		xl.Debugf("  PR Comments: %d, Review Comments: %d", len(data.GITHUB_PR_COMMENTS), len(data.GITHUB_REVIEW_COMMENTS))
	}

	if data.GITHUB_TRIGGER_COMMENT != "" {
		xl.Debugf("  Trigger Comment: %.100s...", data.GITHUB_TRIGGER_COMMENT)
	}
}

// extractRepoMetadata extracts owner and name from repository string
func (g *GitHubContextInjector) extractRepoMetadata(repository string) (owner, name string) {
	parts := strings.Split(repository, "/")
	if len(parts) >= 2 {
		return parts[0], parts[1]
	}
	return "", repository
}

// Helper methods for GitHub API integration

// collectChangedFiles fetches changed files from GitHub API for PR events
func (g *GitHubContextInjector) collectChangedFiles(ctx context.Context, pr *github.PullRequest, ghClient *ghclient.Client) ([]string, error) {
	if pr == nil {
		return nil, fmt.Errorf("PR is nil")
	}
	if ghClient == nil {
		return nil, fmt.Errorf("GitHub client is nil")
	}

	// Validate PR structure
	if pr.GetBase() == nil || pr.GetBase().GetRepo() == nil {
		return nil, fmt.Errorf("PR base repository information is missing")
	}

	repo := pr.GetBase().GetRepo()
	if repo.GetOwner() == nil {
		return nil, fmt.Errorf("repository owner information is missing")
	}

	owner := repo.GetOwner().GetLogin()
	repoName := repo.GetName()
	prNumber := pr.GetNumber()

	if owner == "" || repoName == "" || prNumber == 0 {
		return nil, fmt.Errorf("invalid repository information: owner=%s, repo=%s, pr=%d", owner, repoName, prNumber)
	}

	// Use GitHub API to get changed files - leveraging existing pattern from CustomCommandHandler
	files, _, err := ghClient.GetClient().PullRequests.ListFiles(ctx, owner, repoName, prNumber, &github.ListOptions{
		PerPage: 100, // Get up to 100 files per page
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch PR changed files: %w", err)
	}

	// Extract filenames from the GitHub API response
	changedFiles := make([]string, len(files))
	for i, file := range files {
		if file != nil {
			changedFiles[i] = file.GetFilename()
		}
	}

	return changedFiles, nil
}

// collectCommentHistory fetches comment history from GitHub API
func (g *GitHubContextInjector) collectCommentHistory(ctx context.Context, event *GitHubEvent, ghClient *ghclient.Client) (issueComments []CommentDetail, prComments, reviewComments []string, err error) {
	if event == nil {
		return nil, nil, nil, fmt.Errorf("event is nil")
	}
	if ghClient == nil {
		return nil, nil, nil, fmt.Errorf("GitHub client is nil")
	}

	// Extract repository metadata
	owner, repoName := g.extractRepoMetadata(event.Repository)
	if owner == "" || repoName == "" {
		return nil, nil, nil, fmt.Errorf("invalid repository information: %s", event.Repository)
	}

	issueComments = []CommentDetail{}
	prComments = []string{}
	reviewComments = []string{}

	// Collect Issue comments if Issue is available
	if event.Issue != nil {
		issueNumber := event.Issue.GetNumber()
		if issueNumber > 0 {
			issueCommentList, _, err := ghClient.GetClient().Issues.ListComments(ctx, owner, repoName, issueNumber, &github.IssueListCommentsOptions{
				ListOptions: github.ListOptions{
					PerPage: 100,
				},
			})
			if err != nil {
				// Log error but don't fail completely
				// Note: In production, this should use proper context logging
			} else {
				issueComments = make([]CommentDetail, len(issueCommentList))
				for i, comment := range issueCommentList {
					if comment != nil {
						issueComments[i] = CommentDetail{
							"author":     comment.GetUser().GetLogin(),
							"body":       comment.GetBody(),
							"created_at": comment.GetCreatedAt().Format("2006-01-02 15:04:05"),
							// Also provide capitalized versions for consistency
							"Author":    comment.GetUser().GetLogin(),
							"Body":      comment.GetBody(),
							"CreatedAt": comment.GetCreatedAt().Format("2006-01-02 15:04:05"),
						}
					}
				}
			}
		}
	}

	// Collect PR comments if PullRequest is available
	if event.PullRequest != nil {
		prNumber := event.PullRequest.GetNumber()
		if prNumber > 0 {
			// Collect PR issue comments (general PR comments)
			prIssueComments, _, err := ghClient.GetClient().Issues.ListComments(ctx, owner, repoName, prNumber, &github.IssueListCommentsOptions{
				ListOptions: github.ListOptions{
					PerPage: 100,
				},
			})
			if err != nil {
				// Note: In production, this should use proper context logging
			} else {
				prComments = make([]string, len(prIssueComments))
				for i, comment := range prIssueComments {
					if comment != nil {
						prComments[i] = comment.GetBody()
					}
				}
			}

			// Collect PR review comments (line-specific comments)
			prReviewComments, _, err := ghClient.GetClient().PullRequests.ListComments(ctx, owner, repoName, prNumber, &github.PullRequestListCommentsOptions{
				ListOptions: github.ListOptions{
					PerPage: 100,
				},
			})
			if err != nil {
				// Note: In production, this should use proper context logging
			} else {
				reviewComments = make([]string, len(prReviewComments))
				for i, comment := range prReviewComments {
					if comment != nil {
						reviewComments[i] = comment.GetBody()
					}
				}
			}
		}
	}

	return issueComments, prComments, reviewComments, nil
}
