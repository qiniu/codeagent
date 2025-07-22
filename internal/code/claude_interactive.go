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
	"unsafe"

	"github.com/qbox/codeagent/internal/config"
	"github.com/qbox/codeagent/pkg/models"
	"github.com/qiniu/x/log"
)

// claudeInteractive 交互式Claude Docker实现
type claudeInteractive struct {
	containerName string
	cmd           *exec.Cmd
	ptyMaster     *os.File
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

	// 构建 Docker 命令 - 使用交互式PTY模式
	args := []string{
		"run",
		"-it",                   // 保持STDIN开放并分配伪终端
		"--rm",                  // 容器停止后自动删除
		"--name", containerName, // 设置容器名称
		"-v", fmt.Sprintf("%s:/workspace", workspacePath), // 挂载工作空间
		"-v", fmt.Sprintf("%s:/home/codeagent/.claude", claudeConfigPath), // 挂载 claude 认证信息
		"-w", "/workspace", // 设置工作目录
		"-e", "TERM=xterm-256color", // 设置终端类型
		cfg.Claude.ContainerImage, // 使用配置的 Claude 镜像
		"claude",                  // 启动claude CLI
	}

	// 打印调试信息
	log.Infof("Starting interactive Docker container: docker %s", strings.Join(args, " "))

	// 创建PTY主从设备
	ptyMaster, ptySlave, err := createPTY()
	if err != nil {
		return nil, fmt.Errorf("failed to create PTY: %w", err)
	}

	cmd := exec.Command("docker", args...)
	cmd.Stdin = ptySlave
	cmd.Stdout = ptySlave
	cmd.Stderr = ptySlave
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true, // 创建新的进程组
	}

	// 启动容器
	if err := cmd.Start(); err != nil {
		ptyMaster.Close()
		ptySlave.Close()
		log.Errorf("Failed to start interactive Docker container: %v", err)
		return nil, fmt.Errorf("failed to start interactive Docker container: %w", err)
	}

	// 关闭从设备，只保留主设备
	ptySlave.Close()

	log.Infof("Interactive Docker container started successfully: %s", containerName)

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
		ptyMaster:     ptyMaster,
		session:       session,
		closed:        false,
		ctx:           ctx,
		cancel:        cancel,
	}

	// 等待Claude CLI初始化完成
	if err := claudeInteractive.waitForReady(); err != nil {
		claudeInteractive.Close()
		return nil, fmt.Errorf("claude CLI initialization failed: %w", err)
	}

	return claudeInteractive, nil
}

// createPTY 创建PTY主从设备对
func createPTY() (master, slave *os.File, err error) {
	// 打开PTY主设备
	master, err = os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open PTY master: %w", err)
	}

	// 解锁PTY
	if err := grantpt(master); err != nil {
		master.Close()
		return nil, nil, fmt.Errorf("failed to grantpt: %w", err)
	}

	// 解锁PTY
	if err := unlockpt(master); err != nil {
		master.Close()
		return nil, nil, fmt.Errorf("failed to unlockpt: %w", err)
	}

	// 获取从设备名称
	slaveName, err := ptsname(master)
	if err != nil {
		master.Close()
		return nil, nil, fmt.Errorf("failed to get slave name: %w", err)
	}

	// 打开PTY从设备
	slave, err = os.OpenFile(slaveName, os.O_RDWR, 0)
	if err != nil {
		master.Close()
		return nil, nil, fmt.Errorf("failed to open PTY slave: %w", err)
	}

	return master, slave, nil
}

// PTY相关的系统调用常数
const (
	TIOCSPTLCK = 0x40045431
	TIOCGPTN   = 0x80045430
)

func grantpt(f *os.File) error {
	zero := 0
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), TIOCSPTLCK, uintptr(unsafe.Pointer(&zero)))
	if errno != 0 {
		return errno
	}
	return nil
}

func unlockpt(f *os.File) error {
	zero := 0
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), TIOCSPTLCK, uintptr(unsafe.Pointer(&zero)))
	if errno != 0 {
		return errno
	}
	return nil
}

func ptsname(f *os.File) (string, error) {
	var n int
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), TIOCGPTN, uintptr(unsafe.Pointer(&n)))
	if errno != 0 {
		return "", errno
	}
	return fmt.Sprintf("/dev/pts/%d", n), nil
}

func (c *claudeInteractive) waitForReady() error {
	// 等待Claude CLI启动
	time.Sleep(2 * time.Second)

	// 发送一个简单的测试消息来检测是否准备就绪
	testMessage := "Hello\n"

	// 设置超时
	ctx, cancel := context.WithTimeout(c.ctx, 30*time.Second)
	defer cancel()

	done := make(chan error, 1)

	go func() {
		// 发送测试消息
		if _, err := c.ptyMaster.Write([]byte(testMessage)); err != nil {
			done <- fmt.Errorf("failed to send test message: %w", err)
			return
		}

		// 读取响应 - 使用较短的超时
		buffer := make([]byte, 4096)
		c.ptyMaster.SetReadDeadline(time.Now().Add(5 * time.Second))
		n, err := c.ptyMaster.Read(buffer)
		if err != nil {
			if !strings.Contains(err.Error(), "timeout") {
				done <- fmt.Errorf("error reading initialization response: %w", err)
				return
			}
		}

		if n > 0 {
			response := string(buffer[:n])
			log.Infof("Claude initialization response: %s", strings.TrimSpace(response))
		}

		// PTY连接成功就认为准备就绪
		done <- nil
	}()

	select {
	case err := <-done:
		if err != nil {
			return err
		}
		log.Infof("Claude CLI is ready for interactive PTY mode")
		return nil
	case <-ctx.Done():
		return fmt.Errorf("timeout waiting for Claude CLI to be ready")
	}
}

// connectToExistingContainer 连接到现有的交互式容器
func connectToExistingContainer(containerName string, workspace *models.Workspace) (Code, error) {
	// 通过docker exec连接到现有容器，使用PTY模式
	args := []string{
		"exec",
		"-it",
		containerName,
		"claude",
	}

	// 创建PTY主从设备对
	ptyMaster, ptySlave, err := createPTY()
	if err != nil {
		return nil, fmt.Errorf("failed to create PTY for existing container: %w", err)
	}

	cmd := exec.Command("docker", args...)
	cmd.Stdin = ptySlave
	cmd.Stdout = ptySlave
	cmd.Stderr = ptySlave
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	// 启动exec命令
	if err := cmd.Start(); err != nil {
		ptyMaster.Close()
		ptySlave.Close()
		return nil, fmt.Errorf("failed to connect to existing container: %w", err)
	}

	// 关闭从设备，只保留主设备
	ptySlave.Close()

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
		ptyMaster:     ptyMaster,
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

	log.Infof("Sending interactive message #%d to Claude: %s", c.session.MessageCount, truncateMessage(message, 100))

	// 发送消息到Claude CLI通过PTY
	if _, err := c.ptyMaster.Write([]byte(message + "\n")); err != nil {
		return nil, fmt.Errorf("failed to send message to Claude: %w", err)
	}

	// 创建响应读取器
	responseReader := &InteractiveResponseReader{
		ptyMaster: c.ptyMaster,
		session:   c.session,
		ctx:       c.ctx,
	}

	return &Response{Out: responseReader}, nil
}

// InteractiveResponseReader 处理交互式响应读取
type InteractiveResponseReader struct {
	ptyMaster *os.File
	session   *InteractiveSession
	buffer    bytes.Buffer
	done      bool
	ctx       context.Context
	mutex     sync.Mutex
}

func (r *InteractiveResponseReader) Read(p []byte) (n int, err error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.done {
		return 0, io.EOF
	}

	// 设置读取超时
	r.ptyMaster.SetReadDeadline(time.Now().Add(10 * time.Second))

	// 从PTY主设备读取数据
	buffer := make([]byte, 4096)
	n, err = r.ptyMaster.Read(buffer)
	if err != nil {
		if strings.Contains(err.Error(), "timeout") {
			// 超时可能表示响应完成
			r.done = true
			return 0, io.EOF
		}
		if err == io.EOF {
			r.done = true
		}
		return 0, err
	}

	// 将数据写入缓冲区和返回给调用者
	r.buffer.Write(buffer[:n])
	copy(p, buffer[:n])

	// 检查是否是响应结束标记
	if r.isResponseComplete(buffer[:n]) {
		r.done = true
		return n, io.EOF
	}

	return n, nil
}

// isResponseComplete 检查响应是否完成
func (r *InteractiveResponseReader) isResponseComplete(data []byte) bool {
	// Claude Code的交互式会话通常在完成一个任务后会返回提示符
	// 这里检查常见的结束模式
	responseText := string(data)
	bufferText := r.buffer.String()

	// 检查Claude Code特有的结束模式
	if strings.Contains(responseText, "$ ") || // 命令提示符
		strings.Contains(responseText, "> ") || // 输入提示符
		strings.Contains(bufferText, "Complete") ||
		strings.Contains(bufferText, "Done") ||
		strings.Contains(bufferText, "Error:") ||
		(len(bufferText) > 2000 && strings.Contains(responseText, "\n")) { // 长响应且有换行
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

	// 关闭PTY主设备
	if c.ptyMaster != nil {
		c.ptyMaster.Close()
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

