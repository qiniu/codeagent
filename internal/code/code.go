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
	// 优先使用workspace中指定的AI模型，如果没有则使用配置中的默认模型
	var provider string
	if workspace.AIModel != "" {
		provider = workspace.AIModel
	} else {
		provider = cfg.CodeProvider
	}

	// 根据 code provider 和 use_docker 配置创建相应的代码提供者
	switch provider {
	case ProviderClaude:
		if cfg.UseDocker {
			// 检查是否启用交互式模式
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
