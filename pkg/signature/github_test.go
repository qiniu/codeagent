package signature

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestValidateGitHubSignature(t *testing.T) {
	secret := "my-webhook-secret"
	payload := []byte(`{"action":"opened","number":1}`)

	// Generate valid SHA-256 signature
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	validSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	tests := []struct {
		name        string
		signature   string
		payload     []byte
		secret      string
		wantErr     bool
		expectedErr error
	}{
		{
			name:      "valid signature",
			signature: validSig,
			payload:   payload,
			secret:    secret,
			wantErr:   false,
		},
		{
			name:        "invalid signature",
			signature:   "sha256=0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			payload:     payload,
			secret:      "wrong-secret",
			wantErr:     true,
			expectedErr: ErrInvalidSignature,
		},
		{
			name:        "missing signature",
			signature:   "",
			payload:     payload,
			secret:      secret,
			wantErr:     true,
			expectedErr: ErrMissingSignature,
		},
		{
			name:        "invalid format",
			signature:   "invalid-format",
			payload:     payload,
			secret:      secret,
			wantErr:     true,
			expectedErr: ErrInvalidFormat,
		},
		{
			name:        "wrong secret",
			signature:   validSig,
			payload:     payload,
			secret:      "wrong-secret",
			wantErr:     true,
			expectedErr: ErrInvalidSignature,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateGitHubSignature(tt.signature, tt.payload, tt.secret)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateGitHubSignature() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.expectedErr != nil && err != tt.expectedErr {
				t.Errorf("ValidateGitHubSignature() error = %v, expectedErr %v", err, tt.expectedErr)
			}
		})
	}
}

func TestValidateGitHubSignatureSHA1(t *testing.T) {
	secret := "my-webhook-secret"
	payload := []byte(`{"action":"opened","number":1}`)

	// Generate valid SHA-1 signature
	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write(payload)
	validSig := "sha1=" + hex.EncodeToString(mac.Sum(nil))

	tests := []struct {
		name        string
		signature   string
		payload     []byte
		secret      string
		wantErr     bool
		expectedErr error
	}{
		{
			name:      "valid SHA-1 signature",
			signature: validSig,
			payload:   payload,
			secret:    secret,
			wantErr:   false,
		},
		{
			name:        "invalid SHA-1 signature",
			signature:   "sha1=0123456789abcdef01234567",
			payload:     payload,
			secret:      "wrong-secret",
			wantErr:     true,
			expectedErr: ErrInvalidSignature,
		},
		{
			name:        "missing SHA-1 signature",
			signature:   "",
			payload:     payload,
			secret:      secret,
			wantErr:     true,
			expectedErr: ErrMissingSignature,
		},
		{
			name:        "invalid SHA-1 format",
			signature:   "invalid-format",
			payload:     payload,
			secret:      secret,
			wantErr:     true,
			expectedErr: ErrInvalidFormat,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateGitHubSignatureSHA1(tt.signature, tt.payload, tt.secret)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateGitHubSignatureSHA1() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.expectedErr != nil && err != tt.expectedErr {
				t.Errorf("ValidateGitHubSignatureSHA1() error = %v, expectedErr %v", err, tt.expectedErr)
			}
		})
	}
}
