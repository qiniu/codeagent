package claude

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/qbox/codeagent/internal/config"
	"github.com/qbox/codeagent/pkg/models"

	"github.com/google/go-github/v58/github"
	"github.com/qiniu/x/log"
)

type Executor struct {
	config *config.Config
}

func NewExecutor(cfg *config.Config) *Executor {
	return &Executor{
		config: cfg,
	}
}

// Execute 执行代码生成任务
func (c *Executor) Execute(workspace *models.Workspace, issue *github.Issue) *models.ExecutionResult {
	startTime := time.Now()

	log.Infof("Starting code generation for Issue #%d", issue.GetNumber())

	// 构建 Docker 命令
	args := []string{
		"run",
		"--rm",                                             // 容器运行完后自动删除
		"-v", fmt.Sprintf("%s:/workspace", workspace.Path), // 挂载工作空间
		"-w", "/workspace", // 设置工作目录
		"busybox:latest",                 // 使用 busybox 作为临时镜像
		"sh", "-c", c.buildPrompt(issue), // 执行命令
	}

	// 执行 Docker 命令
	cmd := exec.Command("docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// 设置超时
	ctx, cancel := context.WithTimeout(context.Background(), c.config.Claude.Timeout)
	defer cancel()
	cmd = exec.CommandContext(ctx, "docker", args...)

	err := cmd.Run()
	duration := time.Since(startTime)

	result := &models.ExecutionResult{
		Success:      err == nil,
		Duration:     duration,
		FilesChanged: []string{}, // TODO: 检测文件变更
	}

	if err != nil {
		result.Error = err.Error()
		log.Errorf("Code generation failed: %v", err)
	} else {
		result.Output = fmt.Sprintf("Successfully generated code for Issue #%d in %v",
			issue.GetNumber(), duration)
		log.Infof("Code generation completed: %s", result.Output)
	}

	return result
}

// buildPrompt 构建提示词
func (c *Executor) buildPrompt(issue *github.Issue) string {
	return fmt.Sprintf(`
echo "=== XGo Agent 代码生成任务 ==="
echo "Issue #%d: %s"
echo "描述: %s"
echo ""
echo "开始生成代码..."
echo "这是一个模拟的代码生成过程"
echo "在实际实现中，这里会调用 Claude Code API"
echo ""
echo "生成的文件:"
echo "- src/main.go (示例文件)"
echo "- README.md (更新文档)"
echo ""
echo "代码生成完成！"
`,
		issue.GetNumber(),
		issue.GetTitle(),
		issue.GetBody())
}
