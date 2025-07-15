package webhook

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/qbox/codeagent/internal/agent"
	"github.com/qbox/codeagent/internal/config"

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

// generateTraceID generates a unique trace ID for tracking requests
func generateTraceID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// getTraceID extracts trace ID from context
func getTraceID(ctx context.Context) string {
	if traceID, ok := reqid.FromContext(ctx); ok {
		return traceID
	}
	return "unknown"
}

// HandleWebhook 通用 Webhook 处理器
func (h *Handler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	// 生成跟踪ID
	traceID := generateTraceID()
	ctx := reqid.NewContext(context.Background(), traceID)
	xl := xlog.NewWith(ctx)

	xl.Infof("webhook_received: trace_id=%s method=%s url=%s", traceID, r.Method, r.URL.String())

	// 1. 验证 Webhook 签名（此处省略，建议用 X-Hub-Signature 校验）

	// 2. 获取事件类型
	eventType := r.Header.Get("X-GitHub-Event")
	if eventType == "" {
		xl.Errorf("missing_event_type: trace_id=%s", traceID)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("missing X-GitHub-Event header"))
		return
	}

	xl.Infof("event_type_identified: trace_id=%s event_type=%s", traceID, eventType)

	// 3. 读取请求体
	body, err := io.ReadAll(r.Body)
	if err != nil {
		xl.Errorf("body_read_failed: trace_id=%s error=%v", traceID, err)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid body"))
		return
	}

	xl.Infof("body_read_success: trace_id=%s body_length=%d", traceID, len(body))

	// 4. 根据事件类型分发处理
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
		xl.Warnf("unhandled_event_type: trace_id=%s event_type=%s", traceID, eventType)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("event type not handled"))
	}
}

// handleIssueComment 处理 Issue 评论事件
func (h *Handler) handleIssueComment(ctx context.Context, w http.ResponseWriter, body []byte) {
	xl := xlog.NewWith(ctx)
	traceID := getTraceID(ctx)
	
	xl.Infof("issue_comment_handler_start: trace_id=%s", traceID)
	
	var event github.IssueCommentEvent
	if err := json.Unmarshal(body, &event); err != nil {
		xl.Errorf("issue_comment_unmarshal_failed: trace_id=%s error=%v", traceID, err)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid issue comment event"))
		return
	}

	// 检查是否包含命令
	if event.Comment == nil || event.Issue == nil {
		xl.Infof("issue_comment_no_content: trace_id=%s", traceID)
		w.WriteHeader(http.StatusOK)
		return
	}

	comment := event.Comment.GetBody()
	issueNumber := event.Issue.GetNumber()
	issueTitle := event.Issue.GetTitle()

	xl.Infof("issue_comment_content: trace_id=%s issue_number=%d issue_title=%s comment_length=%d", traceID, issueNumber, issueTitle, len(comment))

	// 检查是否是 PR 评论（Issue 的 PullRequest 字段不为空）
	if event.Issue.PullRequestLinks != nil {
		xl.Info("pr_comment_detected", "trace_id", traceID, "pr_number", issueNumber)
		
		// 这是 PR 评论，处理 /continue 和 /fix 命令
		if strings.HasPrefix(comment, "/continue") {
			xl.Info("continue_command_received", "trace_id", traceID, "pr_number", issueNumber, "pr_title", issueTitle)

			// 提取命令参数
			commandArgs := strings.TrimSpace(strings.TrimPrefix(comment, "/continue"))

			// 异步执行继续任务
			go func(event *github.IssueCommentEvent, args string, traceID string) {
				if err := h.agent.ContinuePRWithArgs(ctx, event, args); err != nil {
					xl.Error("agent_continue_pr_error", "trace_id", traceID, "error", err)
				} else {
					xl.Info("agent_continue_pr_success", "trace_id", traceID)
				}
			}(&event, commandArgs, traceID)

			w.WriteHeader(http.StatusOK)
			w.Write([]byte("pr continue started"))
			return
		} else if strings.HasPrefix(comment, "/fix") {
			xl.Info("fix_command_received", "trace_id", traceID, "pr_number", issueNumber, "pr_title", issueTitle)

			// 提取命令参数
			commandArgs := strings.TrimSpace(strings.TrimPrefix(comment, "/fix"))

			// 异步执行修复任务
			go func(event *github.IssueCommentEvent, args string, traceID string) {
				if err := h.agent.FixPRWithArgs(ctx, event, args); err != nil {
					xl.Error("agent_fix_pr_error", "trace_id", traceID, "error", err)
				} else {
					xl.Info("agent_fix_pr_success", "trace_id", traceID)
				}
			}(&event, commandArgs, traceID)

			w.WriteHeader(http.StatusOK)
			w.Write([]byte("pr fix started"))
			return
		}
	}

	// 处理 Issue 的 /code 命令
	if strings.HasPrefix(comment, "/code") {
		xl.Info("code_command_received", "trace_id", traceID, 
			"issue_url", event.Issue.GetHTMLURL(),
			"issue_title", issueTitle,
			"issue_body_length", len(event.Issue.GetBody()))

		// 异步执行 Agent 任务
		go func(event *github.IssueCommentEvent, traceID string) {
			if err := h.agent.ProcessIssueComment(ctx, event); err != nil {
				xl.Error("agent_process_issue_error", "trace_id", traceID, "error", err)
			} else {
				xl.Info("agent_process_issue_success", "trace_id", traceID)
			}
		}(&event, traceID)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("issue processing started"))
		return
	}

	xl.Info("issue_comment_no_command", "trace_id", traceID)
	w.WriteHeader(http.StatusOK)
}

// handlePRReviewComment 处理 PR 代码行评论事件
func (h *Handler) handlePRReviewComment(ctx context.Context, w http.ResponseWriter, body []byte) {
	xl := xlog.NewWith(ctx)
	traceID := getTraceID(ctx)

	xl.Info("pr_review_comment_handler_start", "trace_id", traceID)

	var event github.PullRequestReviewCommentEvent
	if err := json.Unmarshal(body, &event); err != nil {
		xl.Error("pr_review_comment_unmarshal_failed", "trace_id", traceID, "error", err)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid pr review comment event"))
		return
	}

	prNumber := event.PullRequest.GetNumber()
	prTitle := event.PullRequest.GetTitle()
	xl.Info("pr_review_comment_received", "trace_id", traceID, "pr_number", prNumber, "pr_title", prTitle)

	// 检查是否包含交互命令
	if event.Comment == nil || event.PullRequest == nil {
		xl.Info("pr_review_comment_no_content", "trace_id", traceID)
		w.WriteHeader(http.StatusOK)
		return
	}

	comment := event.Comment.GetBody()
	commentPath := event.Comment.GetPath()
	commentLine := event.Comment.GetLine()

	xl.Info("pr_review_comment_content", "trace_id", traceID, "comment_length", len(comment), "file_path", commentPath, "line", commentLine)

	if strings.HasPrefix(comment, "/continue") {
		xl.Info("continue_command_in_review_comment", "trace_id", traceID, "pr_number", prNumber, "pr_title", prTitle)

		// 提取命令参数
		commandArgs := strings.TrimSpace(strings.TrimPrefix(comment, "/continue"))

		// 异步执行继续任务
		go func(event *github.PullRequestReviewCommentEvent, args string, traceID string) {
			if err := h.agent.ContinuePRFromReviewComment(ctx, event, args); err != nil {
				xl.Error("agent_continue_pr_from_review_comment_error", "trace_id", traceID, "error", err)
			} else {
				xl.Info("agent_continue_pr_from_review_comment_success", "trace_id", traceID)
			}
		}(&event, commandArgs, traceID)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("pr continue from review comment started"))
		return
	} else if strings.HasPrefix(comment, "/fix") {
		xl.Info("fix_command_in_review_comment", "trace_id", traceID, "pr_number", prNumber, "pr_title", prTitle)

		// 提取命令参数
		commandArgs := strings.TrimSpace(strings.TrimPrefix(comment, "/fix"))

		// 异步执行修复任务
		go func(event *github.PullRequestReviewCommentEvent, args string, traceID string) {
			if err := h.agent.FixPRFromReviewComment(ctx, event, args); err != nil {
				xl.Error("agent_fix_pr_from_review_comment_error", "trace_id", traceID, "error", err)
			} else {
				xl.Info("agent_fix_pr_from_review_comment_success", "trace_id", traceID)
			}
		}(&event, commandArgs, traceID)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("pr fix from review comment started"))
		return
	}

	xl.Info("pr_review_comment_no_command", "trace_id", traceID)
	w.WriteHeader(http.StatusOK)
}

// handlePullRequest 处理 PR 事件
func (h *Handler) handlePullRequest(ctx context.Context, w http.ResponseWriter, body []byte) {
	xl := xlog.NewWith(ctx)
	traceID := getTraceID(ctx)

	xl.Info("pull_request_handler_start", "trace_id", traceID)

	var event github.PullRequestEvent
	if err := json.Unmarshal(body, &event); err != nil {
		xl.Error("pull_request_unmarshal_failed", "trace_id", traceID, "error", err)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid pull request event"))
		return
	}

	action := event.GetAction()
	prNumber := event.PullRequest.GetNumber()
	prTitle := event.PullRequest.GetTitle()
	xl.Info("pull_request_event", "trace_id", traceID, "action", action, "pr_number", prNumber, "pr_title", prTitle)

	// 根据 PR 动作类型处理
	switch action {
	case "opened":
		// PR 被创建，可以自动审查
		xl.Info("pr_opened_event", "trace_id", traceID, "pr_number", prNumber)
		go func(pr *github.PullRequest, traceID string) {
			if err := h.agent.ReviewPR(ctx, pr); err != nil {
				xl.Error("agent_review_pr_error", "trace_id", traceID, "error", err)
			} else {
				xl.Info("agent_review_pr_success", "trace_id", traceID)
			}
		}(event.PullRequest, traceID)
	case "synchronize":
		// PR 有新的提交，可以重新审查
		xl.Info("pr_synchronized_event", "trace_id", traceID, "pr_number", prNumber)
		go func(pr *github.PullRequest, traceID string) {
			if err := h.agent.ReviewPR(ctx, pr); err != nil {
				xl.Error("agent_review_pr_sync_error", "trace_id", traceID, "error", err)
			} else {
				xl.Info("agent_review_pr_sync_success", "trace_id", traceID)
			}
		}(event.PullRequest, traceID)
	default:
		xl.Info("pr_action_ignored", "trace_id", traceID, "action", action)
		w.WriteHeader(http.StatusOK)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("pr review started"))
}

// handlePush 处理 Push 事件
func (h *Handler) handlePush(ctx context.Context, w http.ResponseWriter, body []byte) {
	xl := xlog.NewWith(ctx)
	traceID := getTraceID(ctx)

	xl.Info("push_handler_start", "trace_id", traceID)

	var event github.PushEvent
	if err := json.Unmarshal(body, &event); err != nil {
		xl.Error("push_event_unmarshal_failed", "trace_id", traceID, "error", err)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid push event"))
		return
	}

	ref := event.GetRef()
	commits := len(event.Commits)
	xl.Info("push_event_received", "trace_id", traceID, "ref", ref, "commits_count", commits)

	// 可以在这里处理代码推送事件
	// 比如自动运行测试、代码质量检查等

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("push event received"))
}
