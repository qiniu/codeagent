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

// claudeCode Docker implementation with MCP support
type claudeCode struct {
	containerName string
	configGen     *MCPConfigGenerator
}

func NewClaudeDocker(workspace *models.Workspace, cfg *config.Config) (Code, error) {
	// Parse repository information, only get repository name, not including complete URL
	repoName := extractRepoName(workspace.Repository)

	// Generate unique container name using shared function
	containerName := generateContainerName("claude", workspace.Org, repoName, workspace)

	// Initialize MCP support (默认启用)
	configGen := NewMCPConfigGenerator(workspace, cfg, "/usr/local/bin")
	log.Infof("MCP support enabled for Docker container")

	// Check if corresponding container is already running
	if isContainerRunning(containerName) {
		log.Infof("Found existing container: %s, reusing it", containerName)
		return &claudeCode{
			containerName: containerName,
			configGen:     configGen,
		}, nil
	}

	// 确保路径存在
	workspacePath, err := filepath.Abs(workspace.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute workspace path: %w", err)
	}

	claudeConfigPath, err := createIsolatedClaudeConfig(workspace, cfg)
	if err != nil {
		log.Errorf("Failed to create isolated Claude config: %v", err)
		return nil, fmt.Errorf("failed to create isolated Claude config: %w", err)
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

	// Mount processed .codeagent directory and merged agents
	if workspace.ProcessedCodeAgentPath != "" {
		if _, err := os.Stat(workspace.ProcessedCodeAgentPath); err == nil {
			// Mount the entire .codeagent directory for other tools that might need it
			args = append(args, "-v", fmt.Sprintf("%s:/home/codeagent/.codeagent", workspace.ProcessedCodeAgentPath))
			log.Infof("Mounting processed .codeagent directory: %s -> /home/codeagent/.codeagent", workspace.ProcessedCodeAgentPath)

			// Mount merged agents directory directly to Claude's expected location
			agentsPath := filepath.Join(workspace.ProcessedCodeAgentPath, "agents")
			if stat, err := os.Stat(agentsPath); err == nil && stat.IsDir() {
				args = append(args, "-v", fmt.Sprintf("%s:/home/codeagent/.claude/agents", agentsPath))
				log.Infof("Mounting merged agents directory: %s -> /home/codeagent/.claude/agents", agentsPath)
			} else {
				log.Infof("No agents directory found in processed .codeagent path: %s", agentsPath)
			}
		} else {
			log.Warnf("Processed .codeagent directory not found: %s", workspace.ProcessedCodeAgentPath)
		}
	}

	// Add MCP support (默认启用)
	// 添加 MCP 相关环境变量
	args = append(args, "-e", fmt.Sprintf("GITHUB_TOKEN=%s", cfg.GitHub.Token))
	args = append(args, "-e", fmt.Sprintf("REPO_OWNER=%s", workspace.Org))
	args = append(args, "-e", fmt.Sprintf("REPO_NAME=%s", workspace.Repo))
	if workspace.Branch != "" {
		args = append(args, "-e", fmt.Sprintf("BRANCH_NAME=%s", workspace.Branch))
	}
	if workspace.PRNumber > 0 {
		args = append(args, "-e", fmt.Sprintf("PR_NUMBER=%d", workspace.PRNumber))
	}
	if workspace.Issue != nil {
		args = append(args, "-e", fmt.Sprintf("ISSUE_NUMBER=%d", workspace.Issue.GetNumber()))
	}

	log.Infof("Added MCP environment variables for container")

	// 添加 Claude API 相关环境变量
	if cfg.Claude.AuthToken != "" {
		args = append(args, "-e", fmt.Sprintf("ANTHROPIC_AUTH_TOKEN=%s", cfg.Claude.AuthToken))
	} else if cfg.Claude.APIKey != "" {
		args = append(args, "-e", fmt.Sprintf("ANTHROPIC_API_KEY=%s", cfg.Claude.APIKey))
	}
	if cfg.Claude.BaseURL != "" {
		args = append(args, "-e", fmt.Sprintf("ANTHROPIC_BASE_URL=%s", cfg.Claude.BaseURL))
	}
	if cfg.GitHub.GHToken != "" {
		args = append(args, "-e", fmt.Sprintf("GH_TOKEN=%s", cfg.GitHub.GHToken))
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
		configGen:     configGen,
	}, nil
}

func (c *claudeCode) Prompt(message string) (*Response, error) {
	log.Infof("Executing Claude with Docker container %s", c.containerName)
	log.Infof("MCP support enabled, configGen available: %v", c.configGen != nil)

	// 尝试 MCP 模式
	if c.configGen != nil {
		log.Infof("Attempting MCP mode in Docker container")
		response, err := c.executeClaudeWithMCP(message)
		if err != nil {
			log.Errorf("MCP mode failed with error: %v", err)
			log.Warnf("Falling back to normal mode")
			return c.executeClaudeWithoutMCP(message)
		}
		log.Infof("MCP mode executed successfully")
		return response, nil
	}

	// 普通模式（作为后备）
	log.Infof("Using normal mode (MCP configGen not available)")
	return c.executeClaudeWithoutMCP(message)
}

// executeClaudeWithMCP 在Docker容器中使用MCP执行Claude
func (c *claudeCode) executeClaudeWithMCP(message string) (*Response, error) {
	log.Infof("Starting MCP execution in Docker container")

	// 1. 生成MCP配置文件
	log.Infof("Step 1: Generating MCP config file")
	mcpConfigPath, err := c.configGen.CreateTempConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create MCP config: %w", err)
	}

	log.Infof("MCP config file created at: %s", mcpConfigPath)

	// 2. 将MCP配置文件复制到容器中并修复权限
	log.Infof("Step 2: Copying MCP config file to container")
	copyCmd := exec.Command("docker", "cp", mcpConfigPath, c.containerName+":/tmp/mcp-config.json")
	if err := copyCmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to copy MCP config to container: %w", err)
	}
	log.Infof("MCP config file copied to container: /tmp/mcp-config.json")

	// 修复容器内文件权限，确保所有用户都可以读取
	log.Infof("Step 2.1: Fixing file permissions in container")

	// 首先尝试设置文件为全局可读
	chmodCmd := exec.Command("docker", "exec", c.containerName, "chmod", "755", "/tmp/mcp-config.json")
	if err := chmodCmd.Run(); err != nil {
		log.Warnf("Failed to chmod MCP config file: %v", err)
	}
	// 检查文件是否已经可读
	testReadCmd := exec.Command("docker", "exec", c.containerName, "test", "-r", "/tmp/mcp-config.json")
	if err := testReadCmd.Run(); err != nil {
		return nil, fmt.Errorf("MCP config file is not readable: %w", err)
	}

	// 3. 执行Claude CLI with MCP
	log.Infof("Step 3: Executing Claude CLI with MCP configuration")
	args := []string{
		"exec",
		"-e", "HOME=/home/codeagent", // 设置环境变量
		"-e", "CLAUDE_CONFIG_DIR=/home/codeagent/.claude",
		c.containerName,
		"claude",
		"--mcp-config", "/tmp/mcp-config.json",
		"--dangerously-skip-permissions",
		"-p", message,
	}

	log.Infof("Final Claude command: docker %s", strings.Join(args, " "))

	cmd := exec.Command("docker", args...)

	// 打印调试信息
	log.Infof("Executing Claude with MCP in Docker: docker %s", strings.Join(args, " "))

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
	log.Infof("Claude MCP command started successfully in Docker")

	return &Response{Out: stdout}, nil
}

// executeClaudeWithoutMCP 在Docker容器中不使用MCP执行Claude（降级模式）
func (c *claudeCode) executeClaudeWithoutMCP(message string) (*Response, error) {
	log.Infof("Executing Claude in fallback mode (no MCP)")

	args := []string{
		"exec",
		"-e", "HOME=/home/codeagent", // 设置环境变量
		"-e", "CLAUDE_CONFIG_DIR=/home/codeagent/.claude",
		c.containerName,
		"claude",
		"--dangerously-skip-permissions",
		"-c",
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

func createIsolatedClaudeConfig(workspace *models.Workspace, cfg *config.Config) (string, error) {
	repoName := extractRepoName(workspace.Repository)

	// Generate unique config directory name using shared function
	configDirName := generateConfigDirName("claude", workspace.Org, repoName, workspace)

	var isolatedConfigDir string
	if home := os.Getenv("HOME"); home != "" {
		isolatedConfigDir = filepath.Join(home, configDirName)
	} else {
		isolatedConfigDir = filepath.Join("/home/codeagent", configDirName)
	}

	if _, err := os.Stat(isolatedConfigDir); err == nil {
		log.Infof("Isolated Claude config directory already exists: %s", isolatedConfigDir)
		return isolatedConfigDir, nil
	}

	if err := os.MkdirAll(isolatedConfigDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create isolated config directory %s: %w", isolatedConfigDir, err)
	}

	if shouldCopyHostClaudeConfig(cfg) {
		if err := copyHostClaudeConfig(isolatedConfigDir); err != nil {
			log.Warnf("Failed to copy host Claude config: %v", err)
		}
	}

	log.Infof("Created isolated Claude config directory: %s", isolatedConfigDir)
	return isolatedConfigDir, nil
}

func shouldCopyHostClaudeConfig(cfg *config.Config) bool {
	return cfg.Claude.APIKey == "" && cfg.Claude.AuthToken == ""
}

func copyHostClaudeConfig(isolatedConfigDir string) error {
	var hostClaudeDir string
	if home := os.Getenv("HOME"); home != "" {
		hostClaudeDir = filepath.Join(home, ".claude")
	} else {
		return fmt.Errorf("HOME environment variable not set, cannot locate host Claude config")
	}

	if _, err := os.Stat(hostClaudeDir); os.IsNotExist(err) {
		return fmt.Errorf("host Claude config directory does not exist: %s", hostClaudeDir)
	}

	log.Infof("Copying host Claude config from %s to %s", hostClaudeDir, isolatedConfigDir)

	cmd := exec.Command("cp", "-r", hostClaudeDir+"/.", isolatedConfigDir)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to copy Claude config directory: %w", err)
	}

	log.Infof("Successfully copied host Claude config to isolated directory")
	return nil
}
