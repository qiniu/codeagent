package code

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync/atomic"
	"time"

	"github.com/qbox/codeagent/internal/config"
	"github.com/qbox/codeagent/pkg/models"

	"github.com/creack/pty"
)

type geminiCode struct {
	cmd    *exec.Cmd
	ptmx   *os.File
	buf    *tempBuffer
	inited atomic.Bool
}

func NewGemini(workspace *models.Workspace, cfg *config.Config) (Code, error) {
	// 构建 Docker 命令
	// _, repo := parseRepoURL(workspace.Repository)
	// args := []string{
	// 	"run",
	// 	"--rm", // 容器运行完后自动删除
	// 	"-it",
	// 	"-e", "GOOGLE_CLOUD_PROJECT=sb2xbp", // 设置 Google Cloud 项目环境变量
	// 	// "-e", "GEMINI_API_KEY=" + cfg.Gemini.APIKey,
	// 	"-v", fmt.Sprintf("%s:/workspace", workspace.Path), // 挂载工作空间
	// 	"-v", fmt.Sprintf("%s:%s", filepath.Join(os.Getenv("HOME"), ".gemini"), "/root/.gemini"), // 挂载 gemini 认证信息
	// 	"-w", "/workspace", // 设置工作目录
	// 	cfg.Gemini.ContainerImage, // 使用配置的 Claude 镜像
	// 	"gemini",                  // 容器内执行的命令
	// }

	// cmd := exec.Command("docker", args...)
	cmd := exec.Command("/usr/local/bin/node", "/usr/local/bin/gemini")

	cmd.Env = append(cmd.Env, "GOOGLE_CLOUD_PROJECT=codeagent")
	cmd.Dir = workspace.Path
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to start Gemini container: %w", err)
	}

	code := &geminiCode{
		cmd:  cmd,
		ptmx: ptmx,
		buf:  newTempBuffer(),
	}

	go code.run()
	code.Wait()

	return code, nil
}

func (g *geminiCode) Prompt(message string) (*Response, error) {
	for _, char := range message {
		g.ptmx.WriteString(string(char))
		time.Sleep(10 * time.Millisecond) // 模拟打字速度，直接 append CRLF gemini 不会提交
	}

	g.ptmx.Write([]byte{13}) // CR
	g.ptmx.Write([]byte{10}) // LF
	watch := g.buf.Watch(message)
	return &Response{Out: watch}, nil
}

func (g *geminiCode) Close() error {
	if err := g.ptmx.Close(); err != nil {
		return err
	}
	return g.cmd.Wait()
}

func (g *geminiCode) run() {
	go io.Copy(g.buf, g.ptmx)
}

func (g *geminiCode) Wait() {
	for {
		if g.buf.Enterable() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}
