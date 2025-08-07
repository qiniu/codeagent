package code

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/qiniu/codeagent/pkg/models"
	"github.com/qiniu/x/log"
	"github.com/qiniu/x/xlog"
)

// isContainerRunning 检查指定名称的容器是否在运行
func isContainerRunning(containerName string) bool {
	cmd := exec.Command("docker", "ps", "--filter", fmt.Sprintf("name=%s", containerName), "--format", "{{.Names}}")
	output, err := cmd.Output()
	if err != nil {
		log.Warnf("Failed to check container status: %v", err)
		return false
	}

	// 检查输出是否包含容器名称
	return strings.TrimSpace(string(output)) == containerName
}

// extractRepoName 从仓库URL中提取仓库名
func extractRepoName(repoURL string) string {
	// 处理 GitHub URL: https://github.com/owner/repo.git
	if strings.Contains(repoURL, "github.com") {
		parts := strings.Split(repoURL, "/")
		if len(parts) >= 2 {
			repo := strings.TrimSuffix(parts[len(parts)-1], ".git")
			return repo
		}
	}

	// 如果不是标准格式，返回一个安全的名称
	return "repo"
}

// PromptWithRetry 带重试机制的 prompt 调用（通用版本）
func PromptWithRetry(ctx context.Context, code Code, prompt string, maxRetries int) (*Response, error) {
	xl := xlog.NewWith(ctx)
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		xl.Debugf("Prompt attempt %d/%d", attempt, maxRetries)
		resp, err := code.Prompt(prompt)
		if err == nil {
			xl.Infof("Prompt succeeded on attempt %d", attempt)
			return resp, nil
		}

		lastErr = err
		xl.Warnf("Prompt attempt %d failed: %v", attempt, err)

		// 如果是 broken pipe 错误，尝试重新创建 session
		if strings.Contains(err.Error(), "broken pipe") ||
			strings.Contains(err.Error(), "process has already exited") {
			xl.Infof("Detected broken pipe or process exit, will retry...")
		}

		if attempt < maxRetries {
			// 等待一段时间后重试
			sleepDuration := time.Duration(attempt) * 500 * time.Millisecond
			xl.Infof("Waiting %v before retry", sleepDuration)
			time.Sleep(sleepDuration)
		}
	}

	xl.Errorf("All prompt attempts failed after %d attempts", maxRetries)
	return nil, fmt.Errorf("failed after %d attempts, last error: %w", maxRetries, lastErr)
}

// FormatHistoricalComments 格式化历史评论，用于构建上下文（通用版本）
func FormatHistoricalComments(allComments *models.PRAllComments, currentCommentID int64) string {
	if allComments == nil {
		return ""
	}

	var contextParts []string

	// 添加 PR 描述
	if allComments.PRBody != "" {
		contextParts = append(contextParts, fmt.Sprintf("## PR 描述\n%s", allComments.PRBody))
	}

	// 添加历史的一般评论（排除当前评论）
	if len(allComments.IssueComments) > 0 {
		var historyComments []string
		for _, comment := range allComments.IssueComments {
			if comment.GetID() != currentCommentID {
				user := comment.GetUser().GetLogin()
				body := comment.GetBody()
				createdAt := comment.GetCreatedAt().Format("2006-01-02 15:04:05")
				historyComments = append(historyComments, fmt.Sprintf("**%s** (%s):\n%s", user, createdAt, body))
			}
		}
		if len(historyComments) > 0 {
			contextParts = append(contextParts, fmt.Sprintf("## 历史评论\n%s", strings.Join(historyComments, "\n\n")))
		}
	}

	// 添加代码行评论
	if len(allComments.ReviewComments) > 0 {
		var reviewComments []string
		for _, comment := range allComments.ReviewComments {
			if comment.GetID() != currentCommentID {
				user := comment.GetUser().GetLogin()
				body := comment.GetBody()
				path := comment.GetPath()
				line := comment.GetLine()
				createdAt := comment.GetCreatedAt().Format("2006-01-02 15:04:05")
				reviewComments = append(reviewComments, fmt.Sprintf("**%s** (%s) - %s:%d:\n%s", user, createdAt, path, line, body))
			}
		}
		if len(reviewComments) > 0 {
			contextParts = append(contextParts, fmt.Sprintf("## 代码行评论\n%s", strings.Join(reviewComments, "\n\n")))
		}
	}

	// 添加 Review 评论
	if len(allComments.Reviews) > 0 {
		var reviews []string
		for _, review := range allComments.Reviews {
			if review.GetBody() != "" {
				user := review.GetUser().GetLogin()
				body := review.GetBody()
				state := review.GetState()
				createdAt := review.GetSubmittedAt().Format("2006-01-02 15:04:05")
				reviews = append(reviews, fmt.Sprintf("**%s** (%s) - %s:\n%s", user, createdAt, state, body))
			}
		}
		if len(reviews) > 0 {
			contextParts = append(contextParts, fmt.Sprintf("## Review 评论\n%s", strings.Join(reviews, "\n\n")))
		}
	}

	return strings.Join(contextParts, "\n\n")
}

// ParseStructuredOutput 解析AI的三段式输出（通用版本）
func ParseStructuredOutput(output string) (summary, changes, testPlan string) {
	lines := strings.Split(output, "\n")

	currentSection := ""
	var summaryLines, changesLines, testPlanLines []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.Contains(trimmed, models.SectionSummary) {
			currentSection = "summary"
			continue
		} else if strings.Contains(trimmed, models.SectionChanges) {
			currentSection = "changes"
			continue
		} else if strings.Contains(trimmed, models.SectionTestPlan) {
			currentSection = "testplan"
			continue
		}

		switch currentSection {
		case "summary":
			if trimmed != "" {
				summaryLines = append(summaryLines, trimmed)
			}
		case "changes":
			if trimmed != "" {
				changesLines = append(changesLines, trimmed)
			}
		case "testplan":
			if trimmed != "" {
				testPlanLines = append(testPlanLines, trimmed)
			}
		}
	}

	return strings.Join(summaryLines, "\n"),
		strings.Join(changesLines, "\n"),
		strings.Join(testPlanLines, "\n")
}
