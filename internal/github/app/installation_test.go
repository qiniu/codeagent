package app

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToken_IsExpired(t *testing.T) {
	tests := []struct {
		name      string
		token     *Token
		expected  bool
	}{
		{
			name: "not expired",
			token: &Token{
				AccessToken: "test-token",
				ExpiresAt:   time.Now().Add(1 * time.Hour),
			},
			expected: false,
		},
		{
			name: "expired",
			token: &Token{
				AccessToken: "test-token",
				ExpiresAt:   time.Now().Add(-1 * time.Hour),
			},
			expected: true,
		},
		{
			name: "expires soon (within 5 minutes)",
			token: &Token{
				AccessToken: "test-token",
				ExpiresAt:   time.Now().Add(3 * time.Minute),
			},
			expected: true,
		},
		{
			name: "zero expiration",
			token: &Token{
				AccessToken: "test-token",
				ExpiresAt:   time.Time{},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.token.IsExpired())
		})
	}
}

func TestMemoryTokenCache(t *testing.T) {
	cache := NewMemoryTokenCache()

	// Test empty cache
	assert.Equal(t, 0, cache.Size())
	token, found := cache.Get(123)
	assert.False(t, found)
	assert.Nil(t, token)

	// Test set and get
	testToken := &Token{
		AccessToken: "test-token",
		ExpiresAt:   time.Now().Add(1 * time.Hour),
	}
	cache.Set(123, testToken)
	assert.Equal(t, 1, cache.Size())

	retrievedToken, found := cache.Get(123)
	assert.True(t, found)
	assert.Equal(t, testToken.AccessToken, retrievedToken.AccessToken)

	// Test expired token
	expiredToken := &Token{
		AccessToken: "expired-token",
		ExpiresAt:   time.Now().Add(-1 * time.Hour),
	}
	cache.Set(456, expiredToken)
	assert.Equal(t, 2, cache.Size())

	retrievedToken, found = cache.Get(456)
	assert.False(t, found) // Should not return expired token
	assert.Nil(t, retrievedToken)

	// Test delete
	cache.Delete(123)
	assert.Equal(t, 1, cache.Size()) // expired token still in cache until cleanup

	_, found = cache.Get(123)
	assert.False(t, found)

	// Test cleanup
	cache.Cleanup()
	assert.Equal(t, 0, cache.Size()) // expired token should be removed

	// Test clear
	cache.Set(789, testToken)
	assert.Equal(t, 1, cache.Size())
	cache.Clear()
	assert.Equal(t, 0, cache.Size())
}

func TestMemoryTokenCache_GetExpiringInstallations(t *testing.T) {
	cache := NewMemoryTokenCache()

	// Add tokens with different expiration times
	cache.Set(1, &Token{
		AccessToken: "token1",
		ExpiresAt:   time.Now().Add(30 * time.Minute), // Expires in 30 minutes
	})
	cache.Set(2, &Token{
		AccessToken: "token2",
		ExpiresAt:   time.Now().Add(2 * time.Hour), // Expires in 2 hours
	})
	cache.Set(3, &Token{
		AccessToken: "token3",
		ExpiresAt:   time.Now().Add(5 * time.Minute), // Expires in 5 minutes
	})

	// Get installations expiring within 1 hour
	expiring := cache.GetExpiringInstallations(1 * time.Hour)
	assert.Len(t, expiring, 2) // Should include installations 1 and 3
	assert.Contains(t, expiring, int64(1))
	assert.Contains(t, expiring, int64(3))

	// Get installations expiring within 10 minutes
	expiring = cache.GetExpiringInstallations(10 * time.Minute)
	assert.Len(t, expiring, 1) // Should include only installation 3
	assert.Contains(t, expiring, int64(3))
}

func createTestJWTGenerator() *JWTGenerator {
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	return NewJWTGenerator(12345, privateKey)
}

func TestNewInstallationTokenManager(t *testing.T) {
	jwtGen := createTestJWTGenerator()
	
	// Test with custom HTTP client
	customClient := &http.Client{Timeout: 60 * time.Second}
	manager := NewInstallationTokenManager(jwtGen, customClient)
	
	assert.NotNil(t, manager)
	assert.Equal(t, jwtGen, manager.jwtGenerator)
	assert.Equal(t, customClient, manager.httpClient)
	assert.NotNil(t, manager.cache)
	assert.Equal(t, "https://api.github.com", manager.baseURL)

	// Test with nil HTTP client (should create default)
	manager2 := NewInstallationTokenManager(jwtGen, nil)
	assert.NotNil(t, manager2.httpClient)
	assert.Equal(t, 30*time.Second, manager2.httpClient.Timeout)
}

func TestInstallationTokenManager_GetToken_InvalidInputs(t *testing.T) {
	jwtGen := createTestJWTGenerator()
	manager := NewInstallationTokenManager(jwtGen, nil)
	ctx := context.Background()

	// Test invalid installation ID
	_, err := manager.GetToken(ctx, 0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid installation ID")

	// Test with nil JWT generator
	managerNilJWT := NewInstallationTokenManager(nil, nil)
	_, err = managerNilJWT.GetToken(ctx, 123)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "JWT generator is not configured")
}

func TestInstallationTokenManager_GetToken_Success(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Contains(t, r.URL.Path, "/app/installations/123/access_tokens")
		assert.Contains(t, r.Header.Get("Authorization"), "Bearer ")
		assert.Equal(t, "application/vnd.github.v3+json", r.Header.Get("Accept"))

		response := TokenResponse{
			Token:     "ghs_test_token",
			ExpiresAt: time.Now().Add(1 * time.Hour),
			TokenType: "token",
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	jwtGen := createTestJWTGenerator()
	manager := NewInstallationTokenManager(jwtGen, nil)
	manager.SetBaseURL(server.URL)

	ctx := context.Background()
	token, err := manager.GetToken(ctx, 123)

	require.NoError(t, err)
	assert.NotNil(t, token)
	assert.Equal(t, "ghs_test_token", token.AccessToken)
	assert.Equal(t, "token", token.TokenType)
	assert.False(t, token.IsExpired())

	// Test cache hit
	token2, err := manager.GetToken(ctx, 123)
	require.NoError(t, err)
	assert.Equal(t, token.AccessToken, token2.AccessToken)
}

func TestInstallationTokenManager_GetToken_APIError(t *testing.T) {
	// Create test server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message": "Not Found"}`))
	}))
	defer server.Close()

	jwtGen := createTestJWTGenerator()
	manager := NewInstallationTokenManager(jwtGen, nil)
	manager.SetBaseURL(server.URL)

	ctx := context.Background()
	token, err := manager.GetToken(ctx, 123)

	assert.Error(t, err)
	assert.Nil(t, token)
	assert.Contains(t, err.Error(), "GitHub API returned status 404")
}

func TestInstallationTokenManager_RefreshToken(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		response := TokenResponse{
			Token:     "ghs_refreshed_token",
			ExpiresAt: time.Now().Add(1 * time.Hour),
			TokenType: "token",
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	jwtGen := createTestJWTGenerator()
	manager := NewInstallationTokenManager(jwtGen, nil)
	manager.SetBaseURL(server.URL)

	ctx := context.Background()

	// Get initial token
	_, err := manager.GetToken(ctx, 123)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount)

	// Refresh token (should make new API call)
	token2, err := manager.RefreshToken(ctx, 123)
	require.NoError(t, err)
	assert.Equal(t, 2, callCount)
	assert.Equal(t, "ghs_refreshed_token", token2.AccessToken)

	// Verify cache was updated
	token3, err := manager.GetToken(ctx, 123)
	require.NoError(t, err)
	assert.Equal(t, 2, callCount) // No new API call, token from cache
	assert.Equal(t, token2.AccessToken, token3.AccessToken)
}

func TestInstallationTokenManager_CreateTokenWithPermissions(t *testing.T) {
	var receivedBody map[string]interface{}
	
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		
		response := TokenResponse{
			Token:     "ghs_permission_token",
			ExpiresAt: time.Now().Add(1 * time.Hour),
			TokenType: "token",
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	jwtGen := createTestJWTGenerator()
	manager := NewInstallationTokenManager(jwtGen, nil)
	manager.SetBaseURL(server.URL)

	ctx := context.Background()
	permissions := map[string]string{
		"contents": "read",
		"issues":   "write",
	}
	repositories := []string{"repo1", "repo2"}

	token, err := manager.CreateTokenWithPermissions(ctx, 123, permissions, repositories)

	require.NoError(t, err)
	assert.NotNil(t, token)
	assert.Equal(t, "ghs_permission_token", token.AccessToken)

	// Verify request body
	expectedPermissions := map[string]interface{}{
		"contents": "read",
		"issues":   "write",
	}
	assert.Equal(t, expectedPermissions, receivedBody["permissions"])
	assert.Equal(t, []interface{}{"repo1", "repo2"}, receivedBody["repositories"])
}

func TestInstallationTokenManager_CacheOperations(t *testing.T) {
	jwtGen := createTestJWTGenerator()
	manager := NewInstallationTokenManager(jwtGen, nil)

	// Test initial state
	assert.Equal(t, 0, manager.GetCacheSize())

	// Add token to cache
	testToken := &Token{
		AccessToken: "test-token",
		ExpiresAt:   time.Now().Add(1 * time.Hour),
	}
	manager.cache.Set(123, testToken)
	assert.Equal(t, 1, manager.GetCacheSize())

	// Test invalidate
	manager.InvalidateToken(123)
	assert.Equal(t, 0, manager.GetCacheSize())

	// Test clear cache
	manager.cache.Set(123, testToken)
	manager.cache.Set(456, testToken)
	assert.Equal(t, 2, manager.GetCacheSize())
	
	manager.ClearCache()
	assert.Equal(t, 0, manager.GetCacheSize())
}

func TestInstallationTokenManager_ValidateInstallationAccess(t *testing.T) {
	// Success case
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := TokenResponse{
			Token:     "ghs_test_token",
			ExpiresAt: time.Now().Add(1 * time.Hour),
			TokenType: "token",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	jwtGen := createTestJWTGenerator()
	manager := NewInstallationTokenManager(jwtGen, nil)
	manager.SetBaseURL(server.URL)

	ctx := context.Background()
	err := manager.ValidateInstallationAccess(ctx, 123)
	assert.NoError(t, err)

	// Error case - test with invalid installation
	manager.SetBaseURL("http://localhost:99999") // Invalid URL
	err = manager.ValidateInstallationAccess(ctx, 456)
	assert.Error(t, err)
}