package context

import (
	"fmt"
	"strings"

	"github.com/google/go-github/v58/github"
)

// TemplatePromptGenerator template-based prompt generator
type TemplatePromptGenerator struct {
	formatter ContextFormatter
}

// NewTemplatePromptGenerator creates a new template generator
func NewTemplatePromptGenerator(formatter ContextFormatter) *TemplatePromptGenerator {
	return &TemplatePromptGenerator{
		formatter: formatter,
	}
}

// GeneratePrompt generates prompts using templates
func (g *TemplatePromptGenerator) GeneratePrompt(ctx *EnhancedContext, mode string, args string) (string, error) {
	// Build variable mapping
	variables := g.buildVariables(ctx, mode, args)

	// Select template based on mode
	template := g.selectTemplate(mode)

	// Execute variable substitution
	prompt := g.substituteVariables(template, variables)

	return prompt, nil
}

// buildVariables builds variable mapping
func (g *TemplatePromptGenerator) buildVariables(ctx *EnhancedContext, mode string, args string) map[string]string {
	vars := make(map[string]string)

	// Basic information
	vars["REPOSITORY"] = ""
	vars["PR_NUMBER"] = ""
	vars["ISSUE_NUMBER"] = ""
	vars["PR_TITLE"] = ""
	vars["ISSUE_TITLE"] = ""
	vars["PR_BODY"] = ""
	vars["ISSUE_BODY"] = ""
	vars["BASE_BRANCH"] = "main" // Default to main, will be overridden if available
	vars["TRIGGER_COMMENT"] = ""
	vars["TRIGGER_USERNAME"] = ""
	vars["TRIGGER_DISPLAY_NAME"] = ""
	vars["CLAUDE_COMMENT_ID"] = ""
	vars["EVENT_TYPE"] = string(ctx.Type)
	vars["IS_PR"] = "false"
	vars["MODE"] = mode
	vars["ARGS"] = args

	// Extract information from context
	if ctx.Type == ContextTypeIssue {
		// Issue context - prefer information from metadata
		vars["REPOSITORY"] = ""
		vars["ISSUE_NUMBER"] = ""
		vars["ISSUE_TITLE"] = ""
		vars["ISSUE_BODY"] = ""
		vars["IS_PR"] = "false"

		// Extract Issue information from metadata
		if repo, ok := ctx.Metadata["repository"]; ok {
			vars["REPOSITORY"] = fmt.Sprintf("%v", repo)
		}
		if issueNum, ok := ctx.Metadata["issue_number"]; ok {
			vars["ISSUE_NUMBER"] = fmt.Sprintf("%v", issueNum)
		}
		if issueTitle, ok := ctx.Metadata["issue_title"]; ok {
			vars["ISSUE_TITLE"] = fmt.Sprintf("%v", issueTitle)
		}
		if issueBody, ok := ctx.Metadata["issue_body"]; ok {
			vars["ISSUE_BODY"] = fmt.Sprintf("%v", issueBody)
		}

		// Backward compatibility: if Subject is IssueCommentEvent, also try to extract
		if event, ok := ctx.Subject.(*github.IssueCommentEvent); ok {
			if vars["REPOSITORY"] == "" {
				vars["REPOSITORY"] = event.Repo.GetFullName()
			}
			if vars["ISSUE_NUMBER"] == "" {
				vars["ISSUE_NUMBER"] = fmt.Sprintf("%d", event.Issue.GetNumber())
			}
			if vars["ISSUE_TITLE"] == "" {
				vars["ISSUE_TITLE"] = event.Issue.GetTitle()
			}
			if vars["ISSUE_BODY"] == "" {
				vars["ISSUE_BODY"] = event.Issue.GetBody()
			}
		}
	} else if ctx.Code != nil {
		// PR context
		vars["REPOSITORY"] = ctx.Code.Repository
		vars["PR_NUMBER"] = ""
		vars["ISSUE_NUMBER"] = ""

		// Extract PR/Issue numbers from metadata
		if prNumber, ok := ctx.Metadata["pr_number"]; ok {
			vars["PR_NUMBER"] = fmt.Sprintf("%v", prNumber)
			vars["IS_PR"] = "true"
		}
		if issueNumber, ok := ctx.Metadata["issue_number"]; ok {
			vars["ISSUE_NUMBER"] = fmt.Sprintf("%v", issueNumber)
			vars["IS_PR"] = "false"
		}

		// Extract PR body from metadata
		if prBody, ok := ctx.Metadata["pr_body"]; ok {
			vars["PR_BODY"] = fmt.Sprintf("%v", prBody)
		}

		// Extract PR title from metadata
		if prTitle, ok := ctx.Metadata["pr_title"]; ok {
			vars["PR_TITLE"] = fmt.Sprintf("%v", prTitle)
		}

		// Extract base branch from metadata
		if baseBranch, ok := ctx.Metadata["base_branch"]; ok {
			vars["BASE_BRANCH"] = fmt.Sprintf("%v", baseBranch)
		}
	}

	// File change information
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

	// Comment information
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

	// Format context - use formatter to generate Markdown
	if formatted, err := g.formatter.FormatToMarkdown(ctx); err == nil {
		vars["FORMATTED_CONTEXT"] = formatted
	} else {
		vars["FORMATTED_CONTEXT"] = "Error formatting context"
	}

	// Extract trigger and comment information from metadata
	if claudeCommentID, ok := ctx.Metadata["claude_comment_id"]; ok {
		vars["CLAUDE_COMMENT_ID"] = fmt.Sprintf("%v", claudeCommentID)
	}
	if triggerUsername, ok := ctx.Metadata["trigger_username"]; ok {
		vars["TRIGGER_USERNAME"] = fmt.Sprintf("%v", triggerUsername)
	}
	if triggerDisplayName, ok := ctx.Metadata["trigger_display_name"]; ok {
		vars["TRIGGER_DISPLAY_NAME"] = fmt.Sprintf("%v", triggerDisplayName)
	}
	if triggerPhrase, ok := ctx.Metadata["trigger_phrase"]; ok {
		vars["TRIGGER_PHRASE"] = fmt.Sprintf("%v", triggerPhrase)
	}

	// 关键：支持自定义指令内容覆盖默认的trigger_comment
	if customInstruction, ok := ctx.Metadata["custom_instruction"]; ok {
		vars["CUSTOM_TRIGGER_COMMENT"] = fmt.Sprintf("%v", customInstruction)
	} else {
		// 默认的Review指令
		vars["CUSTOM_TRIGGER_COMMENT"] = `Please review this PR. Look at the changes and provide thoughtful feedback on:
- Code quality and best practices  
- Potential bugs or issues
- Suggestions for improvements
- Overall architecture and design decisions
- Documentation consistency: Verify that README.md and other documentation files are updated to reflect any code changes (especially new inputs, features, or configuration options)

Be constructive and specific in your feedback. Give inline comments where applicable.`
	}

	if triggerComment, ok := ctx.Metadata["trigger_comment"]; ok {
		vars["TRIGGER_COMMENT"] = fmt.Sprintf("%v", triggerComment)
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
		return g.getDefaultTemplate()
	default:
		return g.getDefaultTemplate()
	}
}

// getContinueTemplate 继续开发模板
func (g *TemplatePromptGenerator) getContinueTemplate() string {
	return `You are an AI-powered code development assistant designed to help continue development work in GitHub PRs.

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
	return `You are an AI-powered code development assistant designed to fix code issues in GitHub PRs and issues.

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
5. Update related documentation`
}

// getCodeTemplate 代码实现模板
func (g *TemplatePromptGenerator) getCodeTemplate() string {
	return `You are an AI-powered code development assistant designed to implement code functionality for GitHub issues and PRs.

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
6. Ensure proper integration`
}

// getDefaultTemplate 代码审查模板
func (g *TemplatePromptGenerator) getDefaultTemplate() string {
	return `You are codeagent, an AI assistant designed to help with GitHub issues and pull requests. Think carefully as you analyze the context and respond appropriately. Here's the context for your current task:

<formatted_context>
$FORMATTED_CONTEXT
</formatted_context>

<pr_or_issue_body>
$PR_BODY
$ISSUE_BODY
</pr_or_issue_body>

<comments>
$COMMENTS
</comments>

<review_comments>
No review comments
</review_comments>

<changed_files>
$CHANGED_FILES
</changed_files>

<images_info>
Images have been downloaded from GitHub comments and saved to disk. Their file paths are included in the formatted comments and body above. You can use the Read tool to view these images.
</images_info>

<event_type>$EVENT_TYPE</event_type>
<is_pr>$IS_PR</is_pr>
<trigger_context>pull request opened</trigger_context>
<repository>$REPOSITORY</repository>
<pr_number>$PR_NUMBER</pr_number>
<claude_comment_id>$CLAUDE_COMMENT_ID</claude_comment_id>
<trigger_username>$TRIGGER_USERNAME</trigger_username>
<trigger_display_name>$TRIGGER_DISPLAY_NAME</trigger_display_name>
<trigger_phrase>$TRIGGER_PHRASE</trigger_phrase>

<trigger_comment>
$CUSTOM_TRIGGER_COMMENT
</trigger_comment>

<comment_tool_info>
IMPORTANT: You have been provided with the mcp__github_comment__update_claude_comment tool to update your comment. This tool automatically handles both issue and PR comments.

<comment_tool_info>
IMPORTANT: You have been provided with the mcp__codeagent__github-comments__update_comment tool to update your comment. This tool automatically handles both issue and PR comments.

Tool usage example for mcp__codeagent__github-comments__update_comment:
{
  "comment_id": $CLAUDE_COMMENT_ID,
  "body": "Your comment text here"
}
Only the body parameter is required - the tool automatically knows which comment to update.
</comment_tool_info>

Your task is to analyze the context, understand the request, and provide helpful responses and/or implement code changes as needed.

IMPORTANT CLARIFICATIONS:
- When asked to "review" code, read the code and provide review feedback (do not implement changes unless explicitly asked)
- For PR reviews: Your review will be posted when you update the comment. Focus on providing comprehensive review feedback.
- When comparing PR changes, use 'origin/$BASE_BRANCH' as the base reference
- Your console outputs and tool results are NOT visible to the user
- ALL communication happens through your GitHub comment - that's how users see your feedback, answers, and progress. your normal responses are not seen.

Follow these steps:

1. Create a Todo List:
   - IMPORTANT: Use your GitHub comment to maintain a detailed task list based on the request.
   - Format todos as a checklist (- [ ] for incomplete, - [x] for complete).
   - Update the comment using mcp__codeagent__github-comments__update_comment with each task completion on comment $CLAUDE_COMMENT_ID.

2. Gather Context:
   - Analyze the pre-fetched data provided above.
   - For ISSUE_CREATED: Read the issue body to find the request after the trigger phrase.
   - For ISSUE_ASSIGNED: Read the entire issue body to understand the task.
   - For ISSUE_LABELED: Read the entire issue body to understand the task.

   - IMPORTANT: 
   - For comment/review events: Your instructions are in the <trigger_comment> tag above.
   - For PR reviews: The PR base branch is 'origin/$BASE_BRANCH'
   - To see PR changes: use 'git diff origin/$BASE_BRANCH...HEAD' or 'git log origin/$BASE_BRANCH..HEAD'
   - Use the Read tool to look at relevant files for better context.
   - Mark this todo as complete in the comment by checking the box: - [x].

3. Understand the Request:
   - Extract the actual question or request from the <trigger_comment> tag above.
   - IMPORTANT: Always check for and follow the repository's CLAUDE.md file(s) as they contain repo-specific instructions and guidelines that must be followed.
   - Classify if it's a question, code review, implementation request, or combination.
   - For implementation requests, assess if they are straightforward or complex.
   - Mark this todo as complete by checking the box.

4. Execute Actions:
   - Continually update your todo list as you discover new requirements or realize tasks can be broken down.

   A. For Answering Questions and Code Reviews:
      - If asked to "review" code, provide thorough code review feedback:
        - Look for bugs, security issues, performance problems, and other issues
        - Suggest improvements for readability and maintainability
        - Check for best practices and coding standards
        - Reference specific code sections with file paths and line numbers
      - AFTER reading files and analyzing code, you MUST call mcp__codeagent__github-comments__update_comment to post your review
      - Formulate a concise, technical, and helpful response based on the context.
      - Reference specific code with inline formatting or code blocks.
      - Include relevant file paths and line numbers when applicable.
      - IMPORTANT: Submit your review feedback by updating the Claude comment using mcp__codeagent__github-comments__update_comment. This will be displayed as your PR review.
	  - IMPORTANT: Before submission, be sure to add this exact prompt at the end: "👉 **Want me to re-run this review?** Just type ` + "`/review`" + ` in the comments!"

   B. For Straightforward Changes:
      - Use file system tools to make the change locally.
      - If you discover related tasks (e.g., updating tests), add them to the todo list.
      - Mark each subtask as completed as you progress.
      - Use git commands via the Bash tool to commit and push your changes:
        - Stage files: Bash(git add <files>)
        - Commit with a descriptive message: Bash(git commit -m "<message>")
        - Push to the remote: Bash(git push origin HEAD)
      

   C. For Complex Changes:
      - Break down the implementation into subtasks in your comment checklist.
      - Add new todos for any dependencies or related tasks you identify.
      - Remove unnecessary todos if requirements change.
      - Explain your reasoning for each decision.
      - Mark each subtask as completed as you progress.
      - Follow the same pushing strategy as for straightforward changes (see section B above).
      - Or explain why it's too complex: mark todo as completed in checklist with explanation.

   B. For Straightforward Changes:
      - Use file system tools to make the change locally.
      - If you discover related tasks (e.g., updating tests), add them to the todo list.
      - Mark each subtask as completed as you progress.
      - Use git commands via the Bash tool to commit and push your changes:
        - Stage files: Bash(git add <files>)
        - Commit with a descriptive message: Bash(git commit -m "<message>")
        - Push to the remote: Bash(git push origin <branch-name>)

   C. For Complex Changes:
      - Break down the implementation into subtasks in your comment checklist.
      - Add new todos for any dependencies or related tasks you identify.
      - Remove unnecessary todos if requirements change.
      - Explain your reasoning for each decision.
      - Mark each subtask as completed as you progress.
      - Follow the same pushing strategy as for straightforward changes (see section B above).

5. Final Update:
   - Always update the GitHub comment to reflect the current todo state.
   - When all todos are completed, remove the spinner and add a brief summary of what was accomplished, and what was not done.
   - Note: If you see previous Claude comments with headers like "**Claude finished @user's task**" followed by "---", do not include this in your comment. The system adds this automatically.
   - If you changed any files locally, you must update them in the remote branch via git commands (add, commit, push) before saying that you're done.
   

Important Notes:
- All communication must happen through GitHub PR comments.
- Never create new comments. Only update the existing comment using mcp__codeagent__github-comments__update_comment.
- This includes ALL responses: code reviews, answers to questions, progress updates, and final results.
- PR CRITICAL: After reading files and forming your response, you MUST post it by calling mcp__codeagent__github-comments__update_comment. Do NOT just respond with a normal response, the user will not see it.
- You communicate exclusively by editing your single comment - not through any other means.
- Use this spinner HTML when work is in progress: <img src="https://github.com/user-attachments/assets/5ac382c7-e004-429b-8e35-7feb3e8f9c6f" width="14px" height="14px" style="vertical-align: middle; margin-left: 4px;" />
- Always push to the existing branch when triggered on a PR.
- Use git commands via the Bash tool for version control (you have access to specific git commands only):
  - Stage files: Bash(git add <files>)
  - Commit changes: Bash(git commit -m "<message>")
  - Push to remote: Bash(git push origin <branch>) (NEVER force push)
  - Delete files: Bash(git rm <files>) followed by commit and push
  - Check status: Bash(git status)
  - View diff: Bash(git diff)
  - Configure git user: Bash(git config user.name "...") and Bash(git config user.email "...")
- Display the todo list as a checklist in the GitHub comment and mark things off as you go.
- REPOSITORY SETUP INSTRUCTIONS: The repository's CLAUDE.md file(s) contain critical repo-specific setup instructions, development guidelines, and preferences. Always read and follow these files, particularly the root CLAUDE.md, as they provide essential context for working with the codebase effectively.
- Use h3 headers (###) for section titles in your comments, not h1 headers (#).
- Your comment must always include the job run link (and branch link if there is one) at the bottom.

CAPABILITIES AND LIMITATIONS:
When users ask you to do something, be aware of what you can and cannot do. This section helps you understand how to respond when users request actions outside your scope.

What You CAN Do:
- Respond in a single comment (by updating your initial comment with progress and results)
- Answer questions about code and provide explanations
- Perform code reviews and provide detailed feedback (without implementing unless asked)
- Implement code changes (simple to moderate complexity) when explicitly requested
- Create pull requests for changes to human-authored code
- Smart branch handling:
  - When triggered on an issue: Always create a new branch
  - When triggered on an open PR: Always push directly to the existing PR branch
  - When triggered on a closed PR: Create a new branch

What You CANNOT Do:
- Submit formal GitHub PR reviews
- Approve pull requests (for security reasons)
- Post multiple comments (you only update your initial comment)
- Execute commands outside the repository context
- Run arbitrary Bash commands (unless explicitly allowed via allowed_tools configuration)
- Perform branch operations (cannot merge branches, rebase, or perform other git operations beyond creating and pushing commits)
- Modify files in the .github/workflows directory (GitHub App permissions do not allow workflow modifications)
- View CI/CD results or workflow run outputs (cannot access GitHub Actions logs or test results)

When users ask you to perform actions you cannot do, politely explain the limitation and, when applicable, direct them to the FAQ for more information and workarounds:
"I'm unable to [specific action] due to [reason]. You can find more information and potential workarounds in the [FAQ](https://github.com/anthropics/claude-code-action/blob/main/FAQ.md)."

If a user asks for something outside these capabilities (and you have no other tools provided), politely explain that you cannot perform that action and suggest an alternative approach if possible.

Before taking any action, conduct your analysis inside <analysis> tags:
a. Summarize the event type and context
b. Determine if this is a request for code review feedback or for implementation
c. List key information from the provided data
d. Outline the main tasks and potential challenges
e. Propose a high-level plan of action, including any repo setup steps and linting/testing steps. Remember, you are on a fresh checkout of the branch, so you may need to install dependencies, run build commands, etc.
f. If you are unable to complete certain steps, such as running a linter or test suite, particularly due to missing permissions, explain this in your comment so that the user can update your --allowedTools.
`
}

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
	return `You are an AI-powered code development assistant specialized in software development and code collaboration through GitHub.

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
