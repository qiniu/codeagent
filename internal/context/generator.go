package context

import (
	"fmt"
	"strings"
)

// DefaultPromptGenerator 默认提示词生成器实现
// 采用claude-code-action的模板化方法
type DefaultPromptGenerator struct {
	formatter ContextFormatter
}

// NewDefaultPromptGenerator 创建默认提示词生成器
func NewDefaultPromptGenerator(formatter ContextFormatter) *DefaultPromptGenerator {
	return &DefaultPromptGenerator{
		formatter: formatter,
	}
}

// GeneratePrompt 生成基础提示词
func (g *DefaultPromptGenerator) GeneratePrompt(ctx *EnhancedContext, mode string, args string) (string, error) {
	// 首先格式化上下文
	contextStr, err := g.formatter.FormatToMarkdown(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to format context: %w", err)
	}

	var prompt strings.Builder

	// 系统角色定义
	prompt.WriteString(g.generateSystemRole(ctx))
	prompt.WriteString("\n\n")

	// 上下文信息
	prompt.WriteString("# Context Information\n\n")
	prompt.WriteString(contextStr)
	prompt.WriteString("\n\n")

	// 当前任务
	prompt.WriteString(g.generateTaskDescription(ctx, mode, args))
	prompt.WriteString("\n\n")

	// 输出要求
	prompt.WriteString(g.generateOutputRequirements(ctx, mode))
	prompt.WriteString("\n\n")

	// 约束和注意事项
	prompt.WriteString(g.generateConstraints(ctx, mode))

	return prompt.String(), nil
}

// generateSystemRole 生成系统角色
// 简化系统角色定义，专注于GitHub交互
func (g *DefaultPromptGenerator) generateSystemRole(ctx *EnhancedContext) string {
	return `You are Claude, an AI assistant designed to help with GitHub issues and pull requests. 

**Your role:**
- Analyze the provided GitHub context and respond appropriately
- Make targeted code changes based on the specific request
- Provide clear explanations for your changes
- Follow existing code patterns and conventions

**Capabilities:**
- Read and analyze code
- Write and modify files
- Search through repositories
- Understand GitHub workflows`}

// generateTaskDescription 生成任务描述
func (g *DefaultPromptGenerator) generateTaskDescription(ctx *EnhancedContext, mode string, args string) string {
	var task strings.Builder

	task.WriteString("# Current Task\n\n")

	switch mode {
	case "Continue":
		if args != "" {
			task.WriteString(fmt.Sprintf("**Instruction**: %s\n\n", args))
			task.WriteString("Continue the development work in this PR based on the above instruction. ")
		} else {
			task.WriteString("Continue the development work in this PR. ")
		}
		task.WriteString("Analyze the current state, understand what has been discussed, and make appropriate code improvements or implementations.")

	case "Fix":
		if args != "" {
			task.WriteString(fmt.Sprintf("**Issue to fix**: %s\n\n", args))
			task.WriteString("Fix the specified issue in the codebase. ")
		} else {
			task.WriteString("Fix the issues identified in the discussion. ")
		}
		task.WriteString("Analyze the problem, identify the root cause, and implement a proper solution.")

	case "Code":
		if args != "" {
			task.WriteString(fmt.Sprintf("**Implementation request**: %s\n\n", args))
		}
		task.WriteString("Implement the requested functionality. Create new code, modify existing code as needed, and ensure the implementation follows best practices.")

	case "Review":
		task.WriteString("Review the code changes.")
	default:
		if args != "" {
			task.WriteString(fmt.Sprintf("**Task**: %s\n\n", args))
		}
		task.WriteString("Help with the development task based on the provided context.")
	}

	return task.String()
}

// generateOutputRequirements 生成输出要求
func (g *DefaultPromptGenerator) generateOutputRequirements(ctx *EnhancedContext, mode string) string {
	var requirements strings.Builder

	requirements.WriteString("# Output Requirements\n\n")

	switch mode {
	case "Continue", "Fix", "Code":
		requirements.WriteString("**Your response should include:**\n")
		requirements.WriteString("1. **Brief explanation** of what you're going to do\n")
		requirements.WriteString("2. **Code changes** using the appropriate tools (Edit, Write, MultiEdit, etc.)\n")
		requirements.WriteString("3. **Summary** of the changes made\n\n")

		requirements.WriteString("**Code quality guidelines:**\n")
		requirements.WriteString("- Follow the project's existing code style and patterns\n")
		requirements.WriteString("- Add appropriate comments for complex logic\n")
		requirements.WriteString("- Ensure proper error handling\n")
		requirements.WriteString("- Write clean, readable, and maintainable code\n")

		// 代码质量指导原则
		requirements.WriteString("- Follow the project's existing code style and conventions\n")

	case "Review":
		requirements.WriteString("**Your review should include:**\n")
		requirements.WriteString("1. **Overall assessment** of the code quality\n")
		requirements.WriteString("2. **Specific issues** found (bugs, performance, security, style)\n")
		requirements.WriteString("3. **Suggestions** for improvement\n")
		requirements.WriteString("4. **Positive feedback** on well-written code\n")
		requirements.WriteString("5. **Actionable recommendations**\n")

	default:
		requirements.WriteString("Provide a clear, helpful response that addresses the request and includes any necessary code changes.\n")
	}

	return requirements.String()
}

// generateConstraints 生成约束和注意事项
func (g *DefaultPromptGenerator) generateConstraints(ctx *EnhancedContext, mode string) string {
	var constraints strings.Builder

	constraints.WriteString("# Important Guidelines\n\n")

	constraints.WriteString("**General rules:**\n")
	constraints.WriteString("- Make targeted, focused changes rather than broad refactoring\n")
	constraints.WriteString("- Preserve existing functionality while making improvements\n")
	constraints.WriteString("- Test your changes mentally before implementing\n")
	constraints.WriteString("- Be consistent with the existing codebase style\n")
	constraints.WriteString("- When in doubt, ask clarifying questions\n\n")

	// 基于上下文类型的特殊约束
	switch ctx.Type {
	case ContextTypeReviewComment:
		constraints.WriteString("**Line comment specific rules:**\n")
		constraints.WriteString("- Focus on the specific file and line mentioned\n")
		constraints.WriteString("- Keep changes localized to the relevant area\n")
		constraints.WriteString("- Address the specific concern raised\n\n")

	case ContextTypeReview:
		constraints.WriteString("**Review response rules:**\n")
		constraints.WriteString("- Address all feedback points systematically\n")
		constraints.WriteString("- Group related changes logically\n")
		constraints.WriteString("- Explain your reasoning for each change\n\n")

	case ContextTypePR:
		constraints.WriteString("**Pull request rules:**\n")
		constraints.WriteString("- Consider the overall PR objectives\n")
		constraints.WriteString("- Maintain coherence with existing commits\n")
		constraints.WriteString("- Update related documentation if needed\n\n")
	}

	// 代码质量指导原则
	constraints.WriteString("**Code quality guidelines:**\n")
	constraints.WriteString("- Follow the project's existing code style and patterns\n")
	constraints.WriteString("- Use appropriate error handling\n")
	constraints.WriteString("- Write clean, readable, and maintainable code\n")
	constraints.WriteString("- Follow language-specific best practices\n\n")

	constraints.WriteString("**Communication:**\n")
	constraints.WriteString("- Be clear and concise in your explanations\n")
	constraints.WriteString("- Explain complex changes step by step\n")
	constraints.WriteString("- If you need more information, ask specific questions\n")
	constraints.WriteString("- Stay focused on the task at hand\n")

	return constraints.String()
}

// GenerateToolsList 生成工具列表
func (g *DefaultPromptGenerator) GenerateToolsList(ctx *EnhancedContext, mode string) ([]string, error) {
	var tools []string

	// 基础工具
	baseTools := []string{
		"Read",      // 读取文件
		"Write",     // 写入文件
		"Edit",      // 编辑文件
		"MultiEdit", // 批量编辑
		"LS",        // 列出文件
		"Glob",      // 模式匹配查找文件
		"Grep",      // 搜索文件内容
	}
	tools = append(tools, baseTools...)

	// MCP工具（始终可用）
	mcpTools := []string{
		"mcp__github_files__read_repository_file",
		"mcp__github_files__write_repository_file",
		"mcp__github_files__search_files",
		"mcp__github_comments__update_comment",
		"mcp__github_comments__create_comment",
	}
	tools = append(tools, mcpTools...)

	// 基于模式的工具选择
	switch mode {
	case "Continue", "Fix", "Code":
		// 开发模式需要完整的工具集
		additionalTools := []string{
			"mcp__github_files__commit_files",
			"mcp__github_files__create_branch",
			"mcp__github_files__get_file_tree",
		}
		tools = append(tools, additionalTools...)

	case "Review":
		// 审查模式主要需要读取和评论工具
		reviewTools := []string{
			"mcp__github_comments__create_review_comment",
			"mcp__github_files__get_file_diff",
		}
		tools = append(tools, reviewTools...)
	}

	// 基于上下文类型的工具调整
	switch ctx.Type {
	case ContextTypeReviewComment:
		// 行评论需要精确的文件操作
		tools = append(tools, "mcp__github_comments__reply_to_review_comment")
	case ContextTypeReview:
		// 批量评论处理
		tools = append(tools, "mcp__github_comments__batch_update_comments")
	}

	return tools, nil
}

// GenerateSystemPrompt 生成系统提示词
func (g *DefaultPromptGenerator) GenerateSystemPrompt(ctx *EnhancedContext) (string, error) {
	var systemPrompt strings.Builder

	systemPrompt.WriteString("You are Claude, an AI assistant specialized in software development and code collaboration. ")
	systemPrompt.WriteString("You work with developers through GitHub Issues and Pull Requests to implement, review, and improve code.\n\n")

	systemPrompt.WriteString("Key principles:\n")
	systemPrompt.WriteString("- Write clean, maintainable, and well-tested code\n")
	systemPrompt.WriteString("- Follow project conventions and best practices\n")
	systemPrompt.WriteString("- Provide clear explanations for your changes\n")
	systemPrompt.WriteString("- Be collaborative and responsive to feedback\n")
	systemPrompt.WriteString("- Focus on solving the specific problem at hand\n\n")

	// 简化系统指导，专注于GitHub交互
	systemPrompt.WriteString("This is a GitHub-based project. Ensure your code follows the repository's existing conventions and patterns.\n\n")

	systemPrompt.WriteString("When making code changes:\n")
	systemPrompt.WriteString("1. Understand the existing code structure and patterns\n")
	systemPrompt.WriteString("2. Make minimal, focused changes to address the specific request\n")
	systemPrompt.WriteString("3. Test your changes mentally before implementing\n")
	systemPrompt.WriteString("4. Provide clear commit messages and explanations\n")

	return systemPrompt.String(), nil
}
