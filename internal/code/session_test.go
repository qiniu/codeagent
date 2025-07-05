package code

import (
	"io"
	"testing"

	"github.com/google/go-github/v58/github"
	"github.com/qbox/codeagent/internal/config"
	"github.com/qbox/codeagent/pkg/models"
)

func TestSession(t *testing.T) {
	sessionManager := NewSessionManager(&config.Config{
		CodeProvider: ProviderGemini,
		Gemini: config.GeminiConfig{
			ContainerImage: "gemini-cli",
			APIKey:         "AIzaSyCPviVEGCkiac1NhJHvfST9fSi-KBaoaw0",
		},
	})
	workspace := &models.Workspace{
		PullRequest: &github.PullRequest{
			Number: github.Int(1),
		},
		Repository: "https://github.com/qbox/codeagent.git",
		Path:       "/Users/wuxinyi/tmp/codeagent/issue-1347-1751618199105220000",
	}

	// 获取或创建会话
	session, err := sessionManager.GetSession(workspace)
	if err != nil {
		t.Fatalf("Failed to get session: %v", err)
	}

	if session == nil {
		t.Fatal("Expected a valid session, got nil")
	}

	res, err := session.Prompt("当我们通过 Prompt 修改计划时，应该使用 Issue 的内容作为 Prompt 的基础，而不是使用 event.Issue.GetURL()")
	if err != nil {
		t.Fatalf("Failed to prompt session: %v", err)
	}

	out, err := io.ReadAll(res.Out)
	println("Response:", string(out))

	// 关闭会话
	// if err := sessionManager.CloseSession(workspace); err != nil {
	// 	t.Fatalf("Failed to close session: %v", err)
	// }
}
