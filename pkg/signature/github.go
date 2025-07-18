package signature

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrInvalidSignature = errors.New("invalid signature")
	ErrMissingSignature = errors.New("missing signature")
	ErrInvalidFormat    = errors.New("invalid signature format")
)

// ValidateGitHubSignature 验证GitHub webhook签名
// signature: 来自请求头 X-Hub-Signature-256 的签名
// payload: 请求体的原始数据
// secret: webhook配置的secret
func ValidateGitHubSignature(signature string, payload []byte, secret string) error {
	if signature == "" {
		return ErrMissingSignature
	}

	// GitHub签名格式: sha256=<signature>
	const prefix = "sha256="
	if !strings.HasPrefix(signature, prefix) {
		return ErrInvalidFormat
	}

	// 提取签名部分
	sig := strings.TrimPrefix(signature, prefix)
	
	// 解码十六进制签名
	expectedSig, err := hex.DecodeString(sig)
	if err != nil {
		return fmt.Errorf("failed to decode signature: %w", err)
	}

	// 计算HMAC-SHA256
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	computedSig := mac.Sum(nil)

	// 使用恒定时间比较防止时间攻击
	if !hmac.Equal(expectedSig, computedSig) {
		return ErrInvalidSignature
	}

	return nil
}

// ValidateGitHubSignatureSHA1 验证GitHub webhook签名 (SHA1, 已弃用但仍支持)
// signature: 来自请求头 X-Hub-Signature 的签名
// payload: 请求体的原始数据
// secret: webhook配置的secret
func ValidateGitHubSignatureSHA1(signature string, payload []byte, secret string) error {
	if signature == "" {
		return ErrMissingSignature
	}

	// GitHub签名格式: sha1=<signature>
	const prefix = "sha1="
	if !strings.HasPrefix(signature, prefix) {
		return ErrInvalidFormat
	}

	// 提取签名部分
	sig := strings.TrimPrefix(signature, prefix)
	
	// 解码十六进制签名
	expectedSig, err := hex.DecodeString(sig)
	if err != nil {
		return fmt.Errorf("failed to decode signature: %w", err)
	}

	// 计算HMAC-SHA1 (已弃用但仍支持)
	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write(payload)
	computedSig := mac.Sum(nil)

	// 使用恒定时间比较防止时间攻击
	if !hmac.Equal(expectedSig, computedSig) {
		return ErrInvalidSignature
	}

	return nil
}