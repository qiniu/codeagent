package auth

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/go-github/v58/github"
	"github.com/qiniu/codeagent/internal/github/app"
	"golang.org/x/oauth2"
)

// GitHubAppAuthenticator implements Authenticator using GitHub App
type GitHubAppAuthenticator struct {
	tokenManager *app.InstallationTokenManager
	jwtGenerator *app.JWTGenerator
	httpClient   *http.Client
	appInfo      *AppInfo // Cached app information
}

// AppInfo contains cached GitHub App information
type AppInfo struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Owner       string `json:"owner"`
	Description string `json:"description"`
}

// NewGitHubAppAuthenticator creates a new GitHub App authenticator
func NewGitHubAppAuthenticator(tokenManager *app.InstallationTokenManager, jwtGenerator *app.JWTGenerator) *GitHubAppAuthenticator {
	return &GitHubAppAuthenticator{
		tokenManager: tokenManager,
		jwtGenerator: jwtGenerator,
		httpClient:   &http.Client{},
	}
}

// GetClient returns a GitHub client authenticated with GitHub App JWT
// This client can only access app-level APIs, not installation-specific resources
func (g *GitHubAppAuthenticator) GetClient(ctx context.Context) (*github.Client, error) {
	if g.jwtGenerator == nil {
		return nil, fmt.Errorf("JWT generator is not configured")
	}

	// Generate JWT for app-level authentication
	jwt, err := g.jwtGenerator.GenerateJWT(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to generate JWT: %w", err)
	}

	// Create OAuth2 token source with JWT
	ts := oauth2.StaticTokenSource(&oauth2.Token{
		AccessToken: jwt,
		TokenType:   "Bearer",
	})

	// Create HTTP client with OAuth2 transport
	httpClient := oauth2.NewClient(ctx, ts)

	// Create GitHub client
	client := github.NewClient(httpClient)

	return client, nil
}

// GetInstallationClient returns a GitHub client for a specific installation
func (g *GitHubAppAuthenticator) GetInstallationClient(ctx context.Context, installationID int64) (*github.Client, error) {
	if g.tokenManager == nil {
		return nil, fmt.Errorf("token manager is not configured")
	}

	if installationID <= 0 {
		return nil, fmt.Errorf("invalid installation ID: %d", installationID)
	}

	// Get installation access token
	token, err := g.tokenManager.GetToken(ctx, installationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get installation token: %w", err)
	}

	// Create OAuth2 token source with installation token
	ts := oauth2.StaticTokenSource(&oauth2.Token{
		AccessToken: token.AccessToken,
		TokenType:   token.TokenType,
	})

	// Create HTTP client with OAuth2 transport
	httpClient := oauth2.NewClient(ctx, ts)

	// Create GitHub client
	client := github.NewClient(httpClient)

	return client, nil
}

// GetAuthInfo returns authentication information
func (g *GitHubAppAuthenticator) GetAuthInfo() AuthInfo {
	authInfo := AuthInfo{
		Type: AuthTypeApp,
	}

	if g.jwtGenerator != nil {
		authInfo.AppID = g.jwtGenerator.GetAppID()
	}

	// If we have cached app info, use it
	if g.appInfo != nil {
		authInfo.User = g.appInfo.Name
		if g.appInfo.Owner != "" {
			authInfo.User = fmt.Sprintf("%s/%s", g.appInfo.Owner, g.appInfo.Name)
		}
	}

	// GitHub App permissions are installation-specific
	authInfo.Permissions = []string{"installation-specific"}

	return authInfo
}

// IsConfigured returns whether the GitHub App authenticator is properly configured
func (g *GitHubAppAuthenticator) IsConfigured() bool {
	return g.jwtGenerator != nil && g.jwtGenerator.IsConfigured() && g.tokenManager != nil
}

// ValidateAccess validates that the GitHub App can access GitHub
func (g *GitHubAppAuthenticator) ValidateAccess(ctx context.Context) error {
	// Get app-level client
	client, err := g.GetClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create GitHub App client: %w", err)
	}

	// Try to get the app information to validate access
	app, _, err := client.Apps.Get(ctx, "")
	if err != nil {
		return fmt.Errorf("failed to validate GitHub App access: %w", err)
	}

	// Cache app information for GetAuthInfo
	g.appInfo = &AppInfo{
		ID:          app.GetID(),
		Name:        app.GetName(),
		Owner:       app.GetOwner().GetLogin(),
		Description: app.GetDescription(),
	}

	return nil
}

// ValidateInstallationAccess validates access to a specific installation
func (g *GitHubAppAuthenticator) ValidateInstallationAccess(ctx context.Context, installationID int64) error {
	return g.tokenManager.ValidateInstallationAccess(ctx, installationID)
}

// GetAppID returns the GitHub App ID
func (g *GitHubAppAuthenticator) GetAppID() int64 {
	if g.jwtGenerator == nil {
		return 0
	}
	return g.jwtGenerator.GetAppID()
}

// RefreshInstallationToken forces a refresh of the installation token
func (g *GitHubAppAuthenticator) RefreshInstallationToken(ctx context.Context, installationID int64) error {
	if g.tokenManager == nil {
		return fmt.Errorf("token manager is not configured")
	}

	_, err := g.tokenManager.RefreshToken(ctx, installationID)
	return err
}

// SetHTTPClient sets a custom HTTP client (useful for testing)
func (g *GitHubAppAuthenticator) SetHTTPClient(client *http.Client) {
	g.httpClient = client
}

// GetTokenManager returns the underlying token manager (for advanced use cases)
func (g *GitHubAppAuthenticator) GetTokenManager() *app.InstallationTokenManager {
	return g.tokenManager
}

// GetJWTGenerator returns the underlying JWT generator (for advanced use cases)
func (g *GitHubAppAuthenticator) GetJWTGenerator() *app.JWTGenerator {
	return g.jwtGenerator
}
