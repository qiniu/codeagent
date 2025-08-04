package context

import (
	"fmt"
	"sort"
	"strings"
)

// DefaultContextFormatter 默认上下文格式化器实现
type DefaultContextFormatter struct {
	maxTokens int
}

// NewDefaultContextFormatter 创建默认上下文格式化器
func NewDefaultContextFormatter(maxTokens int) *DefaultContextFormatter {
	if maxTokens <= 0 {
		maxTokens = 50000 // 默认最大token数
	}
	return &DefaultContextFormatter{
		maxTokens: maxTokens,
	}
}

// FormatToMarkdown 格式化为Markdown
// 对齐claude-code-action模式，专注于GitHub原生数据展示
func (f *DefaultContextFormatter) FormatToMarkdown(ctx *EnhancedContext) (string, error) {
	return f.formatGitHubContext(ctx), nil
}

// formatGitHubContext 格式化GitHub上下文为Markdown
// 模仿claude-code-action的格式化模式
func (f *DefaultContextFormatter) formatGitHubContext(ctx *EnhancedContext) string {
	var sections []string

	// 1. 基础上下文信息
	sections = append(sections, f.formatBasicContext(ctx))

	// 2. PR或Issue信息
	if ctx.Type == ContextTypePR && ctx.Code != nil {
		sections = append(sections, f.formatPRContext(ctx.Code))
	}

	// 3. 文件变更
	if ctx.Code != nil && len(ctx.Code.Files) > 0 {
		sections = append(sections, f.formatChangedFiles(ctx.Code))
	}

	// 4. 评论上下文
	if len(ctx.Comments) > 0 {
		sections = append(sections, f.formatComments(ctx.Comments))
	}

	return strings.Join(sections, "\n\n")
}

// formatBasicContext 格式化基础上下文信息
func (f *DefaultContextFormatter) formatBasicContext(ctx *EnhancedContext) string {
	var info []string
	
	info = append(info, "## Context")
	info = append(info, fmt.Sprintf("- **Type**: %s", ctx.Type))
	info = append(info, fmt.Sprintf("- **Priority**: %s", f.priorityToString(ctx.Priority)))
	
	if len(ctx.Metadata) > 0 {
		if prNumber, ok := ctx.Metadata["pr_number"]; ok {
			info = append(info, fmt.Sprintf("- **PR Number**: #%v", prNumber))
		}
		if issueNumber, ok := ctx.Metadata["issue_number"]; ok {
			info = append(info, fmt.Sprintf("- **Issue Number**: #%v", issueNumber))
		}
	}
	
	return strings.Join(info, "\n")
}

// formatPRContext 格式化PR上下文
func (f *DefaultContextFormatter) formatPRContext(code *CodeContext) string {
	var sections []string

	sections = append(sections, "## Pull Request")
	sections = append(sections, fmt.Sprintf("**Repository**: %s", code.Repository))
	sections = append(sections, fmt.Sprintf("**Branch**: %s → %s", code.BaseBranch, code.HeadBranch))
	sections = append(sections, fmt.Sprintf("- **Files changed**: %d", code.TotalChanges.Files))
	sections = append(sections, fmt.Sprintf("- **Lines added**: +%d", code.TotalChanges.Additions))
	sections = append(sections, fmt.Sprintf("- **Lines deleted**: -%d", code.TotalChanges.Deletions))

	return strings.Join(sections, "\n")
}

// formatChangedFiles 格式化文件变更
func (f *DefaultContextFormatter) formatChangedFiles(code *CodeContext) string {
	if len(code.Files) == 0 {
		return ""
	}

	var sections []string
	sections = append(sections, "## Changed Files")

	// 限制显示文件数量
	displayFiles := code.Files
	if len(displayFiles) > 20 {
		displayFiles = displayFiles[:20]
	}

	for _, file := range displayFiles {
		sections = append(sections, fmt.Sprintf("- %s (%s) +%d/-%d",
			file.Path, file.Status, file.Additions, file.Deletions))
	}

	if len(code.Files) > 20 {
		sections = append(sections, fmt.Sprintf("... and %d more files", len(code.Files)-20))
	}

	return strings.Join(sections, "\n")
}

// formatComments 格式化评论
func (f *DefaultContextFormatter) formatComments(comments []CommentContext) string {
	if len(comments) == 0 {
		return ""
	}

	// 按时间排序
	sort.Slice(comments, func(i, j int) bool {
		return comments[i].CreatedAt.Before(comments[j].CreatedAt)
	})

	var sections []string
	sections = append(sections, "## Comments")

	// 限制评论数量
	displayComments := comments
	if len(displayComments) > 15 {
		displayComments = displayComments[:15]
	}

	for _, comment := range displayComments {
		sections = append(sections, f.formatSingleComment(comment))
	}

	if len(comments) > 15 {
		sections = append(sections, fmt.Sprintf("... and %d more comments", len(comments)-15))
	}

	return strings.Join(sections, "\n")
}

// formatSingleComment 格式化单个评论
func (f *DefaultContextFormatter) formatSingleComment(comment CommentContext) string {
	timeStr := comment.CreatedAt.Format("Jan 2, 15:04")

	// 基础信息
	header := fmt.Sprintf("**@%s** (%s)", comment.Author, timeStr)

	// 添加位置信息
	if comment.FilePath != "" {
		if comment.StartLine > 0 && comment.StartLine != comment.LineNumber {
			header += fmt.Sprintf(" • `%s:%d-%d`", comment.FilePath, comment.StartLine, comment.LineNumber)
		} else if comment.LineNumber > 0 {
			header += fmt.Sprintf(" • `%s:%d`", comment.FilePath, comment.LineNumber)
		} else {
			header += fmt.Sprintf(" • `%s`", comment.FilePath)
		}
	}

	// 添加Review状态
	if comment.ReviewState != "" {
		header += fmt.Sprintf(" • %s", comment.ReviewState)
	}

	// 处理评论内容
	body := comment.Body
	if len(body) > 300 {
		body = body[:300] + "..."
	}

	// 清理评论内容，移除多余空格
	body = strings.TrimSpace(body)
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	body = strings.ReplaceAll(body, "\n", "\n> ")

	return fmt.Sprintf("**@%s** (%s)\n> %s", comment.Author, timeStr, body)
}

// FormatToStructured 格式化为结构化文本
func (f *DefaultContextFormatter) FormatToStructured(ctx *EnhancedContext) (string, error) {
	// 简化版本，主要用于调试
	sections := []string{
		fmt.Sprintf("Context Type: %s", ctx.Type),
		fmt.Sprintf("Priority: %s", f.priorityToString(ctx.Priority)),
		fmt.Sprintf("Comments: %d", len(ctx.Comments)),
	}

	if ctx.Code != nil {
		sections = append(sections, fmt.Sprintf("Files Changed: %d", ctx.Code.TotalChanges.Files))
		sections = append(sections, fmt.Sprintf("Repository: %s", ctx.Code.Repository))
	}

	return strings.Join(sections, "\n"), nil
}

// TrimToTokenLimit 智能裁剪到token限制
func (f *DefaultContextFormatter) TrimToTokenLimit(ctx *EnhancedContext, maxTokens int) (*EnhancedContext, error) {
	if maxTokens <= 0 {
		maxTokens = f.maxTokens
	}

	// 简单的token估算：大约4个字符=1个token
	estimateTokens := func(text string) int {
		return len(text) / 4
	}

	// 创建副本
	trimmed := &EnhancedContext{
		Type:      ctx.Type,
		Priority:  ctx.Priority,
		Timestamp: ctx.Timestamp,
		Subject:   ctx.Subject,
		Metadata:  make(map[string]interface{}),
	}

	// 复制元数据
	for k, v := range ctx.Metadata {
		trimmed.Metadata[k] = v
	}

	currentTokens := 1000 // 为基础信息预留token

	// 优先保留高优先级内容
	if ctx.Code != nil && currentTokens < maxTokens/2 {
		// 简化代码上下文
		trimmed.Code = &CodeContext{
			Repository:   ctx.Code.Repository,
			BaseBranch:   ctx.Code.BaseBranch,
			HeadBranch:   ctx.Code.HeadBranch,
			TotalChanges: ctx.Code.TotalChanges,
		}

		// 只保留最重要的文件
		for i, file := range ctx.Code.Files {
			if i >= 5 || currentTokens+estimateTokens(file.Path+file.Patch) > maxTokens/2 {
				break
			}

			// 裁剪patch内容
			fileCopy := file
			if len(fileCopy.Patch) > 200 {
				fileCopy.Patch = fileCopy.Patch[:200] + "...(truncated)"
			}

			trimmed.Code.Files = append(trimmed.Code.Files, fileCopy)
			currentTokens += estimateTokens(fileCopy.Path + fileCopy.Patch)
		}
	}


	// 评论历史（按重要性排序）
	if len(ctx.Comments) > 0 && currentTokens < maxTokens {
		// 按时间倒序，优先保留最新的评论
		sortedComments := make([]CommentContext, len(ctx.Comments))
		copy(sortedComments, ctx.Comments)
		sort.Slice(sortedComments, func(i, j int) bool {
			return sortedComments[i].CreatedAt.After(sortedComments[j].CreatedAt)
		})

		for _, comment := range sortedComments {
			estimatedTokens := estimateTokens(comment.Body)
			if currentTokens+estimatedTokens > maxTokens {
				break
			}

			// 裁剪过长的评论
			commentCopy := comment
			if len(commentCopy.Body) > 300 {
				commentCopy.Body = commentCopy.Body[:300] + "...(truncated)"
			}

			trimmed.Comments = append(trimmed.Comments, commentCopy)
			currentTokens += estimatedTokens
		}
	}

	trimmed.TokenCount = currentTokens
	return trimmed, nil
}

// 辅助函数

func (f *DefaultContextFormatter) priorityToString(priority ContextPriority) string {
	switch priority {
	case PriorityLow:
		return "Low"
	case PriorityMedium:
		return "Medium"
	case PriorityHigh:
		return "High"
	case PriorityCritical:
		return "Critical"
	default:
		return "Unknown"
	}
}

func (f *DefaultContextFormatter) getFileStatusEmoji(status string) string {
	switch status {
	case "added":
		return "✅"
	case "modified":
		return "📝"
	case "deleted":
		return "🗑️"
	case "renamed":
		return "🔄"
	default:
		return "📄"
	}
}

func (f *DefaultContextFormatter) getReviewStateEmoji(state string) string {
	switch strings.ToLower(state) {
	case "approved":
		return "✅"
	case "changes_requested":
		return "❌"
	case "commented":
		return "💬"
	default:
		return "👁️"
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
