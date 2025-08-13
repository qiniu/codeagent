package github

import (
	"context"
	"fmt"

	"github.com/google/go-github/v58/github"
	"github.com/qiniu/codeagent/internal/config"
	"github.com/qiniu/codeagent/internal/github/auth"
	"github.com/qiniu/x/log"
)

// GitHubClientManager manages GitHub client creation with automatic authentication detection
type GitHubClientManager struct {
	config        *config.Config
	clientFactory auth.ClientFactory
	authenticator auth.Authenticator
}

// NewGitHubClientManager creates a new GitHub client manager
func NewGitHubClientManager(cfg *config.Config) (*GitHubClientManager, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	// Build authenticator and factory from configuration
	builder := auth.NewAuthenticatorBuilder(cfg)
	clientFactory, err := builder.BuildClientFactory()
	if err != nil {
		return nil, fmt.Errorf("failed to build client factory: %w", err)
	}

	return &GitHubClientManager{
		config:        cfg,
		clientFactory: clientFactory,
		authenticator: clientFactory.GetAuthenticator(),
	}, nil
}

// GetClient returns a GitHub client using automatic authentication detection
func (m *GitHubClientManager) GetClient(ctx context.Context) (*github.Client, error) {
	if m.clientFactory == nil {
		return nil, fmt.Errorf("client factory is not initialized")
	}

	// Get authentication info for logging
	authInfo := m.authenticator.GetAuthInfo()
	log.Infof("Using GitHub authentication: type=%s, user=%s", authInfo.Type, authInfo.User)

	// For GitHub App authentication, we need to detect installation ID from context
	if authInfo.Type == auth.AuthTypeApp {
		if installationID := m.getInstallationIDFromContext(ctx); installationID != 0 {
			log.Infof("Using GitHub App authentication with installation ID: %d", installationID)
			return m.clientFactory.CreateInstallationClient(ctx, installationID)
		}

		// If no installation ID in context, log warning and use default client
		log.Warnf("GitHub App configured but no installation ID found in context, using JWT client")
	}

	// Use default client (PAT or JWT for App)
	return m.clientFactory.CreateClient(ctx)
}

// GetInstallationClient returns a GitHub client for a specific installation
func (m *GitHubClientManager) GetInstallationClient(ctx context.Context, installationID int64) (*github.Client, error) {
	if m.clientFactory == nil {
		return nil, fmt.Errorf("client factory is not initialized")
	}

	authInfo := m.authenticator.GetAuthInfo()
	log.Infof("Creating installation client: type=%s, installation_id=%d", authInfo.Type, installationID)

	return m.clientFactory.CreateInstallationClient(ctx, installationID)
}

// GetAuthInfo returns information about the current authentication
func (m *GitHubClientManager) GetAuthInfo() auth.AuthInfo {
	if m.authenticator == nil {
		return auth.AuthInfo{Type: "none"}
	}
	return m.authenticator.GetAuthInfo()
}

// IsGitHubAppConfigured returns true if GitHub App is configured and being used
func (m *GitHubClientManager) IsGitHubAppConfigured() bool {
	return m.authenticator.GetAuthInfo().Type == auth.AuthTypeApp
}

// IsPATConfigured returns true if PAT is configured and being used
func (m *GitHubClientManager) IsPATConfigured() bool {
	return m.authenticator.GetAuthInfo().Type == auth.AuthTypePAT
}

// ValidateAccess validates that the current authentication can access GitHub
func (m *GitHubClientManager) ValidateAccess(ctx context.Context) error {
	if m.authenticator == nil {
		return fmt.Errorf("authenticator is not initialized")
	}
	return m.authenticator.ValidateAccess(ctx)
}

// DetectAuthMode returns the currently active authentication mode
func (m *GitHubClientManager) DetectAuthMode() string {
	if m.config == nil {
		return "unknown"
	}

	authMode := m.config.GetGitHubAuthMode()
	authInfo := m.GetAuthInfo()

	return fmt.Sprintf("configured=%s, active=%s", authMode, authInfo.Type)
}

// getInstallationIDFromContext extracts installation ID from context
// This is a placeholder - the actual implementation would depend on how
// installation ID is passed through the webhook processing pipeline
func (m *GitHubClientManager) getInstallationIDFromContext(ctx context.Context) int64 {
	// Try to get installation ID from context
	if installationID, ok := ctx.Value("installation_id").(int64); ok {
		return installationID
	}

	// Try alternative context key formats
	if installationID, ok := ctx.Value("github_installation_id").(int64); ok {
		return installationID
	}

	if installationIDStr, ok := ctx.Value("installation_id").(string); ok {
		// Handle string conversion if needed
		_ = installationIDStr // placeholder for string to int64 conversion
	}

	return 0
}

// SetInstallationIDInContext adds installation ID to context for later retrieval
func SetInstallationIDInContext(ctx context.Context, installationID int64) context.Context {
	return context.WithValue(ctx, "installation_id", installationID)
}

// GetInstallationIDFromContext extracts installation ID from context
func GetInstallationIDFromContext(ctx context.Context) (int64, bool) {
	if installationID, ok := ctx.Value("installation_id").(int64); ok {
		return installationID, true
	}
	return 0, false
}
