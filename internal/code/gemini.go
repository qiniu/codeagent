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

type geminiCode struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
}

func NewGemini(workspace *models.Workspace, cfg *config.Config) (Code, error) {
	// 构建 Docker 命令
	args := []string{
		"run",
		"--rm",                                             // 容器运行完后自动删除
		"-v", fmt.Sprintf("%s:/workspace", workspace.Path), // 挂载工作空间
		"-v", fmt.Sprintf("%s:%s", filepath.Join(os.Getenv("HOME"), ".gemini"), "/root/.gemini"), // 挂载 gemini 认证信息
		"-v", cfg.Gemini.BinPath + ":/usr/local/bin/gemini", // 挂载 gemini-cli 二进制
		"-w", "/workspace", // 设置工作目录
		cfg.Gemini.ContainerImage, // 使用配置的 Claude 镜像
		"gemini",                  // 容器内执行的命令
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

	return &geminiCode{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
	}, nil
}

func (g *geminiCode) Prompt(message string) (*Response, error) {
	if _, err := g.stdin.Write([]byte(message + "\n")); err != nil {
		return nil, err
	}
	return &Response{Out: g.stdout}, nil
}

func (g *geminiCode) Close() error {
	if err := g.stdin.Close(); err != nil {
		return err
	}
	return g.cmd.Wait()
}
