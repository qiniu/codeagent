package code

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/qbox/codeagent/internal/config"
	"github.com/qbox/codeagent/pkg/models"
	"github.com/qiniu/x/log"
)

// geminiDocker Docker 实现（交互式模式）
type geminiDocker struct {
	containerName string
}

// isContainerRunning 检查指定名称的容器是否在运行
func isContainerRunning(containerName string) bool {
	cmd := exec.Command("docker", "ps", "--filter", fmt.Sprintf("name=%s", containerName), "--format", "{{.Names}}")
	output, err := cmd.Output()
	if err != nil {
		log.Warnf("Failed to check container status: %v", err)
		return false
	}

	// 检查输出是否包含容器名称
	return strings.TrimSpace(string(output)) == containerName
}

// extractRepoName 从仓库URL中提取仓库名
func extractRepoName(repoURL string) string {
	// 处理 GitHub URL: https://github.com/owner/repo.git
	if strings.Contains(repoURL, "github.com") {
		parts := strings.Split(repoURL, "/")
		if len(parts) >= 2 {
			repo := strings.TrimSuffix(parts[len(parts)-1], ".git")
			return repo
		}
	}

	// 如果不是标准格式，返回一个安全的名称
	return "repo"
}

// NewGeminiDocker 创建 Docker Gemini CLI 实现
func NewGeminiDocker(workspace *models.Workspace, cfg *config.Config) (Code, error) {
	// 解析仓库信息，只获取仓库名，不包含完整URL
	repoName := extractRepoName(workspace.Repository)
	containerName := fmt.Sprintf("gemini-%s-%d", repoName, workspace.PRNumber)

	// 检查是否已经有对应的容器在运行
	if isContainerRunning(containerName) {
		log.Infof("Found existing container: %s, reusing it", containerName)
		return &geminiDocker{
			containerName: containerName,
		}, nil
	}

	// 确保路径存在
	workspacePath, _ := filepath.Abs(workspace.Path)
	sessionPath, _ := filepath.Abs(workspace.SessionPath)
	geminiConfigPath := filepath.Join(os.Getenv("HOME"), ".gemini")

	// 检查是否使用了/tmp目录（在macOS上可能导致挂载问题）
	if strings.HasPrefix(workspacePath, "/tmp/") {
		log.Warnf("Warning: Using /tmp directory may cause mount issues on macOS. Consider using other path instead.")
		log.Warnf("Current workspace path: %s", workspacePath)
	}

	if strings.HasPrefix(sessionPath, "/tmp/") {
		log.Warnf("Warning: Using /tmp directory may cause mount issues on macOS. Consider using other path instead.")
		log.Warnf("Current session path: %s", sessionPath)
	}

	// 检查路径是否存在
	if _, err := os.Stat(workspacePath); os.IsNotExist(err) {
		log.Errorf("Workspace path does not exist: %s", workspacePath)
		return nil, fmt.Errorf("workspace path does not exist: %s", workspacePath)
	}

	if _, err := os.Stat(sessionPath); os.IsNotExist(err) {
		log.Errorf("Session path does not exist: %s", sessionPath)
		return nil, fmt.Errorf("session path does not exist: %s", sessionPath)
	}

	// 构建 Docker 命令
	args := []string{
		"run",
		"--rm",                  // 容器运行完后自动删除
		"-d",                    // 后台运行
		"--name", containerName, // 设置容器名称
		"-e", "GOOGLE_CLOUD_PROJECT=" + repoName, // 设置 Google Cloud 项目环境变量
		"-e", "GEMINI_API_KEY=" + cfg.Gemini.APIKey,
		"-v", fmt.Sprintf("%s:/workspace", workspacePath), // 挂载工作空间
		"-v", fmt.Sprintf("%s:/home/codeagent/.gemini", geminiConfigPath), // 挂载 gemini 认证信息
		"-v", fmt.Sprintf("%s:/home/codeagent/.gemini/tmp", sessionPath), // 挂载临时目录
		"-w", "/workspace", // 设置工作目录
		cfg.Gemini.ContainerImage, // 使用配置的 Gemini 镜像
	}

	// 打印调试信息
	log.Infof("Docker command: docker %s", strings.Join(args, " "))

	cmd := exec.Command("docker", args...)

	// 捕获命令输出
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		log.Errorf("Failed to start Docker container: %v", err)
		log.Errorf("Docker stderr: %s", stderr.String())
		return nil, fmt.Errorf("failed to start Docker container: %w", err)
	}

	// 等待命令完成
	if err := cmd.Wait(); err != nil {
		log.Errorf("docker container failed: %v", err)
		log.Errorf("docker stdout: %s", stdout.String())
		log.Errorf("docker stderr: %s", stderr.String())
		return nil, fmt.Errorf("docker container failed: %w", err)
	}

	log.Infof("docker container started successfully")

	return &geminiDocker{
		containerName: containerName,
	}, nil
}

// Prompt 实现 Code 接口
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

	log.Infof("Executing gemini CLI with docker: %s", strings.Join(args, " "))

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
