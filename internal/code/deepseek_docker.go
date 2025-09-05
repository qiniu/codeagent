package code

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/qiniu/codeagent/internal/config"
	"github.com/qiniu/codeagent/pkg/models"
	"github.com/qiniu/x/log"
)

// deepSeekDocker Docker 实现
type deepSeekDocker struct {
	containerName string
}

// NewDeepSeekDocker 创建 Docker DeepSeek 实现
func NewDeepSeekDocker(workspace *models.Workspace, cfg *config.Config) (Code, error) {
	if cfg.DeepSeek.APIKey == "" {
		return nil, fmt.Errorf("DEEPSEEK_API_KEY is required")
	}

	// 解析仓库信息，只获取仓库名，不包含完整URL
	repoName := extractRepoName(workspace.Repository)

	// Generate unique container name using shared function
	containerName := generateContainerName("deepseek", workspace.Org, repoName, workspace)

	// 检查是否已经有对应的容器在运行
	if isContainerRunning(containerName) {
		log.Infof("Found existing container: %s, reusing it", containerName)
		return &deepSeekDocker{
			containerName: containerName,
		}, nil
	}

	// 确保路径存在
	workspacePath, err := filepath.Abs(workspace.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute workspace path: %w", err)
	}

	sessionPath, err := filepath.Abs(workspace.SessionPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute session path: %w", err)
	}

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
		"-e", "DEEPSEEK_API_KEY=" + cfg.DeepSeek.APIKey,
		"-e", "DEEPSEEK_BASE_URL=" + cfg.DeepSeek.BaseURL,
		"-e", "DEEPSEEK_MODEL=" + cfg.DeepSeek.Model,
		"-v", fmt.Sprintf("%s:/workspace", workspacePath), // 挂载工作空间
		"-v", fmt.Sprintf("%s:/tmp/session", sessionPath), // 挂载临时目录
		"-w", "/workspace", // 设置工作目录
	}

	// Mount processed .codeagent directory if available
	if workspace.ProcessedCodeAgentPath != "" {
		if _, err := os.Stat(workspace.ProcessedCodeAgentPath); err == nil {
			args = append(args, "-v", fmt.Sprintf("%s:/workspace/.codeagent", workspace.ProcessedCodeAgentPath))
			log.Infof("Mounting processed .codeagent directory: %s -> /workspace/.codeagent", workspace.ProcessedCodeAgentPath)
		} else {
			log.Warnf("Processed .codeagent directory not found: %s", workspace.ProcessedCodeAgentPath)
		}
	}

	// Add container image
	args = append(args, cfg.DeepSeek.ContainerImage)

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

	return &deepSeekDocker{
		containerName: containerName,
	}, nil
}

// Prompt 实现 Code 接口
func (d *deepSeekDocker) Prompt(message string) (*Response, error) {
	// 使用内置的 deepseek-cli 工具（假设容器内有这个工具）
	args := []string{
		"exec",
		d.containerName,
		"deepseek-cli",
		"--prompt",
		message,
	}

	cmd := exec.Command("docker", args...)

	log.Infof("Executing DeepSeek CLI with docker: %s", strings.Join(args, " "))

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	// 启动命令
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to execute deepseek: %w", err)
	}

	return &Response{Out: stdout}, nil
}

// Close 实现 Code 接口
func (d *deepSeekDocker) Close() error {
	stopCmd := exec.Command("docker", "rm", "-f", d.containerName)
	return stopCmd.Run()
}