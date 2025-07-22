package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolvePaths(t *testing.T) {
	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "config_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 创建配置文件
	configContent := `workspace:
  base_dir: "./codeagent"
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

	// 验证路径是否被正确解析为绝对路径
	expectedPath := filepath.Join(tempDir, "codeagent")
	if config.Workspace.BaseDir != expectedPath {
		t.Errorf("Expected base_dir to be %s, but got %s", expectedPath, config.Workspace.BaseDir)
	}

	// 验证路径是绝对路径
	if !filepath.IsAbs(config.Workspace.BaseDir) {
		t.Errorf("Expected base_dir to be absolute path, but got %s", config.Workspace.BaseDir)
	}
}

func TestResolvePathsWithAbsolutePath(t *testing.T) {
	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "config_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 创建配置文件，使用绝对路径
	configContent := `workspace:
  base_dir: "/tmp/absolute/path"
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

	// 验证绝对路径保持不变
	expectedPath := "/tmp/absolute/path"
	if config.Workspace.BaseDir != expectedPath {
		t.Errorf("Expected base_dir to be %s, but got %s", expectedPath, config.Workspace.BaseDir)
	}
}

func TestResolvePathsFromEnv(t *testing.T) {
	// 设置环境变量
	os.Setenv("WORKSPACE_BASE_DIR", "./relative/path")
	defer os.Unsetenv("WORKSPACE_BASE_DIR")

	// 加载配置（从环境变量）
	config, err := Load("nonexistent.yaml")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// 验证相对路径被解析为绝对路径
	if !filepath.IsAbs(config.Workspace.BaseDir) {
		t.Errorf("Expected base_dir to be absolute path, but got %s", config.Workspace.BaseDir)
	}

	// 验证路径包含预期的相对部分
	if !strings.HasSuffix(config.Workspace.BaseDir, "relative/path") {
		t.Errorf("Expected base_dir to end with 'relative/path', but got %s", config.Workspace.BaseDir)
	}
}
