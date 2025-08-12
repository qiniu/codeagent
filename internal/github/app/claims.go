package app

import (
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// GitHubAppClaims represents the JWT claims for GitHub App authentication
type GitHubAppClaims struct {
	jwt.RegisteredClaims
}

// NewGitHubAppClaims creates new JWT claims for GitHub App authentication
func NewGitHubAppClaims(appID int64) *GitHubAppClaims {
	now := time.Now()
	
	// GitHub App JWT should expire within 10 minutes
	// We set it to 9 minutes to provide some buffer
	expirationTime := now.Add(9 * time.Minute)
	
	appIDStr := strconv.FormatInt(appID, 10)
	
	return &GitHubAppClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    appIDStr,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expirationTime),
		},
	}
}

// Valid validates the claims
func (c *GitHubAppClaims) Valid() error {
	// Check if the token is expired
	now := time.Now()
	if c.ExpiresAt != nil && c.ExpiresAt.Before(now) {
		return fmt.Errorf("token is expired")
	}
	
	// Check if the token was issued in the future
	if c.IssuedAt != nil && c.IssuedAt.After(now) {
		return fmt.Errorf("token used before issued")
	}
	
	return nil
}