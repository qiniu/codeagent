package code

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/qiniu/codeagent/internal/config"
	"github.com/qiniu/codeagent/pkg/models"
	"github.com/qiniu/x/log"
)

// claudeInteractive Interactive Claude Docker implementation
type claudeInteractive struct {
	containerName string
	cmd           *exec.Cmd
	stdin         io.WriteCloser
	stdout        io.ReadCloser
	mutex         sync.Mutex
	session       *InteractiveSession
	closed        bool
	ctx           context.Context
	cancel        context.CancelFunc
}

// InteractiveSession manages interactive sessions
type InteractiveSession struct {
	ID            string
	CreatedAt     time.Time
	LastActivity  time.Time
	MessageCount  int
	WorkspacePath string
}

func NewClaudeInteractive(workspace *models.Workspace, cfg *config.Config) (Code, error) {
	// Parse repository information, only get repository name, not full URL
	repoName := extractRepoName(workspace.Repository)
	// New container naming rule: claude-interactive-org-repo-PRnumber
	containerName := fmt.Sprintf("claude-interactive-%s-%s-%d", workspace.Org, repoName, workspace.PRNumber)

	// Check if there's already a corresponding container running
	if isContainerRunning(containerName) {
		log.Infof("Found existing interactive container: %s, reusing it", containerName)
		// Connect to existing container
		return connectToExistingContainer(containerName, workspace)
	}

	// Ensure path exists
	workspacePath, _ := filepath.Abs(workspace.Path)

	// Determine claude configuration path
	var claudeConfigPath string
	if home := os.Getenv("HOME"); home != "" {
		claudeConfigPath, _ = filepath.Abs(filepath.Join(home, ".claude"))
	} else {
		claudeConfigPath = "/home/codeagent/.claude"
	}

	// Check if using /tmp directory (may cause mount issues on macOS)
	if strings.HasPrefix(workspacePath, "/tmp/") {
		log.Warnf("Warning: Using /tmp directory may cause mount issues on macOS. Consider using other path instead.")
		log.Warnf("Current workspace path: %s", workspacePath)
	}

	// Check if path exists
	if _, err := os.Stat(workspacePath); os.IsNotExist(err) {
		log.Errorf("Workspace path does not exist: %s", workspacePath)
		return nil, fmt.Errorf("workspace path does not exist: %s", workspacePath)
	}

	// Build Docker command - use simple pipe mode instead of PTY
	args := []string{
		"run",
		"-i",                    // Keep STDIN open
		"--rm",                  // Automatically delete container after stop
		"--name", containerName, // Set container name
		"--entrypoint", "claude", // Directly use claude as entrypoint
		"-v", fmt.Sprintf("%s:/workspace", workspacePath), // Mount workspace
		"-v", fmt.Sprintf("%s:/home/codeagent/.claude", claudeConfigPath), // Mount claude authentication info
		"-w", "/workspace", // Set working directory
		"-e", "TERM=xterm-256color", // Set terminal type
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

	// Add container image - no additional commands needed since using --entrypoint
	args = append(args, cfg.Claude.ContainerImage)

	// Print debug information
	log.Infof("Starting interactive Docker container: docker %s", strings.Join(args, " "))

	cmd := exec.Command("docker", args...)

	// Create pipes
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	cmd.Stderr = cmd.Stdout // Redirect stderr to stdout
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true, // Create new process group
	}

	// Start container
	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		log.Errorf("Failed to start interactive Docker container: %v", err)
		return nil, fmt.Errorf("failed to start interactive Docker container: %w", err)
	}

	log.Infof("Interactive Docker container started successfully: %s", containerName)
	log.Infof("Container process ID: %d", cmd.Process.Pid)
	log.Infof("stdin pipe type: %T", stdin)
	log.Infof("stdout pipe type: %T", stdout)

	// Create context for cancellation
	ctx, cancel := context.WithCancel(context.Background())

	// Create session information
	session := &InteractiveSession{
		ID:            fmt.Sprintf("session-%d", time.Now().Unix()),
		CreatedAt:     time.Now(),
		LastActivity:  time.Now(),
		MessageCount:  0,
		WorkspacePath: workspacePath,
	}

	claudeInteractive := &claudeInteractive{
		containerName: containerName,
		cmd:           cmd,
		stdin:         stdin,
		stdout:        stdout,
		session:       session,
		closed:        false,
		ctx:           ctx,
		cancel:        cancel,
	}

	// Wait for Claude CLI initialization to complete
	if err := claudeInteractive.waitForReady(); err != nil {
		return nil, fmt.Errorf("claude CLI initialization failed: %w", err)
	}

	return claudeInteractive, nil
}

func (c *claudeInteractive) waitForReady() error {
	// Wait for Claude CLI to start
	log.Infof("Waiting for Claude CLI to initialize...")
	time.Sleep(5 * time.Second)

	// Simply assume ready, let the first Prompt do the actual test
	log.Infof("Claude CLI initialization completed (will test on first prompt)")
	return nil
}

// connectToExistingContainer connects to existing interactive container
func connectToExistingContainer(containerName string, workspace *models.Workspace) (Code, error) {
	// Connect to existing container via docker exec
	args := []string{
		"exec",
		"-i",
		containerName,
		"claude",
	}

	cmd := exec.Command("docker", args...)

	// Create pipes
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe for existing container: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, fmt.Errorf("failed to create stdout pipe for existing container: %w", err)
	}

	cmd.Stderr = cmd.Stdout
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	// Start exec command
	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		return nil, fmt.Errorf("failed to connect to existing container: %w", err)
	}

	// Create context for cancellation
	ctx, cancel := context.WithCancel(context.Background())

	// Create session information
	session := &InteractiveSession{
		ID:            fmt.Sprintf("reconnect-session-%d", time.Now().Unix()),
		CreatedAt:     time.Now(),
		LastActivity:  time.Now(),
		MessageCount:  0,
		WorkspacePath: workspace.Path,
	}

	return &claudeInteractive{
		containerName: containerName,
		cmd:           cmd,
		stdin:         stdin,
		stdout:        stdout,
		session:       session,
		closed:        false,
		ctx:           ctx,
		cancel:        cancel,
	}, nil
}

func (c *claudeInteractive) Prompt(message string) (*Response, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.closed {
		return nil, fmt.Errorf("interactive session is closed")
	}

	// Update session information
	c.session.LastActivity = time.Now()
	c.session.MessageCount++

	log.Infof("Sending interactive message #%d to Claude: %s", c.session.MessageCount, message)

	// Send message to Claude CLI via stdin
	messageBytes := []byte(message + "\n")
	log.Debugf("Writing %d bytes to stdin: %s", len(messageBytes), strings.TrimSpace(string(messageBytes)))

	if _, err := c.stdin.Write(messageBytes); err != nil {
		return nil, fmt.Errorf("failed to send message to Claude: %w", err)
	}

	log.Debugf("Successfully wrote message to stdin")

	// Ensure message is sent
	if flusher, ok := c.stdin.(interface{ Flush() error }); ok {
		if err := flusher.Flush(); err != nil {
			log.Warnf("Failed to flush stdin: %v", err)
		} else {
			log.Debugf("Successfully flushed stdin")
		}
	} else {
		log.Debugf("stdin does not support Flush()")
	}

	// Give Claude some time to start processing the message
	time.Sleep(500 * time.Millisecond)

	log.Debugf("Creating InteractiveResponseReader for message #%d", c.session.MessageCount)

	// Create response reader
	responseReader := &InteractiveResponseReader{
		stdout:  c.stdout,
		session: c.session,
		ctx:     c.ctx,
	}

	return &Response{Out: responseReader}, nil
}

// InteractiveResponseReader handles interactive response reading
type InteractiveResponseReader struct {
	stdout  io.ReadCloser
	session *InteractiveSession
	buffer  bytes.Buffer
	done    bool
	ctx     context.Context
	mutex   sync.Mutex
}

func (r *InteractiveResponseReader) Read(p []byte) (n int, err error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.done {
		return 0, io.EOF
	}

	// Read data from stdout
	buffer := make([]byte, 4096)
	n, err = r.stdout.Read(buffer)

	log.Debugf("InteractiveResponseReader: Read %d bytes, error: %v, buffer size: %d", n, err, r.buffer.Len())

	if err != nil {
		if err == io.EOF {
			r.done = true
			log.Infof("InteractiveResponseReader: EOF reached, total buffer size: %d", r.buffer.Len())
		}
		return 0, err
	}

	// If no data read, return 0 instead of error
	if n == 0 {
		log.Debugf("InteractiveResponseReader: No data read, waiting...")
		return 0, nil
	}

	// Write data to buffer and return to caller
	r.buffer.Write(buffer[:n])
	copy(p, buffer[:n])

	// Simplified response completion detection - only end in very clear cases
	if r.isResponseComplete(buffer[:n]) {
		r.done = true
		log.Infof("InteractiveResponseReader: Response complete detected, total buffer size: %d", r.buffer.Len())
		return n, io.EOF
	}

	return n, nil
}

// isResponseComplete checks if response is complete - simplified version
func (r *InteractiveResponseReader) isResponseComplete(data []byte) bool {
	responseText := string(data)
	bufferText := r.buffer.String()

	log.Debugf("InteractiveResponseReader: Checking response completion - responseText: %s, bufferText length: %d",
		strings.TrimSpace(responseText), len(bufferText))

	// Only check completion markers when buffer is large enough
	if len(bufferText) < 500 {
		return false
	}

	// Only check very clear end markers
	if strings.Contains(responseText, "claude>") || // Claude CLI prompt
		strings.Contains(bufferText, "Complete") ||
		strings.Contains(bufferText, "Done") ||
		strings.Contains(bufferText, "Error:") ||
		strings.Contains(bufferText, "Finished") ||
		// Check for obvious task completion markers
		(strings.Contains(bufferText, "## Summary") && strings.Contains(bufferText, "## Changes")) {
		log.Debugf("InteractiveResponseReader: Response complete detected - found completion marker")
		log.Debugf("InteractiveResponseReader: Buffer content (last 200 chars): %s",
			bufferText[func() int {
				if len(bufferText)-200 > 0 {
					return len(bufferText) - 200
				} else {
					return 0
				}
			}():])
		return true
	}

	return false
}

func (c *claudeInteractive) Close() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.closed {
		return nil
	}

	c.closed = true

	log.Infof("Closing interactive Claude session %s (messages: %d)", c.session.ID, c.session.MessageCount)

	// Cancel context
	if c.cancel != nil {
		c.cancel()
	}

	// Close pipes
	if c.stdin != nil {
		c.stdin.Close()
	}
	if c.stdout != nil {
		c.stdout.Close()
	}

	// Terminate process
	if c.cmd != nil && c.cmd.Process != nil {
		c.cmd.Process.Kill()
		c.cmd.Wait()
	}

	// Check container status, but don't delete container (for debugging)
	log.Infof("Checking container status for debugging: %s", c.containerName)

	// Check if container is still running
	checkCmd := exec.Command("docker", "ps", "--filter", fmt.Sprintf("name=%s", c.containerName), "--format", "{{.Names}}")
	output, err := checkCmd.Output()
	if err != nil {
		log.Warnf("Failed to check container status: %v", err)
	} else {
		containerStatus := strings.TrimSpace(string(output))
		if containerStatus == c.containerName {
			log.Infof("Container %s is still running - keeping it for debugging", c.containerName)
			log.Infof("You can manually inspect it with: docker logs %s", c.containerName)
			log.Infof("You can manually test it with: echo '/help' | docker exec -i %s claude", c.containerName)
		} else {
			log.Infof("Container %s is not running (status: %s)", c.containerName, containerStatus)
		}
	}

	// Note: Don't delete container, for debugging purposes
	log.Infof("Container %s left running for debugging purposes", c.containerName)

	return nil
}
