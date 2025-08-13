package app

import (
	"sync"
	"time"
)

// Token represents an installation access token with expiration
type Token struct {
	AccessToken string    `json:"token"`
	ExpiresAt   time.Time `json:"expires_at"`
	TokenType   string    `json:"token_type,omitempty"`
}

// IsExpired checks if the token is expired or close to expiring
// We consider a token expired if it expires within 5 minutes
func (t *Token) IsExpired() bool {
	if t.ExpiresAt.IsZero() {
		return false
	}
	// Consider token expired if it expires within 5 minutes (safety margin)
	return time.Now().Add(5 * time.Minute).After(t.ExpiresAt)
}

// TokenCache defines the interface for token caching
type TokenCache interface {
	Get(installationID int64) (*Token, bool)
	Set(installationID int64, token *Token)
	Delete(installationID int64)
	Clear()
	Size() int
}

// MemoryTokenCache implements an in-memory token cache with TTL support
type MemoryTokenCache struct {
	mu     sync.RWMutex
	tokens map[int64]*Token
}

// NewMemoryTokenCache creates a new in-memory token cache
func NewMemoryTokenCache() *MemoryTokenCache {
	return &MemoryTokenCache{
		tokens: make(map[int64]*Token),
	}
}

// Get retrieves a token from the cache
func (c *MemoryTokenCache) Get(installationID int64) (*Token, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	token, exists := c.tokens[installationID]
	if !exists {
		return nil, false
	}

	// Check if token is expired
	if token.IsExpired() {
		// Don't return expired tokens, but don't delete them here
		// Let the cleanup process handle deletion
		return nil, false
	}

	return token, true
}

// Set stores a token in the cache
func (c *MemoryTokenCache) Set(installationID int64, token *Token) {
	if token == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.tokens[installationID] = token
}

// Delete removes a token from the cache
func (c *MemoryTokenCache) Delete(installationID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.tokens, installationID)
}

// Clear removes all tokens from the cache
func (c *MemoryTokenCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.tokens = make(map[int64]*Token)
}

// Size returns the number of tokens in the cache
func (c *MemoryTokenCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.tokens)
}

// Cleanup removes expired tokens from the cache
func (c *MemoryTokenCache) Cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for installationID, token := range c.tokens {
		if token.IsExpired() {
			delete(c.tokens, installationID)
		}
	}
}

// GetExpiredInstallations returns a list of installation IDs with expired tokens
func (c *MemoryTokenCache) GetExpiredInstallations() []int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var expired []int64
	for installationID, token := range c.tokens {
		if token.IsExpired() {
			expired = append(expired, installationID)
		}
	}

	return expired
}

// GetExpiringInstallations returns installation IDs with tokens expiring within the given duration
func (c *MemoryTokenCache) GetExpiringInstallations(within time.Duration) []int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	threshold := time.Now().Add(within)
	var expiring []int64

	for installationID, token := range c.tokens {
		if !token.ExpiresAt.IsZero() && token.ExpiresAt.Before(threshold) {
			expiring = append(expiring, installationID)
		}
	}

	return expiring
}
