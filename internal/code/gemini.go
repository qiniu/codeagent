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
	"golang.org/x/term"
)

type geminiCode struct {
	cmd    *exec.Cmd
	term   *term.State // 用于保存终端的原始状态
	ptmx   *os.File
	pipr   *os.File // 用于读取输入
	pipw   *os.File // 用于写入输入
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

	cmd.Env = append(cmd.Env, "GOOGLE_CLOUD_PROJECT=sb2xbp")
	cmd.Dir = workspace.Path
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to start Gemini container: %w", err)
	}

	pipr, pipw, err := os.Pipe()
	if err != nil {
		ptmx.Close()
		return nil, fmt.Errorf("failed to create pipe: %w", err)
	}
	oldState, err := term.MakeRaw(int(pipr.Fd()))

	code := &geminiCode{
		cmd:  cmd,
		pipr: pipr,
		pipw: pipw,
		term: oldState,
		ptmx: ptmx,
	}

	go code.run()
	code.Wait()

	return code, nil
}

func (g *geminiCode) Prompt(message string) (*Response, error) {
	watch := g.buf.Watch(message)
	if _, err := g.pipw.Write([]byte(message + "\n")); err != nil {
		return nil, err
	}
	return &Response{Out: watch}, nil
}

func (g *geminiCode) Close() error {
	if err := g.ptmx.Close(); err != nil {
		return err
	}
	term.Restore(int(g.pipr.Fd()), g.term)
	return g.cmd.Wait()
}

func (g *geminiCode) run() {
	io.Copy(g.buf, g.ptmx)
}

func (g *geminiCode) Wait() {
	for {
		if g.buf.Enterable() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// type tempBuffer struct {
// }

// func (t *tempBuffer) Write(p []byte) (int, error) {
// 	return
// }

// func (t *tempBuffer) Watch(key string) io.Reader {
// 	return nil
// }

// func (t *tempBuffer) Enterable() bool {
// 	return false
// }
