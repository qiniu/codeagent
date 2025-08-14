package command

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-github/v58/github"
	githubcontext "github.com/qiniu/codeagent/internal/context"
	"github.com/stretchr/testify/require"
)

// BenchmarkProcessor_ProcessDirectories benchmarks the complete directory processing pipeline
func BenchmarkProcessor_ProcessDirectories(b *testing.B) {
	// Create test environment once
	tempDir, err := os.MkdirTemp("", "codeagent-benchmark")
	require.NoError(b, err)
	defer os.RemoveAll(tempDir)

	// Setup global config with multiple commands and agents
	globalPath := filepath.Join(tempDir, "global", ".codeagent")
	setupBenchmarkGlobalConfig(b, globalPath)

	// Setup repository config
	repoPath := filepath.Join(tempDir, "repo", ".codeagent")
	setupBenchmarkRepoConfig(b, repoPath)

	// Create GitHub event
	githubEvent := &githubcontext.GitHubEvent{
		Type:           "pull_request",
		Repository:     "benchmark/test-repo",
		TriggerUser:    "benchmark-user",
		Action:         "synchronize",
		TriggerComment: "/analyze performance",
		PullRequest: &github.PullRequest{
			Number: github.Int(1),
			Title:  github.String("Performance improvements"),
			Head:   &github.PullRequestBranch{Ref: github.String("feature/perf")},
			Base:   &github.PullRequestBranch{Ref: github.String("main")},
		},
		ChangedFiles:   generateLargeFileList(50), // 50 changed files
		PRComments:     generateCommentList(20),   // 20 PR comments
		ReviewComments: generateCommentList(15),   // 15 review comments
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Create unique processor for each iteration to avoid conflicts
		repoName := fmt.Sprintf("benchmark-repo-%d", i)
		processor := NewContextAwareDirectoryProcessor(globalPath, repoPath, repoName)

		err := processor.ProcessDirectories(githubEvent)
		if err != nil {
			b.Fatalf("Processing failed: %v", err)
		}

		// Load a command to test the complete pipeline
		_, err = processor.LoadCommand("analyze")
		if err != nil {
			b.Fatalf("Command loading failed: %v", err)
		}

		// Cleanup for next iteration
		processor.Cleanup()
	}
}

// BenchmarkProcessor_LargeRepository benchmarks processing with a large repository configuration
func BenchmarkProcessor_LargeRepository(b *testing.B) {
	tempDir, err := os.MkdirTemp("", "codeagent-large-benchmark")
	require.NoError(b, err)
	defer os.RemoveAll(tempDir)

	// Create large configuration with many commands and agents
	globalPath := filepath.Join(tempDir, "global", ".codeagent")
	repoPath := filepath.Join(tempDir, "repo", ".codeagent")

	// Create 50 commands and 30 agents in global config
	setupLargeGlobalConfig(b, globalPath, 50, 30)
	// Create 20 commands and 15 agents in repo config (overrides)
	setupLargeRepoConfig(b, repoPath, 20, 15)

	githubEvent := &githubcontext.GitHubEvent{
		Type:        "issues",
		Repository:  "large/repo",
		TriggerUser: "user",
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		repoName := fmt.Sprintf("large-repo-%d", i)
		processor := NewContextAwareDirectoryProcessor(globalPath, repoPath, repoName)

		err := processor.ProcessDirectories(githubEvent)
		if err != nil {
			b.Fatalf("Processing failed: %v", err)
		}

		processor.Cleanup()
	}
}

// BenchmarkProcessor_ContextInjection benchmarks GitHub context variable injection
func BenchmarkProcessor_ContextInjection(b *testing.B) {
	tempDir, err := os.MkdirTemp("", "codeagent-context-benchmark")
	require.NoError(b, err)
	defer os.RemoveAll(tempDir)

	// Setup minimal config
	globalPath := filepath.Join(tempDir, "global", ".codeagent")
	setupBenchmarkGlobalConfig(b, globalPath)

	// Create event with extensive context data
	githubEvent := &githubcontext.GitHubEvent{
		Type:           "pull_request_review_comment",
		Repository:     "context/benchmark-repo",
		TriggerUser:    "context-user",
		TriggerComment: "Large context injection test with many variables: $GITHUB_REPOSITORY $GITHUB_EVENT_TYPE $GITHUB_TRIGGER_USER",
		PullRequest: &github.PullRequest{
			Number: github.Int(999),
			Title:  github.String("Context injection benchmark with very long title that contains many words and characters to test performance"),
			Head:   &github.PullRequestBranch{Ref: github.String("feature/context-benchmark-long-branch-name")},
			Base:   &github.PullRequestBranch{Ref: github.String("main")},
		},
		Issue: &github.Issue{
			Number: github.Int(888),
			Title:  github.String("Context benchmark issue"),
			Body:   github.String(generateLargeText(1000)), // 1000 words
		},
		ChangedFiles:   generateLargeFileList(100), // 100 files
		IssueComments:  generateCommentList(50),    // 50 issue comments
		PRComments:     generateCommentList(40),    // 40 PR comments
		ReviewComments: generateCommentList(30),    // 30 review comments
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		repoName := fmt.Sprintf("context-repo-%d", i)
		processor := NewContextAwareDirectoryProcessor(globalPath, "", repoName)

		err := processor.ProcessDirectories(githubEvent)
		if err != nil {
			b.Fatalf("Processing failed: %v", err)
		}

		// Load and verify context injection
		cmdDef, err := processor.LoadCommand("analyze")
		if err != nil {
			b.Fatalf("Command loading failed: %v", err)
		}

		// Ensure context was injected (basic check)
		if len(cmdDef.Content) < 100 {
			b.Fatalf("Content seems too short, context injection may have failed")
		}

		processor.Cleanup()
	}
}

// setupBenchmarkGlobalConfig creates a realistic global config for benchmarking
func setupBenchmarkGlobalConfig(b *testing.B, globalPath string) {
	commandsDir := filepath.Join(globalPath, "commands")
	agentsDir := filepath.Join(globalPath, "agents")

	require.NoError(b, os.MkdirAll(commandsDir, 0755))
	require.NoError(b, os.MkdirAll(agentsDir, 0755))

	// Create analyze command with extensive GitHub context usage
	analyzeCmd := `---
description: "Performance benchmark analysis command"
model: "claude-3-5-sonnet"
tools: ["read", "grep", "write"]
---

# Performance Analysis

## Repository Context
- Repository: $GITHUB_REPOSITORY
- Event Type: $GITHUB_EVENT_TYPE
- Trigger User: $GITHUB_TRIGGER_USER
- Action: $GITHUB_ACTION
- Comment: $GITHUB_TRIGGER_COMMENT

{{if .GITHUB_IS_ISSUE}}
## Issue Analysis
- Issue #$GITHUB_ISSUE_NUMBER: $GITHUB_ISSUE_TITLE
- Description: $GITHUB_ISSUE_BODY

### Comment History ({{len .GITHUB_ISSUE_COMMENTS}} comments)
{{range .GITHUB_ISSUE_COMMENTS}}
- {{.}}
{{end}}
{{end}}

{{if .GITHUB_IS_PR}}
## Pull Request Analysis  
- PR #$GITHUB_PR_NUMBER: $GITHUB_PR_TITLE
- Branch: $GITHUB_BRANCH_NAME → $GITHUB_BASE_BRANCH

### Changed Files ({{len .GITHUB_CHANGED_FILES}} files)
{{range .GITHUB_CHANGED_FILES}}
- {{.}}
{{end}}

### PR Comments ({{len .GITHUB_PR_COMMENTS}} comments)
{{range .GITHUB_PR_COMMENTS}}
- {{.}}
{{end}}

### Review Comments ({{len .GITHUB_REVIEW_COMMENTS}} comments)
{{range .GITHUB_REVIEW_COMMENTS}}
- {{.}}
{{end}}
{{end}}

Please perform comprehensive analysis based on the provided context.
`

	require.NoError(b, os.WriteFile(filepath.Join(commandsDir, "analyze.md"), []byte(analyzeCmd), 0644))

	// Create plan command
	planCmd := `---
description: "Benchmark planning command"
model: "claude-3-opus"
---

# Planning for $GITHUB_REPOSITORY

Event: $GITHUB_EVENT_TYPE
User: $GITHUB_TRIGGER_USER

Create comprehensive implementation plan.
`

	require.NoError(b, os.WriteFile(filepath.Join(commandsDir, "plan.md"), []byte(planCmd), 0644))

	// Create code command
	codeCmd := `---
description: "Benchmark code implementation"
model: "claude-3-5-sonnet"
tools: ["all"]
---

# Implementation for $GITHUB_REPOSITORY

Instruction: $GITHUB_TRIGGER_COMMENT

Implement the requested functionality with comprehensive testing.
`

	require.NoError(b, os.WriteFile(filepath.Join(commandsDir, "code.md"), []byte(codeCmd), 0644))
}

// setupBenchmarkRepoConfig creates repository-specific config for benchmarking
func setupBenchmarkRepoConfig(b *testing.B, repoPath string) {
	commandsDir := filepath.Join(repoPath, "commands")
	require.NoError(b, os.MkdirAll(commandsDir, 0755))

	// Override analyze command
	repoAnalyzeCmd := `---
description: "Repository-specific benchmark analysis"
model: "claude-3-5-sonnet"
tools: ["read", "grep", "write", "bash"]
---

# Repository-Specific Analysis

Repository: $GITHUB_REPOSITORY
Context: This is a benchmark test with repository-specific overrides.

{{if .GITHUB_IS_PR}}
## PR Benchmark Analysis
- PR: $GITHUB_PR_TITLE
- Files: {{len .GITHUB_CHANGED_FILES}}
- Comments: {{len .GITHUB_PR_COMMENTS}}
{{end}}

Perform repository-specific analysis optimized for this codebase.
`

	require.NoError(b, os.WriteFile(filepath.Join(commandsDir, "analyze.md"), []byte(repoAnalyzeCmd), 0644))
}

// setupLargeGlobalConfig creates a large number of commands and agents for stress testing
func setupLargeGlobalConfig(b *testing.B, globalPath string, numCommands, numAgents int) {
	commandsDir := filepath.Join(globalPath, "commands")
	agentsDir := filepath.Join(globalPath, "agents")

	require.NoError(b, os.MkdirAll(commandsDir, 0755))
	require.NoError(b, os.MkdirAll(agentsDir, 0755))

	// Create many commands
	for i := 0; i < numCommands; i++ {
		cmdContent := fmt.Sprintf(`---
description: "Global command %d"
model: "claude-3-5-sonnet"
---

# Global Command %d

Repository: $GITHUB_REPOSITORY
Event: $GITHUB_EVENT_TYPE

This is global command number %d for stress testing.
`, i, i, i)

		filename := fmt.Sprintf("cmd%03d.md", i)
		require.NoError(b, os.WriteFile(filepath.Join(commandsDir, filename), []byte(cmdContent), 0644))
	}

	// Create many agents
	for i := 0; i < numAgents; i++ {
		agentContent := fmt.Sprintf(`---
name: "global-agent-%d"
description: "Global agent %d for stress testing"
model: "claude-3-5-sonnet"
---

# Global Agent %d

You are global agent number %d specialized in stress testing scenarios.
`, i, i, i, i)

		filename := fmt.Sprintf("agent%03d.md", i)
		require.NoError(b, os.WriteFile(filepath.Join(agentsDir, filename), []byte(agentContent), 0644))
	}
}

// setupLargeRepoConfig creates repository overrides for stress testing
func setupLargeRepoConfig(b *testing.B, repoPath string, numCommands, numAgents int) {
	commandsDir := filepath.Join(repoPath, "commands")
	agentsDir := filepath.Join(repoPath, "agents")

	require.NoError(b, os.MkdirAll(commandsDir, 0755))
	require.NoError(b, os.MkdirAll(agentsDir, 0755))

	// Override some commands
	for i := 0; i < numCommands; i++ {
		cmdContent := fmt.Sprintf(`---
description: "Repository command %d (override)"
model: "claude-3-5-sonnet"
---

# Repository Command %d (Override)

Repository: $GITHUB_REPOSITORY

This is repository command %d that overrides the global version.
`, i, i, i)

		filename := fmt.Sprintf("cmd%03d.md", i)
		require.NoError(b, os.WriteFile(filepath.Join(commandsDir, filename), []byte(cmdContent), 0644))
	}

	// Override some agents
	for i := 0; i < numAgents; i++ {
		agentContent := fmt.Sprintf(`---
name: "repo-agent-%d"
description: "Repository agent %d (override)"
model: "claude-3-5-sonnet"
---

# Repository Agent %d (Override)

You are repository agent %d that overrides the global version.
`, i, i, i, i)

		filename := fmt.Sprintf("agent%03d.md", i)
		require.NoError(b, os.WriteFile(filepath.Join(agentsDir, filename), []byte(agentContent), 0644))
	}
}

// generateLargeFileList creates a list of file paths for benchmarking
func generateLargeFileList(count int) []string {
	files := make([]string, count)
	for i := 0; i < count; i++ {
		files[i] = fmt.Sprintf("internal/package%d/file%d.go", i%10, i)
	}
	return files
}

// generateCommentList creates a list of comments for benchmarking
func generateCommentList(count int) []string {
	comments := make([]string, count)
	for i := 0; i < count; i++ {
		comments[i] = fmt.Sprintf("Comment %d: This is a benchmark comment for testing context injection performance", i)
	}
	return comments
}

// generateLargeText creates large text content for benchmarking
func generateLargeText(words int) string {
	text := "Performance benchmark text content. "
	result := ""
	for i := 0; i < words; i++ {
		result += fmt.Sprintf("%s Word %d. ", text, i)
	}
	return result
}
