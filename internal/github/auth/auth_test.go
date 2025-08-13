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
	"time"

	"github.com/google/go-github/v58/github"
	"github.com/qiniu/codeagent/internal/config"
	"github.com/qiniu/codeagent/internal/github/app"
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
	// Create test components
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	jwtGenerator := app.NewJWTGenerator(12345, privateKey)
	tokenManager := app.NewInstallationTokenManager(jwtGenerator, nil)
	auth := NewGitHubAppAuthenticator(tokenManager, jwtGenerator)

	t.Run("IsConfigured", func(t *testing.T) {
		assert.True(t, auth.IsConfigured())

		// Test with nil components
		emptyAuth := NewGitHubAppAuthenticator(nil, nil)
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
		// Create test server for installation token API
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/app/installations/123/access_tokens" {
				response := app.TokenResponse{
					Token:     "ghs_test_installation_token",
					ExpiresAt: time.Now().Add(1 * time.Hour),
					TokenType: "token",
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				json.NewEncoder(w).Encode(response)
			}
		}))
		defer server.Close()

		// Set custom base URL for token manager
		tokenManager.SetBaseURL(server.URL)

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

func TestDefaultClientFactory(t *testing.T) {
	auth := NewPATAuthenticator("ghp_test_token")
	factory := NewClientFactory(auth)

	t.Run("GetAuthenticator", func(t *testing.T) {
		assert.Equal(t, auth, factory.GetAuthenticator())
	})

	t.Run("CreateClient", func(t *testing.T) {
		ctx := context.Background()
		client, err := factory.CreateClient(ctx)
		require.NoError(t, err)
		assert.NotNil(t, client)
	})

	t.Run("CreateInstallationClient", func(t *testing.T) {
		ctx := context.Background()
		client, err := factory.CreateInstallationClient(ctx, 123)
		require.NoError(t, err)
		assert.NotNil(t, client)
	})
}

func TestAuthenticatorBuilder(t *testing.T) {
	t.Run("BuildPATAuthenticator", func(t *testing.T) {
		cfg := &config.Config{
			GitHub: config.GitHubConfig{
				Token:    "ghp_test_token",
				AuthMode: config.AuthModeToken,
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
				AuthMode: config.AuthModeApp,
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

	t.Run("BuildAutoModeWithApp", func(t *testing.T) {
		privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)
		privateKeyPEM := privateKeyToPEM(privateKey)

		cfg := &config.Config{
			GitHub: config.GitHubConfig{
				AuthMode: config.AuthModeAuto,
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
		assert.Equal(t, AuthTypeApp, auth.GetAuthInfo().Type)
	})

	t.Run("BuildAutoModeWithPAT", func(t *testing.T) {
		cfg := &config.Config{
			GitHub: config.GitHubConfig{
				Token:    "ghp_test_token",
				AuthMode: config.AuthModeAuto,
			},
		}

		builder := NewAuthenticatorBuilder(cfg)
		auth, err := builder.BuildAuthenticator()
		require.NoError(t, err)
		assert.NotNil(t, auth)
		assert.Equal(t, AuthTypePAT, auth.GetAuthInfo().Type)
	})

	t.Run("BuildClientFactory", func(t *testing.T) {
		cfg := &config.Config{
			GitHub: config.GitHubConfig{
				Token:    "ghp_test_token",
				AuthMode: config.AuthModeToken,
			},
		}

		builder := NewAuthenticatorBuilder(cfg)
		factory, err := builder.BuildClientFactory()
		require.NoError(t, err)
		assert.NotNil(t, factory)
		assert.NotNil(t, factory.GetAuthenticator())
	})

	t.Run("InvalidConfiguration", func(t *testing.T) {
		cfg := &config.Config{
			GitHub: config.GitHubConfig{
				AuthMode: config.AuthModeToken,
				// Missing token
			},
		}

		builder := NewAuthenticatorBuilder(cfg)
		_, err := builder.BuildAuthenticator()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "GitHub token is required")
	})
}

func TestHybridAuthenticator(t *testing.T) {
	primaryAuth := NewPATAuthenticator("ghp_primary_token")
	fallbackAuth := NewPATAuthenticator("ghp_fallback_token")

	hybrid := NewHybridAuthenticator(primaryAuth, fallbackAuth, config.AuthModeAuto)

	t.Run("IsConfigured", func(t *testing.T) {
		assert.True(t, hybrid.IsConfigured())
	})

	t.Run("GetAuthInfo", func(t *testing.T) {
		info := hybrid.GetAuthInfo()
		assert.Equal(t, AuthTypePAT, info.Type)
	})

	t.Run("GetClient", func(t *testing.T) {
		ctx := context.Background()
		client, err := hybrid.GetClient(ctx)
		require.NoError(t, err)
		assert.NotNil(t, client)
	})

	t.Run("FallbackWhenPrimaryFails", func(t *testing.T) {
		// Use unconfigured primary and configured fallback
		emptyPrimary := NewPATAuthenticator("")
		workingFallback := NewPATAuthenticator("ghp_fallback_token")

		hybridWithFallback := NewHybridAuthenticator(emptyPrimary, workingFallback, config.AuthModeAuto)

		ctx := context.Background()
		client, err := hybridWithFallback.GetClient(ctx)
		require.NoError(t, err)
		assert.NotNil(t, client)
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
