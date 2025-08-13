package app

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/qiniu/x/log"
)

// TokenRefresher handles automatic token refresh for installations
type TokenRefresher struct {
	tokenManager *InstallationTokenManager

	// Configuration
	refreshInterval  time.Duration // How often to check for expiring tokens
	refreshThreshold time.Duration // Refresh tokens expiring within this duration
	maxConcurrency   int           // Maximum number of concurrent refresh operations

	// State management
	running bool
	stopCh  chan struct{}
	doneCh  chan struct{}
	mu      sync.RWMutex

	// Metrics
	refreshCount  int64
	failureCount  int64
	lastRefreshAt time.Time
}

// TokenRefreshConfig configures the token refresher
type TokenRefreshConfig struct {
	RefreshInterval  time.Duration // Default: 5 minutes
	RefreshThreshold time.Duration // Default: 10 minutes (refresh tokens expiring within 10 minutes)
	MaxConcurrency   int           // Default: 5
}

// NewTokenRefresher creates a new token refresher
func NewTokenRefresher(tokenManager *InstallationTokenManager, config *TokenRefreshConfig) *TokenRefresher {
	if config == nil {
		config = &TokenRefreshConfig{
			RefreshInterval:  5 * time.Minute,
			RefreshThreshold: 10 * time.Minute,
			MaxConcurrency:   5,
		}
	}

	// Apply defaults
	if config.RefreshInterval <= 0 {
		config.RefreshInterval = 5 * time.Minute
	}
	if config.RefreshThreshold <= 0 {
		config.RefreshThreshold = 10 * time.Minute
	}
	if config.MaxConcurrency <= 0 {
		config.MaxConcurrency = 5
	}

	return &TokenRefresher{
		tokenManager:     tokenManager,
		refreshInterval:  config.RefreshInterval,
		refreshThreshold: config.RefreshThreshold,
		maxConcurrency:   config.MaxConcurrency,
		stopCh:           make(chan struct{}),
		doneCh:           make(chan struct{}),
	}
}

// Start begins the automatic token refresh process
func (r *TokenRefresher) Start(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.running {
		return fmt.Errorf("token refresher is already running")
	}

	r.running = true

	go r.run(ctx)

	log.Infof("Token refresher started with interval %v, threshold %v",
		r.refreshInterval, r.refreshThreshold)

	return nil
}

// Stop stops the automatic token refresh process
func (r *TokenRefresher) Stop() error {
	r.mu.Lock()
	if !r.running {
		r.mu.Unlock()
		return fmt.Errorf("token refresher is not running")
	}
	r.mu.Unlock()

	close(r.stopCh)

	// Wait for the refresh loop to finish
	select {
	case <-r.doneCh:
		log.Infof("Token refresher stopped")
	case <-time.After(30 * time.Second):
		log.Warnf("Token refresher stop timeout")
	}

	r.mu.Lock()
	r.running = false
	r.mu.Unlock()

	return nil
}

// IsRunning returns whether the refresher is currently running
func (r *TokenRefresher) IsRunning() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.running
}

// run is the main refresh loop
func (r *TokenRefresher) run(ctx context.Context) {
	defer close(r.doneCh)

	ticker := time.NewTicker(r.refreshInterval)
	defer ticker.Stop()

	log.Infof("Token refresh loop started")

	for {
		select {
		case <-ctx.Done():
			log.Infof("Token refresher stopped due to context cancellation")
			return
		case <-r.stopCh:
			log.Infof("Token refresher stopped")
			return
		case <-ticker.C:
			r.performRefreshCycle(ctx)
		}
	}
}

// performRefreshCycle checks for expiring tokens and refreshes them
func (r *TokenRefresher) performRefreshCycle(ctx context.Context) {
	start := time.Now()

	// Clean up expired tokens first
	r.tokenManager.CleanupExpiredTokens()

	// Get installations with expiring tokens
	expiringInstallations := r.tokenManager.GetExpiringTokens(r.refreshThreshold)

	if len(expiringInstallations) == 0 {
		return
	}

	log.Infof("Found %d installations with expiring tokens", len(expiringInstallations))

	// Create a semaphore to limit concurrency
	semaphore := make(chan struct{}, r.maxConcurrency)
	var wg sync.WaitGroup

	// Refresh tokens concurrently
	for _, installationID := range expiringInstallations {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			r.refreshInstallationToken(ctx, id)
		}(installationID)
	}

	// Wait for all refresh operations to complete
	wg.Wait()

	r.mu.Lock()
	r.lastRefreshAt = time.Now()
	r.mu.Unlock()

	duration := time.Since(start)
	log.Infof("Refresh cycle completed in %v for %d installations", duration, len(expiringInstallations))
}

// refreshInstallationToken refreshes a token for a specific installation
func (r *TokenRefresher) refreshInstallationToken(ctx context.Context, installationID int64) {
	log.Infof("Refreshing token for installation %d", installationID)

	// Create a context with timeout for this specific refresh
	refreshCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	_, err := r.tokenManager.RefreshToken(refreshCtx, installationID)
	if err != nil {
		log.Errorf("Failed to refresh token for installation %d: %v", installationID, err)

		r.mu.Lock()
		r.failureCount++
		r.mu.Unlock()

		return
	}

	log.Infof("Successfully refreshed token for installation %d", installationID)

	r.mu.Lock()
	r.refreshCount++
	r.mu.Unlock()
}

// RefreshNow triggers an immediate refresh cycle
func (r *TokenRefresher) RefreshNow(ctx context.Context) {
	if !r.IsRunning() {
		log.Warnf("Token refresher is not running, cannot perform immediate refresh")
		return
	}

	log.Infof("Performing immediate token refresh")
	r.performRefreshCycle(ctx)
}

// GetStats returns refresh statistics
func (r *TokenRefresher) GetStats() RefreshStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return RefreshStats{
		RefreshCount:    r.refreshCount,
		FailureCount:    r.failureCount,
		LastRefreshAt:   r.lastRefreshAt,
		IsRunning:       r.running,
		RefreshInterval: r.refreshInterval,
	}
}

// RefreshStats contains statistics about token refresh operations
type RefreshStats struct {
	RefreshCount    int64         `json:"refresh_count"`
	FailureCount    int64         `json:"failure_count"`
	LastRefreshAt   time.Time     `json:"last_refresh_at"`
	IsRunning       bool          `json:"is_running"`
	RefreshInterval time.Duration `json:"refresh_interval"`
}

// SetRefreshInterval updates the refresh interval
func (r *TokenRefresher) SetRefreshInterval(interval time.Duration) {
	if interval <= 0 {
		log.Warnf("Invalid refresh interval %v, ignoring", interval)
		return
	}

	r.mu.Lock()
	r.refreshInterval = interval
	r.mu.Unlock()

	log.Infof("Updated refresh interval to %v", interval)
}

// SetRefreshThreshold updates the refresh threshold
func (r *TokenRefresher) SetRefreshThreshold(threshold time.Duration) {
	if threshold <= 0 {
		log.Warnf("Invalid refresh threshold %v, ignoring", threshold)
		return
	}

	r.mu.Lock()
	r.refreshThreshold = threshold
	r.mu.Unlock()

	log.Infof("Updated refresh threshold to %v", threshold)
}
