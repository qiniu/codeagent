package code

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/qbox/codeagent/internal/config"
	"github.com/qbox/codeagent/pkg/models"
	"github.com/qiniu/x/log"
)

// claudeCode Docker 实现
type claudeCode struct {
	cmd           *exec.Cmd
	containerName string
}

func NewClaudeDocker(workspace *models.Workspace, cfg *config.Config) (Code, error) {
	// 解析仓库信息，只获取仓库名，不包含完整URL
	repoName := extractRepoName(workspace.Repository)
	containerName := fmt.Sprintf("claude-%s-%d", repoName, workspace.PullRequest.GetNumber())

	// 确保路径存在
	workspacePath, _ := filepath.Abs(workspace.Path)

	// 确定claude配置路径
	var claudeConfigPath string
	if home := os.Getenv("HOME"); home != "" {
		claudeConfigPath, _ = filepath.Abs(filepath.Join(home, ".claude"))
	} else {
		claudeConfigPath = "/root/.claude"
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
		"-v", fmt.Sprintf("%s:/root/.claude", claudeConfigPath), // 挂载 claude 认证信息
		"-w", "/workspace", // 设置工作目录
		cfg.Claude.ContainerImage, // 使用配置的 Claude 镜像
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

// claudeLocal 本地 CLI 实现
type claudeLocal struct {
	workspace *models.Workspace
	config    *config.Config
}

// NewClaudeLocal 创建本地 Claude CLI 实现
func NewClaudeLocal(workspace *models.Workspace, cfg *config.Config) (Code, error) {
	// 检查 claude CLI 是否可用
	if err := checkClaudeCLI(); err != nil {
		return nil, fmt.Errorf("claude CLI not available: %w", err)
	}

	return &claudeLocal{
		workspace: workspace,
		config:    cfg,
	}, nil
}

// checkClaudeCLI 检查 claude CLI 是否可用
func checkClaudeCLI() error {
	cmd := exec.Command("claude", "--version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("claude CLI not found or not working: %w", err)
	}
	return nil
}

// Prompt 实现 Code 接口 - 本地 CLI 版本
func (c *claudeLocal) Prompt(message string) (*Response, error) {
	// 执行本地 claude CLI 调用
	output, err := c.executeClaudeLocal(message)
	if err != nil {
		return nil, fmt.Errorf("failed to execute claude prompt: %w", err)
	}

	// 返回结果
	return &Response{
		Out: bytes.NewReader(output),
	}, nil
}

// executeClaudeLocal 执行本地 claude CLI 调用
func (c *claudeLocal) executeClaudeLocal(prompt string) ([]byte, error) {
	// 构建 claude CLI 命令
	args := []string{
		"--dangerously-skip-permissions",
		"-p",
		prompt,
	}

	// 设置超时 - 使用配置中的超时时间，默认为 30 分钟
	timeout := c.config.Claude.Timeout
	if timeout == 0 {
		timeout = 30 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Dir = c.workspace.Path // 设置工作目录，Claude CLI 会自动读取该目录的文件作为上下文

	// 设置环境变量
	cmd.Env = append(os.Environ())

	log.Infof("Executing local claude CLI in directory %s: claude %s", c.workspace.Path, strings.Join(args, " "))

	// 执行命令并获取输出
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			log.Warnf("Claude CLI execution timed out after %s, this might be due to large codebase or complex task", timeout)
			return nil, fmt.Errorf("claude CLI execution timed out: %w", err)
		}

		// 检查是否是 API 密钥相关错误
		outputStr := string(output)
		if strings.Contains(outputStr, "API Error") || strings.Contains(outputStr, "fetch failed") || strings.Contains(outputStr, "authentication") {
			return nil, fmt.Errorf("claude API error - please check CLAUDE_API_KEY: %w, output: %s", err, outputStr)
		}

		// 检查是否是网络相关错误
		if strings.Contains(outputStr, "timeout") || strings.Contains(outputStr, "connection") {
			log.Warnf("Network-related error detected: %s", outputStr)
		}

		return nil, fmt.Errorf("claude CLI execution failed: %w, output: %s", err, outputStr)
	}

	log.Infof("Local claude CLI execution completed successfully")
	return output, nil
}

// Close 实现 Code 接口
func (c *claudeLocal) Close() error {
	// 单次 prompt 模式不需要特殊的清理
	// 每次调用都是独立的进程
	return nil
}
