package webhook

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/qbox/codeagent/internal/agent"
	"github.com/qbox/codeagent/internal/config"
	"github.com/qbox/codeagent/internal/trace"

	"github.com/google/go-github/v58/github"
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
	var traceID trace.TraceID
	var ctx context.Context
	
	switch eventType {
	case "issue_comment":
		traceID = trace.NewTraceID(trace.IssueCommentPrefix)
	case "pull_request_review_comment":
		traceID = trace.NewTraceID(trace.PRReviewPrefix)
	case "pull_request":
		traceID = trace.NewTraceID(trace.PullRequestPrefix)
	case "push":
		traceID = trace.NewTraceID(trace.PushPrefix)
	default:
		traceID = trace.NewTraceID("unknown")
	}
	
	ctx = trace.NewContext(context.Background(), traceID)
	trace.Info(ctx, "Received webhook event: %s", eventType)

	// 4. 读取请求体
	body, err := io.ReadAll(r.Body)
	if err != nil {
		trace.Error(ctx, "Failed to read request body: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid body"))
		return
	}

	trace.Debug(ctx, "Request body size: %d bytes", len(body))

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
		trace.Warn(ctx, "Unhandled event type: %s", eventType)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("event type not handled"))
	}
}

// handleIssueComment 处理 Issue 评论事件
func (h *Handler) handleIssueComment(ctx context.Context, w http.ResponseWriter, body []byte) {
	var event github.IssueCommentEvent
	if err := json.Unmarshal(body, &event); err != nil {
		trace.Error(ctx, "Failed to unmarshal issue comment event: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid issue comment event"))
		return
	}

	// 检查是否包含命令
	if event.Comment == nil || event.Issue == nil {
		trace.Debug(ctx, "Issue comment event missing comment or issue data")
		w.WriteHeader(http.StatusOK)
		return
	}

	comment := event.Comment.GetBody()
	issueNumber := event.Issue.GetNumber()
	issueTitle := event.Issue.GetTitle()

	trace.Info(ctx, "Processing issue comment: issue=#%d, title=%s, comment_length=%d", 
		issueNumber, issueTitle, len(comment))

	// 检查是否是 PR 评论（Issue 的 PullRequest 字段不为空）
	if event.Issue.PullRequestLinks != nil {
		trace.Info(ctx, "Detected PR comment for PR #%d", issueNumber)
		
		// 这是 PR 评论，处理 /continue 和 /fix 命令
		if strings.HasPrefix(comment, "/continue") {
			trace.Info(ctx, "Received /continue command for PR #%d: %s", issueNumber, issueTitle)

			// 提取命令参数
			commandArgs := strings.TrimSpace(strings.TrimPrefix(comment, "/continue"))
			trace.Debug(ctx, "Command args: %s", commandArgs)

			// 异步执行继续任务
			go func(event *github.IssueCommentEvent, args string, traceCtx context.Context) {
				trace.Info(traceCtx, "Starting PR continue task")
				if err := h.agent.ContinuePRWithArgs(traceCtx, event, args); err != nil {
					trace.Error(traceCtx, "Agent continue PR error: %v", err)
				} else {
					trace.Info(traceCtx, "PR continue task completed successfully")
				}
			}(&event, commandArgs, ctx)

			w.WriteHeader(http.StatusOK)
			w.Write([]byte("pr continue started"))
			return
		} else if strings.HasPrefix(comment, "/fix") {
			trace.Info(ctx, "Received /fix command for PR #%d: %s", issueNumber, issueTitle)

			// 提取命令参数
			commandArgs := strings.TrimSpace(strings.TrimPrefix(comment, "/fix"))
			trace.Debug(ctx, "Command args: %s", commandArgs)

			// 异步执行修复任务
			go func(event *github.IssueCommentEvent, args string, traceCtx context.Context) {
				trace.Info(traceCtx, "Starting PR fix task")
				if err := h.agent.FixPRWithArgs(traceCtx, event, args); err != nil {
					trace.Error(traceCtx, "Agent fix PR error: %v", err)
				} else {
					trace.Info(traceCtx, "PR fix task completed successfully")
				}
			}(&event, commandArgs, ctx)

			w.WriteHeader(http.StatusOK)
			w.Write([]byte("pr fix started"))
			return
		}
	}

	// 处理 Issue 的 /code 命令
	if strings.HasPrefix(comment, "/code") {
		trace.Info(ctx, "Received /code command for Issue: %s, title: %s", 
			event.Issue.GetHTMLURL(), issueTitle)

		// 异步执行 Agent 任务
		go func(event *github.IssueCommentEvent, traceCtx context.Context) {
			trace.Info(traceCtx, "Starting issue processing task")
			if err := h.agent.ProcessIssueComment(traceCtx, event); err != nil {
				trace.Error(traceCtx, "Agent process issue error: %v", err)
			} else {
				trace.Info(traceCtx, "Issue processing task completed successfully")
			}
		}(&event, ctx)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("issue processing started"))
		return
	}

	trace.Debug(ctx, "No recognized command found in comment")
	w.WriteHeader(http.StatusOK)
}

// handlePRReviewComment 处理 PR 代码行评论事件
func (h *Handler) handlePRReviewComment(ctx context.Context, w http.ResponseWriter, body []byte) {
	var event github.PullRequestReviewCommentEvent
	if err := json.Unmarshal(body, &event); err != nil {
		trace.Error(ctx, "Failed to unmarshal PR review comment event: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid pr review comment event"))
		return
	}

	prNumber := event.PullRequest.GetNumber()
	prTitle := event.PullRequest.GetTitle()
	trace.Info(ctx, "Received PR review comment for PR #%d: %s", prNumber, prTitle)

	// 检查是否包含交互命令
	if event.Comment == nil || event.PullRequest == nil {
		trace.Debug(ctx, "PR review comment event missing comment or pull request data")
		w.WriteHeader(http.StatusOK)
		return
	}

	comment := event.Comment.GetBody()
	filePath := event.Comment.GetPath()
	line := event.Comment.GetLine()
	trace.Info(ctx, "Processing PR review comment: file=%s, line=%d, comment_length=%d", 
		filePath, line, len(comment))

	if strings.HasPrefix(comment, "/continue") {
		trace.Info(ctx, "Received /continue command in PR review comment for PR #%d: %s", prNumber, prTitle)

		// 提取命令参数
		commandArgs := strings.TrimSpace(strings.TrimPrefix(comment, "/continue"))
		trace.Debug(ctx, "Command args: %s", commandArgs)

		// 异步执行继续任务
		go func(event *github.PullRequestReviewCommentEvent, args string, traceCtx context.Context) {
			trace.Info(traceCtx, "Starting PR continue from review comment task")
			if err := h.agent.ContinuePRFromReviewComment(traceCtx, event, args); err != nil {
				trace.Error(traceCtx, "Agent continue PR from review comment error: %v", err)
			} else {
				trace.Info(traceCtx, "PR continue from review comment task completed successfully")
			}
		}(&event, commandArgs, ctx)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("pr continue from review comment started"))
		return
	} else if strings.HasPrefix(comment, "/fix") {
		trace.Info(ctx, "Received /fix command in PR review comment for PR #%d: %s", prNumber, prTitle)

		// 提取命令参数
		commandArgs := strings.TrimSpace(strings.TrimPrefix(comment, "/fix"))
		trace.Debug(ctx, "Command args: %s", commandArgs)

		// 异步执行修复任务
		go func(event *github.PullRequestReviewCommentEvent, args string, traceCtx context.Context) {
			trace.Info(traceCtx, "Starting PR fix from review comment task")
			if err := h.agent.FixPRFromReviewComment(traceCtx, event, args); err != nil {
				trace.Error(traceCtx, "Agent fix PR from review comment error: %v", err)
			} else {
				trace.Info(traceCtx, "PR fix from review comment task completed successfully")
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
	var event github.PullRequestEvent
	if err := json.Unmarshal(body, &event); err != nil {
		trace.Error(ctx, "Failed to unmarshal pull request event: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid pull request event"))
		return
	}

	action := event.GetAction()
	prNumber := event.PullRequest.GetNumber()
	prTitle := event.PullRequest.GetTitle()
	trace.Info(ctx, "Pull request event: action=%s, number=%d, title=%s", action, prNumber, prTitle)

	// 根据 PR 动作类型处理
	switch action {
	case "opened":
		// PR 被创建，可以自动审查
		trace.Info(ctx, "PR opened, starting review process")
		go func(pr *github.PullRequest, traceCtx context.Context) {
			trace.Info(traceCtx, "Starting PR review task")
			if err := h.agent.ReviewPR(traceCtx, pr); err != nil {
				trace.Error(traceCtx, "Agent review PR error: %v", err)
			} else {
				trace.Info(traceCtx, "PR review task completed successfully")
			}
		}(event.PullRequest, ctx)
	case "synchronize":
		// PR 有新的提交，可以重新审查
		trace.Info(ctx, "PR synchronized, starting re-review process")
		go func(pr *github.PullRequest, traceCtx context.Context) {
			trace.Info(traceCtx, "Starting PR re-review task")
			if err := h.agent.ReviewPR(traceCtx, pr); err != nil {
				trace.Error(traceCtx, "Agent review PR error: %v", err)
			} else {
				trace.Info(traceCtx, "PR re-review task completed successfully")
			}
		}(event.PullRequest, ctx)
	case "closed":
		// PR 被关闭，若已合并则清理
		if event.PullRequest.GetMerged() {
			trace.Info(ctx, "PR closed and merged, starting cleanup process")
			go func(pr *github.PullRequest, traceCtx context.Context) {
				trace.Info(traceCtx, "Starting PR cleanup task")
				if err := h.agent.CleanupAfterPRMerged(traceCtx, pr); err != nil {
					trace.Error(traceCtx, "Agent cleanup after PR merged error: %v", err)
				} else {
					trace.Info(traceCtx, "PR cleanup task completed successfully")
				}
			}(event.PullRequest, ctx)
		} else {
			trace.Info(ctx, "PR closed but not merged, no cleanup needed")
		}
	default:
		trace.Debug(ctx, "Unhandled PR action: %s", action)
		w.WriteHeader(http.StatusOK)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("pr review started"))
}

// handlePush 处理 Push 事件
func (h *Handler) handlePush(ctx context.Context, w http.ResponseWriter, body []byte) {
	var event github.PushEvent
	if err := json.Unmarshal(body, &event); err != nil {
		trace.Error(ctx, "Failed to unmarshal push event: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid push event"))
		return
	}

	ref := event.GetRef()
	commitsCount := len(event.Commits)
	trace.Info(ctx, "Push event received: ref=%s, commits_count=%d", ref, commitsCount)

	// 可以在这里处理代码推送事件
	// 比如自动运行测试、代码质量检查等

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("push event received"))
}
