package code

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/qbox/codeagent/internal/config"
	"github.com/qbox/codeagent/pkg/models"
)

type claudeCode struct {
	cmd           *exec.Cmd
	containerName string
}

func NewClaudeDocker(workspace *models.Workspace, cfg *config.Config) (Code, error) {
	// 构建 Docker 命令
	containerName := fmt.Sprintf("claude-%s-%d", workspace.Repository, workspace.PullRequest.GetNumber())
	args := []string{
		"run",
		"--rm",                  // 容器运行完后自动删除
		"-d",                    // 后台运行
		"--name", containerName, // 设置容器名称
		"-v", fmt.Sprintf("%s:/workspace", workspace.Path), // 挂载工作空间
		"-v", fmt.Sprintf("%s:%s", filepath.Join(os.Getenv("HOME"), ".claude"), "/root/.claude"), // 挂载 claude 认证信息
		"-w", "/workspace", // 设置工作目录
		cfg.Claude.ContainerImage, // 使用配置的 Claude 镜像
	}

	cmd := exec.Command("docker", args...)

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	return &claudeCode{
		cmd:           cmd,
		containerName: containerName,
	}, nil
}

func (c *claudeCode) Prompt(message string) (*Response, error) {
	args := []string{
		"exec",
		c.containerName,
		"claude",
		"--dangerously-skip-permissions",
		"-p",
		message,
	}

	cmd := exec.Command("docker", args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	// 启动命令
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to execute claude: %w", err)
	}
	return &Response{Out: stdout}, nil
}

func (c *claudeCode) Close() error {
	stopCmd := exec.Command("docker", "rm", "-f", c.containerName)
	return stopCmd.Run()
}
