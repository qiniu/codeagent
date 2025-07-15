package main

import (
	"fmt"
	"log"

	"github.com/google/go-github/v58/github"
	"github.com/qbox/codeagent/internal/config"
	"github.com/qbox/codeagent/internal/workspace"
	"github.com/qbox/codeagent/pkg/models"
)

func main() {
	// 测试创建一个模拟的 fork PR
	cfg := &config.Config{
		Workspace: config.WorkspaceConfig{
			BaseDir: "/tmp/test_workspace",
		},
	}

	manager := workspace.NewManager(cfg)

	// 创建一个模拟的 fork PR
	pr := &github.PullRequest{
		Number: github.Int(131),
		Title:  github.String("Support fork PR collaboration"),
		Head: &github.PullRequestBranch{
			Ref: github.String("main0715"),
			Repo: &github.Repository{
				Owner: &github.User{
					Login: github.String("wwcchh0123"),
				},
				Name:     github.String("codeagent"),
				CloneURL: github.String("https://github.com/wwcchh0123/codeagent.git"),
			},
		},
		Base: &github.PullRequestBranch{
			Ref: github.String("main"),
			Repo: &github.Repository{
				Owner: &github.User{
					Login: github.String("qbox"),
				},
				Name:     github.String("codeagent"),
				CloneURL: github.String("https://github.com/qbox/codeagent.git"),
			},
		},
	}

	// 测试创建工作空间
	ws := manager.CreateWorkspaceFromPR(pr)
	if ws == nil {
		log.Fatal("Failed to create workspace")
	}

	// 验证 fork 信息是否正确设置
	if ws.ForkOwner != "wwcchh0123" {
		log.Fatalf("Expected fork owner to be wwcchh0123, got %s", ws.ForkOwner)
	}

	if ws.ForkRepo != "codeagent" {
		log.Fatalf("Expected fork repo to be codeagent, got %s", ws.ForkRepo)
	}

	if ws.ForkURL != "https://github.com/wwcchh0123/codeagent.git" {
		log.Fatalf("Expected fork URL to be https://github.com/wwcchh0123/codeagent.git, got %s", ws.ForkURL)
	}

	fmt.Printf("✓ Fork PR workspace created successfully:\n")
	fmt.Printf("  - Workspace Path: %s\n", ws.Path)
	fmt.Printf("  - Fork Owner: %s\n", ws.ForkOwner)
	fmt.Printf("  - Fork Repo: %s\n", ws.ForkRepo)
	fmt.Printf("  - Fork URL: %s\n", ws.ForkURL)
	fmt.Printf("  - Branch: %s\n", ws.Branch)

	// 测试非 fork PR
	normalPR := &github.PullRequest{
		Number: github.Int(132),
		Title:  github.String("Normal PR"),
		Head: &github.PullRequestBranch{
			Ref: github.String("feature-branch"),
			Repo: &github.Repository{
				Owner: &github.User{
					Login: github.String("qbox"),
				},
				Name:     github.String("codeagent"),
				CloneURL: github.String("https://github.com/qbox/codeagent.git"),
			},
		},
		Base: &github.PullRequestBranch{
			Ref: github.String("main"),
			Repo: &github.Repository{
				Owner: &github.User{
					Login: github.String("qbox"),
				},
				Name:     github.String("codeagent"),
				CloneURL: github.String("https://github.com/qbox/codeagent.git"),
			},
		},
	}

	normalWs := manager.CreateWorkspaceFromPR(normalPR)
	if normalWs == nil {
		log.Fatal("Failed to create normal workspace")
	}

	// 验证非 fork PR 的信息
	if normalWs.ForkOwner != "" {
		log.Fatalf("Expected normal PR to have empty fork owner, got %s", normalWs.ForkOwner)
	}

	fmt.Printf("✓ Normal PR workspace created successfully:\n")
	fmt.Printf("  - Workspace Path: %s\n", normalWs.Path)
	fmt.Printf("  - Fork Owner: %s (empty as expected)\n", normalWs.ForkOwner)
	fmt.Printf("  - Branch: %s\n", normalWs.Branch)

	fmt.Println("\n✓ All tests passed! Fork PR support has been successfully implemented.")
}