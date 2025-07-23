package code

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestSimplePipeCommunication(t *testing.T) {
	// 测试简单的管道通信
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
	// 测试 stdin 管道通信
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

	// 写入数据到 stdin
	testData := "Test input data\n"
	if _, err := stdin.Write([]byte(testData)); err != nil {
		t.Fatalf("Failed to write to stdin: %v", err)
	}

	// 关闭 stdin 以结束 cat 命令
	stdin.Close()

	// 读取输出
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
	// 测试文件读取超时设置
	file, err := os.Open("/dev/null")
	if err != nil {
		t.Skipf("Cannot open /dev/null: %v", err)
	}
	defer file.Close()

	// 设置读取超时
	file.SetReadDeadline(time.Now().Add(100 * time.Millisecond))

	buffer := make([]byte, 1024)
	_, err = file.Read(buffer)

	// 在 /dev/null 上读取应该立即返回 EOF，而不是超时
	if err != nil && !strings.Contains(err.Error(), "EOF") {
		t.Errorf("Expected EOF or timeout, got: %v", err)
	}
}
