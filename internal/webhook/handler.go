package webhook

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/qbox/codeagent/internal/agent"
	"github.com/qbox/codeagent/internal/config"

	"github.com/google/go-github/v58/github"
	"github.com/qiniu/x/log"
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

	// 3. 读取请求体
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid body"))
		return
	}

	// 4. 根据事件类型分发处理
	switch eventType {
	case "issue_comment":
		h.handleIssueComment(w, body)
	case "pull_request_review_comment":
		h.handlePRReviewComment(w, body)
	case "pull_request":
		h.handlePullRequest(w, body)
	case "push":
		h.handlePush(w, body)
	default:
		log.Printf("Unhandled event type: %s", eventType)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("event type not handled"))
	}
}

// handleIssueComment 处理 Issue 评论事件
func (h *Handler) handleIssueComment(w http.ResponseWriter, body []byte) {
	var event github.IssueCommentEvent
	if err := json.Unmarshal(body, &event); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid issue comment event"))
		return
	}

	// 检查是否包含命令
	if event.Comment == nil || event.Issue == nil {
		w.WriteHeader(http.StatusOK)
		return
	}

	comment := event.Comment.GetBody()

	// 检查是否是 PR 评论（Issue 的 PullRequest 字段不为空）
	if event.Issue.PullRequestLinks != nil {
		// 这是 PR 评论，处理 /continue 和 /fix 命令
		if strings.HasPrefix(comment, "/continue") {
			log.Infof("Received /continue command for PR #%d: %s",
				event.Issue.GetNumber(), event.Issue.GetTitle())

			// 提取命令参数
			commandArgs := strings.TrimSpace(strings.TrimPrefix(comment, "/continue"))

			// 异步执行继续任务
			go func(event *github.IssueCommentEvent, args string) {
				if err := h.agent.ContinuePRWithArgs(event, args); err != nil {
					log.Printf("agent continue pr error: %v", err)
				}
			}(&event, commandArgs)

			w.WriteHeader(http.StatusOK)
			w.Write([]byte("pr continue started"))
			return
		} else if strings.HasPrefix(comment, "/fix") {
			log.Infof("Received /fix command for PR #%d: %s",
				event.Issue.GetNumber(), event.Issue.GetTitle())

			// 提取命令参数
			commandArgs := strings.TrimSpace(strings.TrimPrefix(comment, "/fix"))

			// 异步执行修复任务
			go func(event *github.IssueCommentEvent, args string) {
				if err := h.agent.FixPRWithArgs(event, args); err != nil {
					log.Printf("agent fix pr error: %v", err)
				}
			}(&event, commandArgs)

			w.WriteHeader(http.StatusOK)
			w.Write([]byte("pr fix started"))
			return
		}
	}

	// 处理 Issue 的 /code 命令
	if strings.HasPrefix(comment, "/code") {
		log.Infof("Received /code command for Issue: %s, title: %s, body: %s",
			event.Issue.GetHTMLURL(),
			event.Issue.GetTitle(),
			event.Issue.GetBody(),
		)

		// 异步执行 Agent 任务
		go func(event *github.IssueCommentEvent) {
			if err := h.agent.ProcessIssueComment(event); err != nil {
				log.Printf("agent process issue error: %v", err)
			}
		}(&event)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("issue processing started"))
		return
	}

	w.WriteHeader(http.StatusOK)
}

// handlePRReviewComment 处理 PR 代码行评论事件
func (h *Handler) handlePRReviewComment(w http.ResponseWriter, body []byte) {
	var event github.PullRequestReviewCommentEvent
	if err := json.Unmarshal(body, &event); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid pr review comment event"))
		return
	}

	log.Infof("Received PR review comment for PR #%d: %s",
		event.PullRequest.GetNumber(),
		event.PullRequest.GetTitle())

	// 检查是否包含交互命令
	if event.Comment == nil || event.PullRequest == nil {
		w.WriteHeader(http.StatusOK)
		return
	}

	comment := event.Comment.GetBody()
	if strings.HasPrefix(comment, "/continue") {
		log.Infof("Received /continue command in PR review comment for PR #%d: %s",
			event.PullRequest.GetNumber(), event.PullRequest.GetTitle())

		// 提取命令参数
		commandArgs := strings.TrimSpace(strings.TrimPrefix(comment, "/continue"))

		// 异步执行继续任务
		go func(event *github.PullRequestReviewCommentEvent, args string) {
			if err := h.agent.ContinuePRFromReviewComment(event, args); err != nil {
				log.Errorf("agent continue pr from review comment error: %v", err)
			}
		}(&event, commandArgs)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("pr continue from review comment started"))
		return
	} else if strings.HasPrefix(comment, "/fix") {
		log.Infof("Received /fix command in PR review comment for PR #%d: %s",
			event.PullRequest.GetNumber(), event.PullRequest.GetTitle())

		// 提取命令参数
		commandArgs := strings.TrimSpace(strings.TrimPrefix(comment, "/fix"))

		// 异步执行修复任务
		go func(event *github.PullRequestReviewCommentEvent, args string) {
			if err := h.agent.FixPRFromReviewComment(event, args); err != nil {
				log.Errorf("agent fix pr from review comment error: %v", err)
			}
		}(&event, commandArgs)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("pr fix from review comment started"))
		return
	}

	w.WriteHeader(http.StatusOK)
}

// handlePullRequest 处理 PR 事件
func (h *Handler) handlePullRequest(w http.ResponseWriter, body []byte) {
	var event github.PullRequestEvent
	if err := json.Unmarshal(body, &event); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid pull request event"))
		return
	}

	log.Infof("pull request event, action: %s, number: %d, title: %s", event.GetAction(), event.PullRequest.GetNumber(), event.PullRequest.GetTitle())

	// 根据 PR 动作类型处理
	switch event.GetAction() {
	case "opened":
		// PR 被创建，可以自动审查
		go func(pr *github.PullRequest) {
			if err := h.agent.ReviewPR(pr); err != nil {
				log.Errorf("agent review pr error: %v", err)
			}
		}(event.PullRequest)
	case "synchronize":
		// PR 有新的提交，可以重新审查
		go func(pr *github.PullRequest) {
			if err := h.agent.ReviewPR(pr); err != nil {
				log.Errorf("agent review pr error: %v", err)
			}
		}(event.PullRequest)
	case "closed":
		// PR 被关闭，若已合并则清理
		if event.PullRequest.GetMerged() {
			go func(pr *github.PullRequest) {
				if err := h.agent.CleanupAfterPRMerged(pr); err != nil {
					log.Errorf("agent cleanup after pr merged error: %v", err)
				}
			}(event.PullRequest)
		}
	default:
		w.WriteHeader(http.StatusOK)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("pr review started"))
}

// handlePush 处理 Push 事件
func (h *Handler) handlePush(w http.ResponseWriter, body []byte) {
	var event github.PushEvent
	if err := json.Unmarshal(body, &event); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid push event"))
		return
	}

	// 可以在这里处理代码推送事件
	// 比如自动运行测试、代码质量检查等

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("push event received"))
}
