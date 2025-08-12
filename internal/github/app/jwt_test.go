package app

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// generateTestPrivateKey generates a test RSA private key
func generateTestPrivateKey() *rsa.PrivateKey {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic("failed to generate test private key: " + err.Error())
	}
	return key
}

func TestNewJWTGenerator(t *testing.T) {
	privateKey := generateTestPrivateKey()
	appID := int64(12345)

	generator := NewJWTGenerator(appID, privateKey)

	assert.NotNil(t, generator)
	assert.Equal(t, appID, generator.appID)
	assert.Equal(t, privateKey, generator.privateKey)
	assert.True(t, generator.IsConfigured())
}

func TestJWTGenerator_GenerateJWT(t *testing.T) {
	privateKey := generateTestPrivateKey()
	appID := int64(12345)
	generator := NewJWTGenerator(appID, privateKey)

	ctx := context.Background()
	token, err := generator.GenerateJWT(ctx)

	require.NoError(t, err)
	assert.NotEmpty(t, token)

	// Validate the generated token
	claims, err := generator.ValidateJWT(token)
	require.NoError(t, err)
	assert.NotNil(t, claims)
	assert.Equal(t, "12345", claims.Issuer)
}

func TestJWTGenerator_GenerateJWT_InvalidAppID(t *testing.T) {
	privateKey := generateTestPrivateKey()
	generator := NewJWTGenerator(0, privateKey)

	ctx := context.Background()
	token, err := generator.GenerateJWT(ctx)

	assert.Error(t, err)
	assert.Empty(t, token)
	assert.Contains(t, err.Error(), "invalid app ID")
}

func TestJWTGenerator_GenerateJWT_NoPrivateKey(t *testing.T) {
	generator := NewJWTGenerator(12345, nil)

	ctx := context.Background()
	token, err := generator.GenerateJWT(ctx)

	assert.Error(t, err)
	assert.Empty(t, token)
	assert.Contains(t, err.Error(), "private key is not loaded")
}

func TestJWTGenerator_ValidateJWT(t *testing.T) {
	privateKey := generateTestPrivateKey()
	appID := int64(12345)
	generator := NewJWTGenerator(appID, privateKey)

	ctx := context.Background()
	token, err := generator.GenerateJWT(ctx)
	require.NoError(t, err)

	// Validate the token
	claims, err := generator.ValidateJWT(token)
	require.NoError(t, err)
	assert.NotNil(t, claims)
	assert.Equal(t, "12345", claims.Issuer)
	assert.NotNil(t, claims.IssuedAt)
	assert.NotNil(t, claims.ExpiresAt)
}

func TestJWTGenerator_ValidateJWT_InvalidToken(t *testing.T) {
	privateKey := generateTestPrivateKey()
	generator := NewJWTGenerator(12345, privateKey)

	// Try to validate an invalid token
	claims, err := generator.ValidateJWT("invalid.token.string")
	assert.Error(t, err)
	assert.Nil(t, claims)
}

func TestJWTGenerator_IsConfigured(t *testing.T) {
	privateKey := generateTestPrivateKey()

	tests := []struct {
		name       string
		appID      int64
		privateKey *rsa.PrivateKey
		expected   bool
	}{
		{
			name:       "properly configured",
			appID:      12345,
			privateKey: privateKey,
			expected:   true,
		},
		{
			name:       "missing app ID",
			appID:      0,
			privateKey: privateKey,
			expected:   false,
		},
		{
			name:       "missing private key",
			appID:      12345,
			privateKey: nil,
			expected:   false,
		},
		{
			name:       "missing both",
			appID:      0,
			privateKey: nil,
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			generator := NewJWTGenerator(tt.appID, tt.privateKey)
			assert.Equal(t, tt.expected, generator.IsConfigured())
		})
	}
}

func TestNewGitHubAppClaims(t *testing.T) {
	appID := int64(12345)
	claims := NewGitHubAppClaims(appID)

	assert.NotNil(t, claims)
	assert.Equal(t, "12345", claims.Issuer)
	assert.NotNil(t, claims.IssuedAt)
	assert.NotNil(t, claims.ExpiresAt)

	// Check that expiration is within expected range (8-10 minutes from now)
	now := time.Now()
	expectedMin := now.Add(8 * time.Minute)
	expectedMax := now.Add(10 * time.Minute)
	
	assert.True(t, claims.ExpiresAt.After(expectedMin))
	assert.True(t, claims.ExpiresAt.Before(expectedMax))
}

func TestGitHubAppClaims_Valid(t *testing.T) {
	appID := int64(12345)

	t.Run("valid claims", func(t *testing.T) {
		claims := NewGitHubAppClaims(appID)
		assert.NoError(t, claims.Valid())
	})

	t.Run("expired claims", func(t *testing.T) {
		claims := NewGitHubAppClaims(appID)
		// Manually set expiration to past
		pastTime := time.Now().Add(-1 * time.Hour)
		claims.ExpiresAt = jwt.NewNumericDate(pastTime)
		
		err := claims.Valid()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "expired")
	})

	t.Run("future issued claims", func(t *testing.T) {
		claims := NewGitHubAppClaims(appID)
		// Manually set issued time to future
		futureTime := time.Now().Add(1 * time.Hour)
		claims.IssuedAt = jwt.NewNumericDate(futureTime)
		
		err := claims.Valid()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "before issued")
	})
}