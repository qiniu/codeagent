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

// geminiDocker Docker implementation (interactive mode)
type geminiDocker struct {
	containerName string
}

// getGoogleCloudProject gets Google Cloud project ID, prioritize value from config file
func getGoogleCloudProject(cfg *config.Config, repoName string) string {
	if cfg.Gemini.GoogleCloudProject != "" {
		return cfg.Gemini.GoogleCloudProject
	}
	return repoName
}

// NewGeminiDocker creates Docker Gemini CLI implementation
func NewGeminiDocker(workspace *models.Workspace, cfg *config.Config) (Code, error) {
	// Parse repository information, only get repository name, not full URL
	repoName := extractRepoName(workspace.Repository)
	// New container naming rule: gemini-org-repo-PRnumber
	containerName := fmt.Sprintf("gemini-%s-%s-%d", workspace.Org, repoName, workspace.PRNumber)

	// Check if there's already a corresponding container running
	if isContainerRunning(containerName) {
		log.Infof("Found existing container: %s, reusing it", containerName)
		return &geminiDocker{
			containerName: containerName,
		}, nil
	}

	// Ensure paths exist
	workspacePath, _ := filepath.Abs(workspace.Path)
	sessionPath, _ := filepath.Abs(workspace.SessionPath)

	// Determine gemini configuration path
	var geminiConfigPath string
	if home := os.Getenv("HOME"); home != "" {
		geminiConfigPath, _ = filepath.Abs(filepath.Join(home, ".gemini"))
	} else {
		geminiConfigPath = "/home/codeagent/.gemini"
	}

	// Check if using /tmp directory (may cause mount issues on macOS)
	if strings.HasPrefix(workspacePath, "/tmp/") {
		log.Warnf("Warning: Using /tmp directory may cause mount issues on macOS. Consider using other path instead.")
		log.Warnf("Current workspace path: %s", workspacePath)
	}

	if strings.HasPrefix(sessionPath, "/tmp/") {
		log.Warnf("Warning: Using /tmp directory may cause mount issues on macOS. Consider using other path instead.")
		log.Warnf("Current session path: %s", sessionPath)
	}

	// Check if paths exist
	if _, err := os.Stat(workspacePath); os.IsNotExist(err) {
		log.Errorf("Workspace path does not exist: %s", workspacePath)
		return nil, fmt.Errorf("workspace path does not exist: %s", workspacePath)
	}

	if _, err := os.Stat(sessionPath); os.IsNotExist(err) {
		log.Errorf("Session path does not exist: %s", sessionPath)
		return nil, fmt.Errorf("session path does not exist: %s", sessionPath)
	}

	// Build Docker command
	args := []string{
		"run",
		"--rm",                  // Automatically delete container after running
		"-d",                    // Run in background
		"--name", containerName, // Set container name
		"-e", "GOOGLE_CLOUD_PROJECT=" + getGoogleCloudProject(cfg, repoName), // Set Google Cloud project environment variable
		"-e", "GEMINI_API_KEY=" + cfg.Gemini.APIKey,
		"-v", fmt.Sprintf("%s:/workspace", workspacePath), // Mount workspace
		"-v", fmt.Sprintf("%s:/home/codeagent/.gemini", geminiConfigPath), // Mount gemini authentication info
		"-v", fmt.Sprintf("%s:/home/codeagent/.gemini/tmp", sessionPath), // Mount temporary directory
		"-w", "/workspace", // Set working directory
		cfg.Gemini.ContainerImage, // Use configured Gemini image
	}

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

	return &geminiDocker{
		containerName: containerName,
	}, nil
}

// Prompt implements Code interface
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

	// Start command
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to execute gemini: %w", err)
	}

	return &Response{Out: stdout}, nil
}

// Close implements Code interface
func (g *geminiDocker) Close() error {
	stopCmd := exec.Command("docker", "rm", "-f", g.containerName)
	return stopCmd.Run()
}
