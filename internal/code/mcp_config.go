package code

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/qiniu/codeagent/internal/config"
	"github.com/qiniu/codeagent/pkg/models"
	"github.com/qiniu/x/log"
)

// MCPServerConfig MCP服务器配置
type MCPServerConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	Cwd     string            `json:"cwd,omitempty"`
}

// MCPConfig Claude CLI MCP配置
type MCPConfig struct {
	MCPServers map[string]MCPServerConfig `json:"mcpServers"`
}

// MCPConfigGenerator MCP配置生成器（仅支持Docker模式）
type MCPConfigGenerator struct {
	workspace           *models.Workspace
	config              *config.Config
	containerBinaryPath string // Docker容器内的二进制路径
}

// NewMCPConfigGenerator 创建Docker模式的MCP配置生成器
func NewMCPConfigGenerator(workspace *models.Workspace, cfg *config.Config, containerBinaryPath string) *MCPConfigGenerator {
	return &MCPConfigGenerator{
		workspace:           workspace,
		config:              cfg,
		containerBinaryPath: containerBinaryPath,
	}
}

// GenerateConfig 生成MCP配置
func (g *MCPConfigGenerator) GenerateConfig() (*MCPConfig, error) {
	// Docker模式：使用容器内的路径
	serverBinary := filepath.Join(g.containerBinaryPath, "mcp-server")
	workingDir := "/workspace" // Docker容器内的工作目录
	log.Infof("Docker mode: Using container binary path: %s", serverBinary)

	config := &MCPConfig{
		MCPServers: map[string]MCPServerConfig{
			"codeagent": {
				Command: serverBinary,
				Args:    []string{},
				Env:     g.buildEnvironment(),
				Cwd:     workingDir,
			},
		},
	}

	return config, nil
}

// buildEnvironment 构建环境变量
func (g *MCPConfigGenerator) buildEnvironment() map[string]string {
	env := map[string]string{}

	// GitHub Token
	if g.config.GitHub.Token != "" {
		env["GITHUB_TOKEN"] = g.config.GitHub.Token
	}

	// 仓库信息
	if g.workspace.Org != "" {
		env["REPO_OWNER"] = g.workspace.Org
	}
	if g.workspace.Repo != "" {
		env["REPO_NAME"] = g.workspace.Repo
	}
	if g.workspace.Branch != "" {
		env["BRANCH_NAME"] = g.workspace.Branch
	}

	// PR和Issue信息
	if g.workspace.PRNumber > 0 {
		env["PR_NUMBER"] = strconv.Itoa(g.workspace.PRNumber)
	}
	if g.workspace.Issue != nil {
		env["ISSUE_NUMBER"] = strconv.Itoa(g.workspace.Issue.GetNumber())
	}

	// 工作空间路径
	if g.workspace.Path != "" {
		env["WORKSPACE_PATH"] = g.workspace.Path
	}

	// GitHub App配置（如果配置了）
	if g.config.IsGitHubAppConfigured() {
		if g.config.GitHub.App.AppID > 0 {
			env["GITHUB_APP_ID"] = strconv.FormatInt(g.config.GitHub.App.AppID, 10)
		}
		if g.config.GitHub.App.PrivateKey != "" {
			env["GITHUB_APP_PRIVATE_KEY"] = g.config.GitHub.App.PrivateKey
		}
		if g.config.GitHub.App.PrivateKeyPath != "" {
			env["GITHUB_APP_PRIVATE_KEY_PATH"] = g.config.GitHub.App.PrivateKeyPath
		}
	}

	return env
}

// WriteToFile 将配置写入文件
func (c *MCPConfig) WriteToFile(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// CreateTempConfig 创建临时配置文件
func (g *MCPConfigGenerator) CreateTempConfig() (string, error) {
	config, err := g.GenerateConfig()
	if err != nil {
		return "", err
	}

	// 创建临时文件在/tmp目录中
	tempFile, err := os.CreateTemp("/tmp", "codeagent-mcp-*.json")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer tempFile.Close()

	// 设置文件权限，确保所有用户都可以读取（解决Docker权限问题）
	if err := os.Chmod(tempFile.Name(), 0644); err != nil {
		log.Warnf("Failed to set file permissions: %v", err)
	}

	if err := config.WriteToFile(tempFile.Name()); err != nil {
		os.Remove(tempFile.Name())
		return "", err
	}

	return tempFile.Name(), nil
}
