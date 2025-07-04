package code

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/qbox/codeagent/internal/config"
	"github.com/qbox/codeagent/pkg/models"
)

type claudeCode struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
}

func NewClaude(workspace *models.Workspace, cfg *config.Config) (Code, error) {
	// 构建 Docker 命令
	args := []string{
		"run",
		"--rm",                                             // 容器运行完后自动删除
		"-v", fmt.Sprintf("%s:/workspace", workspace.Path), // 挂载工作空间
		"-v", fmt.Sprintf("%s:%s", filepath.Join(os.Getenv("HOME"), ".claude"), "/root/.claude"), // 挂载 claude 认证信息
		"-v", cfg.Claude.BinPath, ":/usr/local/bin/claude", // 挂载 claude-code 二进制
		"-w", "/workspace", // 设置工作目录
		cfg.Claude.ContainerImage, // 使用配置的 Claude 镜像
		"claude",                  // 容器内执行的命令
	}

	cmd := exec.Command("docker", args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	return &claudeCode{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
	}, nil
}

func (c *claudeCode) Prompt(message string) (*Response, error) {
	if _, err := c.stdin.Write([]byte(message + "\n")); err != nil {
		return nil, err
	}
	return &Response{Out: c.stdout}, nil
}

func (c *claudeCode) Close() error {
	if err := c.stdin.Close(); err != nil {
		return err
	}
	return c.cmd.Wait()
}
