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

	"github.com/qbox/codeagent/internal/config"
	"github.com/qbox/codeagent/pkg/models"
	"github.com/qiniu/x/log"
)

// geminiInteractive 交互式Gemini Docker实现
type geminiInteractive struct {
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

func NewGeminiInteractive(workspace *models.Workspace, cfg *config.Config) (Code, error) {
	// 解析仓库信息，只获取仓库名，不包含完整URL
	repoName := extractRepoName(workspace.Repository)
	// 新的容器命名规则：gemini-interactive-组织-仓库-PR号
	containerName := fmt.Sprintf("gemini-interactive-%s-%s-%d", workspace.Org, repoName, workspace.PRNumber)

	// 检查是否已经有对应的容器在运行
	if isContainerRunning(containerName) {
		log.Infof("Found existing interactive container: %s, reusing it", containerName)
		// 连接到现有容器
		return connectToExistingGeminiContainer(containerName, workspace)
	}

	// 确保路径存在
	workspacePath, _ := filepath.Abs(workspace.Path)
	sessionPath, _ := filepath.Abs(workspace.SessionPath)

	// 确定gemini配置路径
	var geminiConfigPath string
	if home := os.Getenv("HOME"); home != "" {
		geminiConfigPath, _ = filepath.Abs(filepath.Join(home, ".gemini"))
	} else {
		geminiConfigPath = "/home/codeagent/.gemini"
	}

	// 检查是否使用了/tmp目录（在macOS上可能导致挂载问题）
	if strings.HasPrefix(workspacePath, "/tmp/") {
		log.Warnf("Warning: Using /tmp directory may cause mount issues on macOS. Consider using other path instead.")
		log.Warnf("Current workspace path: %s", workspacePath)
	}

	if strings.HasPrefix(sessionPath, "/tmp/") {
		log.Warnf("Warning: Using /tmp directory may cause mount issues on macOS. Consider using other path instead.")
		log.Warnf("Current session path: %s", sessionPath)
	}

	// 检查路径是否存在
	if _, err := os.Stat(workspacePath); os.IsNotExist(err) {
		log.Errorf("Workspace path does not exist: %s", workspacePath)
		return nil, fmt.Errorf("workspace path does not exist: %s", workspacePath)
	}

	if _, err := os.Stat(sessionPath); os.IsNotExist(err) {
		log.Errorf("Session path does not exist: %s", sessionPath)
		return nil, fmt.Errorf("session path does not exist: %s", sessionPath)
	}

	// 构建 Docker 命令 - 使用交互式模式
	args := []string{
		"run",
		"-i",                    // 保持STDIN开放
		"--rm",                  // 容器停止后自动删除
		"--name", containerName, // 设置容器名称
		"--entrypoint", "gemini", // 直接使用gemini作为entrypoint
		"-v", fmt.Sprintf("%s:/workspace", workspacePath), // 挂载工作空间
		"-v", fmt.Sprintf("%s:/home/codeagent/.gemini", geminiConfigPath), // 挂载 gemini 认证信息
		"-v", fmt.Sprintf("%s:/home/codeagent/.gemini/tmp", sessionPath), // 挂载临时目录
		"-w", "/workspace", // 设置工作目录
		"-e", "TERM=xterm-256color", // 设置终端类型
	}

	// 添加 Gemini API 相关环境变量
	if cfg.Gemini.APIKey != "" {
		args = append(args, "-e", fmt.Sprintf("GEMINI_API_KEY=%s", cfg.Gemini.APIKey))
	}
	if cfg.Gemini.GoogleCloudProject != "" {
		args = append(args, "-e", fmt.Sprintf("GOOGLE_CLOUD_PROJECT=%s", cfg.Gemini.GoogleCloudProject))
	}

	// 添加容器镜像 - 启动交互式gemini会话
	args = append(args, cfg.Gemini.ContainerImage)

	// 打印调试信息
	log.Infof("Starting interactive Gemini Docker container: docker %s", strings.Join(args, " "))

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
		log.Errorf("Failed to start interactive Gemini Docker container: %v", err)
		return nil, fmt.Errorf("failed to start interactive Gemini Docker container: %w", err)
	}

	log.Infof("Interactive Gemini Docker container started successfully: %s", containerName)
	log.Infof("Container process ID: %d", cmd.Process.Pid)
	log.Infof("stdin pipe type: %T", stdin)
	log.Infof("stdout pipe type: %T", stdout)

	// 创建上下文用于取消操作
	ctx, cancel := context.WithCancel(context.Background())

	// 创建会话信息
	session := &InteractiveSession{
		ID:            fmt.Sprintf("gemini-session-%d", time.Now().Unix()),
		CreatedAt:     time.Now(),
		LastActivity:  time.Now(),
		MessageCount:  0,
		WorkspacePath: workspacePath,
	}

	geminiInteractive := &geminiInteractive{
		containerName: containerName,
		cmd:           cmd,
		stdin:         stdin,
		stdout:        stdout,
		session:       session,
		closed:        false,
		ctx:           ctx,
		cancel:        cancel,
	}

	// 等待Gemini CLI初始化完成
	if err := geminiInteractive.waitForReady(); err != nil {
		return nil, fmt.Errorf("gemini CLI initialization failed: %w", err)
	}

	return geminiInteractive, nil
}

func (g *geminiInteractive) waitForReady() error {
	// 等待Gemini CLI启动
	log.Infof("Waiting for Gemini CLI to initialize...")
	time.Sleep(5 * time.Second)

	// 简单地认为准备就绪，让第一个Prompt来真正测试
	log.Infof("Gemini CLI initialization completed (will test on first prompt)")
	return nil
}

// connectToExistingGeminiContainer 连接到现有的交互式容器
func connectToExistingGeminiContainer(containerName string, workspace *models.Workspace) (Code, error) {
	// 通过docker exec连接到现有容器
	args := []string{
		"exec",
		"-i",
		containerName,
		"gemini",
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
		ID:            fmt.Sprintf("gemini-reconnect-session-%d", time.Now().Unix()),
		CreatedAt:     time.Now(),
		LastActivity:  time.Now(),
		MessageCount:  0,
		WorkspacePath: workspace.Path,
	}

	return &geminiInteractive{
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

func (g *geminiInteractive) Prompt(message string) (*Response, error) {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	if g.closed {
		return nil, fmt.Errorf("interactive session is closed")
	}

	// 更新会话信息
	g.session.LastActivity = time.Now()
	g.session.MessageCount++

	log.Infof("Sending interactive message #%d to Gemini: %s", g.session.MessageCount, message)

	// 发送消息到Gemini CLI通过stdin
	messageBytes := []byte(message + "\n")
	log.Debugf("Writing %d bytes to stdin: %s", len(messageBytes), strings.TrimSpace(string(messageBytes)))

	if _, err := g.stdin.Write(messageBytes); err != nil {
		return nil, fmt.Errorf("failed to send message to Gemini: %w", err)
	}

	log.Debugf("Successfully wrote message to stdin")

	// 确保消息被发送
	if flusher, ok := g.stdin.(interface{ Flush() error }); ok {
		if err := flusher.Flush(); err != nil {
			log.Warnf("Failed to flush stdin: %v", err)
		} else {
			log.Debugf("Successfully flushed stdin")
		}
	} else {
		log.Debugf("stdin does not support Flush()")
	}

	// 给Gemini一些时间开始处理消息
	time.Sleep(500 * time.Millisecond)

	log.Debugf("Creating GeminiInteractiveResponseReader for message #%d", g.session.MessageCount)

	// 创建响应读取器
	responseReader := &GeminiInteractiveResponseReader{
		stdout:  g.stdout,
		session: g.session,
		ctx:     g.ctx,
	}

	return &Response{Out: responseReader}, nil
}

// GeminiInteractiveResponseReader 处理Gemini交互式响应读取
type GeminiInteractiveResponseReader struct {
	stdout  io.ReadCloser
	session *InteractiveSession
	buffer  bytes.Buffer
	done    bool
	ctx     context.Context
	mutex   sync.Mutex
}

func (r *GeminiInteractiveResponseReader) Read(p []byte) (n int, err error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.done {
		return 0, io.EOF
	}

	// 从stdout读取数据
	buffer := make([]byte, 4096)
	n, err = r.stdout.Read(buffer)

	log.Debugf("GeminiInteractiveResponseReader: Read %d bytes, error: %v, buffer size: %d", n, err, r.buffer.Len())

	if err != nil {
		if err == io.EOF {
			r.done = true
			log.Infof("GeminiInteractiveResponseReader: EOF reached, total buffer size: %d", r.buffer.Len())
		}
		return 0, err
	}

	// 如果没有读取到数据，返回0而不是错误
	if n == 0 {
		log.Debugf("GeminiInteractiveResponseReader: No data read, waiting...")
		return 0, nil
	}

	// 将数据写入缓冲区和返回给调用者
	r.buffer.Write(buffer[:n])
	copy(p, buffer[:n])

	// 简化响应完成检测 - 针对Gemini CLI的特点
	if r.isGeminiResponseComplete(buffer[:n]) {
		r.done = true
		log.Infof("GeminiInteractiveResponseReader: Response complete detected, total buffer size: %d", r.buffer.Len())
		return n, io.EOF
	}

	return n, nil
}

// isGeminiResponseComplete 检查Gemini响应是否完成
func (r *GeminiInteractiveResponseReader) isGeminiResponseComplete(data []byte) bool {
	responseText := string(data)
	bufferText := r.buffer.String()

	log.Debugf("GeminiInteractiveResponseReader: Checking response completion - responseText: %s, bufferText length: %d",
		strings.TrimSpace(responseText), len(bufferText))

	// 只有在缓冲区足够大时才检查完成标记
	if len(bufferText) < 500 {
		return false
	}

	// 检查Gemini CLI特有的结束标记
	if strings.Contains(responseText, "gemini>") || // Gemini CLI提示符
		strings.Contains(bufferText, "Complete") ||
		strings.Contains(bufferText, "Done") ||
		strings.Contains(bufferText, "Error:") ||
		strings.Contains(bufferText, "Finished") ||
		// 检查是否有明显的任务完成标记
		(strings.Contains(bufferText, "## 改动摘要") && strings.Contains(bufferText, "## 具体改动")) {
		log.Debugf("GeminiInteractiveResponseReader: Response complete detected - found completion marker")
		log.Debugf("GeminiInteractiveResponseReader: Buffer content (last 200 chars): %s",
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

func (g *geminiInteractive) Close() error {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	if g.closed {
		return nil
	}

	g.closed = true

	log.Infof("Closing interactive Gemini session %s (messages: %d)", g.session.ID, g.session.MessageCount)

	// 取消上下文
	if g.cancel != nil {
		g.cancel()
	}

	// 关闭管道
	if g.stdin != nil {
		g.stdin.Close()
	}
	if g.stdout != nil {
		g.stdout.Close()
	}

	// 终止进程
	if g.cmd != nil && g.cmd.Process != nil {
		g.cmd.Process.Kill()
		g.cmd.Wait()
	}

	// 检查容器状态，但不删除容器（便于调试）
	log.Infof("Checking container status for debugging: %s", g.containerName)

	// 检查容器是否还在运行
	checkCmd := exec.Command("docker", "ps", "--filter", fmt.Sprintf("name=%s", g.containerName), "--format", "{{.Names}}")
	output, err := checkCmd.Output()
	if err != nil {
		log.Warnf("Failed to check container status: %v", err)
	} else {
		containerStatus := strings.TrimSpace(string(output))
		if containerStatus == g.containerName {
			log.Infof("Container %s is still running - keeping it for debugging", g.containerName)
			log.Infof("You can manually inspect it with: docker logs %s", g.containerName)
			log.Infof("You can manually test it with: echo '/help' | docker exec -i %s gemini", g.containerName)
		} else {
			log.Infof("Container %s is not running (status: %s)", g.containerName, containerStatus)
		}
	}

	// 注意：不删除容器，便于调试问题
	log.Infof("Container %s left running for debugging purposes", g.containerName)

	return nil
}
