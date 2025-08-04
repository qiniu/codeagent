package context

import (
	"fmt"
	"strings"
)

// TemplatePromptGenerator 基于模板的提示词生成器
// 模仿claude-code-action的变量替换模式
type TemplatePromptGenerator struct {
	formatter ContextFormatter
}

// NewTemplatePromptGenerator 创建新的模板生成器
func NewTemplatePromptGenerator(formatter ContextFormatter) *TemplatePromptGenerator {
	return &TemplatePromptGenerator{
		formatter: formatter,
	}
}

// GeneratePrompt 使用模板生成提示词
func (g *TemplatePromptGenerator) GeneratePrompt(ctx *EnhancedContext, mode string, args string) (string, error) {
	// 构建变量映射
	variables := g.buildVariables(ctx, mode, args)

	// 根据模式选择模板
	template := g.selectTemplate(mode)

	// 执行变量替换
	prompt := g.substituteVariables(template, variables)

	return prompt, nil
}

// buildVariables 构建变量映射
func (g *TemplatePromptGenerator) buildVariables(ctx *EnhancedContext, mode string, args string) map[string]string {
	vars := make(map[string]string)

	// 基础信息
	vars["REPOSITORY"] = ""
	vars["PR_NUMBER"] = ""
	vars["ISSUE_NUMBER"] = ""
	vars["PR_TITLE"] = ""
	vars["ISSUE_TITLE"] = ""
	vars["PR_BODY"] = ""
	vars["ISSUE_BODY"] = ""
	vars["TRIGGER_COMMENT"] = ""
	vars["TRIGGER_USERNAME"] = ""
	vars["EVENT_TYPE"] = string(ctx.Type)
	vars["IS_PR"] = "false"
	vars["MODE"] = mode
	vars["ARGS"] = args

	// 从上下文中提取信息
	if ctx.Code != nil {
		vars["REPOSITORY"] = ctx.Code.Repository
		vars["PR_NUMBER"] = ""
		vars["ISSUE_NUMBER"] = ""
		
		// 从metadata中提取PR/Issue编号
		if prNumber, ok := ctx.Metadata["pr_number"]; ok {
			vars["PR_NUMBER"] = fmt.Sprintf("%v", prNumber)
			vars["IS_PR"] = "true"
		}
		if issueNumber, ok := ctx.Metadata["issue_number"]; ok {
			vars["ISSUE_NUMBER"] = fmt.Sprintf("%v", issueNumber)
			vars["IS_PR"] = "false"
		}
	}

	// 文件变更信息
	if ctx.Code != nil && len(ctx.Code.Files) > 0 {
		var filesBuilder strings.Builder
		for _, file := range ctx.Code.Files {
			filesBuilder.WriteString(fmt.Sprintf("- %s (%s) +%d/-%d\n", 
				file.Path, file.Status, file.Additions, file.Deletions))
		}
		vars["CHANGED_FILES"] = filesBuilder.String()
	} else {
		vars["CHANGED_FILES"] = "No files changed"
	}

	// 评论信息
	if len(ctx.Comments) > 0 {
		var commentsBuilder strings.Builder
		for _, comment := range ctx.Comments {
			commentsBuilder.WriteString(fmt.Sprintf("**@%s** (%s)\n%s\n\n", 
				comment.Author, 
				comment.CreatedAt.Format("Jan 2, 15:04"),
				comment.Body))
		}
		vars["COMMENTS"] = commentsBuilder.String()
	} else {
		vars["COMMENTS"] = "No comments"
	}

	// 格式化上下文 - 使用格式化器生成Markdown
	if formatted, err := g.formatter.FormatToMarkdown(ctx); err == nil {
		vars["FORMATTED_CONTEXT"] = formatted
	} else {
		vars["FORMATTED_CONTEXT"] = "Error formatting context"
	}

	return vars
}

// selectTemplate 根据模式选择模板
func (g *TemplatePromptGenerator) selectTemplate(mode string) string {
	switch mode {
	case "Continue":
		return g.getContinueTemplate()
	case "Fix":
		return g.getFixTemplate()
	case "Code":
		return g.getCodeTemplate()
	case "Review":
		return g.getReviewTemplate()
	default:
		return g.getDefaultTemplate()
	}
}

// getDefaultTemplate 默认模板
func (g *TemplatePromptGenerator) getDefaultTemplate() string {
	return `You are Claude, an AI assistant designed to help with GitHub issues and pull requests.

## Context Information

Repository: $REPOSITORY
Event Type: $EVENT_TYPE
Mode: $MODE

### Current Request
$ARGS

### Files Changed
$CHANGED_FILES

### Comments
$COMMENTS

### Full Context
$FORMATTED_CONTEXT

## Your Task

Help with the development task based on the provided context. Make targeted, focused changes rather than broad refactoring.

## Guidelines

- Follow existing code patterns and conventions
- Provide clear explanations for your changes
- Make minimal, focused changes to address the specific request
- Test your changes mentally before implementing
- Be consistent with the existing codebase style

## Output Requirements

Your response should include:
1. Brief explanation of what you're going to do
2. Code changes using appropriate tools
3. Summary of the changes made`
}

// getContinueTemplate 继续开发模板
func (g *TemplatePromptGenerator) getContinueTemplate() string {
	return `You are Claude, an AI assistant designed to help continue development work in GitHub PRs.

## Context Information

Repository: $REPOSITORY
PR #$PR_NUMBER

### PR Details
$FORMATTED_CONTEXT

### Files Changed
$CHANGED_FILES

### Comments
$COMMENTS

## Your Task

Continue the development work in this PR. Analyze the current state, understand what has been discussed, and make appropriate code improvements or implementations.

## Implementation Request
$ARGS

## Guidelines

- Continue existing work patterns
- Address any pending issues or feedback
- Maintain consistency with existing code
- Provide clear commit messages
- Focus on completing the PR objectives

## Steps

1. Review the current state of changes
2. Identify what needs to be completed
3. Implement the necessary changes
4. Update documentation if needed
5. Ensure all tests pass (if applicable)`
}

// getFixTemplate 修复问题模板
func (g *TemplatePromptGenerator) getFixTemplate() string {
	return `You are Claude, an AI assistant designed to fix code issues in GitHub PRs and issues.

## Context Information

Repository: $REPOSITORY
$IS_PR: PR #$PR_NUMBER | Issue #$ISSUE_NUMBER

### Current Context
$FORMATTED_CONTEXT

### Files Changed
$CHANGED_FILES

### Comments
$COMMENTS

## Issue to Fix
$ARGS

## Your Task

Fix the specified issue in the codebase. Analyze the problem, identify the root cause, and implement a proper solution.

## Guidelines

- Focus on the specific issue mentioned
- Ensure the fix doesn't break existing functionality
- Add appropriate tests if needed
- Document any significant changes
- Provide clear explanation of the fix

## Steps

1. Analyze the problem description
2. Identify the root cause
3. Implement the fix
4. Verify the solution works
5. Update related documentation`}

// getCodeTemplate 代码实现模板
func (g *TemplatePromptGenerator) getCodeTemplate() string {
	return `You are Claude, an AI assistant designed to implement code functionality for GitHub issues and PRs.

## Context Information

Repository: $REPOSITORY
$IS_PR: PR #$PR_NUMBER | Issue #$ISSUE_NUMBER

### Current Context
$FORMATTED_CONTEXT

### Files Affected
$CHANGED_FILES

### Comments
$COMMENTS

## Implementation Request
$ARGS

## Your Task

Implement the requested functionality. Create new code, modify existing code as needed, and ensure the implementation follows best practices.

## Guidelines

- Follow the project's coding standards
- Write clean, maintainable code
- Add appropriate error handling
- Include necessary documentation
- Consider edge cases and testing

## Steps

1. Understand the requirements
2. Plan the implementation approach
3. Write the code
4. Test the implementation
5. Document the changes
6. Ensure proper integration`}

// getReviewTemplate 代码审查模板
func (g *TemplatePromptGenerator) getReviewTemplate() string {
	return `You are Claude, an AI assistant designed to review code changes in GitHub PRs.

## Context Information

Repository: $REPOSITORY
PR #$PR_NUMBER

### PR Details
$FORMATTED_CONTEXT

### Changed Files
$CHANGED_FILES

### Comments
$COMMENTS

## Review Task

Review the code changes in this PR. Provide thorough feedback on code quality, potential issues, and suggestions for improvement.

## Guidelines

- Look for bugs, security issues, and performance problems
- Check for code quality and maintainability
- Ensure best practices are followed
- Provide constructive feedback
- Reference specific code sections with file paths and line numbers

## Review Areas

1. **Code Quality**: Is the code clean and maintainable?
2. **Functionality**: Does it work as intended?
3. **Performance**: Are there any performance concerns?
4. **Security**: Any security vulnerabilities?
5. **Testing**: Are tests adequate and comprehensive?
6. **Documentation**: Is the code well-documented?

## Output Format

Provide your review as:
1. Overall assessment
2. Specific issues found (with file/line references)
3. Suggestions for improvement
4. Positive feedback on well-written code`}

// substituteVariables 执行变量替换
func (g *TemplatePromptGenerator) substituteVariables(template string, variables map[string]string) string {
	result := template
	
	// 按字母顺序排序，确保替换顺序一致
	keys := make([]string, 0, len(variables))
	for k := range variables {
		keys = append(keys, k)
	}
	
	// 替换变量
	for _, key := range keys {
		value := variables[key]
		placeholder := fmt.Sprintf("$%s", key)
		result = strings.ReplaceAll(result, placeholder, value)
	}
	
	return result
}

// GenerateToolsList 生成工具列表
func (g *TemplatePromptGenerator) GenerateToolsList(ctx *EnhancedContext, mode string) ([]string, error) {
	// 基础工具集
	tools := []string{
		"Read",
		"Write",
		"Edit",
		"MultiEdit",
		"LS",
		"Glob",
		"Grep",
		"Bash",
	}

	// 根据模式添加特定工具
	switch mode {
	case "Continue", "Fix", "Code":
		// 开发模式需要完整的工具集
		tools = append(tools,
			"Bash(git add:*)",
			"Bash(git commit:*)",
			"Bash(git push:*)",
			"Bash(git status:*)",
			"Bash(git diff:*)",
		)
	case "Review":
		// 审查模式主要需要读取和搜索工具
		tools = append(tools,
			"Bash(git log:*)",
			"Bash(git show:*)",
		)
	}

	return tools, nil
}

// GenerateSystemPrompt 生成系统提示词
func (g *TemplatePromptGenerator) GenerateSystemPrompt(ctx *EnhancedContext) (string, error) {
	return `You are Claude, an AI assistant specialized in software development and code collaboration through GitHub.

Key principles:
- Write clean, maintainable, and well-tested code
- Follow project conventions and best practices
- Provide clear explanations for your changes
- Be collaborative and responsive to feedback
- Focus on solving the specific problem at hand

When making code changes:
1. Understand the existing code structure and patterns
2. Make minimal, focused changes to address the specific request
3. Test your changes mentally before implementing
4. Provide clear commit messages and explanations

Communication style:
- Be clear and concise
- Use technical language appropriately
- Provide step-by-step explanations when needed
- Always update your GitHub comment to reflect progress`, nil
}