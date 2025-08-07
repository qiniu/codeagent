package webhook

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/qiniu/codeagent/internal/config"
)

func TestHandleWebhook_SignatureValidation(t *testing.T) {
	// Create test configuration
	cfg := &config.Config{
		Server: config.ServerConfig{
			WebhookSecret: "test-secret",
		},
	}

	// Create handler
	handler := NewHandler(cfg, nil)

	// Test data
	payload := []byte(`{"action":"opened","number":1}`)

	// Generate valid signature
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
			expectedStatus: http.StatusBadRequest, // Will return 400 due to missing X-GitHub-Event header
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
			// Create request
			req := httptest.NewRequest("POST", "/hook", bytes.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			if tt.signature != "" {
				req.Header.Set("X-Hub-Signature-256", tt.signature)
			}

			// Create response recorder
			rr := httptest.NewRecorder()

			// Call handler
			handler.HandleWebhook(rr, req)

			// Check status code
			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, rr.Code)
			}

			// Check response body
			if rr.Body.String() != tt.expectedBody {
				t.Errorf("Expected body %q, got %q", tt.expectedBody, rr.Body.String())
			}
		})
	}
}

func TestHandleWebhook_NoSecretConfigured(t *testing.T) {
	// Create configuration without secret
	cfg := &config.Config{
		Server: config.ServerConfig{
			WebhookSecret: "",
		},
	}

	// Create handler
	handler := NewHandler(cfg, nil)

	// Test data
	payload := []byte(`{"action":"opened","number":1}`)

	// Create request (no signature)
	req := httptest.NewRequest("POST", "/hook", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	// Create response recorder
	rr := httptest.NewRecorder()

	// Call handler
	handler.HandleWebhook(rr, req)

	// When no webhook secret is configured, should skip signature verification
	// But will return 400 due to missing X-GitHub-Event header
	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}

	expectedBody := "missing X-GitHub-Event header"
	if rr.Body.String() != expectedBody {
		t.Errorf("Expected body %q, got %q", expectedBody, rr.Body.String())
	}
}
