package code

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestSimplePipeCommunication(t *testing.T) {
	// Test simple pipe communication
	cmd := exec.Command("echo", "Hello World")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("Failed to create stdout pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start command: %v", err)
	}

	buffer := make([]byte, 1024)
	n, err := stdout.Read(buffer)
	if err != nil {
		t.Fatalf("Failed to read from stdout: %v", err)
	}

	output := string(buffer[:n])
	if !strings.Contains(output, "Hello World") {
		t.Errorf("Expected 'Hello World', got '%s'", strings.TrimSpace(output))
	}

	if err := cmd.Wait(); err != nil {
		t.Fatalf("Command failed: %v", err)
	}
}

func TestStdinPipeCommunication(t *testing.T) {
	// Test stdin pipe communication
	cmd := exec.Command("cat")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("Failed to create stdin pipe: %v", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		t.Fatalf("Failed to create stdout pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		t.Fatalf("Failed to start command: %v", err)
	}

	// Write data to stdin
	testData := "Test input data\n"
	if _, err := stdin.Write([]byte(testData)); err != nil {
		t.Fatalf("Failed to write to stdin: %v", err)
	}

	// Close stdin to end cat command
	stdin.Close()

	// Read output
	buffer := make([]byte, 1024)
	n, err := stdout.Read(buffer)
	if err != nil {
		t.Fatalf("Failed to read from stdout: %v", err)
	}

	output := string(buffer[:n])
	if output != testData {
		t.Errorf("Expected '%s', got '%s'", testData, output)
	}

	if err := cmd.Wait(); err != nil {
		t.Fatalf("Command failed: %v", err)
	}
}

func TestFileReadDeadline(t *testing.T) {
	// Test file read deadline setting
	file, err := os.Open("/dev/null")
	if err != nil {
		t.Skipf("Cannot open /dev/null: %v", err)
	}
	defer file.Close()

	// Set read timeout
	file.SetReadDeadline(time.Now().Add(100 * time.Millisecond))

	buffer := make([]byte, 1024)
	_, err = file.Read(buffer)

	// Reading from /dev/null should immediately return EOF, not timeout
	if err != nil && !strings.Contains(err.Error(), "EOF") {
		t.Errorf("Expected EOF or timeout, got: %v", err)
	}
}
