package code

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/qbox/codeagent/internal/config"
	"github.com/qbox/codeagent/pkg/models"
	"github.com/qiniu/x/log"
)

// geminiDocker Docker 实现（交互式模式）
type geminiDocker struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
}

// NewGeminiDocker 创建 Docker Gemini CLI 实现
func NewGeminiDocker(workspace *models.Workspace, cfg *config.Config) (Code, error) {
	// 解析仓库信息
	_, repo := parseRepoURL(workspace.Repository)

	// 构建 Docker 命令
	args := []string{
		"run",
		"--rm",                               // 容器运行完后自动删除
		"-it",                                // 交互式终端
		"-e", "GOOGLE_CLOUD_PROJECT=" + repo, // 设置 Google Cloud 项目环境变量
		"-v", fmt.Sprintf("%s:/workspace", workspace.Path), // 挂载工作空间
		"-v", fmt.Sprintf("%s:%s", filepath.Join(os.Getenv("HOME"), ".gemini"), "/root/.gemini"), // 挂载 gemini 认证信息
		"-w", "/workspace", // 设置工作目录
		cfg.Gemini.ContainerImage, // 使用配置的 Gemini 镜像
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

	log.Infof("Started Docker gemini CLI container")

	return &geminiDocker{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
	}, nil
}

// Prompt 实现 Code 接口 - Docker 版本（交互式）
func (g *geminiDocker) Prompt(message string) (*Response, error) {
	if _, err := g.stdin.Write([]byte(message + "\n")); err != nil {
		return nil, fmt.Errorf("failed to write to gemini stdin: %w", err)
	}
	return &Response{Out: g.stdout}, nil
}

// Close 实现 Code 接口
func (g *geminiDocker) Close() error {
	if err := g.stdin.Close(); err != nil {
		return err
	}
	return g.cmd.Wait()
}
