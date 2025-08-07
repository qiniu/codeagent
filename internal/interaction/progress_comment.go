package interaction

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/qiniu/codeagent/pkg/models"

	githubapi "github.com/google/go-github/v58/github"
	"github.com/qiniu/x/xlog"
)

// GitHubCommentClient GitHub评论客户端接口
type GitHubCommentClient interface {
	CreateComment(ctx context.Context, owner, repo string, issueNumber int, body string) (*githubapi.IssueComment, error)
	UpdateComment(ctx context.Context, owner, repo string, commentID int64, body string) error
}

// ProgressCommentManager 进度评论管理器
type ProgressCommentManager struct {
	github      GitHubCommentClient
	context     *models.CommentContext
	tracker     *models.ProgressTracker
	lastUpdate  time.Time
	updateMutex sync.Mutex
	testMode    bool // 测试模式下不限制更新频率
}

// NewProgressCommentManager 创建进度评论管理器
func NewProgressCommentManager(github GitHubCommentClient, repo *githubapi.Repository, issueNumber int) *ProgressCommentManager {
	return &ProgressCommentManager{
		github: github,
		context: &models.CommentContext{
			Repository:  repo,
			IssueNumber: issueNumber,
			CreatedAt:   time.Now(),
		},
		tracker:  models.NewProgressTracker(),
		testMode: false,
	}
}

// InitializeProgress 初始化进度评论
func (pcm *ProgressCommentManager) InitializeProgress(ctx context.Context, tasks []*models.Task) error {
	xl := xlog.NewWith(ctx)

	pcm.updateMutex.Lock()
	defer pcm.updateMutex.Unlock()

	// 添加任务到跟踪器
	for _, task := range tasks {
		pcm.tracker.AddTask(task)
	}

	// 生成初始评论内容
	content := pcm.renderInitialComment()
	pcm.context.InitialContent = content
	pcm.context.LastContent = content

	// 创建GitHub评论
	comment, err := pcm.github.CreateComment(
		ctx,
		pcm.context.Repository.Owner.GetLogin(),
		pcm.context.Repository.GetName(),
		pcm.context.IssueNumber,
		content,
	)
	if err != nil {
		return fmt.Errorf("failed to create progress comment: %w", err)
	}

	pcm.context.CommentID = comment.ID
	pcm.lastUpdate = time.Now()

	xl.Infof("Created progress comment with ID: %d", *comment.ID)
	return nil
}

// UpdateTask 更新任务状态
func (pcm *ProgressCommentManager) UpdateTask(ctx context.Context, taskID string, status models.TaskStatus, message ...string) error {
	xl := xlog.NewWith(ctx)

	pcm.updateMutex.Lock()
	defer pcm.updateMutex.Unlock()

	task := pcm.tracker.GetTask(taskID)
	if task == nil {
		return fmt.Errorf("task not found: %s", taskID)
	}

	// 更新任务状态
	switch status {
	case models.TaskStatusInProgress:
		task.Start()
		pcm.tracker.SetCurrentTask(taskID)
		if len(message) > 0 {
			pcm.tracker.StartSpinner(message[0])
		}
	case models.TaskStatusCompleted:
		task.Complete()
		pcm.tracker.StopSpinner()
	case models.TaskStatusFailed:
		var err error
		if len(message) > 0 {
			err = fmt.Errorf("%s", message[0])
		}
		task.Fail(err)
		pcm.tracker.StopSpinner()
	case models.TaskStatusSkipped:
		reason := ""
		if len(message) > 0 {
			reason = message[0]
		}
		task.Skip(reason)
	}

	xl.Infof("Updated task %s to status %s", taskID, status)

	// 更新评论内容
	return pcm.updateComment(ctx)
}

// ShowSpinner 显示Spinner动画
func (pcm *ProgressCommentManager) ShowSpinner(ctx context.Context, message string) error {
	pcm.updateMutex.Lock()
	defer pcm.updateMutex.Unlock()

	pcm.tracker.StartSpinner(message)
	return pcm.updateComment(ctx)
}

// HideSpinner 隐藏Spinner动画
func (pcm *ProgressCommentManager) HideSpinner(ctx context.Context) error {
	pcm.updateMutex.Lock()
	defer pcm.updateMutex.Unlock()

	pcm.tracker.StopSpinner()
	return pcm.updateComment(ctx)
}

// FinalizeComment 完成评论（最终状态）
func (pcm *ProgressCommentManager) FinalizeComment(ctx context.Context, result *models.ProgressExecutionResult) error {
	xl := xlog.NewWith(ctx)

	pcm.updateMutex.Lock()
	defer pcm.updateMutex.Unlock()

	// 更新跟踪器状态
	if result.Success {
		pcm.tracker.Complete()
	} else {
		pcm.tracker.Fail(fmt.Errorf("%s", result.Error))
	}

	// 生成最终评论内容
	content := pcm.renderFinalComment(result)
	pcm.context.LastContent = content

	// 更新GitHub评论
	if pcm.context.CommentID != nil {
		err := pcm.github.UpdateComment(
			ctx,
			pcm.context.Repository.Owner.GetLogin(),
			pcm.context.Repository.GetName(),
			*pcm.context.CommentID,
			content,
		)
		if err != nil {
			return fmt.Errorf("failed to finalize comment: %w", err)
		}

		pcm.context.UpdateCount++
		now := time.Now()
		pcm.context.LastUpdatedAt = &now

		xl.Infof("Finalized progress comment")
	}

	return nil
}

// updateComment 更新评论内容（内部方法）
func (pcm *ProgressCommentManager) updateComment(ctx context.Context) error {
	if pcm.context.CommentID == nil {
		return fmt.Errorf("comment not initialized")
	}

	// 限制更新频率（避免过于频繁的API调用）
	if !pcm.testMode && time.Since(pcm.lastUpdate) < 2*time.Second {
		return nil
	}

	// 生成当前进度内容
	content := pcm.renderProgressUpdate()
	pcm.context.LastContent = content

	// 更新GitHub评论
	err := pcm.github.UpdateComment(
		ctx,
		pcm.context.Repository.Owner.GetLogin(),
		pcm.context.Repository.GetName(),
		*pcm.context.CommentID,
		content,
	)
	if err != nil {
		return fmt.Errorf("failed to update comment: %w", err)
	}

	pcm.context.UpdateCount++
	pcm.lastUpdate = time.Now()
	now := time.Now()
	pcm.context.LastUpdatedAt = &now

	return nil
}

// renderInitialComment 渲染初始评论内容
func (pcm *ProgressCommentManager) renderInitialComment() string {
	var sb strings.Builder

	sb.WriteString("## 🤖 CodeAgent is working on this...\n\n")

	// 任务列表
	for _, task := range pcm.tracker.Tasks {
		sb.WriteString(fmt.Sprintf("%s %s\n", task.GetStatusIcon(), task.Description))
	}

	sb.WriteString("\n---\n")
	sb.WriteString(fmt.Sprintf("*Started at: %s*\n", pcm.tracker.StartTime.Format("15:04:05 MST")))

	return sb.String()
}

// renderProgressUpdate 渲染进度更新内容
func (pcm *ProgressCommentManager) renderProgressUpdate() string {
	var sb strings.Builder

	sb.WriteString("## 🤖 CodeAgent is working on this...\n\n")

	// 任务列表
	for _, task := range pcm.tracker.Tasks {
		icon := task.GetStatusIcon()
		description := task.Description

		// 为当前任务添加额外信息
		if task.Name == pcm.tracker.CurrentTaskName && task.IsActive() {
			if pcm.tracker.Spinner.Active {
				icon = pcm.tracker.Spinner.GetCurrentFrame()
				if pcm.tracker.Spinner.Message != "" {
					description += fmt.Sprintf(" - %s", pcm.tracker.Spinner.Message)
				}
			}
		}

		// 添加持续时间（对于已完成的任务）
		if task.IsCompleted() && task.Duration > 0 {
			description += fmt.Sprintf(" *(%.1fs)*", task.Duration.Seconds())
		}

		// 添加错误信息（对于失败的任务）
		if task.IsFailed() && task.Error != "" {
			description += fmt.Sprintf(" - **Error**: %s", task.Error)
		}

		sb.WriteString(fmt.Sprintf("%s %s\n", icon, description))
	}

	// 当前Spinner信息
	if pcm.tracker.Spinner.Active && pcm.tracker.Spinner.Message != "" {
		sb.WriteString(fmt.Sprintf("\n%s Working on: %s\n",
			pcm.tracker.Spinner.GetCurrentFrame(),
			pcm.tracker.Spinner.Message))
	}

	// 进度信息
	progress := pcm.tracker.GetOverallProgress()
	completedTasks := pcm.tracker.GetCompletedTasksCount()
	totalTasks := len(pcm.tracker.Tasks)

	sb.WriteString(fmt.Sprintf("\n**Progress**: %.0f%% (%d/%d tasks completed)\n",
		progress*100, completedTasks, totalTasks))

	sb.WriteString("\n---\n")
	sb.WriteString(fmt.Sprintf("*Started: %s", pcm.tracker.StartTime.Format("15:04:05 MST")))

	if pcm.tracker.Status == models.TaskStatusInProgress {
		elapsed := time.Since(pcm.tracker.StartTime)
		sb.WriteString(fmt.Sprintf(" | Elapsed: %s*\n", formatDuration(elapsed)))
	} else {
		sb.WriteString("*\n")
	}

	return sb.String()
}

// renderFinalComment 渲染最终评论内容
func (pcm *ProgressCommentManager) renderFinalComment(result *models.ProgressExecutionResult) string {
	var sb strings.Builder

	if result.Success {
		sb.WriteString("## ✅ CodeAgent completed successfully!\n\n")
	} else {
		sb.WriteString("## ❌ CodeAgent encountered an error\n\n")
	}

	// 最终任务状态
	for _, task := range pcm.tracker.Tasks {
		icon := task.GetStatusIcon()
		description := task.Description

		// 添加持续时间
		if task.Duration > 0 {
			description += fmt.Sprintf(" *(%.1fs)*", task.Duration.Seconds())
		}

		// 添加错误信息
		if task.IsFailed() && task.Error != "" {
			description += fmt.Sprintf(" - **Error**: %s", task.Error)
		}

		sb.WriteString(fmt.Sprintf("%s %s\n", icon, description))
	}

	// 结果摘要
	if result.Summary != "" {
		sb.WriteString(fmt.Sprintf("\n### Summary\n%s\n", result.Summary))
	}

	// 文件变更信息
	if len(result.FilesChanged) > 0 {
		sb.WriteString("\n### Files Changed\n")
		for _, file := range result.FilesChanged {
			sb.WriteString(fmt.Sprintf("- `%s`\n", file))
		}
	}

	// 分支和PR信息
	if result.BranchName != "" {
		sb.WriteString(fmt.Sprintf("\n### Branch\n`%s`\n", result.BranchName))
	}

	if result.PullRequestURL != "" {
		sb.WriteString(fmt.Sprintf("\n### Pull Request\n[View Pull Request](%s)\n", result.PullRequestURL))
	}

	// 错误信息
	if !result.Success && result.Error != "" {
		sb.WriteString(fmt.Sprintf("\n### Error Details\n```\n%s\n```\n", result.Error))
	}

	// 时间统计
	sb.WriteString("\n---\n")
	sb.WriteString(fmt.Sprintf("*Completed in %s*\n", formatDuration(result.Duration)))

	return sb.String()
}

// GetTracker 获取进度跟踪器
func (pcm *ProgressCommentManager) GetTracker() *models.ProgressTracker {
	return pcm.tracker
}

// GetContext 获取评论上下文
func (pcm *ProgressCommentManager) GetContext() *models.CommentContext {
	return pcm.context
}

// SetTestMode 设置测试模式（用于测试中禁用频率限制）
func (pcm *ProgressCommentManager) SetTestMode(testMode bool) {
	pcm.testMode = testMode
}

// formatDuration 格式化持续时间
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	} else if d < time.Hour {
		return fmt.Sprintf("%.1fm", d.Minutes())
	} else {
		return fmt.Sprintf("%.1fh", d.Hours())
	}
}
