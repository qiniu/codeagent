package code

import (
	"fmt"
	"io"

	"github.com/qbox/codeagent/internal/config"
	"github.com/qbox/codeagent/pkg/models"
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
	switch cfg.CodeProvider {
	case ProviderClaude:
		return NewClaude(workspace, cfg)
	case ProviderGemini:
		return NewGemini(workspace, cfg)
	default:
		return nil, fmt.Errorf("unsupported code provider: %s", cfg.CodeProvider)
	}
}
