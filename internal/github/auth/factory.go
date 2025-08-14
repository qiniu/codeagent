package auth

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v58/github"
	"github.com/qiniu/codeagent/internal/config"
)

// DefaultClientFactory implements ClientFactory
type DefaultClientFactory struct {
	authenticator Authenticator
}

// NewClientFactory creates a new client factory with the given authenticator
func NewClientFactory(authenticator Authenticator) *DefaultClientFactory {
	return &DefaultClientFactory{
		authenticator: authenticator,
	}
}

// CreateClient creates a GitHub client using the default authentication
func (f *DefaultClientFactory) CreateClient(ctx context.Context) (*github.Client, error) {
	if f.authenticator == nil {
		return nil, fmt.Errorf("authenticator is not configured")
	}

	return f.authenticator.GetClient(ctx)
}

// CreateInstallationClient creates a GitHub client for a specific installation
func (f *DefaultClientFactory) CreateInstallationClient(ctx context.Context, installationID int64) (*github.Client, error) {
	if f.authenticator == nil {
		return nil, fmt.Errorf("authenticator is not configured")
	}

	return f.authenticator.GetInstallationClient(ctx, installationID)
}

// GetAuthenticator returns the underlying authenticator
func (f *DefaultClientFactory) GetAuthenticator() Authenticator {
	return f.authenticator
}

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

// BuildClientFactory builds a client factory from configuration
func (b *AuthenticatorBuilder) BuildClientFactory() (ClientFactory, error) {
	authenticator, err := b.BuildAuthenticator()
	if err != nil {
		return nil, err
	}

	return NewClientFactory(authenticator), nil
}

// HybridAuthenticator wraps multiple authenticators and provides fallback behavior
type HybridAuthenticator struct {
	primary  Authenticator
	fallback Authenticator
}

// NewHybridAuthenticator creates a hybrid authenticator with primary and fallback
func NewHybridAuthenticator(primary, fallback Authenticator) *HybridAuthenticator {
	return &HybridAuthenticator{
		primary:  primary,
		fallback: fallback,
	}
}

// GetClient tries primary first, then fallback
func (h *HybridAuthenticator) GetClient(ctx context.Context) (*github.Client, error) {
	if h.primary != nil && h.primary.IsConfigured() {
		client, err := h.primary.GetClient(ctx)
		if err == nil {
			return client, nil
		}
		// Log error but try fallback
		fmt.Printf("Warning: primary authenticator failed: %v\n", err)
	}

	if h.fallback != nil && h.fallback.IsConfigured() {
		return h.fallback.GetClient(ctx)
	}

	return nil, fmt.Errorf("no working authenticator available")
}

// GetInstallationClient tries primary first, then fallback
func (h *HybridAuthenticator) GetInstallationClient(ctx context.Context, installationID int64) (*github.Client, error) {
	if h.primary != nil && h.primary.IsConfigured() {
		client, err := h.primary.GetInstallationClient(ctx, installationID)
		if err == nil {
			return client, nil
		}
		// Log error but try fallback
		fmt.Printf("Warning: primary authenticator failed for installation %d: %v\n", installationID, err)
	}

	if h.fallback != nil && h.fallback.IsConfigured() {
		return h.fallback.GetInstallationClient(ctx, installationID)
	}

	return nil, fmt.Errorf("no working authenticator available for installation %d", installationID)
}

// GetAuthInfo returns info from the working authenticator
func (h *HybridAuthenticator) GetAuthInfo() AuthInfo {
	if h.primary != nil && h.primary.IsConfigured() {
		return h.primary.GetAuthInfo()
	}

	if h.fallback != nil && h.fallback.IsConfigured() {
		return h.fallback.GetAuthInfo()
	}

	return AuthInfo{Type: "none"}
}

// IsConfigured returns true if at least one authenticator is configured
func (h *HybridAuthenticator) IsConfigured() bool {
	return (h.primary != nil && h.primary.IsConfigured()) ||
		(h.fallback != nil && h.fallback.IsConfigured())
}

// ValidateAccess validates the working authenticator
func (h *HybridAuthenticator) ValidateAccess(ctx context.Context) error {
	if h.primary != nil && h.primary.IsConfigured() {
		if err := h.primary.ValidateAccess(ctx); err == nil {
			return nil
		}
	}

	if h.fallback != nil && h.fallback.IsConfigured() {
		return h.fallback.ValidateAccess(ctx)
	}

	return fmt.Errorf("no working authenticator available")
}
