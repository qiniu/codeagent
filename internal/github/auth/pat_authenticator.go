package auth

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/go-github/v58/github"
	"golang.org/x/oauth2"
)

// PATAuthenticator implements Authenticator using Personal Access Token
type PATAuthenticator struct {
	token      string
	httpClient *http.Client
	userInfo   *github.User // Cached user information
}

// NewPATAuthenticator creates a new PAT authenticator
func NewPATAuthenticator(token string) *PATAuthenticator {
	return &PATAuthenticator{
		token: token,
	}
}

// GetClient returns a GitHub client authenticated with PAT
func (p *PATAuthenticator) GetClient(ctx context.Context) (*github.Client, error) {
	if p.token == "" {
		return nil, fmt.Errorf("GitHub token is not configured")
	}

	// Create OAuth2 token source
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: p.token})
	
	// Create HTTP client with OAuth2 transport
	httpClient := oauth2.NewClient(ctx, ts)
	
	// Create GitHub client
	client := github.NewClient(httpClient)
	
	return client, nil
}

// GetInstallationClient returns the same client as GetClient for PAT auth
// PAT doesn't have installation-specific clients
func (p *PATAuthenticator) GetInstallationClient(ctx context.Context, installationID int64) (*github.Client, error) {
	// For PAT authentication, installation ID is ignored
	// PAT has access to repositories based on the token's permissions
	return p.GetClient(ctx)
}

// GetAuthInfo returns authentication information
func (p *PATAuthenticator) GetAuthInfo() AuthInfo {
	authInfo := AuthInfo{
		Type: AuthTypePAT,
	}

	// If we have cached user info, use it
	if p.userInfo != nil {
		authInfo.User = p.userInfo.GetLogin()
		// PAT permissions are implicit based on token scope
		authInfo.Permissions = []string{"read", "write"} // Simplified - actual permissions depend on token scope
	}

	return authInfo
}

// IsConfigured returns whether the PAT authenticator is properly configured
func (p *PATAuthenticator) IsConfigured() bool {
	return p.token != ""
}

// ValidateAccess validates that the PAT can access GitHub
func (p *PATAuthenticator) ValidateAccess(ctx context.Context) error {
	client, err := p.GetClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create GitHub client: %w", err)
	}

	// Try to get the authenticated user to validate the token
	user, _, err := client.Users.Get(ctx, "")
	if err != nil {
		return fmt.Errorf("failed to validate GitHub access: %w", err)
	}

	// Cache user information for GetAuthInfo
	p.userInfo = user

	return nil
}

// GetToken returns the configured token (for internal use)
func (p *PATAuthenticator) GetToken() string {
	return p.token
}

// SetHTTPClient sets a custom HTTP client (useful for testing)
func (p *PATAuthenticator) SetHTTPClient(client *http.Client) {
	p.httpClient = client
}