package code

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/qiniu/codeagent/internal/config"
	"github.com/qiniu/codeagent/pkg/models"
	"github.com/qiniu/x/log"
)

// deepSeekLocal 本地 HTTP API 实现
type deepSeekLocal struct {
	workspace *models.Workspace
	config    *config.Config
	client    *http.Client
}

// NewDeepSeekLocal 创建本地 DeepSeek API 实现
func NewDeepSeekLocal(workspace *models.Workspace, cfg *config.Config) (Code, error) {
	if cfg.DeepSeek.APIKey == "" {
		return nil, fmt.Errorf("DEEPSEEK_API_KEY is required")
	}

	client := &http.Client{
		Timeout: cfg.DeepSeek.Timeout,
	}

	return &deepSeekLocal{
		workspace: workspace,
		config:    cfg,
		client:    client,
	}, nil
}

type deepSeekRequest struct {
	Model    string          `json:"model"`
	Messages []deepSeekMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

type deepSeekMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type deepSeekResponse struct {
	Choices []struct {
		Message deepSeekMessage `json:"message"`
	} `json:"choices"`
}

// Prompt 实现 Code 接口 - HTTP API 版本
func (d *deepSeekLocal) Prompt(message string) (*Response, error) {
	// 构建请求
	reqBody := deepSeekRequest{
		Model: d.config.DeepSeek.Model,
		Messages: []deepSeekMessage{
			{
				Role:    "user",
				Content: message,
			},
		},
		Stream: false,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// 设置超时
	timeout := d.config.DeepSeek.Timeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// 创建 HTTP 请求
	req, err := http.NewRequestWithContext(ctx, "POST", d.config.DeepSeek.BaseURL+"/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+d.config.DeepSeek.APIKey)

	log.Infof("Executing DeepSeek API call to %s with model %s", d.config.DeepSeek.BaseURL, d.config.DeepSeek.Model)

	// 发送请求
	resp, err := d.client.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			log.Warnf("DeepSeek API call timed out after %s", timeout)
			return nil, fmt.Errorf("deepseek API call timed out: %w", err)
		}
		return nil, fmt.Errorf("failed to call DeepSeek API: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// 检查响应状态码
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DeepSeek API returned status %d: %s", resp.StatusCode, string(body))
	}

	// 解析响应
	var deepSeekResp deepSeekResponse
	if err := json.Unmarshal(body, &deepSeekResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(deepSeekResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in DeepSeek response")
	}

	// 获取生成的内容
	content := deepSeekResp.Choices[0].Message.Content

	log.Infof("DeepSeek API call completed successfully")
	return &Response{
		Out: strings.NewReader(content),
	}, nil
}

// Close 实现 Code 接口
func (d *deepSeekLocal) Close() error {
	// HTTP 客户端不需要特殊的清理
	return nil
}