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

// claudeCode Docker implementation
type claudeCode struct {
	containerName string
}

func NewClaudeDocker(workspace *models.Workspace, cfg *config.Config) (Code, error) {
	// Parse repository information, only get repository name, not including full URL
	repoName := extractRepoName(workspace.Repository)
	// New container naming rule: claude-organization-repository-PR number
	containerName := fmt.Sprintf("claude-%s-%s-%d", workspace.Org, repoName, workspace.PRNumber)

	// Check if corresponding container is already running
	if isContainerRunning(containerName) {
		log.Infof("Found existing container: %s, reusing it", containerName)
		return &claudeCode{
			containerName: containerName,
		}, nil
	}

	// Ensure path exists
	workspacePath, _ := filepath.Abs(workspace.Path)

	claudeConfigPath, err := createIsolatedClaudeConfig(workspace, cfg)
	if err != nil {
		log.Errorf("Failed to create isolated Claude config: %v", err)
		return nil, fmt.Errorf("failed to create isolated Claude config: %w", err)
	}

	// Check if /tmp directory is used (may cause mount issues on macOS)
	if strings.HasPrefix(workspacePath, "/tmp/") {
		log.Warnf("Warning: Using /tmp directory may cause mount issues on macOS. Consider using other path instead.")
		log.Warnf("Current workspace path: %s", workspacePath)
	}

	// Check if path exists
	if _, err := os.Stat(workspacePath); os.IsNotExist(err) {
		log.Errorf("Workspace path does not exist: %s", workspacePath)
		return nil, fmt.Errorf("workspace path does not exist: %s", workspacePath)
	}

	// Build Docker command
	args := []string{
		"run",
		"--rm",                  // Automatically delete container after running
		"-d",                    // Run in background
		"--name", containerName, // Set container name
		"-v", fmt.Sprintf("%s:/workspace", workspacePath), // Mount workspace
		"-v", fmt.Sprintf("%s:/home/codeagent/.claude", claudeConfigPath), // Mount claude authentication information
		"-w", "/workspace", // Set working directory
	}

	// Add Claude API related environment variables
	if cfg.Claude.AuthToken != "" {
		args = append(args, "-e", fmt.Sprintf("ANTHROPIC_AUTH_TOKEN=%s", cfg.Claude.AuthToken))
	} else if cfg.Claude.APIKey != "" {
		args = append(args, "-e", fmt.Sprintf("ANTHROPIC_API_KEY=%s", cfg.Claude.APIKey))
	}
	if cfg.Claude.BaseURL != "" {
		args = append(args, "-e", fmt.Sprintf("ANTHROPIC_BASE_URL=%s", cfg.Claude.BaseURL))
	}

	// Add container image
	args = append(args, cfg.Claude.ContainerImage)

	// Print debug information
	log.Infof("Docker command: docker %s", strings.Join(args, " "))

	cmd := exec.Command("docker", args...)

	// Capture command output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		log.Errorf("Failed to start Docker container: %v", err)
		log.Errorf("Docker stderr: %s", stderr.String())
		return nil, fmt.Errorf("failed to start Docker container: %w", err)
	}

	// Wait for command completion
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
		"-c",
		"-p",
		message,
	}

	// Print debug information
	log.Infof("Executing claude command: docker %s", strings.Join(args, " "))

	cmd := exec.Command("docker", args...)

	// Capture stderr for debugging
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Errorf("Failed to start claude command: %v", err)
		log.Errorf("Stderr: %s", stderr.String())
		return nil, err
	}

	// Start command
	if err := cmd.Start(); err != nil {
		log.Errorf("Failed to start claude command: %v", err)
		log.Errorf("Stderr: %s", stderr.String())
		return nil, fmt.Errorf("failed to execute claude: %w", err)
	}

	// Don't wait for command completion, let caller handle output stream
	// Error handling will be done when caller reads
	return &Response{Out: stdout}, nil
}

func (c *claudeCode) Close() error {
	stopCmd := exec.Command("docker", "rm", "-f", c.containerName)
	return stopCmd.Run()
}

func createIsolatedClaudeConfig(workspace *models.Workspace, cfg *config.Config) (string, error) {
	repoName := extractRepoName(workspace.Repository)
	configDirName := fmt.Sprintf(".claude-%s-%s-%d", workspace.Org, repoName, workspace.PRNumber)

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
