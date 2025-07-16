package webhook

import (
	"context"
	"encoding/json"
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
	// 1. 验证 Webhook 签名（此处省略，建议用 X-Hub-Signature 校验）

	// 2. 获取事件类型
	eventType := r.Header.Get("X-GitHub-Event")
	if eventType == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("missing X-GitHub-Event header"))
		return
	}

	// 3. 创建追踪 ID 和上下文
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

	// 4. 读取请求体
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Errorf("Failed to read request body: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid body"))
		return
	}

	logger.Debugf("Request body size: %d bytes", len(body))

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
	
	log.Infof("Received PR review for PR #%d: %s, review ID: %d", prNumber, prTitle, reviewID)

	// 检查是否包含批量处理命令
	if event.Review == nil || event.PullRequest == nil {
		log.Debugf("PR review event missing review or pull request data")
		w.WriteHeader(http.StatusOK)
		return
	}

	log.Infof("Processing PR review: review_body_length=%d", len(reviewBody))

	// 检查 review body 是否包含 /continue 或 /fix 命令
	if strings.HasPrefix(reviewBody, "/continue") || strings.HasPrefix(reviewBody, "/fix") {
		var command string
		var commandArgs string
		
		if strings.HasPrefix(reviewBody, "/continue") {
			command = "/continue"
			commandArgs = strings.TrimSpace(strings.TrimPrefix(reviewBody, "/continue"))
		} else {
			command = "/fix"
			commandArgs = strings.TrimSpace(strings.TrimPrefix(reviewBody, "/fix"))
		}

		log.Infof("Received %s command in PR review for PR #%d: %s", command, prNumber, prTitle)
		log.Debugf("Command args: %s", commandArgs)

		// 异步执行批量处理任务
		go func(event *github.PullRequestReviewEvent, cmd string, args string, traceCtx context.Context) {
			traceLog := xlog.NewWith(traceCtx)
			traceLog.Infof("Starting PR batch processing from review task")
			if err := h.agent.ProcessPRFromReview(traceCtx, event, cmd, args); err != nil {
				traceLog.Errorf("Agent process PR from review error: %v", err)
			} else {
				traceLog.Infof("PR batch processing from review task completed successfully")
			}
		}(&event, command, commandArgs, ctx)

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
