package interaction

import (
	"github.com/qiniu/codeagent/pkg/models"
)

// TaskFactory 任务工厂
// 为不同的操作类型创建预定义的任务列表
type TaskFactory struct{}

// NewTaskFactory 创建任务工厂
func NewTaskFactory() *TaskFactory {
	return &TaskFactory{}
}

// CreateIssueProcessingTasks 创建Issue处理任务列表
// 对应 /code 命令的处理流程
func (tf *TaskFactory) CreateIssueProcessingTasks() []*models.Task {
	return []*models.Task{
		models.NewTask("gather-context", "gather-context", "Gathering context and analyzing issue"),
		models.NewTask("setup-workspace", "setup-workspace", "Setting up workspace and creating branch"),
		models.NewTask("generate-code", "generate-code", "Generating code implementation"),
		models.NewTask("commit-changes", "commit-changes", "Committing changes to repository"),
		models.NewTask("create-pr", "create-pr", "Creating pull request"),
	}
}

// CreatePRContinueTasks 创建PR继续任务列表
// 对应 /continue 命令的处理流程
func (tf *TaskFactory) CreatePRContinueTasks() []*models.Task {
	return []*models.Task{
		models.NewTask("gather-context", "gather-context", "Gathering PR context and comments"),
		models.NewTask("analyze-changes", "analyze-changes", "Analyzing existing changes and requirements"),
		models.NewTask("prepare-workspace", "prepare-workspace", "Preparing workspace for modifications"),
		models.NewTask("implement-changes", "implement-changes", "Implementing requested changes"),
		models.NewTask("commit-updates", "commit-updates", "Committing updates to PR branch"),
	}
}

// CreatePRFixTasks 创建PR修复任务列表
// 对应 /fix 命令的处理流程
func (tf *TaskFactory) CreatePRFixTasks() []*models.Task {
	return []*models.Task{
		models.NewTask("gather-context", "gather-context", "Gathering PR context and issue details"),
		models.NewTask("identify-problems", "identify-problems", "Identifying problems and errors"),
		models.NewTask("prepare-workspace", "prepare-workspace", "Preparing workspace for fixes"),
		models.NewTask("apply-fixes", "apply-fixes", "Applying fixes to resolve issues"),
		models.NewTask("commit-fixes", "commit-fixes", "Committing fixes to PR branch"),
	}
}

// CreatePRReviewTasks 创建PR审查任务列表
// 对应自动PR审查流程
func (tf *TaskFactory) CreatePRReviewTasks() []*models.Task {
	return []*models.Task{
		models.NewTask("gather-context", "gather-context", "Gathering PR details and changed files"),
		models.NewTask("analyze-code", "analyze-code", "Analyzing code quality and changes"),
		models.NewTask("run-checks", "run-checks", "Running automated checks and tests"),
		models.NewTask("generate-review", "generate-review", "Generating review comments"),
		models.NewTask("submit-review", "submit-review", "Submitting review to GitHub"),
	}
}

// CreateBatchReviewTasks 创建批量Review处理任务列表
// 对应PR Review中的批量命令处理
func (tf *TaskFactory) CreateBatchReviewTasks() []*models.Task {
	return []*models.Task{
		models.NewTask("gather-context", "gather-context", "Gathering PR and review context"),
		models.NewTask("process-comments", "process-comments", "Processing review comments"),
		models.NewTask("prepare-workspace", "prepare-workspace", "Preparing workspace for changes"),
		models.NewTask("implement-feedback", "implement-feedback", "Implementing review feedback"),
		models.NewTask("commit-changes", "commit-changes", "Committing feedback implementations"),
	}
}

// CreateCustomTasks 创建自定义任务列表
func (tf *TaskFactory) CreateCustomTasks(taskDefinitions []TaskDefinition) []*models.Task {
	tasks := make([]*models.Task, 0, len(taskDefinitions))
	
	for _, def := range taskDefinitions {
		task := models.NewTask(def.ID, def.Name, def.Description)
		if def.Metadata != nil {
			task.Metadata = def.Metadata
		}
		tasks = append(tasks, task)
	}
	
	return tasks
}

// TaskDefinition 任务定义
type TaskDefinition struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// GetTasksForCommand 根据命令类型获取任务列表
func (tf *TaskFactory) GetTasksForCommand(command string, isPR bool) []*models.Task {
	switch command {
	case models.CommandCode:
		if isPR {
			// PR中的/code命令当作/continue处理
			return tf.CreatePRContinueTasks()
		}
		return tf.CreateIssueProcessingTasks()
		
	case models.CommandContinue:
		return tf.CreatePRContinueTasks()
		
	case models.CommandFix:
		return tf.CreatePRFixTasks()
		
	default:
		// 默认的通用任务列表
		return []*models.Task{
			models.NewTask("gather-context", "gather-context", "Gathering context"),
			models.NewTask("process-request", "process-request", "Processing request"),
			models.NewTask("generate-response", "generate-response", "Generating response"),
			models.NewTask("finalize", "finalize", "Finalizing results"),
		}
	}
}

// GetTasksForOperation 根据操作类型获取任务列表
func (tf *TaskFactory) GetTasksForOperation(operation string) []*models.Task {
	switch operation {
	case "pr_review":
		return tf.CreatePRReviewTasks()
	case "batch_review":
		return tf.CreateBatchReviewTasks()
	default:
		return tf.GetTasksForCommand("", false)
	}
}