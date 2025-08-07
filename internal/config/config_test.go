package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePaths(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "config-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create configuration file
	configContent := `workspace:
  base_dir: "./relative/path"
`
	configPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Load configuration
	config, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify path has been resolved to absolute path
	expectedPath := filepath.Join(tempDir, "relative", "path")
	if config.Workspace.BaseDir != expectedPath {
		t.Errorf("Expected base_dir to be %s, got %s", expectedPath, config.Workspace.BaseDir)
	}
}

func TestResolvePathsWithAbsolutePath(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "config-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create configuration file using absolute path
	absolutePath := "/absolute/path"
	configContent := fmt.Sprintf(`workspace:
  base_dir: "%s"
`, absolutePath)
	configPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Load configuration
	config, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify absolute path remains unchanged
	if config.Workspace.BaseDir != absolutePath {
		t.Errorf("Expected base_dir to remain %s, got %s", absolutePath, config.Workspace.BaseDir)
	}
}

func TestResolvePathsFromEnv(t *testing.T) {
	// Set environment variable
	originalEnv := os.Getenv("WORKSPACE_BASE_DIR")
	defer os.Setenv("WORKSPACE_BASE_DIR", originalEnv)

	relativePath := "./env/relative/path"
	os.Setenv("WORKSPACE_BASE_DIR", relativePath)

	// Load configuration (from environment variables)
	config, err := Load("nonexistent.yaml")
	if err != nil {
		t.Fatalf("Failed to load config from env: %v", err)
	}

	// Get current working directory
	currentDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}

	// Verify path has been resolved to absolute path
	expectedPath := filepath.Join(currentDir, "env", "relative", "path")
	if config.Workspace.BaseDir != expectedPath {
		t.Errorf("Expected base_dir to be %s, got %s", expectedPath, config.Workspace.BaseDir)
	}
}

func TestClaudeEnvironmentVariables(t *testing.T) {
	// Save original environment variables
	originalAPIKey := os.Getenv("ANTHROPIC_API_KEY")
	originalBaseURL := os.Getenv("ANTHROPIC_BASE_URL")
	defer func() {
		os.Setenv("ANTHROPIC_API_KEY", originalAPIKey)
		os.Setenv("ANTHROPIC_BASE_URL", originalBaseURL)
	}()

	// Set test environment variables
	testAPIKey := "test-api-key-123"
	testBaseURL := "https://test-api.anthropic.com"
	os.Setenv("ANTHROPIC_API_KEY", testAPIKey)
	os.Setenv("ANTHROPIC_BASE_URL", testBaseURL)

	// Load configuration (from environment variables)
	config, err := Load("nonexistent.yaml")
	if err != nil {
		t.Fatalf("Failed to load config from env: %v", err)
	}

	// Verify environment variables have been loaded correctly
	if config.Claude.APIKey != testAPIKey {
		t.Errorf("Expected Claude API key to be %s, got %s", testAPIKey, config.Claude.APIKey)
	}

	if config.Claude.BaseURL != testBaseURL {
		t.Errorf("Expected Claude base URL to be %s, got %s", testBaseURL, config.Claude.BaseURL)
	}
}

func TestClaudeConfigFileOverride(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "config-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Save original environment variables
	originalAPIKey := os.Getenv("ANTHROPIC_API_KEY")
	originalBaseURL := os.Getenv("ANTHROPIC_BASE_URL")
	defer func() {
		os.Setenv("ANTHROPIC_API_KEY", originalAPIKey)
		os.Setenv("ANTHROPIC_BASE_URL", originalBaseURL)
	}()

	// Set environment variable
	envAPIKey := "env-api-key"
	envBaseURL := "https://env-api.anthropic.com"
	os.Setenv("ANTHROPIC_API_KEY", envAPIKey)
	os.Setenv("ANTHROPIC_BASE_URL", envBaseURL)

	// Create configuration file with different values
	configContent := `claude:
  api_key: file-api-key
  base_url: https://file-api.anthropic.com
`
	configPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Load configuration
	config, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify environment variables override configuration file values
	if config.Claude.APIKey != envAPIKey {
		t.Errorf("Expected Claude API key to be %s (from env), got %s", envAPIKey, config.Claude.APIKey)
	}

	if config.Claude.BaseURL != envBaseURL {
		t.Errorf("Expected Claude base URL to be %s (from env), got %s", envBaseURL, config.Claude.BaseURL)
	}
}
