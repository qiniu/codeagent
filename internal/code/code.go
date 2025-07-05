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
	// 根据 code provider 和 use_docker 配置创建相应的代码提供者
	switch cfg.CodeProvider {
	case ProviderClaude:
		// Claude 目前只支持 Docker 模式
		if !cfg.UseDocker {
			return nil, fmt.Errorf("Claude only supports Docker mode currently")
		}
		return NewClaudeDocker(workspace, cfg)
	case ProviderGemini:
		if cfg.UseDocker {
			return NewGeminiDocker(workspace, cfg)
		}
		return NewGeminiLocal(workspace, cfg)
	default:
		return nil, fmt.Errorf("unsupported code provider: %s", cfg.CodeProvider)
	}
}
