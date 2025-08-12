package app

import (
	"context"
	"crypto/rsa"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

// JWTGenerator handles GitHub App JWT generation and signing
type JWTGenerator struct {
	appID      int64
	privateKey *rsa.PrivateKey
}

// NewJWTGenerator creates a new JWT generator for GitHub App authentication
func NewJWTGenerator(appID int64, privateKey *rsa.PrivateKey) *JWTGenerator {
	return &JWTGenerator{
		appID:      appID,
		privateKey: privateKey,
	}
}

// GenerateJWT generates a signed JWT token for GitHub App authentication
func (j *JWTGenerator) GenerateJWT(ctx context.Context) (string, error) {
	if j.privateKey == nil {
		return "", fmt.Errorf("private key is not loaded")
	}

	if j.appID <= 0 {
		return "", fmt.Errorf("invalid app ID: %d", j.appID)
	}

	// Create claims
	claims := NewGitHubAppClaims(j.appID)

	// Create token with claims
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)

	// Sign token with private key
	tokenString, err := token.SignedString(j.privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign JWT token: %w", err)
	}

	return tokenString, nil
}

// ValidateJWT validates a JWT token and returns the claims
func (j *JWTGenerator) ValidateJWT(tokenString string) (*GitHubAppClaims, error) {
	if j.privateKey == nil {
		return nil, fmt.Errorf("private key is not loaded")
	}

	// Parse and validate token
	token, err := jwt.ParseWithClaims(tokenString, &GitHubAppClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Validate signing method
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return &j.privateKey.PublicKey, nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse JWT token: %w", err)
	}

	// Extract claims
	if claims, ok := token.Claims.(*GitHubAppClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, fmt.Errorf("invalid JWT token or claims")
}

// GetAppID returns the configured App ID
func (j *JWTGenerator) GetAppID() int64 {
	return j.appID
}

// IsConfigured checks if the JWT generator is properly configured
func (j *JWTGenerator) IsConfigured() bool {
	return j.appID > 0 && j.privateKey != nil
}