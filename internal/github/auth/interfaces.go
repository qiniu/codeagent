package auth

import (
	"context"

	"github.com/google/go-github/v58/github"
)

// AuthType represents the type of authentication being used
type AuthType string

const (
	AuthTypePAT AuthType = "pat" // Personal Access Token
	AuthTypeApp AuthType = "app" // GitHub App
)

// AuthInfo contains information about the current authentication
type AuthInfo struct {
	Type        AuthType `json:"type"`
	User        string   `json:"user"`         // PAT user or App name
	Permissions []string `json:"permissions"`  // Available permissions
	AppID       int64    `json:"app_id,omitempty"` // GitHub App ID (only for App auth)
}

// Authenticator defines the interface for GitHub authentication
type Authenticator interface {
	// GetClient returns a GitHub client authenticated with the configured method
	GetClient(ctx context.Context) (*github.Client, error)
	
	// GetInstallationClient returns a GitHub client for a specific installation (GitHub App only)
	// For PAT authenticators, this should return the same as GetClient
	GetInstallationClient(ctx context.Context, installationID int64) (*github.Client, error)
	
	// GetAuthInfo returns information about the current authentication
	GetAuthInfo() AuthInfo
	
	// IsConfigured returns whether the authenticator is properly configured
	IsConfigured() bool
	
	// ValidateAccess validates that the authenticator can access GitHub
	ValidateAccess(ctx context.Context) error
}

// ClientFactory creates GitHub clients using the configured authentication method
type ClientFactory interface {
	// CreateClient creates a GitHub client using the default authentication
	CreateClient(ctx context.Context) (*github.Client, error)
	
	// CreateInstallationClient creates a GitHub client for a specific installation
	CreateInstallationClient(ctx context.Context, installationID int64) (*github.Client, error)
	
	// GetAuthenticator returns the underlying authenticator
	GetAuthenticator() Authenticator
}