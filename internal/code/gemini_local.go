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

// geminiLocal Local CLI implementation
type geminiLocal struct {
	workspace *models.Workspace
	config    *config.Config
}

// NewGeminiLocal creates local Gemini CLI implementation
func NewGeminiLocal(workspace *models.Workspace, cfg *config.Config) (Code, error) {
	// Check if gemini CLI is available
	if err := checkGeminiCLI(); err != nil {
		return nil, fmt.Errorf("gemini CLI not available: %w", err)
	}

	return &geminiLocal{
		workspace: workspace,
		config:    cfg,
	}, nil
}

// checkGeminiCLI checks if gemini CLI is available
func checkGeminiCLI() error {
	cmd := exec.Command("gemini", "--version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gemini CLI not found or not working: %w", err)
	}
	return nil
}

// Prompt implements Code interface - local CLI version
func (g *geminiLocal) Prompt(message string) (*Response, error) {
	// Execute local gemini CLI call
	output, err := g.executeGeminiLocal(message)
	if err != nil {
		return nil, fmt.Errorf("failed to execute gemini prompt: %w", err)
	}

	// Return result
	return &Response{
		Out: bytes.NewReader(output),
	}, nil
}

// executeGeminiLocal executes local gemini CLI call
func (g *geminiLocal) executeGeminiLocal(prompt string) ([]byte, error) {
	// Build gemini CLI command
	args := []string{
		"-y",
		"--prompt", prompt,
	}

	// Set timeout - use timeout from config, default 5 minutes
	timeout := g.config.Gemini.Timeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gemini", args...)
	cmd.Dir = g.workspace.Path // Set working directory, Gemini CLI will automatically read files in this directory as context

	// Set environment variables
	cmd.Env = os.Environ()

	log.Infof("Executing local gemini CLI in directory %s: gemini %s", g.workspace.Path, strings.Join(args, " "))

	// Execute command and get output
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			log.Warnf("Gemini CLI execution timed out after %s, this might be due to large codebase or complex task", timeout)
			return nil, fmt.Errorf("gemini CLI execution timed out: %w", err)
		}

		// Check if it's API key related error
		outputStr := string(output)
		if strings.Contains(outputStr, "API Error") || strings.Contains(outputStr, "fetch failed") {
			return nil, fmt.Errorf("gemini API error - please check GOOGLE_API_KEY: %w, output: %s", err, outputStr)
		}

		// Check if it's network related error
		if strings.Contains(outputStr, "timeout") || strings.Contains(outputStr, "connection") {
			log.Warnf("Network-related error detected: %s", outputStr)
		}

		return nil, fmt.Errorf("gemini CLI execution failed: %w, output: %s", err, outputStr)
	}

	log.Infof("Local gemini CLI execution completed successfully")
	return output, nil
}

// Close implements Code interface
func (g *geminiLocal) Close() error {
	// Single prompt mode doesn't need special cleanup
	// Each call is an independent process
	return nil
}

func parseRepoURL(repoURL string) (owner, repo string) {
	// Handle HTTPS URL: https://github.com/owner/repo.git
	if strings.Contains(repoURL, "github.com") {
		parts := strings.Split(repoURL, "/")
		if len(parts) >= 2 {
			repo = strings.TrimSuffix(parts[len(parts)-1], ".git")
			owner = parts[len(parts)-2]
		}
	}
	return owner, repo
}
