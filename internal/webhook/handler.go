package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/qbox/codeagent/internal/agent"
	"github.com/qbox/codeagent/internal/config"

	"github.com/google/go-github/v58/github"
	"github.com/qiniu/x/xlog"
)

type Handler struct {
	config *config.Config
	agent  *agent.Agent
}

func NewHandler(cfg *config.Config, agent *agent.Agent) *Handler {
	return &Handler{config: cfg, agent: agent}
}

// HandleWebhook 通用 Webhook 处理器
func (h *Handler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	// 1. 读取请求体（需要在验证签名之前读取）
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid body"))
		return
	}

	// 2. 验证 Webhook 签名
	if !h.validateSignature(r, body) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("invalid signature"))
		return
	}

	// 3. 获取事件类型
	eventType := r.Header.Get("X-GitHub-Event")
	if eventType == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("missing X-GitHub-Event header"))
		return
	}

	// 4. 创建追踪 ID 和上下文
	// 使用 X-GitHub-Delivery header 作为 trace ID，截短到前8位
	deliveryID := r.Header.Get("X-GitHub-Delivery")
	var traceID string
	if deliveryID != "" && len(deliveryID) > 8 {
		traceID = deliveryID[:8]
	} else if deliveryID != "" {
		traceID = deliveryID
	} else {
		traceID = "unknown"
	}

	logger := xlog.New(traceID)
	ctx := context.WithValue(context.Background(), "logger", logger)
	logger.Infof("Received webhook event: %s", eventType)
	logger.Debugf("Request body size: %d bytes", len(body))

	// 5. 根据事件类型分发处理
	switch eventType {
	case "issue_comment":
		h.handleIssueComment(ctx, w, body)
	case "pull_request_review_comment":
		h.handlePRReviewComment(ctx, w, body)
	case "pull_request":
		h.handlePullRequest(ctx, w, body)
	case "push":
		h.handlePush(ctx, w, body)
	default:
		logger.Warnf("Unhandled event type: %s", eventType)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("event type not handled"))
	}
}

// handleIssueComment 处理 Issue 评论事件
func (h *Handler) handleIssueComment(ctx context.Context, w http.ResponseWriter, body []byte) {
	log := xlog.NewWith(ctx)

	var event github.IssueCommentEvent
	if err := json.Unmarshal(body, &event); err != nil {
		log.Errorf("Failed to unmarshal issue comment event: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid issue comment event"))
		return
	}

	// 检查是否包含命令
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

	// 检查是否是 PR 评论（Issue 的 PullRequest 字段不为空）
	if event.Issue.PullRequestLinks != nil {
		log.Infof("Detected PR comment for PR #%d", issueNumber)

		// 这是 PR 评论，处理 /continue 和 /fix 命令
		if strings.HasPrefix(comment, "/continue") {
			log.Infof("Received /continue command for PR #%d: %s", issueNumber, issueTitle)

			// 提取命令参数
			commandArgs := strings.TrimSpace(strings.TrimPrefix(comment, "/continue"))
			log.Debugf("Command args: %s", commandArgs)

			// 异步执行继续任务
			go func(event *github.IssueCommentEvent, args string, traceCtx context.Context) {
				traceLog := xlog.NewWith(traceCtx)
				traceLog.Infof("Starting PR continue task")
				if err := h.agent.ContinuePRWithArgs(traceCtx, event, args); err != nil {
					traceLog.Errorf("Agent continue PR error: %v", err)
				} else {
					traceLog.Infof("PR continue task completed successfully")
				}
			}(&event, commandArgs, ctx)

			w.WriteHeader(http.StatusOK)
			w.Write([]byte("pr continue started"))
			return
		} else if strings.HasPrefix(comment, "/fix") {
			log.Infof("Received /fix command for PR #%d: %s", issueNumber, issueTitle)

			// 提取命令参数
			commandArgs := strings.TrimSpace(strings.TrimPrefix(comment, "/fix"))
			log.Debugf("Command args: %s", commandArgs)

			// 异步执行修复任务
			go func(event *github.IssueCommentEvent, args string, traceCtx context.Context) {
				traceLog := xlog.NewWith(traceCtx)
				traceLog.Infof("Starting PR fix task")
				if err := h.agent.FixPRWithArgs(traceCtx, event, args); err != nil {
					traceLog.Errorf("Agent fix PR error: %v", err)
				} else {
					traceLog.Infof("PR fix task completed successfully")
				}
			}(&event, commandArgs, ctx)

			w.WriteHeader(http.StatusOK)
			w.Write([]byte("pr fix started"))
			return
		}
	}

	// 处理 Issue 的 /code 命令
	if strings.HasPrefix(comment, "/code") {
		log.Infof("Received /code command for Issue: %s, title: %s",
			event.Issue.GetHTMLURL(), issueTitle)

		// 异步执行 Agent 任务
		go func(event *github.IssueCommentEvent, traceCtx context.Context) {
			traceLog := xlog.NewWith(traceCtx)
			traceLog.Infof("Starting issue processing task")
			if err := h.agent.ProcessIssueComment(traceCtx, event); err != nil {
				traceLog.Errorf("Agent process issue error: %v", err)
			} else {
				traceLog.Infof("Issue processing task completed successfully")
			}
		}(&event, ctx)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("issue processing started"))
		return
	}

	log.Debugf("No recognized command found in comment")
	w.WriteHeader(http.StatusOK)
}

// handlePRReviewComment 处理 PR 代码行评论事件
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

	// 检查是否包含交互命令
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

		// 提取命令参数
		commandArgs := strings.TrimSpace(strings.TrimPrefix(comment, "/continue"))
		log.Debugf("Command args: %s", commandArgs)

		// 异步执行继续任务
		go func(event *github.PullRequestReviewCommentEvent, args string, traceCtx context.Context) {
			traceLog := xlog.NewWith(traceCtx)
			traceLog.Infof("Starting PR continue from review comment task")
			if err := h.agent.ContinuePRFromReviewComment(traceCtx, event, args); err != nil {
				traceLog.Errorf("Agent continue PR from review comment error: %v", err)
			} else {
				traceLog.Infof("PR continue from review comment task completed successfully")
			}
		}(&event, commandArgs, ctx)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("pr continue from review comment started"))
		return
	} else if strings.HasPrefix(comment, "/fix") {
		log.Infof("Received /fix command in PR review comment for PR #%d: %s", prNumber, prTitle)

		// 提取命令参数
		commandArgs := strings.TrimSpace(strings.TrimPrefix(comment, "/fix"))
		log.Debugf("Command args: %s", commandArgs)

		// 异步执行修复任务
		go func(event *github.PullRequestReviewCommentEvent, args string, traceCtx context.Context) {
			traceLog := xlog.NewWith(traceCtx)
			traceLog.Infof("Starting PR fix from review comment task")
			if err := h.agent.FixPRFromReviewComment(traceCtx, event, args); err != nil {
				traceLog.Errorf("Agent fix PR from review comment error: %v", err)
			} else {
				traceLog.Infof("PR fix from review comment task completed successfully")
			}
		}(&event, commandArgs, ctx)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("pr fix from review comment started"))
		return
	}

	w.WriteHeader(http.StatusOK)
}

// handlePullRequest 处理 PR 事件
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

	// 根据 PR 动作类型处理
	switch action {
	case "opened":
		// PR 被创建，可以自动审查
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
		// PR 有新的提交，可以重新审查
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
		// PR 被关闭，若已合并则清理
		if event.PullRequest.GetMerged() {
			log.Infof("PR closed and merged, starting cleanup process")
			go func(pr *github.PullRequest, traceCtx context.Context) {
				traceLog := xlog.NewWith(traceCtx)
				traceLog.Infof("Starting PR cleanup task")
				if err := h.agent.CleanupAfterPRMerged(traceCtx, pr); err != nil {
					traceLog.Errorf("Agent cleanup after PR merged error: %v", err)
				} else {
					traceLog.Infof("PR cleanup task completed successfully")
				}
			}(event.PullRequest, ctx)
		} else {
			log.Infof("PR closed but not merged, no cleanup needed")
		}
	default:
		log.Debugf("Unhandled PR action: %s", action)
		w.WriteHeader(http.StatusOK)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("pr review started"))
}

// handlePush 处理 Push 事件
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

	// 可以在这里处理代码推送事件
	// 比如自动运行测试、代码质量检查等

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("push event received"))
}

// validateSignature validates the GitHub webhook signature
func (h *Handler) validateSignature(r *http.Request, body []byte) bool {
	// Skip validation if no webhook secret is configured
	if h.config.Server.WebhookSecret == "" {
		return true
	}

	signature := r.Header.Get("X-Hub-Signature-256")
	if signature == "" {
		// Fall back to old signature header
		signature = r.Header.Get("X-Hub-Signature")
		if signature == "" {
			return false
		}
	}

	// Remove the "sha256=" or "sha1=" prefix
	var actualSignature string
	if strings.HasPrefix(signature, "sha256=") {
		actualSignature = strings.TrimPrefix(signature, "sha256=")
	} else if strings.HasPrefix(signature, "sha1=") {
		actualSignature = strings.TrimPrefix(signature, "sha1=")
	} else {
		return false
	}

	// Generate expected signature
	mac := hmac.New(sha256.New, []byte(h.config.Server.WebhookSecret))
	mac.Write(body)
	expectedSignature := hex.EncodeToString(mac.Sum(nil))

	// Compare signatures
	return hmac.Equal([]byte(actualSignature), []byte(expectedSignature))
}
