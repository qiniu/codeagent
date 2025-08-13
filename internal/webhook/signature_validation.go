package webhook

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/qiniu/codeagent/pkg/signature"
)

// shouldValidateSignature 决定是否需要签名验证
// 基于请求头中是否存在签名来判断，而不是 webhook 类型
func (h *Handler) shouldValidateSignature(r *http.Request, body []byte) (bool, string) {
	// 检查是否存在签名头
	sig256 := r.Header.Get("X-Hub-Signature-256")
	sig1 := r.Header.Get("X-Hub-Signature")

	// 如果存在任何签名头，说明需要验证
	if sig256 != "" || sig1 != "" {
		// 检测 webhook 类型用于日志
		var payload map[string]interface{}
		webhookType := "repository"
		if err := json.Unmarshal(body, &payload); err == nil {
			if installation, exists := payload["installation"]; exists {
				webhookType = "github_app"
				log.Printf("GitHub App webhook with signature detected (installation: %v)", installation)
			} else {
				log.Printf("Repository webhook with signature detected")
			}
		}

		log.Printf("Signature validation required for %s webhook", webhookType)
		if sig256 != "" {
			return true, sig256
		}
		return true, sig1
	}

	// 没有签名头，检查是否是 GitHub App
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err == nil {
		if installation, exists := payload["installation"]; exists {
			log.Printf("GitHub App webhook without signature detected (installation: %v) - skipping validation", installation)
			return false, ""
		}
	}

	log.Printf("No signature headers found and no GitHub App installation detected")
	return false, ""
}

// ValidateWebhookSignature 智能的 webhook 签名验证
func (h *Handler) ValidateWebhookSignature(r *http.Request, body []byte) error {
	// 检测是否需要签名验证，以及使用哪个签名
	needValidation, signatureHeader := h.shouldValidateSignature(r, body)

	if !needValidation {
		log.Printf("Signature validation not required")
		return nil
	}

	// 需要验证但没有配置 secret
	if h.config.Server.WebhookSecret == "" {
		log.Printf("Signature validation required but no webhook secret configured")
		return signature.ErrMissingSignature
	}

	// 进行签名验证
	if signatureHeader == "" {
		log.Printf("Signature validation required but no signature header found")
		return signature.ErrMissingSignature
	}

	// 根据签名类型进行验证
	if r.Header.Get("X-Hub-Signature-256") != "" {
		log.Printf("Validating SHA-256 signature")
		return signature.ValidateGitHubSignature(signatureHeader, body, h.config.Server.WebhookSecret)
	} else {
		log.Printf("Validating SHA-1 signature")
		return signature.ValidateGitHubSignatureSHA1(signatureHeader, body, h.config.Server.WebhookSecret)
	}
}
