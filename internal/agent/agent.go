package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/qbox/codeagent/internal/claude"
	"github.com/qbox/codeagent/internal/config"
	ghclient "github.com/qbox/codeagent/internal/github"
	"github.com/qbox/codeagent/internal/workspace"
	"github.com/qbox/codeagent/pkg/models"

	"github.com/google/go-github/v58/github"
	"github.com/qiniu/x/log"
)

type Agent struct {
	config    *config.Config
	github    *ghclient.Client
	workspace *workspace.Manager
	claude    *claude.Executor
}

func New(cfg *config.Config, workspaceManager *workspace.Manager) *Agent {
	// 初始化 GitHub 客户端
	githubClient, err := ghclient.NewClient(cfg)
	if err != nil {
		log.Errorf("Failed to create GitHub client: %v", err)
		return nil
	}

	// 初始化 Claude 执行器
	claudeExecutor := claude.NewExecutor(cfg)

	return &Agent{
		config:    cfg,
		github:    githubClient,
		workspace: workspaceManager,
		claude:    claudeExecutor,
	}
}

// mockFileModification 模拟文件修改，用于测试二次提交流程
func (a *Agent) mockFileModification(workspace *models.Workspace) error {
	log.Infof("Mocking file modification for testing...")

	// 创建一个模拟的代码文件
	codeFile := filepath.Join(workspace.Path, "main.go")
	content := fmt.Sprintf(`package main

import "fmt"

// 这是由 XGo Agent 模拟生成的代码
// Issue #%d: %s
// 生成时间: %s

func main() {
	fmt.Println("Hello from XGo Agent!")
	fmt.Println("This is a mock implementation for testing.")
}

// 模拟的功能实现
func processData() {
	fmt.Println("Processing data...")
}

func validateInput() bool {
	return true
}
`, workspace.Issue.GetNumber(), workspace.Issue.GetTitle(), time.Now().Format("2006-01-02 15:04:05"))

	if err := os.WriteFile(codeFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to create mock file: %w", err)
	}

	// 创建一个 README 文件
	readmeFile := filepath.Join(workspace.Path, "README.md")
	readmeContent := fmt.Sprintf(`# 实现 Issue #%d

## 问题描述
%s

## 实现方案
这是一个由 XGo Agent 自动生成的实现方案。

### 功能特性
- 基础功能实现
- 错误处理
- 测试用例

### 使用方法
`+"`"+`bash
go run main.go
`+"`"+`

---
*此文件由 XGo Agent 自动生成，用于测试二次提交流程*
`, workspace.Issue.GetNumber(), workspace.Issue.GetBody())

	if err := os.WriteFile(readmeFile, []byte(readmeContent), 0644); err != nil {
		return fmt.Errorf("failed to create README file: %w", err)
	}

	log.Infof("Mock files created: %s, %s", codeFile, readmeFile)
	return nil
}

// ProcessIssue 处理 Issue 事件，生成代码（保留向后兼容）
func (a *Agent) ProcessIssue(issue *github.Issue) error {
	// 1. 准备临时工作空间
	ws := a.workspace.Prepare(issue)
	if ws.ID == "" {
		return fmt.Errorf("failed to prepare workspace")
	}

	// 确保处理完成后清理工作空间
	defer func() {
		a.workspace.Cleanup(ws)
		log.Infof("Cleaned up workspace: %s", ws.ID)
	}()

	// 2. 创建分支并推送
	if err := a.github.CreateBranch(&ws); err != nil {
		log.Errorf("Failed to create branch: %v", err)
		return err
	}

	// 3. 创建初始 PR
	pr, err := a.github.CreatePullRequest(&ws)
	if err != nil {
		log.Errorf("Failed to create PR: %v", err)
		return err
	}

	// 4. 执行 Claude Code
	result := a.claude.Execute(&ws, issue)

	// 4.5. Mock 文件修改（用于测试二次提交）
	if err := a.mockFileModification(&ws); err != nil {
		log.Errorf("Failed to mock file modification: %v", err)
		return err
	}

	// 5. 提交变更并更新 PR
	if err := a.github.CommitAndPush(&ws, result); err != nil {
		log.Errorf("Failed to commit and push: %v", err)
		return err
	}

	log.Infof("Successfully processed Issue #%d, PR: %s", issue.GetNumber(), pr.GetHTMLURL())
	return nil
}

// ProcessIssueComment 处理 Issue 评论事件，包含完整的仓库信息
func (a *Agent) ProcessIssueComment(event *github.IssueCommentEvent) error {
	// 1. 准备临时工作空间，传递完整事件
	ws := a.workspace.PrepareFromEvent(event)
	if ws.ID == "" {
		return fmt.Errorf("failed to prepare workspace")
	}

	// 确保处理完成后清理工作空间
	defer func() {
		a.workspace.Cleanup(ws)
		log.Infof("Cleaned up workspace: %s", ws.ID)
	}()

	// 2. 创建分支并推送
	if err := a.github.CreateBranch(&ws); err != nil {
		log.Errorf("Failed to create branch: %v", err)
		return err
	}

	// 3. 创建初始 PR
	pr, err := a.github.CreatePullRequest(&ws)
	if err != nil {
		log.Errorf("Failed to create PR: %v", err)
		return err
	}

	// 4. 执行 Claude Code
	result := a.claude.Execute(&ws, event.Issue)

	// 4.5. Mock 文件修改（用于测试二次提交）
	if err := a.mockFileModification(&ws); err != nil {
		log.Errorf("Failed to mock file modification: %v", err)
		return err
	}

	// 5. 提交变更并更新 PR
	if err := a.github.CommitAndPush(&ws, result); err != nil {
		log.Errorf("Failed to commit and push: %v", err)
		return err
	}

	log.Infof("Successfully processed Issue #%d, PR: %s", event.Issue.GetNumber(), pr.GetHTMLURL())
	return nil
}

// ContinuePR 继续处理 PR 中的任务
func (a *Agent) ContinuePR(pr *github.PullRequest) error {
	// TODO: 实现继续处理 PR 的逻辑
	return nil
}

// FixPR 修复 PR 中的问题
func (a *Agent) FixPR(pr *github.PullRequest) error {
	// TODO: 实现修复 PR 的逻辑
	return nil
}

// ReviewPR 审查 PR
func (a *Agent) ReviewPR(pr *github.PullRequest) error {
	// TODO: 实现 PR 审查逻辑
	return nil
}
