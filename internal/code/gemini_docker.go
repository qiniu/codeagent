package code

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/qbox/codeagent/internal/config"
	"github.com/qbox/codeagent/pkg/models"
	"github.com/qiniu/x/log"
)

// geminiDocker Docker 实现（交互式模式）
type geminiDocker struct {
	cmd           *exec.Cmd
	containerName string
}

// NewGeminiDocker 创建 Docker Gemini CLI 实现
func NewGeminiDocker(workspace *models.Workspace, cfg *config.Config) (Code, error) {
	// 解析仓库信息
	_, repo := parseRepoURL(workspace.Repository)
	containerName := fmt.Sprintf("gemini-%s-%d", workspace.Repository, workspace.PullRequest.GetNumber())

	// 构建 Docker 命令
	args := []string{
		"run",
		"--rm",                  // 容器运行完后自动删除
		"-d",                    // 后台运行
		"--name", containerName, // 设置容器名称
		"-e", "GOOGLE_CLOUD_PROJECT=" + repo, // 设置 Google Cloud 项目环境变量
		"-e", "GEMINI_API_KEY=" + cfg.Gemini.APIKey,
		"-v", fmt.Sprintf("%s:/workspace", workspace.Path), // 挂载工作空间
		"-v", fmt.Sprintf("%s:%s", filepath.Join(os.Getenv("HOME"), ".gemini"), "/root/.gemini"), // 挂载 gemini 认证信息
		"-v", workspace.SessionPath + ":/root/.gemini/tmp", // 挂载临时目录
		"-w", "/workspace", // 设置工作目录
		cfg.Gemini.ContainerImage, // 使用配置的 Gemini 镜像
	}

	cmd := exec.Command("docker", args...)

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	log.Infof("Started Docker gemini CLI container")

	return &geminiDocker{
		cmd:           cmd,
		containerName: containerName,
	}, nil
}

// Prompt 实现 Code 接口 - Docker 版本（交互式）
func (g *geminiDocker) Prompt(message string) (*Response, error) {
	args := []string{
		"exec",
		g.containerName,
		"gemini",
		"-y",
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
		return nil, fmt.Errorf("failed to execute gemini: %w", err)
	}

	return &Response{Out: stdout}, nil
}

// Close 实现 Code 接口
func (g *geminiDocker) Close() error {
	stopCmd := exec.Command("docker", "rm", "-f", g.containerName)
	return stopCmd.Run()
}
