package auth

import (
	"fmt"
	"net/http"
	"os"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/qiniu/codeagent/internal/config"
)

// AuthenticatorBuilder helps build authenticators from configuration
type AuthenticatorBuilder struct {
	config *config.Config
}

// NewAuthenticatorBuilder creates a new authenticator builder
func NewAuthenticatorBuilder(cfg *config.Config) *AuthenticatorBuilder {
	return &AuthenticatorBuilder{config: cfg}
}

// BuildAuthenticator builds an authenticator based on the configuration
// Automatically prioritizes GitHub App over PAT if both are configured
func (b *AuthenticatorBuilder) BuildAuthenticator() (Authenticator, error) {
	if b.config == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	// Validate configuration first
	if err := b.config.ValidateGitHubConfig(); err != nil {
		return nil, fmt.Errorf("invalid GitHub configuration: %w", err)
	}

	// Prioritize GitHub App over PAT
	if b.config.IsGitHubAppConfigured() {
		appAuth, err := b.buildAppAuthenticator()
		if err == nil {
			return appAuth, nil
		}
		// Log the error but continue to PAT fallback
		fmt.Printf("Warning: GitHub App configuration failed: %v\n", err)
	}

	// Fallback to PAT if GitHub App is not configured or failed
	if b.config.IsGitHubTokenConfigured() {
		return b.buildPATAuthenticator()
	}

	return nil, fmt.Errorf("no valid GitHub authentication configuration found")
}

// buildPATAuthenticator builds a PAT authenticator
func (b *AuthenticatorBuilder) buildPATAuthenticator() (Authenticator, error) {
	if !b.config.IsGitHubTokenConfigured() {
		return nil, fmt.Errorf("GitHub token is not configured")
	}

	return NewPATAuthenticator(b.config.GitHub.Token), nil
}

// buildAppAuthenticator builds a GitHub App authenticator using ghinstallation
func (b *AuthenticatorBuilder) buildAppAuthenticator() (Authenticator, error) {
	if !b.config.IsGitHubAppConfigured() {
		return nil, fmt.Errorf("GitHub App is not configured")
	}

	appConfig := b.config.GitHub.App

	// Determine private key source and create transport
	var transport *ghinstallation.AppsTransport
	var err error

	if appConfig.PrivateKeyPath != "" {
		// Load from file path
		transport, err = ghinstallation.NewAppsTransportKeyFromFile(http.DefaultTransport, appConfig.AppID, appConfig.PrivateKeyPath)
	} else if appConfig.PrivateKeyEnv != "" {
		// Load from environment variable
		privateKeyData := os.Getenv(appConfig.PrivateKeyEnv)
		if privateKeyData == "" {
			return nil, fmt.Errorf("private key environment variable %s is empty", appConfig.PrivateKeyEnv)
		}
		transport, err = ghinstallation.NewAppsTransport(http.DefaultTransport, appConfig.AppID, []byte(privateKeyData))
	} else if appConfig.PrivateKey != "" {
		// Load from direct configuration
		transport, err = ghinstallation.NewAppsTransport(http.DefaultTransport, appConfig.AppID, []byte(appConfig.PrivateKey))
	} else {
		return nil, fmt.Errorf("no private key source configured")
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create GitHub App transport: %w", err)
	}

	// Create authenticator
	return NewGitHubAppAuthenticator(transport, appConfig.AppID), nil
}
