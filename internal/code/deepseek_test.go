package code

import (
	"testing"
	"time"

	"github.com/qiniu/codeagent/internal/config"
	"github.com/qiniu/codeagent/pkg/models"
)

func TestNewDeepSeekLocal(t *testing.T) {
	workspace := &models.Workspace{
		Path: "/tmp/test",
	}

	cfg := &config.Config{
		DeepSeek: config.DeepSeekConfig{
			APIKey:  "test-key",
			BaseURL: "https://api.deepseek.com",
			Model:   "deepseek-chat",
			Timeout: 30 * time.Minute,
		},
	}

	deepseek, err := NewDeepSeekLocal(workspace, cfg)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if deepseek == nil {
		t.Fatal("Expected deepseek instance, got nil")
	}

	// Test Close
	err = deepseek.Close()
	if err != nil {
		t.Fatalf("Expected no error on close, got %v", err)
	}
}

func TestNewDeepSeekLocalMissingAPIKey(t *testing.T) {
	workspace := &models.Workspace{
		Path: "/tmp/test",
	}

	cfg := &config.Config{
		DeepSeek: config.DeepSeekConfig{
			BaseURL: "https://api.deepseek.com",
			Model:   "deepseek-chat",
			Timeout: 30 * time.Minute,
		},
	}

	_, err := NewDeepSeekLocal(workspace, cfg)
	if err == nil {
		t.Fatal("Expected error for missing API key, got nil")
	}
	
	expectedErr := "DEEPSEEK_API_KEY is required"
	if err.Error() != expectedErr {
		t.Fatalf("Expected error '%s', got '%s'", expectedErr, err.Error())
	}
}