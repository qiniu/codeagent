package code

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/qbox/codeagent/internal/config"
	"github.com/qbox/codeagent/pkg/models"
	"github.com/qiniu/x/log"
)

// claudeInteractive 交互式Claude Docker实现
type claudeInteractive struct {
	containerName string
	cmd           *exec.Cmd
	stdin         io.WriteCloser
	stdout        io.ReadCloser
	stderr        io.ReadCloser
	mutex         sync.Mutex
	session       *InteractiveSession
	closed        bool
}

// InteractiveSession 管理交互式会话
type InteractiveSession struct {
	ID            string
	CreatedAt     time.Time
	LastActivity  time.Time
	MessageCount  int
	WorkspacePath string
}

func NewClaudeInteractive(workspace *models.Workspace, cfg *config.Config) (Code, error) {
	// 解析仓库信息，只获取仓库名，不包含完整URL
	repoName := extractRepoName(workspace.Repository)
	// 新的容器命名规则：claude-interactive-组织-仓库-PR号
	containerName := fmt.Sprintf("claude-interactive-%s-%s-%d", workspace.Org, repoName, workspace.PRNumber)

	// 检查是否已经有对应的容器在运行
	if isContainerRunning(containerName) {
		log.Infof("Found existing interactive container: %s, reusing it", containerName)
		// 连接到现有容器
		return connectToExistingContainer(containerName, workspace)
	}

	// 确保路径存在
	workspacePath, _ := filepath.Abs(workspace.Path)

	// 确定claude配置路径
	var claudeConfigPath string
	if home := os.Getenv("HOME"); home != "" {
		claudeConfigPath, _ = filepath.Abs(filepath.Join(home, ".claude"))
	} else {
		claudeConfigPath = "/home/codeagent/.claude"
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

	// 构建 Docker 命令 - 使用交互式模式
	args := []string{
		"run",
		"-i",                    // 保持STDIN开放
		"--rm",                  // 容器停止后自动删除
		"--name", containerName, // 设置容器名称
		"-v", fmt.Sprintf("%s:/workspace", workspacePath), // 挂载工作空间
		"-v", fmt.Sprintf("%s:/home/codeagent/.claude", claudeConfigPath), // 挂载 claude 认证信息
		"-w", "/workspace", // 设置工作目录
		cfg.Claude.ContainerImage, // 使用配置的 Claude 镜像
		"claude",           // 启动claude CLI
	}

	// 打印调试信息
	log.Infof("Starting interactive Docker container: docker %s", strings.Join(args, " "))

	cmd := exec.Command("docker", args...)

	// 获取stdin、stdout、stderr管道
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		stdin.Close()
		stdout.Close()
		return nil, fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	// 启动容器
	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		stderr.Close()
		log.Errorf("Failed to start interactive Docker container: %v", err)
		return nil, fmt.Errorf("failed to start interactive Docker container: %w", err)
	}

	log.Infof("Interactive Docker container started successfully: %s", containerName)

	// 创建会话信息
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
		stderr:        stderr,
		session:       session,
		closed:        false,
	}

	// 等待Claude CLI初始化完成
	if err := claudeInteractive.waitForReady(); err != nil {
		claudeInteractive.Close()
		return nil, fmt.Errorf("claude CLI initialization failed: %w", err)
	}

	return claudeInteractive, nil
}

// waitForReady 等待Claude CLI准备就绪
func (c *claudeInteractive) waitForReady() error {
	// 发送一个简单的测试消息
	testMessage := "Hello, are you ready?"
	
	// 设置超时
	timeout := time.NewTimer(30 * time.Second)
	defer timeout.Stop()

	done := make(chan error, 1)
	
	go func() {
		// 发送测试消息
		if _, err := c.stdin.Write([]byte(testMessage + "\n")); err != nil {
			done <- fmt.Errorf("failed to send test message: %w", err)
			return
		}

		// 读取响应
		scanner := bufio.NewScanner(c.stdout)
		for scanner.Scan() {
			line := scanner.Text()
			log.Infof("Claude initialization response: %s", line)
			// 如果看到Claude的响应，说明已经准备就绪
			if strings.Contains(line, "Hello") || strings.Contains(line, "ready") || len(line) > 10 {
				done <- nil
				return
			}
		}
		
		if err := scanner.Err(); err != nil {
			done <- fmt.Errorf("error reading initialization response: %w", err)
		} else {
			done <- fmt.Errorf("no valid response received during initialization")
		}
	}()

	select {
	case err := <-done:
		if err != nil {
			return err
		}
		log.Infof("Claude CLI is ready for interactive mode")
		return nil
	case <-timeout.C:
		return fmt.Errorf("timeout waiting for Claude CLI to be ready")
	}
}

// connectToExistingContainer 连接到现有的交互式容器
func connectToExistingContainer(containerName string, workspace *models.Workspace) (Code, error) {
	// 通过docker exec连接到现有容器
	args := []string{
		"exec",
		"-i",
		containerName,
		"claude",
	}

	cmd := exec.Command("docker", args...)

	// 获取stdin、stdout、stderr管道
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdin pipe for existing container: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, fmt.Errorf("failed to get stdout pipe for existing container: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		stdin.Close()
		stdout.Close()
		return nil, fmt.Errorf("failed to get stderr pipe for existing container: %w", err)
	}

	// 启动exec命令
	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		stderr.Close()
		return nil, fmt.Errorf("failed to connect to existing container: %w", err)
	}

	// 创建会话信息
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
		stderr:        stderr,
		session:       session,
		closed:        false,
	}, nil
}

func (c *claudeInteractive) Prompt(message string) (*Response, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.closed {
		return nil, fmt.Errorf("interactive session is closed")
	}

	// 更新会话信息
	c.session.LastActivity = time.Now()
	c.session.MessageCount++

	log.Infof("Sending interactive message #%d to Claude: %s", c.session.MessageCount, truncateMessage(message, 100))

	// 发送消息到Claude CLI
	if _, err := c.stdin.Write([]byte(message + "\n")); err != nil {
		return nil, fmt.Errorf("failed to send message to Claude: %w", err)
	}

	// 创建响应读取器
	responseReader := &InteractiveResponseReader{
		stdout:  c.stdout,
		stderr:  c.stderr,
		session: c.session,
	}

	return &Response{Out: responseReader}, nil
}

// InteractiveResponseReader 处理交互式响应读取
type InteractiveResponseReader struct {
	stdout  io.ReadCloser
	stderr  io.ReadCloser
	session *InteractiveSession
	buffer  bytes.Buffer
	done    bool
}

func (r *InteractiveResponseReader) Read(p []byte) (n int, err error) {
	if r.done {
		return 0, io.EOF
	}

	// 从stdout读取数据
	buffer := make([]byte, 4096)
	n, err = r.stdout.Read(buffer)
	if err != nil {
		if err == io.EOF {
			r.done = true
		}
		return 0, err
	}

	// 将数据写入缓冲区和返回给调用者
	r.buffer.Write(buffer[:n])
	copy(p, buffer[:n])

	// 检查是否是响应结束标记（这里可以根据需要自定义）
	if r.isResponseComplete(buffer[:n]) {
		r.done = true
		return n, io.EOF
	}

	return n, nil
}

// isResponseComplete 检查响应是否完成
func (r *InteractiveResponseReader) isResponseComplete(data []byte) bool {
	// 简单的检查：如果看到特定的结束标记或者长时间没有新数据
	// 这里可以根据Claude Code的输出特征来优化
	responseText := string(data)
	
	// 检查常见的结束模式
	if strings.Contains(responseText, "\n\n") && 
	   (strings.Contains(responseText, "Complete") || 
		strings.Contains(responseText, "Done") ||
		len(responseText) > 1000) {
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

	// 关闭输入输出管道
	if c.stdin != nil {
		c.stdin.Close()
	}
	if c.stdout != nil {
		c.stdout.Close()
	}
	if c.stderr != nil {
		c.stderr.Close()
	}

	// 终止进程
	if c.cmd != nil && c.cmd.Process != nil {
		c.cmd.Process.Kill()
		c.cmd.Wait()
	}

	// 停止并删除容器
	stopCmd := exec.Command("docker", "rm", "-f", c.containerName)
	if err := stopCmd.Run(); err != nil {
		log.Warnf("Failed to remove container %s: %v", c.containerName, err)
	}

	return nil
}

// truncateMessage 截断长消息用于日志显示
func truncateMessage(message string, maxLen int) string {
	if len(message) <= maxLen {
		return message
	}
	return message[:maxLen] + "..."
}