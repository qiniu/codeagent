package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v58/github"
	"github.com/qiniu/codeagent/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPATAuthenticator(t *testing.T) {
	auth := NewPATAuthenticator("ghp_test_token")

	t.Run("IsConfigured", func(t *testing.T) {
		assert.True(t, auth.IsConfigured())

		emptyAuth := NewPATAuthenticator("")
		assert.False(t, emptyAuth.IsConfigured())
	})

	t.Run("GetAuthInfo", func(t *testing.T) {
		info := auth.GetAuthInfo()
		assert.Equal(t, AuthTypePAT, info.Type)
	})

	t.Run("GetClient", func(t *testing.T) {
		ctx := context.Background()
		client, err := auth.GetClient(ctx)
		require.NoError(t, err)
		assert.NotNil(t, client)
	})

	t.Run("GetInstallationClient", func(t *testing.T) {
		ctx := context.Background()
		// For PAT, installation client should be the same as regular client
		client, err := auth.GetInstallationClient(ctx, 123)
		require.NoError(t, err)
		assert.NotNil(t, client)
	})

	t.Run("ValidateAccess", func(t *testing.T) {
		// Create test server that returns user info
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/user" {
				response := github.User{
					Login: github.String("testuser"),
					ID:    github.Int64(12345),
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(response)
			}
		}))
		defer server.Close()

		// Create custom HTTP client that uses our test server
		httpClient := &http.Client{
			Transport: &http.Transport{},
		}

		// Override GitHub API base URL for testing
		testAuth := NewPATAuthenticator("ghp_test_token")
		testAuth.SetHTTPClient(httpClient)

		ctx := context.Background()
		client, err := testAuth.GetClient(ctx)
		require.NoError(t, err)

		// Override the base URL to point to our test server
		client.BaseURL, _ = parseURL(server.URL + "/")

		// Now validate access by getting user info directly
		user, _, err := client.Users.Get(ctx, "")
		require.NoError(t, err)
		assert.Equal(t, "testuser", user.GetLogin())
	})
}

func TestGitHubAppAuthenticator(t *testing.T) {
	// Generate test private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// Convert to PEM format
	privateKeyPEM := privateKeyToPEM(privateKey)

	// Create ghinstallation transport
	transport, err := ghinstallation.NewAppsTransport(http.DefaultTransport, 12345, []byte(privateKeyPEM))
	require.NoError(t, err)

	auth := NewGitHubAppAuthenticator(transport, 12345)

	t.Run("IsConfigured", func(t *testing.T) {
		assert.True(t, auth.IsConfigured())

		// Test with nil components
		emptyAuth := NewGitHubAppAuthenticator(nil, 0)
		assert.False(t, emptyAuth.IsConfigured())
	})

	t.Run("GetAuthInfo", func(t *testing.T) {
		info := auth.GetAuthInfo()
		assert.Equal(t, AuthTypeApp, info.Type)
		assert.Equal(t, int64(12345), info.AppID)
	})

	t.Run("GetClient", func(t *testing.T) {
		ctx := context.Background()
		client, err := auth.GetClient(ctx)
		require.NoError(t, err)
		assert.NotNil(t, client)
	})

	t.Run("GetInstallationClient", func(t *testing.T) {
		ctx := context.Background()
		client, err := auth.GetInstallationClient(ctx, 123)
		require.NoError(t, err)
		assert.NotNil(t, client)
	})

	t.Run("ValidateAccess", func(t *testing.T) {
		// Create test server that returns app info
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/app" {
				response := github.App{
					ID:    github.Int64(12345),
					Name:  github.String("test-app"),
					Owner: &github.User{Login: github.String("testowner")},
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(response)
			}
		}))
		defer server.Close()

		ctx := context.Background()
		client, err := auth.GetClient(ctx)
		require.NoError(t, err)

		// Override the base URL to point to our test server
		client.BaseURL, _ = parseURL(server.URL + "/")

		// Get app info to validate access
		app, _, err := client.Apps.Get(ctx, "")
		require.NoError(t, err)
		assert.Equal(t, int64(12345), app.GetID())
	})
}

func TestAuthenticatorBuilder(t *testing.T) {
	t.Run("BuildPATAuthenticator", func(t *testing.T) {
		cfg := &config.Config{
			GitHub: config.GitHubConfig{
				Token: "ghp_test_token",
			},
		}

		builder := NewAuthenticatorBuilder(cfg)
		auth, err := builder.BuildAuthenticator()
		require.NoError(t, err)
		assert.NotNil(t, auth)
		assert.True(t, auth.IsConfigured())
		assert.Equal(t, AuthTypePAT, auth.GetAuthInfo().Type)
	})

	t.Run("BuildAppAuthenticator", func(t *testing.T) {
		// Generate test private key
		privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)

		// Convert to PEM format
		privateKeyPEM := privateKeyToPEM(privateKey)

		cfg := &config.Config{
			GitHub: config.GitHubConfig{
				App: config.GitHubAppConfig{
					AppID:      12345,
					PrivateKey: privateKeyPEM,
				},
			},
		}

		builder := NewAuthenticatorBuilder(cfg)
		auth, err := builder.BuildAuthenticator()
		require.NoError(t, err)
		assert.NotNil(t, auth)
		assert.True(t, auth.IsConfigured())
		assert.Equal(t, AuthTypeApp, auth.GetAuthInfo().Type)
	})

	t.Run("BuildAppAuthenticatorWithPATFallback", func(t *testing.T) {
		privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)
		privateKeyPEM := privateKeyToPEM(privateKey)

		cfg := &config.Config{
			GitHub: config.GitHubConfig{
				Token: "ghp_test_token", // This should be ignored since App has priority
				App: config.GitHubAppConfig{
					AppID:      12345,
					PrivateKey: privateKeyPEM,
				},
			},
		}

		builder := NewAuthenticatorBuilder(cfg)
		auth, err := builder.BuildAuthenticator()
		require.NoError(t, err)
		assert.NotNil(t, auth)
		assert.Equal(t, AuthTypeApp, auth.GetAuthInfo().Type) // Should prioritize App over PAT
	})

	t.Run("BuildPATWhenNoApp", func(t *testing.T) {
		cfg := &config.Config{
			GitHub: config.GitHubConfig{
				Token: "ghp_test_token",
			},
		}

		builder := NewAuthenticatorBuilder(cfg)
		auth, err := builder.BuildAuthenticator()
		require.NoError(t, err)
		assert.NotNil(t, auth)
		assert.Equal(t, AuthTypePAT, auth.GetAuthInfo().Type)
	})

	t.Run("InvalidConfiguration", func(t *testing.T) {
		cfg := &config.Config{
			GitHub: config.GitHubConfig{
				// No token or app configured
			},
		}

		builder := NewAuthenticatorBuilder(cfg)
		_, err := builder.BuildAuthenticator()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "either GitHub token or GitHub App must be configured")
	})
}

// Helper functions

func parseURL(s string) (*url.URL, error) {
	return url.Parse(s)
}

func privateKeyToPEM(key *rsa.PrivateKey) string {
	// Convert RSA private key to PEM format
	privateKeyDER := x509.MarshalPKCS1PrivateKey(key)

	// Create PEM block
	privateKeyBlock := pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyDER,
	}

	// Encode to PEM format
	return string(pem.EncodeToMemory(&privateKeyBlock))
}
