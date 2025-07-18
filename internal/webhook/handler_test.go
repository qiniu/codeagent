package webhook

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/qbox/codeagent/internal/config"
)

func TestHandleWebhook_SignatureValidation(t *testing.T) {
	// 创建测试配置
	cfg := &config.Config{
		Server: config.ServerConfig{
			WebhookSecret: "test-secret",
		},
	}

	// 创建处理器
	handler := NewHandler(cfg, nil)

	// 测试数据
	payload := []byte(`{"action":"opened","number":1}`)

	// 生成有效签名
	mac := hmac.New(sha256.New, []byte(cfg.Server.WebhookSecret))
	mac.Write(payload)
	validSignature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	tests := []struct {
		name           string
		signature      string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "valid signature",
			signature:      validSignature,
			expectedStatus: http.StatusBadRequest, // 由于没有 X-GitHub-Event 头，会返回 400
			expectedBody:   "missing X-GitHub-Event header",
		},
		{
			name:           "invalid signature",
			signature:      "sha256=invalid",
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   "invalid signature\n",
		},
		{
			name:           "missing signature",
			signature:      "",
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   "missing signature\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建请求
			req := httptest.NewRequest("POST", "/hook", bytes.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			if tt.signature != "" {
				req.Header.Set("X-Hub-Signature-256", tt.signature)
			}

			// 创建响应记录器
			rr := httptest.NewRecorder()

			// 调用处理器
			handler.HandleWebhook(rr, req)

			// 检查状态码
			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, rr.Code)
			}

			// 检查响应体
			if rr.Body.String() != tt.expectedBody {
				t.Errorf("Expected body %q, got %q", tt.expectedBody, rr.Body.String())
			}
		})
	}
}

func TestHandleWebhook_NoSecretConfigured(t *testing.T) {
	// 创建无密钥配置
	cfg := &config.Config{
		Server: config.ServerConfig{
			WebhookSecret: "",
		},
	}

	// 创建处理器
	handler := NewHandler(cfg, nil)

	// 测试数据
	payload := []byte(`{"action":"opened","number":1}`)

	// 创建请求（无签名）
	req := httptest.NewRequest("POST", "/hook", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	// 创建响应记录器
	rr := httptest.NewRecorder()

	// 调用处理器
	handler.HandleWebhook(rr, req)

	// 当没有配置 webhook secret 时，应该跳过签名验证
	// 但由于没有 X-GitHub-Event 头，会返回 400
	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}

	expectedBody := "missing X-GitHub-Event header"
	if rr.Body.String() != expectedBody {
		t.Errorf("Expected body %q, got %q", expectedBody, rr.Body.String())
	}
}
