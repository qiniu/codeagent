package auth

import (
	"context"
	"fmt"
	"net/http"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v58/github"
)

// GitHubAppAuthenticator implements Authenticator using GitHub App with ghinstallation
type GitHubAppAuthenticator struct {
	appsTransport *ghinstallation.AppsTransport
	appID         int64
	httpClient    *http.Client
	appInfo       *AppInfo // Cached app information
}

// AppInfo contains cached GitHub App information
type AppInfo struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Owner       string `json:"owner"`
	Description string `json:"description"`
}

// NewGitHubAppAuthenticator creates a new GitHub App authenticator using ghinstallation
func NewGitHubAppAuthenticator(appsTransport *ghinstallation.AppsTransport, appID int64) *GitHubAppAuthenticator {
	return &GitHubAppAuthenticator{
		appsTransport: appsTransport,
		appID:         appID,
		httpClient:    &http.Client{Transport: appsTransport},
	}
}

// GetClient returns a GitHub client authenticated with GitHub App JWT
// This client can only access app-level APIs, not installation-specific resources
func (g *GitHubAppAuthenticator) GetClient(ctx context.Context) (*github.Client, error) {
	if g.appsTransport == nil {
		return nil, fmt.Errorf("GitHub App transport is not configured")
	}

	// Create HTTP client with the Apps transport for JWT authentication
	httpClient := &http.Client{Transport: g.appsTransport}
	
	// Create GitHub client
	client := github.NewClient(httpClient)

	return client, nil
}

// GetInstallationClient returns a GitHub client for a specific installation using ghinstallation
func (g *GitHubAppAuthenticator) GetInstallationClient(ctx context.Context, installationID int64) (*github.Client, error) {
	if g.appsTransport == nil {
		return nil, fmt.Errorf("GitHub App transport is not configured")
	}

	if installationID <= 0 {
		return nil, fmt.Errorf("invalid installation ID: %d", installationID)
	}

	// Create installation transport using ghinstallation
	installationTransport := ghinstallation.NewFromAppsTransport(g.appsTransport, installationID)

	// Create HTTP client with installation transport
	httpClient := &http.Client{Transport: installationTransport}

	// Create GitHub client
	client := github.NewClient(httpClient)

	return client, nil
}

// GetAuthInfo returns authentication information
func (g *GitHubAppAuthenticator) GetAuthInfo() AuthInfo {
	authInfo := AuthInfo{
		Type:  AuthTypeApp,
		AppID: g.appID,
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
	return g.appsTransport != nil && g.appID > 0
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
	// Try to create an installation client and make a simple API call
	client, err := g.GetInstallationClient(ctx, installationID)
	if err != nil {
		return fmt.Errorf("failed to create installation client: %w", err)
	}

	// Try to get installation information to validate access
	_, _, err = client.Apps.GetInstallation(ctx, installationID)
	if err != nil {
		return fmt.Errorf("failed to validate installation %d access: %w", installationID, err)
	}

	return nil
}

// GetAppID returns the GitHub App ID
func (g *GitHubAppAuthenticator) GetAppID() int64 {
	return g.appID
}

// SetHTTPClient sets a custom HTTP client (useful for testing)
func (g *GitHubAppAuthenticator) SetHTTPClient(client *http.Client) {
	g.httpClient = client
}