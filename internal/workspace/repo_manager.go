package workspace

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"strconv"

	"github.com/qiniu/x/log"
)

// RepoManager Repository manager, responsible for managing worktrees of a single repository
type RepoManager struct {
	repoPath  string
	repoURL   string
	worktrees map[string]*WorktreeInfo // key: "aiModel-prNumber" or "prNumber" (backward compatible)
	mutex     sync.RWMutex
}

// WorktreeInfo worktree information
// Example:
// worktree /Users/jicarl/codeagent/qbox/codeagent
// HEAD 6446817fba0a257f73b311c93126041b63ab6f78
// branch refs/heads/main

// worktree /Users/jicarl/codeagent/qbox/codeagent/issue-11-1752143989
// HEAD 5c2df7724d26a27c154b90f519b6d4f4efdd1436
// branch refs/heads/codeagent/issue-11-1752143989
type WorktreeInfo struct {
	Worktree string
	Head     string
	Branch   string
}

// generateWorktreeKey generates key for worktree
func generateWorktreeKey(aiModel string, prNumber int) string {
	if aiModel != "" {
		return fmt.Sprintf("%s-%d", aiModel, prNumber)
	}
	return fmt.Sprintf("%d", prNumber)
}

// NewRepoManager creates new repository manager
func NewRepoManager(repoPath, repoURL string) *RepoManager {
	return &RepoManager{
		repoPath:  repoPath,
		repoURL:   repoURL,
		worktrees: make(map[string]*WorktreeInfo),
	}
}

// Initialize initializes repository (first clone)
func (r *RepoManager) Initialize() error {
	log.Infof("Starting repository initialization: %s", r.repoPath)

	// Create repository directory
	if err := os.MkdirAll(r.repoPath, 0755); err != nil {
		return fmt.Errorf("failed to create repo directory: %w", err)
	}

	// Clone repository (with timeout)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "clone", r.repoURL, ".")
	cmd.Dir = r.repoPath

	log.Infof("Executing git clone: %s", strings.Join(cmd.Args, " "))

	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("git clone timed out after 5 minutes: %w", err)
		}
		return fmt.Errorf("failed to clone repository: %w, output: %s", err, string(output))
	}

	// Configure Git safe directory
	cmd = exec.Command("git", "config", "--local", "--add", "safe.directory", r.repoPath)
	cmd.Dir = r.repoPath
	configOutput, err := cmd.CombinedOutput()
	if err != nil {
		log.Warnf("Failed to configure safe directory: %v\nCommand output: %s", err, string(configOutput))
	}

	// Configure rebase as default pull strategy
	cmd = exec.Command("git", "config", "--local", "pull.rebase", "true")
	cmd.Dir = r.repoPath
	rebaseConfigOutput, err := cmd.CombinedOutput()
	if err != nil {
		log.Warnf("Failed to configure pull.rebase: %v\nCommand output: %s", err, string(rebaseConfigOutput))
	}

	log.Infof("Successfully initialized repository: %s", r.repoPath)
	return nil
}

// isInitialized checks if repository is initialized
func (r *RepoManager) isInitialized() bool {
	gitDir := filepath.Join(r.repoPath, ".git")
	_, err := os.Stat(gitDir)
	return err == nil
}

// GetWorktree gets worktree for specified PR (backward compatible, default no AI model)
func (r *RepoManager) GetWorktree(prNumber int) *WorktreeInfo {
	return r.GetWorktreeWithAI(prNumber, "")
}

// GetWorktreeWithAI gets worktree for specified PR and AI model
func (r *RepoManager) GetWorktreeWithAI(prNumber int, aiModel string) *WorktreeInfo {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	key := generateWorktreeKey(aiModel, prNumber)
	return r.worktrees[key]
}

// RemoveWorktree removes worktree for specified PR (backward compatible, default no AI model)
func (r *RepoManager) RemoveWorktree(prNumber int) error {
	return r.RemoveWorktreeWithAI(prNumber, "")
}

// RemoveWorktreeWithAI removes worktree for specified PR and AI model
func (r *RepoManager) RemoveWorktreeWithAI(prNumber int, aiModel string) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	key := generateWorktreeKey(aiModel, prNumber)
	worktree := r.worktrees[key]
	if worktree == nil {
		log.Infof("Worktree for PR #%d with AI model %s not found in memory, skipping removal", prNumber, aiModel)
		return nil // Already doesn't exist
	}

	// Check if worktree directory exists
	if _, err := os.Stat(worktree.Worktree); os.IsNotExist(err) {
		log.Infof("Worktree directory %s does not exist, removing from memory only", worktree.Worktree)
		// Directory doesn't exist, only remove from memory
		delete(r.worktrees, key)
		return nil
	}

	// Delete worktree
	cmd := exec.Command("git", "worktree", "remove", "--force", worktree.Worktree)
	cmd.Dir = r.repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Errorf("Failed to remove worktree: %v, output: %s", err, string(output))
		// Even if deletion fails, remove from mapping to avoid memory state inconsistency
		log.Warnf("Removing worktree from memory despite removal failure")
	} else {
		log.Infof("Successfully removed worktree: %s", worktree.Worktree)
	}

	// Delete related local branch (if exists)
	if worktree.Branch != "" {
		log.Infof("Attempting to delete local branch: %s", worktree.Branch)
		branchCmd := exec.Command("git", "branch", "-D", worktree.Branch)
		branchCmd.Dir = r.repoPath
		branchOutput, err := branchCmd.CombinedOutput()
		if err != nil {
			log.Warnf("Failed to delete local branch %s: %v, output: %s", worktree.Branch, err, string(branchOutput))
			// Branch deletion failure is not fatal, branch may not exist or be in use
		} else {
			log.Infof("Successfully deleted local branch: %s", worktree.Branch)
		}
	}

	// Remove from mapping
	delete(r.worktrees, key)

	log.Infof("Removed worktree for PR #%d with AI model %s from memory", prNumber, aiModel)
	return nil
}

// ListWorktrees lists all worktrees
func (r *RepoManager) ListWorktrees() ([]*WorktreeInfo, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	// Get Git worktree list
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = r.repoPath
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list worktrees: %w", err)
	}

	return r.parseWorktreeList(string(output))
}

// parseWorktreeList parses worktree list output
func (r *RepoManager) parseWorktreeList(output string) ([]*WorktreeInfo, error) {
	var worktrees []*WorktreeInfo
	lines := strings.Split(strings.TrimSpace(output), "\n")

	log.Infof("Parsing worktree list output: %s", output)

	// Filter out empty lines
	var filteredLines []string
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			filteredLines = append(filteredLines, line)
		}
	}

	for i := 0; i < len(filteredLines); i += 3 {
		if i+2 >= len(filteredLines) {
			break
		}

		// Parse worktree path (first line)
		pathLine := strings.TrimSpace(filteredLines[i])
		if !strings.HasPrefix(pathLine, "worktree ") {
			log.Warnf("Invalid worktree line: %s", pathLine)
			continue
		}
		path := strings.TrimPrefix(pathLine, "worktree ")

		// Skip HEAD line (second line)
		headLine := strings.TrimSpace(filteredLines[i+1])
		if !strings.HasPrefix(headLine, "HEAD ") {
			log.Warnf("Invalid HEAD line: %s", headLine)
			continue
		}
		head := strings.TrimPrefix(headLine, "HEAD ")

		// Parse branch information (third line)
		branchLine := strings.TrimSpace(filteredLines[i+2])
		var branch string
		if !strings.HasPrefix(branchLine, "branch ") {
			log.Warnf("Invalid branch line: %s", branchLine)
			continue
		}
		branch = strings.TrimPrefix(branchLine, "branch ")

		worktree := &WorktreeInfo{
			Worktree: path,
			Head:     head,
			Branch:   branch,
		}
		log.Infof("Found worktree: %s, head: %s, branch: %s", path, head, branch)
		worktrees = append(worktrees, worktree)
	}

	log.Infof("Parsed %d worktrees", len(worktrees))
	return worktrees, nil
}

// CreateWorktreeWithName creates worktree using specified name
func (r *RepoManager) CreateWorktreeWithName(worktreeName string, branch string, createNewBranch bool) (*WorktreeInfo, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	log.Infof("Creating worktree with name: %s, branch: %s, createNewBranch: %v", worktreeName, branch, createNewBranch)

	// Ensure repository is initialized
	if !r.isInitialized() {
		log.Infof("Repository not initialized, initializing: %s", r.repoPath)
		if err := r.Initialize(); err != nil {
			return nil, err
		}
	} else {
		// Repository exists, ensure main repository code is up to date
		if err := r.updateMainRepository(); err != nil {
			log.Warnf("Failed to update main repository: %v", err)
			// Don't block worktree creation due to update failure, but log warning
		}
	}

	// Create worktree path (same level as repository directory)
	orgDir := filepath.Dir(r.repoPath)
	worktreePath := filepath.Join(orgDir, worktreeName)
	log.Infof("Worktree path: %s", worktreePath)

	// Create worktree
	var cmd *exec.Cmd
	if createNewBranch {
		// Create worktree for new branch
		// First check what the default branch is
		log.Infof("Checking default branch for new branch creation")
		defaultBranchCmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
		defaultBranchCmd.Dir = r.repoPath
		defaultBranchOutput, err := defaultBranchCmd.Output()
		if err != nil {
			log.Errorf("Failed to get default branch, using 'main': %v", err)
			defaultBranchOutput = []byte("main")
		}
		defaultBranch := strings.TrimSpace(string(defaultBranchOutput))
		if defaultBranch == "" {
			defaultBranch = "main"
		}

		log.Infof("Creating new branch worktree: git worktree add -b %s %s %s", branch, worktreePath, defaultBranch)
		cmd = exec.Command("git", "worktree", "add", "-b", branch, worktreePath, defaultBranch)
	} else {
		// Create worktree for existing branch
		// First check if local branch already exists
		log.Infof("Checking if local branch exists: %s", branch)
		localBranchCmd := exec.Command("git", "show-ref", "--verify", "--quiet", fmt.Sprintf("refs/heads/%s", branch))
		localBranchCmd.Dir = r.repoPath
		localBranchExists := localBranchCmd.Run() == nil

		if localBranchExists {
			log.Infof("Local branch %s already exists, creating worktree without -b flag", branch)
			// Local branch already exists, create worktree directly without -b flag
			cmd = exec.Command("git", "worktree", "add", worktreePath, branch)
		} else {
			// Local branch doesn't exist, check if remote branch exists
			log.Infof("Local branch does not exist, checking if remote branch exists: origin/%s", branch)
			checkCmd := exec.Command("git", "ls-remote", "--heads", "origin", branch)
			checkCmd.Dir = r.repoPath
			checkOutput, err := checkCmd.CombinedOutput()
			if err != nil {
				log.Errorf("Failed to check remote branch: %v, output: %s", err, string(checkOutput))
			} else if strings.TrimSpace(string(checkOutput)) == "" {
				log.Errorf("Remote branch origin/%s does not exist, will create new branch", branch)
				// If remote branch doesn't exist, create new branch
				defaultBranchCmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
				defaultBranchCmd.Dir = r.repoPath
				defaultBranchOutput, err := defaultBranchCmd.Output()
				if err != nil {
					log.Warnf("Failed to get default branch, using 'main': %v", err)
					defaultBranchOutput = []byte("main")
				}
				defaultBranch := strings.TrimSpace(string(defaultBranchOutput))
				if defaultBranch == "" {
					defaultBranch = "main"
				}
				cmd = exec.Command("git", "worktree", "add", "-b", branch, worktreePath, defaultBranch)
			} else {
				log.Infof("Remote branch exists, creating worktree: git worktree add -b %s %s origin/%s", branch, worktreePath, branch)
				cmd = exec.Command("git", "worktree", "add", "-b", branch, worktreePath, fmt.Sprintf("origin/%s", branch))
			}
		}
	}

	if cmd == nil {
		// If command hasn't been set yet, use default new branch creation method
		log.Infof("Using default new branch creation: git worktree add -b %s %s main", branch, worktreePath)
		cmd = exec.Command("git", "worktree", "add", "-b", branch, worktreePath, "main")
	}

	cmd.Dir = r.repoPath

	log.Infof("Executing command: %s", strings.Join(cmd.Args, " "))
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Errorf("Failed to create worktree: %v, output: %s", err, string(output))
		return nil, fmt.Errorf("failed to create worktree: %w, output: %s", err, string(output))
	}

	// Configure Git safe directory
	cmd = exec.Command("git", "config", "--local", "--add", "safe.directory", worktreePath)
	cmd.Dir = worktreePath // Configure safe directory in worktree directory
	configOutput, err := cmd.CombinedOutput()
	if err != nil {
		log.Warnf("Failed to configure safe directory: %v\nCommand output: %s", err, string(configOutput))
	} else {
		log.Infof("Successfully configured safe directory: %s", worktreePath)
	}

	log.Infof("Worktree creation output: %s", string(output))

	// Create worktree information
	worktree := &WorktreeInfo{
		Worktree: worktreePath,
		Branch:   branch,
	}

	log.Infof("Successfully created worktree: %s", worktreePath)
	return worktree, nil
}

// RegisterWorktree registers single worktree to memory (backward compatible, default no AI model)
func (r *RepoManager) RegisterWorktree(prNumber int, worktree *WorktreeInfo) {
	r.RegisterWorktreeWithAI(prNumber, "", worktree)
}

// RegisterWorktreeWithAI registers single worktree to memory (supports AI model)
func (r *RepoManager) RegisterWorktreeWithAI(prNumber int, aiModel string, worktree *WorktreeInfo) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	key := generateWorktreeKey(aiModel, prNumber)
	r.worktrees[key] = worktree
}

// GetRepoPath gets repository path
func (r *RepoManager) GetRepoPath() string {
	return r.repoPath
}

// GetRepoURL gets repository URL
func (r *RepoManager) GetRepoURL() string {
	return r.repoURL
}

// GetWorktreeCount gets worktree count
func (r *RepoManager) GetWorktreeCount() int {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return len(r.worktrees)
}

// updateMainRepository updates main repository code to latest version
func (r *RepoManager) updateMainRepository() error {
	log.Infof("Updating main repository: %s", r.repoPath)

	// 1. Get latest remote references
	cmd := exec.Command("git", "fetch", "--all", "--prune")
	cmd.Dir = r.repoPath
	fetchOutput, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to fetch latest changes: %w, output: %s", err, string(fetchOutput))
	}
	log.Infof("Fetched latest changes for main repository")

	// 2. Get current branch
	cmd = exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = r.repoPath
	currentBranchOutput, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get current branch: %w", err)
	}
	currentBranch := strings.TrimSpace(string(currentBranchOutput))

	// 3. Check for uncommitted changes
	cmd = exec.Command("git", "status", "--porcelain")
	cmd.Dir = r.repoPath
	statusOutput, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to check status: %w", err)
	}

	hasChanges := strings.TrimSpace(string(statusOutput)) != ""
	if hasChanges {
		// Main repository should not have uncommitted changes, this violates best practices
		log.Warnf("Main repository has uncommitted changes, this violates best practices")
		log.Warnf("Uncommitted changes:\n%s", string(statusOutput))

		// For safety, stash these changes
		cmd = exec.Command("git", "stash", "push", "-m", "Auto-stash from updateMainRepository")
		cmd.Dir = r.repoPath
		stashOutput, err := cmd.CombinedOutput()
		if err != nil {
			log.Warnf("Failed to stash changes: %v, output: %s", err, string(stashOutput))
		} else {
			log.Infof("Stashed uncommitted changes in main repository")
		}
	}

	// 4. Use rebase to update to latest version
	remoteBranch := fmt.Sprintf("origin/%s", currentBranch)
	cmd = exec.Command("git", "rebase", remoteBranch)
	cmd.Dir = r.repoPath
	rebaseOutput, err := cmd.CombinedOutput()
	if err != nil {
		// rebase failed, try reset to remote branch
		log.Warnf("Rebase failed, attempting hard reset: %v, output: %s", err, string(rebaseOutput))

		cmd = exec.Command("git", "reset", "--hard", remoteBranch)
		cmd.Dir = r.repoPath
		resetOutput, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to reset to remote branch: %w, output: %s", err, string(resetOutput))
		}
		log.Infof("Hard reset main repository to %s", remoteBranch)
	} else {
		log.Infof("Successfully rebased main repository to %s", remoteBranch)
	}

	// 5. Clean up unused references
	cmd = exec.Command("git", "gc", "--auto")
	cmd.Dir = r.repoPath
	gcOutput, err := cmd.CombinedOutput()
	if err != nil {
		log.Warnf("Failed to run git gc: %v, output: %s", err, string(gcOutput))
	}

	log.Infof("Main repository updated successfully")
	return nil
}

// EnsureMainRepositoryUpToDate ensures main repository is up to date (public method, can be called externally)
func (r *RepoManager) EnsureMainRepositoryUpToDate() error {
	if !r.isInitialized() {
		return fmt.Errorf("repository not initialized")
	}
	return r.updateMainRepository()
}

// RestoreWorktrees scans worktrees on disk and registers them to memory
func (r *RepoManager) RestoreWorktrees() error {
	worktrees, err := r.ListWorktrees()
	if err != nil {
		return err
	}
	for _, wt := range worktrees {
		// Only handle worktree directories containing -pr-
		base := filepath.Base(wt.Worktree)
		if strings.Contains(base, "-pr-") {
			// Parse directory name format: {aiModel}-{repo}-pr-{prNumber}-{timestamp}
			// Find position of "pr"
			parts := strings.Split(base, "-")
			prIndex := -1
			for i, part := range parts {
				if part == "pr" {
					prIndex = i
					break
				}
			}

			if prIndex != -1 && prIndex >= 2 && prIndex < len(parts)-2 {
				// Extract AI model and PR number
				aiModel := strings.Join(parts[:prIndex-1], "-")
				_ = parts[prIndex-1] // repo name, not used but extracted for clarity
				prNumber, err := strconv.Atoi(parts[prIndex+1])
				if err == nil {
					// Verify if AI model is valid
					if aiModel == "gemini" || aiModel == "claude" {
						r.RegisterWorktreeWithAI(prNumber, aiModel, wt)
						log.Infof("Restored worktree for PR #%d with AI model %s: %s", prNumber, aiModel, wt.Worktree)
					} else {
						// Backward compatible: if no valid AI model, register using default method
						r.RegisterWorktree(prNumber, wt)
						log.Infof("Restored worktree for PR #%d (no AI model): %s", prNumber, wt.Worktree)
					}
				}
			}
		}
	}
	return nil
}
