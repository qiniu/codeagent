package github

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/qiniu/codeagent/internal/config"
	"github.com/qiniu/x/log"
)

// RateLimitMonitor monitors and logs GitHub API rate limit usage
type RateLimitMonitor struct {
	config     *config.GitHubAPIConfig
	mutex      sync.RWMutex
	restLimit  *RateLimitStatus
	graphLimit *RateLimitStatus
	
	// Statistics
	restCalls    int64
	graphCalls   int64
	totalSaved   int64  // API calls saved by using GraphQL
}

// RateLimitStatus represents the current rate limit status
type RateLimitStatus struct {
	Limit     int       `json:"limit"`
	Remaining int       `json:"remaining"`
	ResetAt   time.Time `json:"reset_at"`
	LastCheck time.Time `json:"last_check"`
}

// NewRateLimitMonitor creates a new rate limit monitor
func NewRateLimitMonitor(config *config.GitHubAPIConfig) *RateLimitMonitor {
	return &RateLimitMonitor{
		config:     config,
		restLimit:  &RateLimitStatus{},
		graphLimit: &RateLimitStatus{},
	}
}

// RecordRESTAPICall records a REST API call and its rate limit info
func (m *RateLimitMonitor) RecordRESTAPICall(limit, remaining int, resetAt time.Time) {
	if !m.config.EnableRateMonitoring {
		return
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.restCalls++
	m.restLimit = &RateLimitStatus{
		Limit:     limit,
		Remaining: remaining,
		ResetAt:   resetAt,
		LastCheck: time.Now(),
	}

	m.checkAndWarnRateLimit("REST", remaining, limit)
	m.logRateLimitStatus("REST")
}

// RecordGraphQLAPICall records a GraphQL API call and its rate limit info
func (m *RateLimitMonitor) RecordGraphQLAPICall(limit, remaining, cost int, resetAt time.Time, callsReplaced int) {
	if !m.config.EnableRateMonitoring {
		return
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.graphCalls++
	m.totalSaved += int64(callsReplaced - 1) // -1 because we still made 1 call
	
	m.graphLimit = &RateLimitStatus{
		Limit:     limit,
		Remaining: remaining,
		ResetAt:   resetAt,
		LastCheck: time.Now(),
	}

	m.checkAndWarnRateLimit("GraphQL", remaining, limit)
	
	log.Infof("GraphQL API call - Cost: %d, Remaining: %d/%d, Replaced %d REST calls, Total saved: %d", 
		cost, remaining, limit, callsReplaced, m.totalSaved)
}

// checkAndWarnRateLimit checks if rate limit is below threshold and logs warning
func (m *RateLimitMonitor) checkAndWarnRateLimit(apiType string, remaining, limit int) {
	if remaining <= m.config.RateLimitThreshold {
		percentage := float64(remaining) / float64(limit) * 100
		log.Warnf("%s API rate limit warning: %d/%d remaining (%.1f%%), resets at %s", 
			apiType, remaining, limit, percentage, 
			m.getResetTime(apiType).Format("15:04:05"))
		
		if percentage < 10 {
			log.Errorf("%s API rate limit critically low: %d/%d remaining (%.1f%%)", 
				apiType, remaining, limit, percentage)
		}
	}
}

// logRateLimitStatus logs current rate limit status for debugging
func (m *RateLimitMonitor) logRateLimitStatus(apiType string) {
	var status *RateLimitStatus
	if apiType == "REST" {
		status = m.restLimit
	} else {
		status = m.graphLimit
	}
	
	log.Debugf("%s API rate limit: %d/%d remaining, resets at %s", 
		apiType, status.Remaining, status.Limit, status.ResetAt.Format("15:04:05"))
}

// getResetTime returns the reset time for the specified API type
func (m *RateLimitMonitor) getResetTime(apiType string) time.Time {
	if apiType == "REST" {
		return m.restLimit.ResetAt
	}
	return m.graphLimit.ResetAt
}

// GetStatistics returns current rate limit statistics
func (m *RateLimitMonitor) GetStatistics() *RateLimitStatistics {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	return &RateLimitStatistics{
		RESTCalls:     m.restCalls,
		GraphQLCalls:  m.graphCalls,
		TotalSaved:    m.totalSaved,
		RESTLimit:     m.copyRateLimitStatus(m.restLimit),
		GraphQLLimit:  m.copyRateLimitStatus(m.graphLimit),
		LastUpdated:   time.Now(),
	}
}

// copyRateLimitStatus creates a copy of RateLimitStatus
func (m *RateLimitMonitor) copyRateLimitStatus(status *RateLimitStatus) *RateLimitStatus {
	if status == nil {
		return nil
	}
	return &RateLimitStatus{
		Limit:     status.Limit,
		Remaining: status.Remaining,
		ResetAt:   status.ResetAt,
		LastCheck: status.LastCheck,
	}
}

// RateLimitStatistics contains comprehensive rate limit statistics
type RateLimitStatistics struct {
	RESTCalls     int64             `json:"rest_calls"`
	GraphQLCalls  int64             `json:"graphql_calls"`
	TotalSaved    int64             `json:"total_saved"`
	RESTLimit     *RateLimitStatus  `json:"rest_limit"`
	GraphQLLimit  *RateLimitStatus  `json:"graphql_limit"`
	LastUpdated   time.Time         `json:"last_updated"`
}

// LogStatistics logs comprehensive rate limit statistics
func (m *RateLimitMonitor) LogStatistics() {
	if !m.config.EnableRateMonitoring {
		return
	}
	
	stats := m.GetStatistics()
	
	log.Infof("=== GitHub API Rate Limit Statistics ===")
	log.Infof("REST API calls: %d", stats.RESTCalls)
	log.Infof("GraphQL API calls: %d", stats.GraphQLCalls)
	log.Infof("Total API calls saved: %d", stats.TotalSaved)
	
	if stats.RESTLimit != nil && stats.RESTLimit.Limit > 0 {
		restPercent := float64(stats.RESTLimit.Remaining) / float64(stats.RESTLimit.Limit) * 100
		log.Infof("REST API rate limit: %d/%d (%.1f%% remaining)", 
			stats.RESTLimit.Remaining, stats.RESTLimit.Limit, restPercent)
	}
	
	if stats.GraphQLLimit != nil && stats.GraphQLLimit.Limit > 0 {
		graphPercent := float64(stats.GraphQLLimit.Remaining) / float64(stats.GraphQLLimit.Limit) * 100
		log.Infof("GraphQL API rate limit: %d/%d (%.1f%% remaining)", 
			stats.GraphQLLimit.Remaining, stats.GraphQLLimit.Limit, graphPercent)
	}
	
	if stats.TotalSaved > 0 {
		log.Infof("🎉 GraphQL efficiency: Saved %d API calls (%d%% reduction)", 
			stats.TotalSaved, 
			int(float64(stats.TotalSaved)/float64(stats.RESTCalls+stats.GraphQLCalls)*100))
	}
	
	log.Infof("=========================================")
}

// IsRateLimitCritical checks if any API rate limit is critically low
func (m *RateLimitMonitor) IsRateLimitCritical() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	// Check REST API rate limit
	if m.restLimit != nil && m.restLimit.Limit > 0 {
		restPercent := float64(m.restLimit.Remaining) / float64(m.restLimit.Limit) * 100
		if restPercent < 10 {
			return true
		}
	}
	
	// Check GraphQL API rate limit
	if m.graphLimit != nil && m.graphLimit.Limit > 0 {
		graphPercent := float64(m.graphLimit.Remaining) / float64(m.graphLimit.Limit) * 100
		if graphPercent < 10 {
			return true
		}
	}
	
	return false
}

// WaitForRateLimit waits for rate limit to reset if necessary
func (m *RateLimitMonitor) WaitForRateLimit(ctx context.Context, apiType string) error {
	m.mutex.RLock()
	var status *RateLimitStatus
	if apiType == "REST" {
		status = m.restLimit
	} else {
		status = m.graphLimit
	}
	
	if status == nil || status.Remaining > 100 {
		m.mutex.RUnlock()
		return nil // Plenty of requests remaining
	}
	
	resetAt := status.ResetAt
	m.mutex.RUnlock()
	
	// If rate limit is very low, wait for reset
	if resetAt.After(time.Now()) {
		waitDuration := time.Until(resetAt)
		if waitDuration > 0 && waitDuration < 1*time.Hour {
			log.Warnf("Rate limit critically low for %s API, waiting %v for reset", apiType, waitDuration)
			
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(waitDuration):
				log.Infof("Rate limit should be reset for %s API", apiType)
				return nil
			}
		}
	}
	
	return nil
}

// StartPeriodicLogging starts periodic logging of rate limit statistics
func (m *RateLimitMonitor) StartPeriodicLogging(ctx context.Context, interval time.Duration) {
	if !m.config.EnableRateMonitoring || interval <= 0 {
		return
	}
	
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.LogStatistics()
			}
		}
	}()
}