package webhook

import (
	"context"
	"io"
	"net/http"

	"github.com/qiniu/codeagent/internal/agent"
	"github.com/qiniu/codeagent/internal/config"
	"github.com/qiniu/codeagent/pkg/signature"

	"github.com/qiniu/x/reqid"
	"github.com/qiniu/x/xlog"
)

type Handler struct {
	config        *config.Config
	enhancedAgent *agent.EnhancedAgent
}

func NewHandler(cfg *config.Config, enhancedAgent *agent.EnhancedAgent) *Handler {
	return &Handler{
		config:        cfg,
		enhancedAgent: enhancedAgent,
	}
}

// HandleWebhook webhook handler using Enhanced Agent
func (h *Handler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	xlog.New("").Infof("Using Enhanced Agent for webhook processing")
	h.handleEnhancedWebhook(w, r)
}

// handleEnhancedWebhook Enhanced Agent webhook处理 - 使用新的事件系统
func (h *Handler) handleEnhancedWebhook(w http.ResponseWriter, r *http.Request) {
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
	deliveryID := r.Header.Get("X-GitHub-Delivery")
	var traceID string
	if deliveryID != "" && len(deliveryID) > 8 {
		traceID = deliveryID[:8]
	} else if deliveryID != "" {
		traceID = deliveryID
	} else {
		traceID = "unknown"
	}

	ctx := reqid.NewContext(context.Background(), traceID)
	xl := xlog.NewWith(ctx)
	xl.Infof("Received webhook event via Enhanced Handler: %s", eventType)
	xl.Debugf("Request body size: %d bytes", len(body))

	// 5. 使用Enhanced Agent的统一事件处理，传递原始字节数据
	go func(eventType string, payload []byte, deliveryID string, traceCtx context.Context) {
		traceLog := xlog.NewWith(traceCtx)
		traceLog.Infof("Starting Enhanced Agent event processing: %s", eventType)

		if err := h.enhancedAgent.ProcessGitHubWebhookEvent(traceCtx, eventType, deliveryID, payload); err != nil {
			traceLog.Warnf("Enhanced Agent event processing error: %v", err)
		} else {
			traceLog.Infof("Enhanced Agent event processing completed successfully")
		}
	}(eventType, body, deliveryID, ctx)

	// 7. 返回成功响应
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("enhanced event processing started"))
}
