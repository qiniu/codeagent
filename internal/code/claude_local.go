package code

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/qiniu/codeagent/internal/config"
	"github.com/qiniu/codeagent/pkg/models"
	"github.com/qiniu/x/log"
)

// claudeLocal Local CLI implementation
type claudeLocal struct {
	workspace *models.Workspace
	config    *config.Config
}

// NewClaudeLocal creates local Claude CLI implementation
func NewClaudeLocal(workspace *models.Workspace, cfg *config.Config) (Code, error) {
	// Check if claude CLI is available
	if err := checkClaudeCLI(); err != nil {
		return nil, fmt.Errorf("claude CLI not available: %w", err)
	}

	return &claudeLocal{
		workspace: workspace,
		config:    cfg,
	}, nil
}

// checkClaudeCLI checks if claude CLI is available
func checkClaudeCLI() error {
	cmd := exec.Command("claude", "--version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("claude CLI not found or not working: %w", err)
	}
	return nil
}

// Prompt implements Code interface - local CLI version
func (c *claudeLocal) Prompt(message string) (*Response, error) {
	// Execute local claude CLI call
	output, err := c.executeClaudeLocal(message)
	if err != nil {
		return nil, fmt.Errorf("failed to execute claude prompt: %w", err)
	}

	// Return result
	return &Response{
		Out: bytes.NewReader(output),
	}, nil
}

// executeClaudeLocal executes local claude CLI call
func (c *claudeLocal) executeClaudeLocal(prompt string) ([]byte, error) {
	// Build claude CLI command
	args := []string{
		"-p",
		prompt,
	}

	// Set timeout - use timeout from config, default 5 minutes
	timeout := c.config.Claude.Timeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Dir = c.workspace.Path // Set working directory, Claude CLI will automatically read files in this directory as context

	// Set environment variables
	cmd.Env = os.Environ()

	log.Infof("Executing local claude CLI in directory %s: claude %s", c.workspace.Path, strings.Join(args, " "))

	// Execute command and get output
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			log.Warnf("Claude CLI execution timed out after %s, this might be due to large codebase or complex task", timeout)
			return nil, fmt.Errorf("claude CLI execution timed out: %w", err)
		}

		// Check if it's API key related error
		outputStr := string(output)
		if strings.Contains(outputStr, "API Error") || strings.Contains(outputStr, "fetch failed") || strings.Contains(outputStr, "authentication") {
			return nil, fmt.Errorf("claude API error - please check CLAUDE_API_KEY: %w, output: %s", err, outputStr)
		}

		// Check if it's network related error
		if strings.Contains(outputStr, "timeout") || strings.Contains(outputStr, "connection") {
			log.Warnf("Network-related error detected: %s", outputStr)
		}

		return nil, fmt.Errorf("claude CLI execution failed: %w, output: %s", err, outputStr)
	}

	log.Infof("Local claude CLI execution completed successfully")
	return output, nil
}

// Close implements Code interface
func (c *claudeLocal) Close() error {
	// Single prompt mode doesn't need special cleanup
	// Each call is an independent process
	return nil
}
