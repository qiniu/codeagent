package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePaths(t *testing.T) {
	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "config-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 创建配置文件
	configContent := `workspace:
  base_dir: "./relative/path"
`
	configPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// 加载配置
	config, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// 验证路径已解析为绝对路径
	expectedPath := filepath.Join(tempDir, "relative", "path")
	if config.Workspace.BaseDir != expectedPath {
		t.Errorf("Expected base_dir to be %s, got %s", expectedPath, config.Workspace.BaseDir)
	}
}

func TestResolvePathsWithAbsolutePath(t *testing.T) {
	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "config-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 创建配置文件，使用绝对路径
	absolutePath := "/absolute/path"
	configContent := fmt.Sprintf(`workspace:
  base_dir: "%s"
`, absolutePath)
	configPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// 加载配置
	config, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// 验证绝对路径保持不变
	if config.Workspace.BaseDir != absolutePath {
		t.Errorf("Expected base_dir to remain %s, got %s", absolutePath, config.Workspace.BaseDir)
	}
}

func TestResolvePathsFromEnv(t *testing.T) {
	// 设置环境变量
	originalEnv := os.Getenv("WORKSPACE_BASE_DIR")
	defer os.Setenv("WORKSPACE_BASE_DIR", originalEnv)

	relativePath := "./env/relative/path"
	os.Setenv("WORKSPACE_BASE_DIR", relativePath)

	// 加载配置（从环境变量）
	config, err := Load("nonexistent.yaml")
	if err != nil {
		t.Fatalf("Failed to load config from env: %v", err)
	}

	// 获取当前工作目录
	currentDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}

	// 验证路径已解析为绝对路径
	expectedPath := filepath.Join(currentDir, "env", "relative", "path")
	if config.Workspace.BaseDir != expectedPath {
		t.Errorf("Expected base_dir to be %s, got %s", expectedPath, config.Workspace.BaseDir)
	}
}

func TestClaudeEnvironmentVariables(t *testing.T) {
	// 保存原始环境变量
	originalAPIKey := os.Getenv("ANTHROPIC_API_KEY")
	originalBaseURL := os.Getenv("ANTHROPIC_BASE_URL")
	defer func() {
		os.Setenv("ANTHROPIC_API_KEY", originalAPIKey)
		os.Setenv("ANTHROPIC_BASE_URL", originalBaseURL)
	}()

	// 设置测试环境变量
	testAPIKey := "test-api-key-123"
	testBaseURL := "https://test-api.anthropic.com"
	os.Setenv("ANTHROPIC_API_KEY", testAPIKey)
	os.Setenv("ANTHROPIC_BASE_URL", testBaseURL)

	// 加载配置（从环境变量）
	config, err := Load("nonexistent.yaml")
	if err != nil {
		t.Fatalf("Failed to load config from env: %v", err)
	}

	// 验证环境变量已正确加载
	if config.Claude.APIKey != testAPIKey {
		t.Errorf("Expected Claude API key to be %s, got %s", testAPIKey, config.Claude.APIKey)
	}

	if config.Claude.BaseURL != testBaseURL {
		t.Errorf("Expected Claude base URL to be %s, got %s", testBaseURL, config.Claude.BaseURL)
	}
}

func TestClaudeConfigFileOverride(t *testing.T) {
	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "config-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 保存原始环境变量
	originalAPIKey := os.Getenv("ANTHROPIC_API_KEY")
	originalBaseURL := os.Getenv("ANTHROPIC_BASE_URL")
	defer func() {
		os.Setenv("ANTHROPIC_API_KEY", originalAPIKey)
		os.Setenv("ANTHROPIC_BASE_URL", originalBaseURL)
	}()

	// 设置环境变量
	envAPIKey := "env-api-key"
	envBaseURL := "https://env-api.anthropic.com"
	os.Setenv("ANTHROPIC_API_KEY", envAPIKey)
	os.Setenv("ANTHROPIC_BASE_URL", envBaseURL)

	// 创建配置文件，包含不同的值
	configContent := `claude:
  api_key: file-api-key
  base_url: https://file-api.anthropic.com
`
	configPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// 加载配置
	config, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// 验证环境变量覆盖了配置文件的值
	if config.Claude.APIKey != envAPIKey {
		t.Errorf("Expected Claude API key to be %s (from env), got %s", envAPIKey, config.Claude.APIKey)
	}

	if config.Claude.BaseURL != envBaseURL {
		t.Errorf("Expected Claude base URL to be %s (from env), got %s", envBaseURL, config.Claude.BaseURL)
	}
}
