package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitHubAppConfig(t *testing.T) {
	// Test environment variable loading for GitHub App
	originalEnv := map[string]string{
		"GITHUB_APP_ID":               os.Getenv("GITHUB_APP_ID"),
		"GITHUB_APP_PRIVATE_KEY_PATH": os.Getenv("GITHUB_APP_PRIVATE_KEY_PATH"),
		"GITHUB_APP_PRIVATE_KEY_ENV":  os.Getenv("GITHUB_APP_PRIVATE_KEY_ENV"),
		"GITHUB_APP_PRIVATE_KEY":      os.Getenv("GITHUB_APP_PRIVATE_KEY"),
	}

	// Clean up environment variables after test
	defer func() {
		for key, value := range originalEnv {
			if value == "" {
				os.Unsetenv(key)
			} else {
				os.Setenv(key, value)
			}
		}
	}()

	// Set test environment variables
	os.Setenv("GITHUB_APP_ID", "12345")
	os.Setenv("GITHUB_APP_PRIVATE_KEY_PATH", "/path/to/key.pem")
	os.Setenv("GITHUB_APP_PRIVATE_KEY_ENV", "PRIVATE_KEY_CONTENT")
	os.Setenv("GITHUB_APP_PRIVATE_KEY", "test-private-key")

	config := &Config{}
	config.loadFromEnv()

	assert.Equal(t, int64(12345), config.GitHub.App.AppID)
	assert.Equal(t, "/path/to/key.pem", config.GitHub.App.PrivateKeyPath)
	assert.Equal(t, "PRIVATE_KEY_CONTENT", config.GitHub.App.PrivateKeyEnv)
	assert.Equal(t, "test-private-key", config.GitHub.App.PrivateKey)
}

func TestValidateGitHubConfig(t *testing.T) {
	tests := []struct {
		name          string
		config        GitHubConfig
		expectError   bool
		errorContains string
	}{
		{
			name: "valid PAT configuration",
			config: GitHubConfig{
				Token: "ghp_test_token",
			},
			expectError: false,
		},
		{
			name: "valid GitHub App configuration",
			config: GitHubConfig{
				App: GitHubAppConfig{
					AppID:          12345,
					PrivateKeyPath: "/path/to/key.pem",
				},
			},
			expectError: false,
		},
		{
			name: "valid configuration with both (App should take priority)",
			config: GitHubConfig{
				Token: "ghp_test_token",
				App: GitHubAppConfig{
					AppID:          12345,
					PrivateKeyPath: "/path/to/key.pem",
				},
			},
			expectError: false,
		},
		{
			name: "invalid app configuration without app ID",
			config: GitHubConfig{
				App: GitHubAppConfig{
					PrivateKeyPath: "/path/to/key.pem",
				},
			},
			expectError:   true,
			errorContains: "GitHub authentication is required",
		},
		{
			name: "invalid app configuration without private key",
			config: GitHubConfig{
				App: GitHubAppConfig{
					AppID: 12345,
				},
			},
			expectError:   true,
			errorContains: "GitHub authentication is required",
		},
		{
			name:          "no authentication configured",
			config:        GitHubConfig{},
			expectError:   true,
			errorContains: "GitHub authentication is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{GitHub: tt.config}
			err := config.ValidateGitHubConfig()

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIsGitHubAppConfigured(t *testing.T) {
	tests := []struct {
		name     string
		config   GitHubConfig
		expected bool
	}{
		{
			name: "fully configured app",
			config: GitHubConfig{
				App: GitHubAppConfig{
					AppID:          12345,
					PrivateKeyPath: "/path/to/key.pem",
				},
			},
			expected: true,
		},
		{
			name: "app with private key env",
			config: GitHubConfig{
				App: GitHubAppConfig{
					AppID:         12345,
					PrivateKeyEnv: "PRIVATE_KEY",
				},
			},
			expected: true,
		},
		{
			name: "app with direct private key",
			config: GitHubConfig{
				App: GitHubAppConfig{
					AppID:      12345,
					PrivateKey: "direct-key-content",
				},
			},
			expected: true,
		},
		{
			name: "missing app ID",
			config: GitHubConfig{
				App: GitHubAppConfig{
					PrivateKeyPath: "/path/to/key.pem",
				},
			},
			expected: false,
		},
		{
			name: "missing private key",
			config: GitHubConfig{
				App: GitHubAppConfig{
					AppID: 12345,
				},
			},
			expected: false,
		},
		{
			name:     "empty config",
			config:   GitHubConfig{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{GitHub: tt.config}
			assert.Equal(t, tt.expected, config.IsGitHubAppConfigured())
		})
	}
}

func TestIsGitHubTokenConfigured(t *testing.T) {
	tests := []struct {
		name     string
		config   GitHubConfig
		expected bool
	}{
		{
			name: "token configured",
			config: GitHubConfig{
				Token: "ghp_test_token",
			},
			expected: true,
		},
		{
			name:     "token not configured",
			config:   GitHubConfig{},
			expected: false,
		},
		{
			name: "empty token",
			config: GitHubConfig{
				Token: "",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{GitHub: tt.config}
			assert.Equal(t, tt.expected, config.IsGitHubTokenConfigured())
		})
	}
}

func TestGetGitHubAuthType(t *testing.T) {
	tests := []struct {
		name     string
		config   GitHubConfig
		expected string
	}{
		{
			name: "app configuration (should prioritize over token)",
			config: GitHubConfig{
				Token: "ghp_test_token",
				App: GitHubAppConfig{
					AppID:          12345,
					PrivateKeyPath: "/path/to/key.pem",
				},
			},
			expected: "app",
		},
		{
			name: "app configuration only",
			config: GitHubConfig{
				App: GitHubAppConfig{
					AppID:          12345,
					PrivateKeyPath: "/path/to/key.pem",
				},
			},
			expected: "app",
		},
		{
			name: "token configuration only",
			config: GitHubConfig{
				Token: "ghp_test_token",
			},
			expected: "token",
		},
		{
			name:     "no configuration",
			config:   GitHubConfig{},
			expected: "none",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{GitHub: tt.config}
			assert.Equal(t, tt.expected, config.GetGitHubAuthType())
		})
	}
}

func TestConfigSetDefaults(t *testing.T) {
	config := &Config{}
	config.SetDefaults()

	assert.Equal(t, "/tmp/codeagent", config.Workspace.BaseDir)
	assert.Equal(t, 8080, config.Server.Port)
	assert.NotZero(t, config.Workspace.CleanupAfter)
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name          string
		config        Config
		expectError   bool
		errorContains string
	}{
		{
			name: "valid configuration with PAT",
			config: Config{
				Server: ServerConfig{
					Port:          8080,
					WebhookSecret: "test-secret",
				},
				GitHub: GitHubConfig{
					Token: "ghp_test_token",
				},
				CodeProvider: "claude",
			},
			expectError: false,
		},
		{
			name: "valid configuration with GitHub App",
			config: Config{
				Server: ServerConfig{
					Port:          8080,
					WebhookSecret: "test-secret",
				},
				GitHub: GitHubConfig{
					App: GitHubAppConfig{
						AppID:          12345,
						PrivateKeyPath: "/path/to/key.pem",
					},
				},
				CodeProvider: "gemini",
			},
			expectError: false,
		},
		{
			name: "missing webhook secret",
			config: Config{
				Server: ServerConfig{
					Port: 8080,
				},
				GitHub: GitHubConfig{
					Token: "ghp_test_token",
				},
				CodeProvider: "claude",
			},
			expectError:   true,
			errorContains: "webhook secret is required",
		},
		{
			name: "invalid port",
			config: Config{
				Server: ServerConfig{
					Port:          70000,
					WebhookSecret: "test-secret",
				},
				GitHub: GitHubConfig{
					Token: "ghp_test_token",
				},
				CodeProvider: "claude",
			},
			expectError:   true,
			errorContains: "invalid server port",
		},
		{
			name: "invalid code provider",
			config: Config{
				Server: ServerConfig{
					Port:          8080,
					WebhookSecret: "test-secret",
				},
				GitHub: GitHubConfig{
					Token: "ghp_test_token",
				},
				CodeProvider: "invalid",
			},
			expectError:   true,
			errorContains: "invalid code provider",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := tt.config
			err := config.Validate()

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestBackwardCompatibility(t *testing.T) {
	// Test that existing configurations still work
	originalEnv := map[string]string{
		"GITHUB_TOKEN":   os.Getenv("GITHUB_TOKEN"),
		"WEBHOOK_SECRET": os.Getenv("WEBHOOK_SECRET"),
		"CODE_PROVIDER":  os.Getenv("CODE_PROVIDER"),
	}

	defer func() {
		for key, value := range originalEnv {
			if value == "" {
				os.Unsetenv(key)
			} else {
				os.Setenv(key, value)
			}
		}
	}()

	// Set traditional environment variables
	os.Setenv("GITHUB_TOKEN", "ghp_test_token")
	os.Setenv("WEBHOOK_SECRET", "test-secret")
	os.Setenv("CODE_PROVIDER", "claude")

	config := loadFromEnv()
	require.NotNil(t, config)

	// Validate that the configuration is valid
	err := config.Validate()
	assert.NoError(t, err)

	// Check that auth type is detected as token
	assert.Equal(t, "token", config.GetGitHubAuthType())
	assert.True(t, config.IsGitHubTokenConfigured())
	assert.False(t, config.IsGitHubAppConfigured())
}
