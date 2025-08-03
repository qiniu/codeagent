package interaction

import (
	"context"
	"testing"
	"time"

	"github.com/qiniu/codeagent/pkg/models"

	githubapi "github.com/google/go-github/v58/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockGitHubClient 用于测试的模拟GitHub客户端
type MockGitHubClient struct {
	comments map[int64]string
	nextID   int64
}

func NewMockGitHubClient() *MockGitHubClient {
	return &MockGitHubClient{
		comments: make(map[int64]string),
		nextID:   1,
	}
}

func (m *MockGitHubClient) CreateComment(ctx context.Context, owner, repo string, issueNumber int, body string) (*githubapi.IssueComment, error) {
	id := m.nextID
	m.nextID++
	m.comments[id] = body
	
	return &githubapi.IssueComment{
		ID:   &id,
		Body: &body,
	}, nil
}

func (m *MockGitHubClient) UpdateComment(ctx context.Context, owner, repo string, commentID int64, body string) error {
	m.comments[commentID] = body
	return nil
}

func (m *MockGitHubClient) GetComment(commentID int64) string {
	return m.comments[commentID]
}

func TestProgressCommentManager_BasicFlow(t *testing.T) {
	// 创建测试环境
	mockGitHub := NewMockGitHubClient()
	repo := &githubapi.Repository{
		Name: githubapi.String("test-repo"),
		Owner: &githubapi.User{
			Login: githubapi.String("test-owner"),
		},
	}
	
	pcm := NewProgressCommentManager(mockGitHub, repo, 123)
	pcm.SetTestMode(true) // 启用测试模式，禁用频率限制
	ctx := context.Background()
	
	// 创建任务工厂和任务
	factory := NewTaskFactory()
	tasks := factory.CreateIssueProcessingTasks()
	
	// 1. 初始化进度评论
	err := pcm.InitializeProgress(ctx, tasks)
	require.NoError(t, err)
	
	// 验证评论已创建
	assert.NotNil(t, pcm.context.CommentID)
	initialContent := mockGitHub.GetComment(*pcm.context.CommentID)
	assert.Contains(t, initialContent, "CodeAgent is working on this")
	assert.Contains(t, initialContent, "Gathering context and analyzing issue")
	
	// 2. 开始第一个任务
	err = pcm.UpdateTask(ctx, "gather-context", models.TaskStatusInProgress, "Analyzing issue details")
	require.NoError(t, err)
	
	// 验证内容已更新
	updatedContent := mockGitHub.GetComment(*pcm.context.CommentID)
	assert.Contains(t, updatedContent, "⠋") // spinner frame
	assert.Contains(t, updatedContent, "Analyzing issue details")
	
	// 3. 完成第一个任务
	err = pcm.UpdateTask(ctx, "gather-context", models.TaskStatusCompleted)
	require.NoError(t, err)
	
	// 4. 开始第二个任务
	err = pcm.UpdateTask(ctx, "setup-workspace", models.TaskStatusInProgress, "Creating new branch")
	require.NoError(t, err)
	
	// 5. 完成所有任务并最终化
	for _, task := range tasks[1:] {
		err = pcm.UpdateTask(ctx, task.ID, models.TaskStatusCompleted)
		require.NoError(t, err)
	}
	
	// 6. 最终化评论
	result := &models.ProgressExecutionResult{
		Success:        true,
		Summary:        "Successfully implemented the requested feature",
		FilesChanged:   []string{"src/main.go", "src/utils.go"},
		BranchName:     "feature/issue-123",
		PullRequestURL: "https://github.com/test-owner/test-repo/pull/456",
		Duration:       30 * time.Second,
	}
	
	err = pcm.FinalizeComment(ctx, result)
	require.NoError(t, err)
	
	// 验证最终内容
	finalContent := mockGitHub.GetComment(*pcm.context.CommentID)
	assert.Contains(t, finalContent, "CodeAgent completed successfully")
	assert.Contains(t, finalContent, "Successfully implemented the requested feature")
	assert.Contains(t, finalContent, "src/main.go")
	assert.Contains(t, finalContent, "feature/issue-123")
}

func TestProgressCommentManager_TaskFailure(t *testing.T) {
	mockGitHub := NewMockGitHubClient()
	repo := &githubapi.Repository{
		Name: githubapi.String("test-repo"),
		Owner: &githubapi.User{
			Login: githubapi.String("test-owner"),
		},
	}
	
	pcm := NewProgressCommentManager(mockGitHub, repo, 123)
	pcm.SetTestMode(true) // 启用测试模式，禁用频率限制
	ctx := context.Background()
	
	// 创建任务
	tasks := []*models.Task{
		models.NewTask("task1", "task1", "First task"),
		models.NewTask("task2", "task2", "Second task"),
	}
	
	// 初始化
	err := pcm.InitializeProgress(ctx, tasks)
	require.NoError(t, err)
	
	// 第一个任务成功
	err = pcm.UpdateTask(ctx, "task1", models.TaskStatusInProgress)
	require.NoError(t, err)
	err = pcm.UpdateTask(ctx, "task1", models.TaskStatusCompleted)
	require.NoError(t, err)
	
	// 第二个任务失败
	err = pcm.UpdateTask(ctx, "task2", models.TaskStatusInProgress)
	require.NoError(t, err)
	err = pcm.UpdateTask(ctx, "task2", models.TaskStatusFailed, "Connection timeout")
	require.NoError(t, err)
	
	// 最终化失败结果
	result := &models.ProgressExecutionResult{
		Success: false,
		Error:   "Task failed due to connection timeout",
		Duration: 15 * time.Second,
	}
	
	err = pcm.FinalizeComment(ctx, result)
	require.NoError(t, err)
	
	// 验证失败内容
	finalContent := mockGitHub.GetComment(*pcm.context.CommentID)
	assert.Contains(t, finalContent, "CodeAgent encountered an error")
	assert.Contains(t, finalContent, "Connection timeout")
	assert.Contains(t, finalContent, "❌") // failed icon
}

func TestSpinnerState(t *testing.T) {
	spinner := &models.SpinnerState{}
	
	// 测试开始Spinner
	spinner.Start("Processing data")
	assert.True(t, spinner.Active)
	assert.Equal(t, "Processing data", spinner.Message)
	
	// 测试获取动画帧
	frame := spinner.GetCurrentFrame()
	assert.NotEmpty(t, frame)
	assert.Contains(t, models.SpinnerFrames, frame)
	
	// 测试停止Spinner
	spinner.Stop()
	assert.False(t, spinner.Active)
	assert.Empty(t, spinner.Message)
}

func TestTaskFactory(t *testing.T) {
	factory := NewTaskFactory()
	
	// 测试Issue处理任务
	issueTasks := factory.CreateIssueProcessingTasks()
	assert.Len(t, issueTasks, 5)
	assert.Equal(t, "gather-context", issueTasks[0].ID)
	assert.Equal(t, "create-pr", issueTasks[4].ID)
	
	// 测试PR继续任务
	prTasks := factory.CreatePRContinueTasks()
	assert.Len(t, prTasks, 5)
	assert.Equal(t, "gather-context", prTasks[0].ID)
	assert.Equal(t, "commit-updates", prTasks[4].ID)
	
	// 测试根据命令获取任务
	codeTasks := factory.GetTasksForCommand(models.CommandCode, false)
	assert.Len(t, codeTasks, 5) // Issue的/code命令
	
	continueTasks := factory.GetTasksForCommand(models.CommandCode, true)
	assert.Len(t, continueTasks, 5) // PR中的/code当作/continue处理
	
	fixTasks := factory.GetTasksForCommand(models.CommandFix, true)
	assert.Len(t, fixTasks, 5) // /fix命令
}