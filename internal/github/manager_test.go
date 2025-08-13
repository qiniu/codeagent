package github

import (
	"context"
	"testing"

	"github.com/qiniu/codeagent/internal/config"
	"github.com/qiniu/codeagent/internal/github/auth"
)

func TestNewGitHubClientManager(t *testing.T) {
	tests := []struct {
		name           string
		config         *config.Config
		expectError    bool
		expectedAuth   auth.AuthType
	}{
		{
			name:        "nil config should fail",
			config:      nil,
			expectError: true,
		},
		{
			name: "PAT configuration",
			config: &config.Config{
				GitHub: config.GitHubConfig{
					Token: "test-token",
				},
			},
			expectError:  false,
			expectedAuth: auth.AuthTypePAT,
		},
		{
			name: "GitHub App configuration",
			config: &config.Config{
				GitHub: config.GitHubConfig{
					App: config.GitHubAppConfig{
						AppID:          12345,
						PrivateKeyPath: "/nonexistent/path", // This will fail, but we're just testing structure
					},
				},
			},
			expectError: true, // Will fail due to missing private key file
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, err := NewGitHubClientManager(tt.config)
			
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}
			
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			
			if manager == nil {
				t.Errorf("Manager should not be nil")
				return
			}
			
			// Test auth info
			authInfo := manager.GetAuthInfo()
			if authInfo.Type != tt.expectedAuth {
				t.Errorf("Expected auth type %s, got %s", tt.expectedAuth, authInfo.Type)
			}
		})
	}
}

func TestGitHubClientManager_AuthModeDetection(t *testing.T) {
	tests := []struct {
		name     string
		config   *config.Config
		expected string
	}{
		{
			name: "PAT detection",
			config: &config.Config{
				GitHub: config.GitHubConfig{
					Token: "test-token",
				},
			},
			expected: "pat",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, err := NewGitHubClientManager(tt.config)
			if err != nil {
				t.Fatalf("Failed to create manager: %v", err)
			}

			if manager.IsPATConfigured() != (tt.expected == "pat") {
				t.Errorf("PAT detection mismatch")
			}

			if manager.IsGitHubAppConfigured() != (tt.expected == "app") {
				t.Errorf("GitHub App detection mismatch")
			}
		})
	}
}

func TestGitHubClientManager_ContextInstallationID(t *testing.T) {
	// Test context installation ID handling
	ctx := context.Background()
	
	// Test setting installation ID in context
	testID := int64(12345)
	ctxWithID := SetInstallationIDInContext(ctx, testID)
	
	// Test retrieving installation ID from context
	retrievedID, ok := GetInstallationIDFromContext(ctxWithID)
	if !ok {
		t.Errorf("Failed to retrieve installation ID from context")
	}
	
	if retrievedID != testID {
		t.Errorf("Expected installation ID %d, got %d", testID, retrievedID)
	}
	
	// Test context without installation ID
	_, ok = GetInstallationIDFromContext(ctx)
	if ok {
		t.Errorf("Should not find installation ID in empty context")
	}
}

func TestGitHubClientManager_GetClient(t *testing.T) {
	// Test with valid PAT configuration
	config := &config.Config{
		GitHub: config.GitHubConfig{
			Token: "test-token",
		},
	}
	
	manager, err := NewGitHubClientManager(config)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	
	ctx := context.Background()
	client, err := manager.GetClient(ctx)
	if err != nil {
		t.Errorf("Failed to get client: %v", err)
	}
	
	if client == nil {
		t.Errorf("Client should not be nil")
	}
}

func TestGitHubClientManager_AuthInfo(t *testing.T) {
	config := &config.Config{
		GitHub: config.GitHubConfig{
			Token: "test-token",
		},
	}
	
	manager, err := NewGitHubClientManager(config)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	
	authInfo := manager.GetAuthInfo()
	if authInfo.Type != auth.AuthTypePAT {
		t.Errorf("Expected PAT auth type, got %s", authInfo.Type)
	}
	
	detectResult := manager.DetectAuthMode()
	if detectResult == "" {
		t.Errorf("DetectAuthMode should return non-empty string")
	}
}