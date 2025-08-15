package webhook

import (
	"context"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/qiniu/codeagent/internal/agent"
	"github.com/qiniu/codeagent/internal/config"
	"github.com/qiniu/codeagent/internal/workspace"

	"github.com/qiniu/x/reqid"
	"github.com/qiniu/x/xlog"
)

type Handler struct {
	config           *config.Config
	workspaceManager *workspace.Manager
}

// NewHandler creates webhook handler with factory pattern for EnhancedAgent
func NewHandler(cfg *config.Config, workspaceManager *workspace.Manager) *Handler {
	return &Handler{
		config:           cfg,
		workspaceManager: workspaceManager,
	}
}

// parseCommandArgs parses command arguments, extracts AI model and other parameters
func parseCommandArgs(comment, command string, defaultAIModel string) (aiModel, args string) {
	// Extract command arguments
	commandArgs := strings.TrimSpace(strings.TrimPrefix(comment, command))

	// Check if AI model parameters are included
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

// HandleWebhook Webhook 处理器 - 使用 Enhanced Agent
func (h *Handler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
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

	// 2. 智能验证 Webhook 签名（复用统一的签名验证逻辑）
	if err := h.ValidateWebhookSignature(r, body); err != nil {
		log.Printf("Enhanced webhook signature validation failed: %v", err)
		http.Error(w, "invalid signature", http.StatusUnauthorized)
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
	xl.Debugf("Received webhook event: %s (size: %d bytes)", eventType, len(body))

	// 5. 提取 installation ID
	installationID, err := agent.ExtractInstallationIDFromPayload(body)
	if err != nil {
		xl.Debugf("Failed to extract installation ID: %v, continuing with PAT mode", err)
		installationID = 0
	}

	// 6. 使用新架构：为每个请求创建专用的EnhancedAgent
	go func(eventType string, payload []byte, deliveryID string, installationID int64, traceCtx context.Context) {
		traceLog := xlog.NewWith(traceCtx)
		traceLog.Debugf("Starting event processing: %s (installation_id=%d)", eventType, installationID)

		// 为该请求创建专用的EnhancedAgent
		enhancedAgent, err := agent.NewEnhancedAgent(h.config, h.workspaceManager, installationID)
		if err != nil {
			traceLog.Errorf("Failed to create EnhancedAgent for installation %d: %v", installationID, err)
			return
		}
		defer func() {
			if shutdownErr := enhancedAgent.Shutdown(traceCtx); shutdownErr != nil {
				traceLog.Warnf("Failed to shutdown EnhancedAgent: %v", shutdownErr)
			}
		}()

		if err := enhancedAgent.ProcessGitHubWebhookEvent(traceCtx, eventType, deliveryID, payload); err != nil {
			// 对于不支持的事件类型，使用DEBUG级别
			if strings.Contains(err.Error(), "unsupported event type") {
				traceLog.Debugf("Skipped unsupported event: %s", eventType)
			} else {
				traceLog.Warnf("Event processing error: %v", err)
			}
		} else {
			traceLog.Infof("Event processed successfully: %s", eventType)
		}
	}(eventType, body, deliveryID, installationID, ctx)

	// 7. 返回成功响应
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("enhanced event processing started"))
}
