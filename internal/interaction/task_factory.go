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
		models.NewTask(models.TaskNameGatherContext, "Gathering context and analyzing issue"),
		models.NewTask(models.TaskNameSetupWorkspace, "Setting up workspace and creating branch"),
		models.NewTask(models.TaskNameGenerateCode, "Generating code implementation"),
		models.NewTask(models.TaskNameCommitChanges, "Committing changes to repository"),
		models.NewTask(models.TaskNameCreatePR, "Creating pull request"),
	}
}

// CreatePRContinueTasks 创建PR继续任务列表
// 对应 /continue 命令的处理流程
func (tf *TaskFactory) CreatePRContinueTasks() []*models.Task {
	return []*models.Task{
		models.NewTask(models.TaskNameGatherContext, "Gathering PR context and comments"),
		models.NewTask(models.TaskNameAnalyzeChanges, "Analyzing existing changes and requirements"),
		models.NewTask(models.TaskNamePrepareWorkspace, "Preparing workspace for modifications"),
		models.NewTask(models.TaskNameImplementChanges, "Implementing requested changes"),
		models.NewTask(models.TaskNameCommitUpdates, "Committing updates to PR branch"),
	}
}

// CreatePRFixTasks 创建PR修复任务列表
// 对应 /fix 命令的处理流程
func (tf *TaskFactory) CreatePRFixTasks() []*models.Task {
	return []*models.Task{
		models.NewTask(models.TaskNameGatherContext, "Gathering PR context and issue details"),
		models.NewTask(models.TaskNameIdentifyProblems, "Identifying problems and errors"),
		models.NewTask(models.TaskNamePrepareWorkspace, "Preparing workspace for fixes"),
		models.NewTask(models.TaskNameApplyFixes, "Applying fixes to resolve issues"),
		models.NewTask(models.TaskNameCommitFixes, "Committing fixes to PR branch"),
	}
}

// CreatePRReviewTasks 创建PR审查任务列表
// 对应自动PR审查流程
func (tf *TaskFactory) CreatePRReviewTasks() []*models.Task {
	return []*models.Task{
		models.NewTask(models.TaskNameGatherContext, "Gathering PR details and changed files"),
		models.NewTask(models.TaskNameAnalyzeCode, "Analyzing code quality and changes"),
		models.NewTask(models.TaskNameRunChecks, "Running automated checks and tests"),
		models.NewTask(models.TaskNameGenerateReview, "Generating review comments"),
		models.NewTask(models.TaskNameSubmitReview, "Submitting review to GitHub"),
	}
}

// CreateBatchReviewTasks 创建批量Review处理任务列表
// 对应PR Review中的批量命令处理
func (tf *TaskFactory) CreateBatchReviewTasks() []*models.Task {
	return []*models.Task{
		models.NewTask(models.TaskNameGatherContext, "Gathering PR and review context"),
		models.NewTask(models.TaskNameProcessComments, "Processing review comments"),
		models.NewTask(models.TaskNamePrepareWorkspace, "Preparing workspace for changes"),
		models.NewTask(models.TaskNameImplementFeedback, "Implementing review feedback"),
		models.NewTask(models.TaskNameCommitChanges, "Committing feedback implementations"),
	}
}

// CreateCustomTasks 创建自定义任务列表
func (tf *TaskFactory) CreateCustomTasks(taskDefinitions []TaskDefinition) []*models.Task {
	tasks := make([]*models.Task, 0, len(taskDefinitions))

	for _, def := range taskDefinitions {
		task := models.NewTask(def.Name, def.Description)
		if def.Metadata != nil {
			task.Metadata = def.Metadata
		}
		tasks = append(tasks, task)
	}

	return tasks
}

// TaskDefinition 任务定义
type TaskDefinition struct {
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
			models.NewTask(models.TaskNameGatherContext, "Gathering context"),
			models.NewTask("process-request", "Processing request"),
			models.NewTask("generate-response", "Generating response"),
			models.NewTask("finalize", "Finalizing results"),
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
