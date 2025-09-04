package context

import (
	"context"
	
	"github.com/qiniu/codeagent/internal/config"
	ghclient "github.com/qiniu/codeagent/internal/github"
	"github.com/qiniu/x/log"
	"github.com/qiniu/x/xlog"
)

// Factory 上下文工厂，提供统一的创建和管理接口
type Factory struct {
	collector ContextCollector
	formatter ContextFormatter
	generator PromptGenerator
}

// NewFactory 创建新的上下文工厂
func NewFactory(clientManager ghclient.ClientManagerInterface, logger *xlog.Logger) *Factory {
	// 根据配置选择生成器类型
	// 使用模板生成器
	collector := NewDefaultContextCollector(clientManager)
	formatter := NewDefaultContextFormatter(50000) // 50k tokens limit
	generator := NewTemplatePromptGenerator(formatter)

	return &Factory{
		collector: collector,
		formatter: formatter,
		generator: generator,
	}
}

// NewFactoryWithConfig 根据配置创建上下文工厂，支持GraphQL
func NewFactoryWithConfig(clientManager ghclient.ClientManagerInterface, cfg *config.Config, logger *xlog.Logger) *Factory {
	var collector ContextCollector
	
	// 检查是否应该使用GraphQL
	if cfg.GitHub.API.UseGraphQL {
		log.Infof("🔧 Creating context collector with GraphQL support enabled")
		
		// 尝试创建GraphQL客户端
		graphqlClient, err := clientManager.GetGraphQLClient(context.Background())
		if err != nil {
			log.Warnf("Failed to create GraphQL client, falling back to REST API: %v", err)
			collector = NewDefaultContextCollector(clientManager)
		} else {
			// 创建支持GraphQL的收集器
			collector = NewDefaultContextCollectorWithGraphQL(clientManager, graphqlClient)
			log.Infof("✅ GraphQL context collector initialized successfully")
			
			// 如果配置了fallback，设置降级选项
			if cfg.GitHub.API.GraphQLFallback {
				if defaultCollector, ok := collector.(*DefaultContextCollector); ok {
					defaultCollector.EnableGraphQL(true)
					log.Infof("📊 GraphQL fallback to REST API enabled")
				}
			}
		}
	} else {
		log.Infof("🔧 Creating context collector with REST API only")
		collector = NewDefaultContextCollector(clientManager)
	}
	
	formatter := NewDefaultContextFormatter(50000) // 50k tokens limit
	generator := NewTemplatePromptGenerator(formatter)

	return &Factory{
		collector: collector,
		formatter: formatter,
		generator: generator,
	}
}

// CreateManager 创建上下文管理器
func (f *Factory) CreateManager() *ContextManager {
	return &ContextManager{
		Collector: f.collector,
		Formatter: f.formatter,
		Generator: f.generator,
	}
}

// GetCollector 获取上下文收集器
func (f *Factory) GetCollector() ContextCollector {
	return f.collector
}

// GetFormatter 获取上下文格式化器
func (f *Factory) GetFormatter() ContextFormatter {
	return f.formatter
}

// GetGenerator 获取提示词生成器
func (f *Factory) GetGenerator() PromptGenerator {
	return f.generator
}

// CreateEnhancedContext 创建增强上下文（简化版本，专注于GitHub数据）
func (f *Factory) CreateEnhancedContext(eventType string, payload interface{}) (*EnhancedContext, error) {
	return f.collector.CollectBasicContext(eventType, payload)
}

// CreateGitHubContext 创建GitHub原生上下文
func (f *Factory) CreateGitHubContext(repoFullName string, prNumber int) (*GitHubContext, error) {
	return f.collector.CollectGitHubContext(repoFullName, prNumber)
}

// GeneratePromptWithContext 使用上下文生成提示词
func (f *Factory) GeneratePromptWithContext(ctx *EnhancedContext, mode string, args string) (string, error) {
	return f.generator.GeneratePrompt(ctx, mode, args)
}

// TrimContext 智能裁剪上下文到指定token限制
func (f *Factory) TrimContext(ctx *EnhancedContext, maxTokens int) (*EnhancedContext, error) {
	return f.formatter.TrimToTokenLimit(ctx, maxTokens)
}

// FormatToMarkdown 格式化为Markdown
func (f *Factory) FormatToMarkdown(ctx *EnhancedContext) (string, error) {
	return f.formatter.FormatToMarkdown(ctx)
}

// FormatToStructured 格式化为结构化文本
func (f *Factory) FormatToStructured(ctx *EnhancedContext) (string, error) {
	return f.formatter.FormatToStructured(ctx)
}
