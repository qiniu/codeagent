package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// InstallationTokenManager manages installation access tokens for GitHub Apps
type InstallationTokenManager struct {
	jwtGenerator *JWTGenerator
	httpClient   *http.Client
	cache        TokenCache
	baseURL      string
}

// TokenResponse represents the response from GitHub's installation token API
type TokenResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	TokenType string    `json:"token_type"`
}

// NewInstallationTokenManager creates a new installation token manager
func NewInstallationTokenManager(jwtGenerator *JWTGenerator, httpClient *http.Client) *InstallationTokenManager {
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 30 * time.Second,
		}
	}

	return &InstallationTokenManager{
		jwtGenerator: jwtGenerator,
		httpClient:   httpClient,
		cache:        NewMemoryTokenCache(),
		baseURL:      "https://api.github.com",
	}
}

// NewInstallationTokenManagerWithCache creates a new installation token manager with custom cache
func NewInstallationTokenManagerWithCache(jwtGenerator *JWTGenerator, httpClient *http.Client, cache TokenCache) *InstallationTokenManager {
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 30 * time.Second,
		}
	}

	return &InstallationTokenManager{
		jwtGenerator: jwtGenerator,
		httpClient:   httpClient,
		cache:        cache,
		baseURL:      "https://api.github.com",
	}
}

// GetToken retrieves an installation access token, using cache when possible
func (m *InstallationTokenManager) GetToken(ctx context.Context, installationID int64) (*Token, error) {
	if m.jwtGenerator == nil {
		return nil, fmt.Errorf("JWT generator is not configured")
	}

	if installationID <= 0 {
		return nil, fmt.Errorf("invalid installation ID: %d", installationID)
	}

	// Try to get token from cache first
	if token, found := m.cache.Get(installationID); found {
		return token, nil
	}

	// Generate new token
	token, err := m.generateNewToken(ctx, installationID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate new token for installation %d: %w", installationID, err)
	}

	// Cache the new token
	m.cache.Set(installationID, token)

	return token, nil
}

// RefreshToken forcefully refreshes a token for the given installation
func (m *InstallationTokenManager) RefreshToken(ctx context.Context, installationID int64) (*Token, error) {
	// Remove existing token from cache
	m.cache.Delete(installationID)

	// Generate new token
	return m.GetToken(ctx, installationID)
}

// generateNewToken generates a new installation access token from GitHub API
func (m *InstallationTokenManager) generateNewToken(ctx context.Context, installationID int64) (*Token, error) {
	// Generate JWT for authentication
	jwt, err := m.jwtGenerator.GenerateJWT(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to generate JWT: %w", err)
	}

	// Prepare request to GitHub API
	url := fmt.Sprintf("%s/app/installations/%d/access_tokens", m.baseURL, installationID)
	
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader([]byte("{}")))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "codeagent/1.0")

	// Make the request
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request to GitHub API: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check for HTTP errors
	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("GitHub API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	// Create token object
	token := &Token{
		AccessToken: tokenResp.Token,
		ExpiresAt:   tokenResp.ExpiresAt,
		TokenType:   tokenResp.TokenType,
	}

	return token, nil
}

// InvalidateToken removes a token from the cache
func (m *InstallationTokenManager) InvalidateToken(installationID int64) {
	m.cache.Delete(installationID)
}

// ClearCache clears all cached tokens
func (m *InstallationTokenManager) ClearCache() {
	m.cache.Clear()
}

// GetCacheSize returns the number of cached tokens
func (m *InstallationTokenManager) GetCacheSize() int {
	return m.cache.Size()
}

// CleanupExpiredTokens removes expired tokens from the cache
func (m *InstallationTokenManager) CleanupExpiredTokens() {
	if memCache, ok := m.cache.(*MemoryTokenCache); ok {
		memCache.Cleanup()
	}
}

// GetExpiringTokens returns installation IDs with tokens expiring within the specified duration
func (m *InstallationTokenManager) GetExpiringTokens(within time.Duration) []int64 {
	if memCache, ok := m.cache.(*MemoryTokenCache); ok {
		return memCache.GetExpiringInstallations(within)
	}
	return nil
}

// SetBaseURL sets a custom base URL for the GitHub API (useful for testing)
func (m *InstallationTokenManager) SetBaseURL(baseURL string) {
	m.baseURL = baseURL
}

// ValidateInstallationAccess validates that we can access the installation
func (m *InstallationTokenManager) ValidateInstallationAccess(ctx context.Context, installationID int64) error {
	// Try to get a token - this will fail if we don't have access
	_, err := m.GetToken(ctx, installationID)
	return err
}

// CreateTokenWithPermissions creates a token with specific permissions and repositories
func (m *InstallationTokenManager) CreateTokenWithPermissions(ctx context.Context, installationID int64, permissions map[string]string, repositories []string) (*Token, error) {
	if m.jwtGenerator == nil {
		return nil, fmt.Errorf("JWT generator is not configured")
	}

	if installationID <= 0 {
		return nil, fmt.Errorf("invalid installation ID: %d", installationID)
	}

	// Generate JWT for authentication
	jwt, err := m.jwtGenerator.GenerateJWT(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to generate JWT: %w", err)
	}

	// Prepare request body
	requestBody := map[string]interface{}{}
	if len(permissions) > 0 {
		requestBody["permissions"] = permissions
	}
	if len(repositories) > 0 {
		requestBody["repositories"] = repositories
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	// Prepare request to GitHub API
	url := fmt.Sprintf("%s/app/installations/%d/access_tokens", m.baseURL, installationID)
	
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "codeagent/1.0")

	// Make the request
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request to GitHub API: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check for HTTP errors
	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("GitHub API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	// Create token object
	token := &Token{
		AccessToken: tokenResp.Token,
		ExpiresAt:   tokenResp.ExpiresAt,
		TokenType:   tokenResp.TokenType,
	}

	// Cache with a special key for permission-specific tokens
	// We don't cache these as they might have different permissions
	// than the default installation token

	return token, nil
}