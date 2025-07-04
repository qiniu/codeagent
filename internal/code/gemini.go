package code

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
	_, repo := parseRepoURL(workspace.Repository)
	args := []string{
		"run",
		"--rm", // 容器运行完后自动删除
		"-it",
		"-e", "GOOGLE_CLOUD_PROJECT=" + repo, // 设置 Google Cloud 项目环境变量
		"-v", fmt.Sprintf("%s:/workspace", workspace.Path), // 挂载工作空间
		"-v", fmt.Sprintf("%s:%s", filepath.Join(os.Getenv("HOME"), ".gemini"), "/root/.gemini"), // 挂载 gemini 认证信息
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

func parseRepoURL(repoURL string) (owner, repo string) {
	// 处理 HTTPS URL: https://github.com/owner/repo.git
	if strings.Contains(repoURL, "github.com") {
		parts := strings.Split(repoURL, "/")
		if len(parts) >= 2 {
			repo = strings.TrimSuffix(parts[len(parts)-1], ".git")
			owner = parts[len(parts)-2]
		}
	}
	return owner, repo
}
