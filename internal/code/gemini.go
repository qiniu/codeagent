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

// geminiLocal 本地 CLI 实现
type geminiLocal struct {
	workspace *models.Workspace
	config    *config.Config
}

// NewGeminiLocal 创建本地 Gemini CLI 实现
func NewGeminiLocal(workspace *models.Workspace, cfg *config.Config) (Code, error) {
	// 检查 gemini CLI 是否可用
	if err := checkGeminiCLI(); err != nil {
		return nil, fmt.Errorf("gemini CLI not available: %w", err)
	}

	return &geminiLocal{
		workspace: workspace,
		config:    cfg,
	}, nil
}

// checkGeminiCLI 检查 gemini CLI 是否可用
func checkGeminiCLI() error {
	cmd := exec.Command("gemini", "--version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gemini CLI not found or not working: %w", err)
	}
	return nil
}

// Prompt 实现 Code 接口 - 本地 CLI 版本
func (g *geminiLocal) Prompt(message string) (*Response, error) {
	// 执行本地 gemini CLI 调用
	output, err := g.executeGeminiLocal(message)
	if err != nil {
		return nil, fmt.Errorf("failed to execute gemini prompt: %w", err)
	}

	// 返回结果
	return &Response{
		Out: bytes.NewReader(output),
	}, nil
}

// executeGeminiLocal 执行本地 gemini CLI 调用
func (g *geminiLocal) executeGeminiLocal(prompt string) ([]byte, error) {
	// 构建 gemini CLI 命令
	args := []string{
		"--prompt", prompt,
	}

	// 设置超时
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gemini", args...)
	cmd.Dir = g.workspace.Path // 设置工作目录，Gemini CLI 会自动读取该目录的文件作为上下文

	// 设置环境变量
	cmd.Env = append(os.Environ())

	log.Infof("Executing local gemini CLI in directory %s: gemini %s", g.workspace.Path, strings.Join(args, " "))

	// 执行命令并获取输出
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("gemini CLI execution timed out: %w", err)
		}

		// 检查是否是 API 密钥相关错误
		outputStr := string(output)
		if strings.Contains(outputStr, "API Error") || strings.Contains(outputStr, "fetch failed") {
			return nil, fmt.Errorf("gemini API error - please check GOOGLE_API_KEY: %w, output: %s", err, outputStr)
		}

		return nil, fmt.Errorf("gemini CLI execution failed: %w, output: %s", err, outputStr)
	}

	log.Infof("Local gemini CLI execution completed successfully")
	return output, nil
}

// Close 实现 Code 接口
func (g *geminiLocal) Close() error {
	// 单次 prompt 模式不需要特殊的清理
	// 每次调用都是独立的进程
	return nil
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
