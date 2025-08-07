package webhook

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/qiniu/codeagent/internal/agent"
	"github.com/qiniu/codeagent/internal/config"
	"github.com/qiniu/codeagent/pkg/signature"

	"github.com/google/go-github/v58/github"
	"github.com/qiniu/x/reqid"
	"github.com/qiniu/x/xlog"
)

type Handler struct {
	config *config.Config
	agent  *agent.Agent
}

func NewHandler(cfg *config.Config, agent *agent.Agent) *Handler {
	return &Handler{config: cfg, agent: agent}
}

// parseCommandArgs parses command arguments, extracts AI model and other parameters
func parseCommandArgs(comment, command string, defaultAIModel string) (aiModel, args string) {
	// Extract command arguments
	commandArgs := strings.TrimSpace(strings.TrimPrefix(comment, command))

	// Check if contains AI model parameters
	if strings.HasPrefix(commandArgs, "-claude") {
		aiModel = "claude"
		args = strings.TrimSpace(strings.TrimPrefix(commandArgs, "-claude"))
	} else if strings.HasPrefix(commandArgs, "-gemini") {
		aiModel = "gemini"
		args = strings.TrimSpace(strings.TrimPrefix(commandArgs, "-gemini"))
	} else {
		// No AI model specified, use default configuration
		aiModel = defaultAIModel
		args = commandArgs
	}

	return aiModel, args
}

// HandleWebhook generic Webhook handler
func (h *Handler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	// 1. Read request body (need to read before signature verification)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}

	// 2. Verify Webhook signature
	if h.config.Server.WebhookSecret != "" {
		// Prioritize SHA-256 signature
		sig256 := r.Header.Get("X-Hub-Signature-256")
		if sig256 != "" {
			if err := signature.ValidateGitHubSignature(sig256, body, h.config.Server.WebhookSecret); err != nil {
				http.Error(w, "invalid signature", http.StatusUnauthorized)
				return
			}
		} else {
			// If no SHA-256 signature, try SHA-1 signature (deprecated but still supported)
			sig1 := r.Header.Get("X-Hub-Signature")
			if sig1 != "" {
				if err := signature.ValidateGitHubSignatureSHA1(sig1, body, h.config.Server.WebhookSecret); err != nil {
					http.Error(w, "invalid signature", http.StatusUnauthorized)
					return
				}
			} else {
				http.Error(w, "missing signature", http.StatusUnauthorized)
				return
			}
		}
	}

	// 3. Get event type
	eventType := r.Header.Get("X-GitHub-Event")
	if eventType == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("missing X-GitHub-Event header"))
		return
	}

	// 4. Create trace ID and context
	// Use X-GitHub-Delivery header as trace ID, truncate to first 8 characters
	deliveryID := r.Header.Get("X-GitHub-Delivery")
	var traceID string
	if deliveryID != "" && len(deliveryID) > 8 {
		traceID = deliveryID[:8]
	} else if deliveryID != "" {
		traceID = deliveryID
	} else {
		traceID = "unknown"
	}

	// Use reqid.NewContext to store traceID in context
	ctx := reqid.NewContext(context.Background(), traceID)
	xl := xlog.NewWith(ctx)
	xl.Infof("Received webhook event: %s", eventType)
	xl.Debugf("Request body size: %d bytes", len(body))

	// 5. Dispatch handling based on event type
	switch eventType {
	case "issue_comment":
		h.handleIssueComment(ctx, w, body)
	case "pull_request_review_comment":
		h.handlePRReviewComment(ctx, w, body)
	case "pull_request_review":
		h.handlePRReview(ctx, w, body)
	case "pull_request":
		h.handlePullRequest(ctx, w, body)
	case "push":
		h.handlePush(ctx, w, body)
	default:
		xl.Warnf("Unhandled event type: %s", eventType)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("event type not handled"))
	}
}

// handleIssueComment handles Issue comment events
func (h *Handler) handleIssueComment(ctx context.Context, w http.ResponseWriter, body []byte) {
	log := xlog.NewWith(ctx)

	var event github.IssueCommentEvent
	if err := json.Unmarshal(body, &event); err != nil {
		log.Errorf("Failed to unmarshal issue comment event: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid issue comment event"))
		return
	}

	// Check if contains commands
	if event.Comment == nil || event.Issue == nil {
		log.Debugf("Issue comment event missing comment or issue data")
		w.WriteHeader(http.StatusOK)
		return
	}

	comment := event.Comment.GetBody()
	issueNumber := event.Issue.GetNumber()
	issueTitle := event.Issue.GetTitle()

	log.Infof("Processing issue comment: issue=#%d, title=%s, comment_length=%d",
		issueNumber, issueTitle, len(comment))

	// Check if this is a PR comment (Issue's PullRequest field is not empty)
	if event.Issue.PullRequestLinks != nil {
		log.Infof("Detected PR comment for PR #%d", issueNumber)

		// This is a PR comment, handle /continue and /fix commands
		if strings.HasPrefix(comment, "/continue") {
			log.Infof("Received /continue command for PR #%d: %s", issueNumber, issueTitle)

			// Parse AI model parameters
			aiModel, args := parseCommandArgs(comment, "/continue", h.config.CodeProvider)
			log.Infof("Parsed AI model: %s, args: %s", aiModel, args)

			// Execute continue task asynchronously
			go func(event *github.IssueCommentEvent, aiModel, args string, traceCtx context.Context) {
				traceLog := xlog.NewWith(traceCtx)
				traceLog.Infof("Starting PR continue task with AI model: %s", aiModel)
				if err := h.agent.ContinuePRWithArgsAndAI(traceCtx, event, aiModel, args); err != nil {
					traceLog.Errorf("Agent continue PR error: %v", err)
				} else {
					traceLog.Infof("PR continue task completed successfully")
				}
			}(&event, aiModel, args, ctx)

			w.WriteHeader(http.StatusOK)
			w.Write([]byte("pr continue started"))
			return
		} else if strings.HasPrefix(comment, "/fix") {
			log.Infof("Received /fix command for PR #%d: %s", issueNumber, issueTitle)

			// Parse AI model parameters
			aiModel, args := parseCommandArgs(comment, "/fix", h.config.CodeProvider)
			log.Infof("Parsed AI model: %s, args: %s", aiModel, args)

			// Execute fix task asynchronously
			go func(event *github.IssueCommentEvent, aiModel, args string, traceCtx context.Context) {
				traceLog := xlog.NewWith(traceCtx)
				traceLog.Infof("Starting PR fix task with AI model: %s", aiModel)
				if err := h.agent.FixPRWithArgsAndAI(traceCtx, event, aiModel, args); err != nil {
					traceLog.Errorf("Agent fix PR error: %v", err)
				} else {
					traceLog.Infof("PR fix task completed successfully")
				}
			}(&event, aiModel, args, ctx)

			w.WriteHeader(http.StatusOK)
			w.Write([]byte("pr fix started"))
			return
		}
	}

	// Handle Issue's /code command
	if strings.HasPrefix(comment, "/code") {
		log.Infof("Received /code command for Issue: %s, title: %s",
			event.Issue.GetHTMLURL(), issueTitle)

		// Parse AI model parameters
		aiModel, args := parseCommandArgs(comment, "/code", h.config.CodeProvider)
		log.Infof("Parsed AI model: %s, args: %s", aiModel, args)

		// Execute Agent task asynchronously
		go func(event *github.IssueCommentEvent, aiModel, args string, traceCtx context.Context) {
			traceLog := xlog.NewWith(traceCtx)
			traceLog.Infof("Starting issue processing task with AI model: %s", aiModel)
			if err := h.agent.ProcessIssueCommentWithAI(traceCtx, event, aiModel, args); err != nil {
				traceLog.Errorf("Agent process issue error: %v", err)
			} else {
				traceLog.Infof("Issue processing task completed successfully")
			}
		}(&event, aiModel, args, ctx)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("issue processing started"))
		return
	}

	log.Debugf("No recognized command found in comment")
	w.WriteHeader(http.StatusOK)
}

// handlePRReviewComment handles PR code line comment events
func (h *Handler) handlePRReviewComment(ctx context.Context, w http.ResponseWriter, body []byte) {
	log := xlog.NewWith(ctx)

	var event github.PullRequestReviewCommentEvent
	if err := json.Unmarshal(body, &event); err != nil {
		log.Errorf("Failed to unmarshal PR review comment event: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid pr review comment event"))
		return
	}

	prNumber := event.PullRequest.GetNumber()
	prTitle := event.PullRequest.GetTitle()
	log.Infof("Received PR review comment for PR #%d: %s", prNumber, prTitle)

	// Check if contains interactive commands
	if event.Comment == nil || event.PullRequest == nil {
		log.Debugf("PR review comment event missing comment or pull request data")
		w.WriteHeader(http.StatusOK)
		return
	}

	comment := event.Comment.GetBody()
	filePath := event.Comment.GetPath()
	line := event.Comment.GetLine()
	log.Infof("Processing PR review comment: file=%s, line=%d, comment_length=%d",
		filePath, line, len(comment))

	if strings.HasPrefix(comment, "/continue") {
		log.Infof("Received /continue command in PR review comment for PR #%d: %s", prNumber, prTitle)

		// Parse AI model parameters
		aiModel, args := parseCommandArgs(comment, "/continue", h.config.CodeProvider)
		log.Infof("Parsed AI model: %s, args: %s", aiModel, args)

		// Execute continue task asynchronously
		go func(event *github.PullRequestReviewCommentEvent, aiModel, args string, traceCtx context.Context) {
			traceLog := xlog.NewWith(traceCtx)
			traceLog.Infof("Starting PR continue from review comment task with AI model: %s", aiModel)
			if err := h.agent.ContinuePRFromReviewCommentWithAI(traceCtx, event, aiModel, args); err != nil {
				traceLog.Errorf("Agent continue PR from review comment error: %v", err)
			} else {
				traceLog.Infof("PR continue from review comment task completed successfully")
			}
		}(&event, aiModel, args, ctx)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("pr continue from review comment started"))
		return
	} else if strings.HasPrefix(comment, "/fix") {
		log.Infof("Received /fix command in PR review comment for PR #%d: %s", prNumber, prTitle)

		// Parse AI model parameters
		aiModel, args := parseCommandArgs(comment, "/fix", h.config.CodeProvider)
		log.Infof("Parsed AI model: %s, args: %s", aiModel, args)

		// Execute fix task asynchronously
		go func(event *github.PullRequestReviewCommentEvent, aiModel, args string, traceCtx context.Context) {
			traceLog := xlog.NewWith(traceCtx)
			traceLog.Infof("Starting PR fix from review comment task with AI model: %s", aiModel)
			if err := h.agent.FixPRFromReviewCommentWithAI(traceCtx, event, aiModel, args); err != nil {
				traceLog.Errorf("Agent fix PR from review comment error: %v", err)
			} else {
				traceLog.Infof("PR fix from review comment task completed successfully")
			}
		}(&event, aiModel, args, ctx)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("pr fix from review comment started"))
		return
	}

	w.WriteHeader(http.StatusOK)
}

// handlePRReview handles PR review events
func (h *Handler) handlePRReview(ctx context.Context, w http.ResponseWriter, body []byte) {
	log := xlog.NewWith(ctx)

	var event github.PullRequestReviewEvent
	if err := json.Unmarshal(body, &event); err != nil {
		log.Errorf("Failed to unmarshal PR review event: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid pr review event"))
		return
	}

	prNumber := event.PullRequest.GetNumber()
	prTitle := event.PullRequest.GetTitle()
	reviewID := event.Review.GetID()
	reviewBody := event.Review.GetBody()

	action := event.GetAction()
	log.Infof("Received PR review for PR #%d: %s, review ID: %d, action: %s", prNumber, prTitle, reviewID, action)

	// Check if contains batch processing commands
	if event.Review == nil || event.PullRequest == nil {
		log.Debugf("PR review event missing review or pull request data")
		w.WriteHeader(http.StatusOK)
		return
	}

	// Only process "submitted" events, avoid duplicate triggering during editing or other operations
	if action != "submitted" {
		log.Debugf("Ignoring PR review event with action: %s (only process 'submitted' events)", action)
		w.WriteHeader(http.StatusOK)
		return
	}

	log.Infof("Processing PR review: review_body_length=%d", len(reviewBody))

	// Check if review body contains /continue or /fix commands
	if strings.HasPrefix(reviewBody, "/continue") || strings.HasPrefix(reviewBody, "/fix") {
		var command string
		var aiModel string
		var args string

		if strings.HasPrefix(reviewBody, "/continue") {
			command = "/continue"
			aiModel, args = parseCommandArgs(reviewBody, "/continue", h.config.CodeProvider)
		} else {
			command = "/fix"
			aiModel, args = parseCommandArgs(reviewBody, "/fix", h.config.CodeProvider)
		}

		log.Infof("Received %s command in PR review for PR #%d: %s", command, prNumber, prTitle)
		log.Infof("Parsed AI model: %s, args: %s", aiModel, args)

		// Get trigger user information for @mentioning user in AI feedback
		triggerUser := ""
		if event.Review != nil && event.Review.User != nil {
			triggerUser = event.Review.User.GetLogin()
		}

		// Execute batch processing task asynchronously
		go func(event *github.PullRequestReviewEvent, cmd string, aiModel, args string, triggerUser string, traceCtx context.Context) {
			traceLog := xlog.NewWith(traceCtx)
			traceLog.Infof("Starting PR batch processing from review task with AI model: %s", aiModel)
			if err := h.agent.ProcessPRFromReviewWithTriggerUserAndAI(traceCtx, event, cmd, aiModel, args, triggerUser); err != nil {
				traceLog.Errorf("Agent process PR from review error: %v", err)
			} else {
				traceLog.Infof("PR batch processing from review task completed successfully")
			}
		}(&event, command, aiModel, args, triggerUser, ctx)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("pr batch processing from review started"))
		return
	}

	log.Debugf("No recognized batch command found in review body")
	w.WriteHeader(http.StatusOK)
}

// handlePullRequest handles PR events
func (h *Handler) handlePullRequest(ctx context.Context, w http.ResponseWriter, body []byte) {
	log := xlog.NewWith(ctx)

	var event github.PullRequestEvent
	if err := json.Unmarshal(body, &event); err != nil {
		log.Errorf("Failed to unmarshal pull request event: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid pull request event"))
		return
	}

	action := event.GetAction()
	prNumber := event.PullRequest.GetNumber()
	prTitle := event.PullRequest.GetTitle()
	log.Infof("Pull request event: action=%s, number=%d, title=%s", action, prNumber, prTitle)

	// Handle based on PR action type
	switch action {
	case "opened":
		// PR created, can automatically review
		log.Infof("PR opened, starting review process")
		go func(pr *github.PullRequest, traceCtx context.Context) {
			traceLog := xlog.NewWith(traceCtx)
			traceLog.Infof("Starting PR review task")
			if err := h.agent.ReviewPR(traceCtx, pr); err != nil {
				traceLog.Errorf("Agent review PR error: %v", err)
			} else {
				traceLog.Infof("PR review task completed successfully")
			}
		}(event.PullRequest, ctx)
	case "synchronize":
		// PR has new commits, can re-review
		log.Infof("PR synchronized, starting re-review process")
		go func(pr *github.PullRequest, traceCtx context.Context) {
			traceLog := xlog.NewWith(traceCtx)
			traceLog.Infof("Starting PR re-review task")
			if err := h.agent.ReviewPR(traceCtx, pr); err != nil {
				traceLog.Errorf("Agent review PR error: %v", err)
			} else {
				traceLog.Infof("PR re-review task completed successfully")
			}
		}(event.PullRequest, ctx)
	case "closed":
		// PR closed, perform cleanup (regardless of whether merged)
		log.Infof("PR closed, starting cleanup process (merged: %v)", event.PullRequest.GetMerged())
		go func(pr *github.PullRequest, traceCtx context.Context) {
			traceLog := xlog.NewWith(traceCtx)
			traceLog.Infof("Starting PR cleanup task")
			if err := h.agent.CleanupAfterPRClosed(traceCtx, pr); err != nil {
				traceLog.Errorf("Agent cleanup after PR closed error: %v", err)
			} else {
				traceLog.Infof("PR cleanup task completed successfully")
			}
		}(event.PullRequest, ctx)
	default:
		log.Debugf("Unhandled PR action: %s", action)
		w.WriteHeader(http.StatusOK)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("pr review started"))
}

// handlePush handles Push events
func (h *Handler) handlePush(ctx context.Context, w http.ResponseWriter, body []byte) {
	log := xlog.NewWith(ctx)

	var event github.PushEvent
	if err := json.Unmarshal(body, &event); err != nil {
		log.Errorf("Failed to unmarshal push event: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid push event"))
		return
	}

	ref := event.GetRef()
	commitsCount := len(event.Commits)
	log.Infof("Push event received: ref=%s, commits_count=%d", ref, commitsCount)

	// Can handle code push events here
	// Such as automatically running tests, code quality checks, etc.

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("push event received"))
}
