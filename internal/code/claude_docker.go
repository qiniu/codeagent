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

// claudeCode Docker 实现
type claudeCode struct {
	containerName string
}

func NewClaudeDocker(workspace *models.Workspace, cfg *config.Config) (Code, error) {
	// 解析仓库信息，只获取仓库名，不包含完整URL
	repoName := extractRepoName(workspace.Repository)
	// 新的容器命名规则：claude-组织-仓库-PR号
	containerName := fmt.Sprintf("claude-%s-%s-%d", workspace.Org, repoName, workspace.PRNumber)

	// 检查是否已经有对应的容器在运行
	if isContainerRunning(containerName) {
		log.Infof("Found existing container: %s, reusing it", containerName)
		return &claudeCode{
			containerName: containerName,
		}, nil
	}

	// 确保路径存在
	workspacePath, _ := filepath.Abs(workspace.Path)

	// 确定claude配置路径
	var claudeConfigPath string
	if home := os.Getenv("HOME"); home != "" {
		claudeConfigPath, _ = filepath.Abs(filepath.Join(home, ".claude"))
	} else {
		claudeConfigPath = "/home/codeagent/.claude"
	}

	// 检查是否使用了/tmp目录（在macOS上可能导致挂载问题）
	if strings.HasPrefix(workspacePath, "/tmp/") {
		log.Warnf("Warning: Using /tmp directory may cause mount issues on macOS. Consider using other path instead.")
		log.Warnf("Current workspace path: %s", workspacePath)
	}

	// 检查路径是否存在
	if _, err := os.Stat(workspacePath); os.IsNotExist(err) {
		log.Errorf("Workspace path does not exist: %s", workspacePath)
		return nil, fmt.Errorf("workspace path does not exist: %s", workspacePath)
	}

	// 构建 Docker 命令
	args := []string{
		"run",
		"--rm",                  // 容器运行完后自动删除
		"-d",                    // 后台运行
		"--name", containerName, // 设置容器名称
		"-v", fmt.Sprintf("%s:/workspace", workspacePath), // 挂载工作空间
		"-v", fmt.Sprintf("%s:/home/codeagent/.claude", claudeConfigPath), // 挂载 claude 认证信息
		"-w", "/workspace", // 设置工作目录
	}

	// 添加 Claude API 相关环境变量
	if cfg.Claude.AuthToken != "" {
		args = append(args, "-e", fmt.Sprintf("ANTHROPIC_AUTH_TOKEN=%s", cfg.Claude.AuthToken))
	} else if cfg.Claude.APIKey != "" {
		args = append(args, "-e", fmt.Sprintf("ANTHROPIC_API_KEY=%s", cfg.Claude.APIKey))
	}
	if cfg.Claude.BaseURL != "" {
		args = append(args, "-e", fmt.Sprintf("ANTHROPIC_BASE_URL=%s", cfg.Claude.BaseURL))
	}

	// 添加容器镜像
	args = append(args, cfg.Claude.ContainerImage)

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

	return &claudeCode{
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

	// 打印调试信息
	log.Infof("Executing claude command: docker %s", strings.Join(args, " "))

	cmd := exec.Command("docker", args...)

	// 捕获stderr用于调试
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Errorf("Failed to start claude command: %v", err)
		log.Errorf("Stderr: %s", stderr.String())
		return nil, err
	}

	// 启动命令
	if err := cmd.Start(); err != nil {
		log.Errorf("Failed to start claude command: %v", err)
		log.Errorf("Stderr: %s", stderr.String())
		return nil, fmt.Errorf("failed to execute claude: %w", err)
	}

	// 不等待命令完成，让调用方处理输出流
	// 错误处理将在调用方读取时进行
	return &Response{Out: stdout}, nil
}

func (c *claudeCode) Close() error {
	stopCmd := exec.Command("docker", "rm", "-f", c.containerName)
	return stopCmd.Run()
}
