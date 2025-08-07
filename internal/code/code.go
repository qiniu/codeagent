package code

import (
	"fmt"
	"io"

	"github.com/qiniu/codeagent/internal/config"
	"github.com/qiniu/codeagent/pkg/models"
)

const (
	ProviderClaude = "claude"
	ProviderGemini = "gemini"
)

type Response struct {
	Out io.Reader
}

type Code interface {
	Prompt(message string) (*Response, error)
	Close() error
}

func New(workspace *models.Workspace, cfg *config.Config) (Code, error) {
	// Prioritize using AI model specified in workspace, if not available use default model from configuration
	var provider string
	if workspace.AIModel != "" {
		provider = workspace.AIModel
	} else {
		provider = cfg.CodeProvider
	}

	// Create corresponding code provider based on code provider and use_docker configuration
	switch provider {
	case ProviderClaude:
		if cfg.UseDocker {
			// Check if interactive mode is enabled
			if cfg.Claude.Interactive {
				return NewClaudeInteractive(workspace, cfg)
			}
			return NewClaudeDocker(workspace, cfg)
		}
		return NewClaudeLocal(workspace, cfg)
	case ProviderGemini:
		if cfg.UseDocker {
			return NewGeminiDocker(workspace, cfg)
		}
		return NewGeminiLocal(workspace, cfg)
	default:
		return nil, fmt.Errorf("unsupported code provider: %s", provider)
	}
}
