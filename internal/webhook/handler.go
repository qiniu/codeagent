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

// parseCommandArgs 解析命令参数，提取AI模型和其他参数
func parseCommandArgs(comment, command string, defaultAIModel string) (aiModel, args string) {
	// 提取命令参数
	commandArgs := strings.TrimSpace(strings.TrimPrefix(comment, command))

	// 检查是否包含AI模型参数
	if strings.HasPrefix(commandArgs, "-claude") {
		aiModel = "claude"
		args = strings.TrimSpace(strings.TrimPrefix(commandArgs, "-claude"))
	} else if strings.HasPrefix(commandArgs, "-gemini") {
		aiModel = "gemini"
		args = strings.TrimSpace(strings.TrimPrefix(commandArgs, "-gemini"))
	} else {
		// 没有指定AI模型，使用默认配置
		aiModel = defaultAIModel
		args = commandArgs
	}

	return aiModel, args
}

// HandleWebhook 通用 Webhook 处理器
func (h *Handler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	// 1. 读取请求体 (需要在签名验证前读取)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}

	// 2. 验证 Webhook 签名
	if h.config.Server.WebhookSecret != "" {
		// 优先使用 SHA-256 签名
		sig256 := r.Header.Get("X-Hub-Signature-256")
		if sig256 != "" {
			if err := signature.ValidateGitHubSignature(sig256, body, h.config.Server.WebhookSecret); err != nil {
				http.Error(w, "invalid signature", http.StatusUnauthorized)
				return
			}
		} else {
			// 如果没有 SHA-256 签名，尝试 SHA-1 签名 (已弃用但仍支持)
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

	// 使用 reqid.NewContext 将 traceID 存储到 context 中
	ctx := reqid.NewContext(context.Background(), traceID)
	xl := xlog.NewWith(ctx)
	xl.Infof("Received webhook event: %s", eventType)
	xl.Debugf("Request body size: %d bytes", len(body))

	// 5. 根据事件类型分发处理
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

	if event.Issue.IsPullRequest() {
		log.Infof("Detected PR comment for PR #%d", issueNumber)

		// 这是 PR 评论，处理 /continue 和 /fix 命令
		if strings.HasPrefix(comment, "/continue") {
			log.Infof("Received /continue command for PR #%d: %s", issueNumber, issueTitle)

			// 解析AI模型参数
			aiModel, args := parseCommandArgs(comment, "/continue", h.config.CodeProvider)
			log.Infof("Parsed AI model: %s, args: %s", aiModel, args)

			// 异步执行继续任务
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

			// 解析AI模型参数
			aiModel, args := parseCommandArgs(comment, "/fix", h.config.CodeProvider)
			log.Infof("Parsed AI model: %s, args: %s", aiModel, args)

			// 异步执行修复任务
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

	// Handle /code command for issues only (not for PRs)
	if strings.HasPrefix(comment, "/code") && !event.Issue.IsPullRequest() {
		log.Infof("Received /code command for Issue: %s, title: %s",
			event.Issue.GetHTMLURL(), issueTitle)

		// 解析AI模型参数
		aiModel, args := parseCommandArgs(comment, "/code", h.config.CodeProvider)
		log.Infof("Parsed AI model: %s, args: %s", aiModel, args)

		// 异步执行 Agent 任务
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

		// 解析AI模型参数
		aiModel, args := parseCommandArgs(comment, "/continue", h.config.CodeProvider)
		log.Infof("Parsed AI model: %s, args: %s", aiModel, args)

		// 异步执行继续任务
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

		// 解析AI模型参数
		aiModel, args := parseCommandArgs(comment, "/fix", h.config.CodeProvider)
		log.Infof("Parsed AI model: %s, args: %s", aiModel, args)

		// 异步执行修复任务
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

// handlePRReview 处理 PR review 事件
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

	// 检查是否包含批量处理命令
	if event.Review == nil || event.PullRequest == nil {
		log.Debugf("PR review event missing review or pull request data")
		w.WriteHeader(http.StatusOK)
		return
	}

	// 只处理 "submitted" 事件，避免在编辑或其他操作时重复触发
	if action != "submitted" {
		log.Debugf("Ignoring PR review event with action: %s (only process 'submitted' events)", action)
		w.WriteHeader(http.StatusOK)
		return
	}

	log.Infof("Processing PR review: review_body_length=%d", len(reviewBody))

	// 检查 review body 是否包含 /continue 或 /fix 命令
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

		// 获取触发用户信息，用于在AI反馈中@用户
		triggerUser := ""
		if event.Review != nil && event.Review.User != nil {
			triggerUser = event.Review.User.GetLogin()
		}

		// 异步执行批量处理任务
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
		// PR 被关闭，执行清理（无论是否合并）
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
