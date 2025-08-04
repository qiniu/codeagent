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

// claudeInteractive 交互式Claude Docker实现
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
	workspacePath, err := filepath.Abs(workspace.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute workspace path: %w", err)
	}

	// 确定claude配置路径
	var claudeConfigPath string
	if home := os.Getenv("HOME"); home != "" {
		var err error
		claudeConfigPath, err = filepath.Abs(filepath.Join(home, ".claude"))
		if err != nil {
			return nil, fmt.Errorf("failed to get absolute claude config path: %w", err)
		}
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

	// 构建 Docker 命令 - 使用简单的管道模式而不是 PTY
	args := []string{
		"run",
		"-i",                    // 保持STDIN开放
		"--rm",                  // 容器停止后自动删除
		"--name", containerName, // 设置容器名称
		"--entrypoint", "claude", // 直接使用claude作为entrypoint
		"-v", fmt.Sprintf("%s:/workspace", workspacePath), // 挂载工作空间
		"-v", fmt.Sprintf("%s:/home/codeagent/.claude", claudeConfigPath), // 挂载 claude 认证信息
		"-w", "/workspace", // 设置工作目录
		"-e", "TERM=xterm-256color", // 设置终端类型
	}

	// 添加 Claude API 相关环境变量
	if cfg.Claude.AuthToken != "" {
		args = append(args, "-e", fmt.Sprintf("ANTHROPIC_AUTH_TOKEN=%s", cfg.Claude.AuthToken))
	} else if cfg.Claude.APIKey != "" {
		args = append(args, "-e", fmt.Sprintf("ANTHROPIC_API_KEY=%s", cfg.Claude.APIKey))
	}
	if cfg.Claude.BaseURL != "" {
		args = append(args, "-e", fmt.Sprintf("ANTHROPIC_BASE_URL=%s", cfg.Claude.BaseURL))
	}

	// 添加容器镜像 - 不需要额外命令，因为使用了--entrypoint
	args = append(args, cfg.Claude.ContainerImage)

	// 打印调试信息
	log.Infof("Starting interactive Docker container: docker %s", strings.Join(args, " "))

	cmd := exec.Command("docker", args...)

	// 创建管道
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	cmd.Stderr = cmd.Stdout // 将 stderr 重定向到 stdout
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true, // 创建新的进程组
	}

	// 启动容器
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

	// 创建上下文用于取消操作
	ctx, cancel := context.WithCancel(context.Background())

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
		session:       session,
		closed:        false,
		ctx:           ctx,
		cancel:        cancel,
	}

	// 等待Claude CLI初始化完成
	if err := claudeInteractive.waitForReady(); err != nil {
		return nil, fmt.Errorf("claude CLI initialization failed: %w", err)
	}

	return claudeInteractive, nil
}

func (c *claudeInteractive) waitForReady() error {
	// 等待Claude CLI启动
	log.Infof("Waiting for Claude CLI to initialize...")
	time.Sleep(5 * time.Second)

	// 简单地认为准备就绪，让第一个Prompt来真正测试
	log.Infof("Claude CLI initialization completed (will test on first prompt)")
	return nil
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

	// 创建管道
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

	// 启动exec命令
	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		return nil, fmt.Errorf("failed to connect to existing container: %w", err)
	}

	// 创建上下文用于取消操作
	ctx, cancel := context.WithCancel(context.Background())

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

	// 更新会话信息
	c.session.LastActivity = time.Now()
	c.session.MessageCount++

	log.Infof("Sending interactive message #%d to Claude: %s", c.session.MessageCount, message)

	// 发送消息到Claude CLI通过stdin
	messageBytes := []byte(message + "\n")
	log.Debugf("Writing %d bytes to stdin: %s", len(messageBytes), strings.TrimSpace(string(messageBytes)))

	if _, err := c.stdin.Write(messageBytes); err != nil {
		return nil, fmt.Errorf("failed to send message to Claude: %w", err)
	}

	log.Debugf("Successfully wrote message to stdin")

	// 确保消息被发送
	if flusher, ok := c.stdin.(interface{ Flush() error }); ok {
		if err := flusher.Flush(); err != nil {
			log.Warnf("Failed to flush stdin: %v", err)
		} else {
			log.Debugf("Successfully flushed stdin")
		}
	} else {
		log.Debugf("stdin does not support Flush()")
	}

	// 给Claude一些时间开始处理消息
	time.Sleep(500 * time.Millisecond)

	log.Debugf("Creating InteractiveResponseReader for message #%d", c.session.MessageCount)

	// 创建响应读取器
	responseReader := &InteractiveResponseReader{
		stdout:  c.stdout,
		session: c.session,
		ctx:     c.ctx,
	}

	return &Response{Out: responseReader}, nil
}

// InteractiveResponseReader 处理交互式响应读取
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

	// 从stdout读取数据
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

	// 如果没有读取到数据，返回0而不是错误
	if n == 0 {
		log.Debugf("InteractiveResponseReader: No data read, waiting...")
		return 0, nil
	}

	// 将数据写入缓冲区和返回给调用者
	r.buffer.Write(buffer[:n])
	copy(p, buffer[:n])

	// 简化响应完成检测 - 只在非常明确的情况下才结束
	if r.isResponseComplete(buffer[:n]) {
		r.done = true
		log.Infof("InteractiveResponseReader: Response complete detected, total buffer size: %d", r.buffer.Len())
		return n, io.EOF
	}

	return n, nil
}

// isResponseComplete 检查响应是否完成 - 简化版本
func (r *InteractiveResponseReader) isResponseComplete(data []byte) bool {
	responseText := string(data)
	bufferText := r.buffer.String()

	log.Debugf("InteractiveResponseReader: Checking response completion - responseText: %s, bufferText length: %d",
		strings.TrimSpace(responseText), len(bufferText))

	// 只有在缓冲区足够大时才检查完成标记
	if len(bufferText) < 500 {
		return false
	}

	// 只检查非常明确的结束标记
	if strings.Contains(responseText, "claude>") || // Claude CLI提示符
		strings.Contains(bufferText, "Complete") ||
		strings.Contains(bufferText, "Done") ||
		strings.Contains(bufferText, "Error:") ||
		strings.Contains(bufferText, "Finished") ||
		// 检查是否有明显的任务完成标记
		(strings.Contains(bufferText, "## 改动摘要") && strings.Contains(bufferText, "## 具体改动")) {
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

	// 取消上下文
	if c.cancel != nil {
		c.cancel()
	}

	// 关闭管道
	if c.stdin != nil {
		c.stdin.Close()
	}
	if c.stdout != nil {
		c.stdout.Close()
	}

	// 终止进程
	if c.cmd != nil && c.cmd.Process != nil {
		c.cmd.Process.Kill()
		c.cmd.Wait()
	}

	// 检查容器状态，但不删除容器（便于调试）
	log.Infof("Checking container status for debugging: %s", c.containerName)

	// 检查容器是否还在运行
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

	// 注意：不删除容器，便于调试问题
	log.Infof("Container %s left running for debugging purposes", c.containerName)

	return nil
}
