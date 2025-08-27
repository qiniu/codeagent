package models

import (
	"time"
	"github.com/google/go-github/v58/github"
)

// MCP协议相关数据模型

// GitHubContextWrapper 包装GitHubContext以实现统一接口
type GitHubContextWrapper struct {
	repo *github.Repository
}

// NewGitHubContextWrapper 创建GitHubContext包装器
func NewGitHubContextWrapper(owner, name string) GitHubContext {
	return &GitHubContextWrapper{
		repo: &github.Repository{
			Owner: &github.User{Login: &owner},
			Name:  &name,
		},
	}
}

func (g *GitHubContextWrapper) GetEventType() EventType {
	return EventPullRequest
}

func (g *GitHubContextWrapper) GetRepository() *github.Repository {
	return g.repo
}

func (g *GitHubContextWrapper) GetSender() *github.User {
	return nil
}

func (g *GitHubContextWrapper) GetRawEvent() interface{} {
	return nil
}

func (g *GitHubContextWrapper) GetEventAction() string {
	return "mcp"
}

func (g *GitHubContextWrapper) GetDeliveryID() string {
	return "mcp-server"
}

func (g *GitHubContextWrapper) GetTimestamp() time.Time {
	return time.Now()
}