package modes

import (
	"context"
	"testing"

	"github.com/qiniu/codeagent/internal/config"
)

func TestReviewHandler_isAccountExcluded(t *testing.T) {
	// 创建测试配置
	config := &config.Config{
		Review: config.ReviewConfig{
			ExcludedAccounts: []string{
				"dependabot",
				"renovate",
				"test-bot",
			},
		},
	}

	// 创建ReviewHandler实例
	handler := &ReviewHandler{
		config: config,
	}

	ctx := context.Background()

	tests := []struct {
		name     string
		username string
		expected bool
	}{
		{
			name:     "[bot] suffix account should be excluded",
			username: "someuser[bot]",
			expected: true,
		},
		{
			name:     "[BOT] suffix account should be excluded",
			username: "someuser[BOT]",
			expected: true,
		},
		{
			name:     "exact match dependabot should be excluded",
			username: "dependabot",
			expected: true,
		},
		{
			name:     "exact match renovate should be excluded",
			username: "renovate",
			expected: true,
		},
		{
			name:     "exact match test-bot should be excluded",
			username: "test-bot",
			expected: true,
		},
		{
			name:     "my-bot should not be excluded (not in config)",
			username: "my-bot",
			expected: false,
		},
		{
			name:     "normal user should not be excluded",
			username: "normaluser",
			expected: false,
		},
		{
			name:     "user with bot in middle should not be excluded",
			username: "mybotuser",
			expected: false,
		},
		{
			name:     "case insensitive match should work",
			username: "DEPENDABOT",
			expected: true,
		},
		{
			name:     "user with -bot suffix should not be excluded (not [bot])",
			username: "someuser-bot",
			expected: false,
		},
		{
			name:     "user with -BOT suffix should not be excluded (not [BOT])",
			username: "someuser-BOT",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.isAccountExcluded(ctx, tt.username)
			if result != tt.expected {
				t.Errorf("isAccountExcluded(%s) = %v, want %v", tt.username, result, tt.expected)
			}
		})
	}
}

func TestReviewHandler_isAccountExcluded_EmptyConfig(t *testing.T) {
	// 测试空配置的情况
	config := &config.Config{
		Review: config.ReviewConfig{
			ExcludedAccounts: []string{},
		},
	}

	handler := &ReviewHandler{
		config: config,
	}

	ctx := context.Background()

	tests := []struct {
		name     string
		username string
		expected bool
	}{
		{
			name:     "[bot] suffix should still be excluded even with empty config",
			username: "someuser[bot]",
			expected: true,
		},
		{
			name:     "normal user should not be excluded with empty config",
			username: "normaluser",
			expected: false,
		},
		{
			name:     "-bot suffix should not be excluded with empty config (not [bot])",
			username: "someuser-bot",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.isAccountExcluded(ctx, tt.username)
			if result != tt.expected {
				t.Errorf("isAccountExcluded(%s) = %v, want %v", tt.username, result, tt.expected)
			}
		})
	}
}

func TestReviewHandler_isAccountExcluded_WildcardPatterns(t *testing.T) {
	// 测试通配符模式
	config := &config.Config{
		Review: config.ReviewConfig{
			ExcludedAccounts: []string{
				"*dependabot*",
				"*test*",
				"prefix-*",
				"*-suffix",
			},
		},
	}

	handler := &ReviewHandler{
		config: config,
	}

	ctx := context.Background()

	tests := []struct {
		name     string
		username string
		expected bool
	}{
		{
			name:     "pattern *dependabot* should match dependabot",
			username: "dependabot",
			expected: true,
		},
		{
			name:     "pattern *dependabot* should match my-dependabot-user",
			username: "my-dependabot-user",
			expected: true,
		},
		{
			name:     "pattern *test* should match testuser",
			username: "testuser",
			expected: true,
		},
		{
			name:     "pattern prefix-* should match prefix-anything",
			username: "prefix-anything",
			expected: true,
		},
		{
			name:     "pattern *-suffix should match anything-suffix",
			username: "anything-suffix",
			expected: true,
		},
		{
			name:     "pattern should not match unrelated user",
			username: "unrelated",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.isAccountExcluded(ctx, tt.username)
			if result != tt.expected {
				t.Errorf("isAccountExcluded(%s) = %v, want %v", tt.username, result, tt.expected)
			}
		})
	}
}
