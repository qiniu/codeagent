package models

import (
	"time"

	"github.com/google/go-github/v58/github"
)

// TaskStatus 任务状态枚举
type TaskStatus string

const (
	TaskStatusPending    TaskStatus = "pending"     // ⏳ 等待执行
	TaskStatusInProgress TaskStatus = "in_progress" // 🔄 正在执行
	TaskStatusCompleted  TaskStatus = "completed"   // ✅ 已完成
	TaskStatusFailed     TaskStatus = "failed"      // ❌ 执行失败
	TaskStatusSkipped    TaskStatus = "skipped"     // ⏭️ 已跳过
)

// Task 代表一个可跟踪的任务
type Task struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Status      TaskStatus        `json:"status"`
	StartTime   *time.Time        `json:"start_time,omitempty"`
	EndTime     *time.Time        `json:"end_time,omitempty"`
	Duration    time.Duration     `json:"duration"`
	Error       string            `json:"error,omitempty"`
	SubTasks    []*Task           `json:"sub_tasks,omitempty"`
	Progress    float64           `json:"progress"` // 0.0 - 1.0
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// NewTask 创建新任务
func NewTask(id, name, description string) *Task {
	return &Task{
		ID:          id,
		Name:        name,
		Description: description,
		Status:      TaskStatusPending,
		Progress:    0.0,
		Metadata:    make(map[string]string),
	}
}

// Start 开始执行任务
func (t *Task) Start() {
	now := time.Now()
	t.StartTime = &now
	t.Status = TaskStatusInProgress
}

// Complete 完成任务
func (t *Task) Complete() {
	now := time.Now()
	t.EndTime = &now
	t.Status = TaskStatusCompleted
	t.Progress = 1.0
	if t.StartTime != nil {
		t.Duration = now.Sub(*t.StartTime)
	}
}

// Fail 任务失败
func (t *Task) Fail(err error) {
	now := time.Now()
	t.EndTime = &now
	t.Status = TaskStatusFailed
	if err != nil {
		t.Error = err.Error()
	}
	if t.StartTime != nil {
		t.Duration = now.Sub(*t.StartTime)
	}
}

// Skip 跳过任务
func (t *Task) Skip(reason string) {
	t.Status = TaskStatusSkipped
	if reason != "" {
		t.Error = reason
	}
}

// SetProgress 设置任务进度
func (t *Task) SetProgress(progress float64) {
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}
	t.Progress = progress
}

// IsActive 检查任务是否处于活动状态
func (t *Task) IsActive() bool {
	return t.Status == TaskStatusInProgress
}

// IsCompleted 检查任务是否已完成
func (t *Task) IsCompleted() bool {
	return t.Status == TaskStatusCompleted
}

// IsFailed 检查任务是否失败
func (t *Task) IsFailed() bool {
	return t.Status == TaskStatusFailed
}

// GetStatusIcon 获取状态对应的图标
func (t *Task) GetStatusIcon() string {
	switch t.Status {
	case TaskStatusPending:
		return "⏳"
	case TaskStatusInProgress:
		return "🔄"
	case TaskStatusCompleted:
		return "✅"
	case TaskStatusFailed:
		return "❌"
	case TaskStatusSkipped:
		return "⏭️"
	default:
		return "❓"
	}
}

// SpinnerState Spinner动画状态
type SpinnerState struct {
	Active     bool      `json:"active"`
	Message    string    `json:"message"`
	StartTime  time.Time `json:"start_time"`
	FrameIndex int       `json:"frame_index"`
}

// SpinnerFrames Spinner动画帧（对应claude-code-action的spinner）
var SpinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// GetCurrentFrame 获取当前动画帧
func (s *SpinnerState) GetCurrentFrame() string {
	if !s.Active || len(SpinnerFrames) == 0 {
		return ""
	}

	// 基于时间计算当前帧
	elapsed := time.Since(s.StartTime)
	frameIndex := int(elapsed.Milliseconds()/100) % len(SpinnerFrames)
	return SpinnerFrames[frameIndex]
}

// Start 开始Spinner动画
func (s *SpinnerState) Start(message string) {
	s.Active = true
	s.Message = message
	s.StartTime = time.Now()
	s.FrameIndex = 0
}

// Stop 停止Spinner动画
func (s *SpinnerState) Stop() {
	s.Active = false
	s.Message = ""
}

// ProgressTracker 进度跟踪器
type ProgressTracker struct {
	Tasks         []*Task       `json:"tasks"`
	CurrentTaskID string        `json:"current_task_id"`
	StartTime     time.Time     `json:"start_time"`
	EndTime       *time.Time    `json:"end_time,omitempty"`
	Status        TaskStatus    `json:"status"`
	Spinner       *SpinnerState `json:"spinner"`
	TotalDuration time.Duration `json:"total_duration"`
}

// NewProgressTracker 创建新的进度跟踪器
func NewProgressTracker() *ProgressTracker {
	return &ProgressTracker{
		Tasks:     make([]*Task, 0),
		StartTime: time.Now(),
		Status:    TaskStatusPending,
		Spinner:   &SpinnerState{},
	}
}

// AddTask 添加任务
func (pt *ProgressTracker) AddTask(task *Task) {
	pt.Tasks = append(pt.Tasks, task)
}

// GetTask 根据ID获取任务
func (pt *ProgressTracker) GetTask(id string) *Task {
	for _, task := range pt.Tasks {
		if task.ID == id {
			return task
		}
	}
	return nil
}

// SetCurrentTask 设置当前任务
func (pt *ProgressTracker) SetCurrentTask(id string) {
	pt.CurrentTaskID = id
	if task := pt.GetTask(id); task != nil {
		task.Start()
		pt.Status = TaskStatusInProgress
	}
}

// GetCurrentTask 获取当前任务
func (pt *ProgressTracker) GetCurrentTask() *Task {
	if pt.CurrentTaskID == "" {
		return nil
	}
	return pt.GetTask(pt.CurrentTaskID)
}

// CompleteCurrentTask 完成当前任务
func (pt *ProgressTracker) CompleteCurrentTask() {
	if task := pt.GetCurrentTask(); task != nil {
		task.Complete()
	}
	pt.CurrentTaskID = ""
}

// FailCurrentTask 当前任务失败
func (pt *ProgressTracker) FailCurrentTask(err error) {
	if task := pt.GetCurrentTask(); task != nil {
		task.Fail(err)
	}
	pt.Status = TaskStatusFailed
	pt.CurrentTaskID = ""
}

// StartSpinner 开始Spinner动画
func (pt *ProgressTracker) StartSpinner(message string) {
	pt.Spinner.Start(message)
}

// StopSpinner 停止Spinner动画
func (pt *ProgressTracker) StopSpinner() {
	pt.Spinner.Stop()
}

// Complete 完成整个进度跟踪
func (pt *ProgressTracker) Complete() {
	now := time.Now()
	pt.EndTime = &now
	pt.Status = TaskStatusCompleted
	pt.TotalDuration = now.Sub(pt.StartTime)
	pt.StopSpinner()
}

// Fail 整个进度跟踪失败
func (pt *ProgressTracker) Fail(err error) {
	now := time.Now()
	pt.EndTime = &now
	pt.Status = TaskStatusFailed
	pt.StopSpinner()
}

// GetOverallProgress 获取整体进度
func (pt *ProgressTracker) GetOverallProgress() float64 {
	if len(pt.Tasks) == 0 {
		return 0.0
	}

	var totalProgress float64
	for _, task := range pt.Tasks {
		switch task.Status {
		case TaskStatusCompleted:
			totalProgress += 1.0
		case TaskStatusInProgress:
			totalProgress += task.Progress
		case TaskStatusFailed, TaskStatusSkipped:
			// 失败或跳过的任务不计入进度
		}
	}

	return totalProgress / float64(len(pt.Tasks))
}

// GetCompletedTasksCount 获取已完成任务数量
func (pt *ProgressTracker) GetCompletedTasksCount() int {
	count := 0
	for _, task := range pt.Tasks {
		if task.IsCompleted() {
			count++
		}
	}
	return count
}

// GetFailedTasksCount 获取失败任务数量
func (pt *ProgressTracker) GetFailedTasksCount() int {
	count := 0
	for _, task := range pt.Tasks {
		if task.IsFailed() {
			count++
		}
	}
	return count
}

// HasErrors 检查是否有错误
func (pt *ProgressTracker) HasErrors() bool {
	return pt.GetFailedTasksCount() > 0
}

// ProgressExecutionResult 带进度信息的执行结果
type ProgressExecutionResult struct {
	Success        bool                   `json:"success"`
	Output         string                 `json:"output"`
	Error          string                 `json:"error,omitempty"`
	FilesChanged   []string               `json:"files_changed"`
	Duration       time.Duration          `json:"duration"`
	Summary        string                 `json:"summary"`
	CommitSHA      string                 `json:"commit_sha,omitempty"`
	BranchName     string                 `json:"branch_name,omitempty"`
	PullRequestURL string                 `json:"pull_request_url,omitempty"`
	TaskResults    []*Task                `json:"task_results"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// CommentContext 评论上下文
type CommentContext struct {
	Repository     *github.Repository `json:"repository"`
	IssueNumber    int                `json:"issue_number"`
	CommentID      *int64             `json:"comment_id,omitempty"`
	InitialContent string             `json:"initial_content"`
	LastContent    string             `json:"last_content"`
	UpdateCount    int                `json:"update_count"`
	CreatedAt      time.Time          `json:"created_at"`
	LastUpdatedAt  *time.Time         `json:"last_updated_at,omitempty"`
}
