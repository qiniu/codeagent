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

// ValidateGitHubSignature validates GitHub webhook signature
// signature: signature from X-Hub-Signature-256 request header
// payload: raw request body data
// secret: webhook configured secret
func ValidateGitHubSignature(signature string, payload []byte, secret string) error {
	if signature == "" {
		return ErrMissingSignature
	}

	// GitHub signature format: sha256=<signature>
	const prefix = "sha256="
	if !strings.HasPrefix(signature, prefix) {
		return ErrInvalidFormat
	}

	// Extract signature part
	sig := strings.TrimPrefix(signature, prefix)

	// Decode hexadecimal signature
	expectedSig, err := hex.DecodeString(sig)
	if err != nil {
		return fmt.Errorf("failed to decode signature: %w", err)
	}

	// Calculate HMAC-SHA256
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	computedSig := mac.Sum(nil)

	// Use constant-time comparison to prevent timing attacks
	if !hmac.Equal(expectedSig, computedSig) {
		return ErrInvalidSignature
	}

	return nil
}

// ValidateGitHubSignatureSHA1 validates GitHub webhook signature (SHA1, deprecated but still supported)
// signature: signature from X-Hub-Signature request header
// payload: raw request body data
// secret: webhook configured secret
func ValidateGitHubSignatureSHA1(signature string, payload []byte, secret string) error {
	if signature == "" {
		return ErrMissingSignature
	}

	// GitHub signature format: sha1=<signature>
	const prefix = "sha1="
	if !strings.HasPrefix(signature, prefix) {
		return ErrInvalidFormat
	}

	// Extract signature part
	sig := strings.TrimPrefix(signature, prefix)

	// Decode hexadecimal signature
	expectedSig, err := hex.DecodeString(sig)
	if err != nil {
		return fmt.Errorf("failed to decode signature: %w", err)
	}

	// Calculate HMAC-SHA1 (deprecated but still supported)
	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write(payload)
	computedSig := mac.Sum(nil)

	// Use constant-time comparison to prevent timing attacks
	if !hmac.Equal(expectedSig, computedSig) {
		return ErrInvalidSignature
	}

	return nil
}
