package code

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/qbox/codeagent/internal/config"
	"github.com/qbox/codeagent/pkg/models"
	"github.com/qiniu/x/log"
)

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
		if strings.Contains(outputStr, "API Error") || strings.Contains(outputStr, "fetch failed") {
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