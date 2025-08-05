package context

import (
	ghclient "github.com/qiniu/codeagent/internal/github"
	"github.com/qiniu/x/xlog"
)

// Factory 上下文工厂，提供统一的创建和管理接口
type Factory struct {
	collector ContextCollector
	formatter ContextFormatter
	generator PromptGenerator
}

// NewFactory 创建新的上下文工厂
func NewFactory(githubClient *ghclient.Client, logger *xlog.Logger) *Factory {
	// 根据配置选择生成器类型
	// 使用模板生成器
	collector := NewDefaultContextCollector(githubClient)
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
