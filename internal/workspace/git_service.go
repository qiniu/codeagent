package workspace

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/qiniu/x/log"
)

// GitService handles all Git operations
type GitService interface {
	CloneRepository(repoURL, clonePath, branch string, createNewBranch bool) error
	GetRemoteURL(repoPath string) (string, error)
	GetCurrentBranch(repoPath string) (string, error)
	GetCurrentCommit(repoPath string) (string, error)
	GetBranchCommit(repoPath, branch string) (string, error)
	ValidateBranch(repoPath, expectedBranch string) bool
	ConfigureSafeDirectory(repoPath string) error
	ConfigurePullStrategy(repoPath string) error
	CreateAndCheckoutBranch(repoPath, branchName string) error
	CheckoutBranch(repoPath, branchName string) error
	CreateTrackingBranch(repoPath, branchName string) error
}

type gitService struct{}

// NewGitService creates a new Git service instance
func NewGitService() GitService {
	return &gitService{}
}

// CloneRepository clones a repository with optional branch creation
func (g *gitService) CloneRepository(repoURL, clonePath, branch string, createNewBranch bool) error {
	log.Infof("Cloning repository: %s to %s, branch: %s, createNewBranch: %v", repoURL, clonePath, branch, createNewBranch)

	// Create parent directory if needed
	if err := os.MkdirAll(filepath.Dir(clonePath), 0755); err != nil {
		return DirectoryError("clone_prepare", clonePath, err)
	}

	// Clone the repository with shallow depth for efficiency
	var cmd *exec.Cmd
	if createNewBranch {
		// Clone the default branch first, then create new branch
		cmd = exec.Command("git", "clone", "--depth", "50", repoURL, clonePath)
	} else {
		// Try to clone specific branch directly
		cmd = exec.Command("git", "clone", "--depth", "50", "--branch", branch, repoURL, clonePath)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		if !createNewBranch {
			// If direct branch clone failed, try cloning default branch first
			log.Warnf("Failed to clone specific branch %s directly, cloning default branch: %v", branch, err)
			cmd = exec.Command("git", "clone", "--depth", "50", repoURL, clonePath)
			output, err = cmd.CombinedOutput()
		}
		if err != nil {
			return GitError("clone", clonePath, fmt.Errorf("%s: %w", string(output), err))
		}
	}

	// Configure Git settings
	if err := g.ConfigureSafeDirectory(clonePath); err != nil {
		log.Warnf("Failed to configure safe directory: %v", err)
	}

	if err := g.ConfigurePullStrategy(clonePath); err != nil {
		log.Warnf("Failed to configure pull strategy: %v", err)
	}

	// Handle branch operations
	if createNewBranch {
		if err := g.CreateAndCheckoutBranch(clonePath, branch); err != nil {
			return err
		}
		log.Infof("Created new branch: %s", branch)
	} else if branch != "" {
		// Verify we're on the correct branch or switch to it
		if !g.ValidateBranch(clonePath, branch) {
			if err := g.CheckoutBranch(clonePath, branch); err != nil {
				// Branch might not exist locally, try creating tracking branch
				if err := g.CreateTrackingBranch(clonePath, branch); err != nil {
					log.Warnf("Failed to switch to branch %s: %v", branch, err)
				} else {
					log.Infof("Created tracking branch: %s", branch)
				}
			} else {
				log.Infof("Switched to existing branch: %s", branch)
			}
		}
	}

	log.Infof("Successfully cloned repository to: %s", clonePath)
	return nil
}

// GetRemoteURL retrieves the remote URL for a repository
func (g *gitService) GetRemoteURL(repoPath string) (string, error) {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return "", GitError("get_remote_url", repoPath, err)
	}
	return strings.TrimSpace(string(output)), nil
}

// GetCurrentBranch gets the current branch name
func (g *gitService) GetCurrentBranch(repoPath string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return "", GitError("get_current_branch", repoPath, err)
	}
	return strings.TrimSpace(string(output)), nil
}

// GetCurrentCommit gets the current commit hash
func (g *gitService) GetCurrentCommit(repoPath string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get current commit: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// GetBranchCommit gets the commit hash for a specific branch
func (g *gitService) GetBranchCommit(repoPath, branch string) (string, error) {
	cmd := exec.Command("git", "rev-parse", fmt.Sprintf("origin/%s", branch))
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get branch commit for %s: %w", branch, err)
	}
	return strings.TrimSpace(string(output)), nil
}

// ValidateBranch checks if the current branch matches the expected branch
func (g *gitService) ValidateBranch(repoPath, expectedBranch string) bool {
	currentBranch, err := g.GetCurrentBranch(repoPath)
	if err != nil {
		log.Infof("Failed to get current branch: %v", err)
		return false
	}

	log.Infof("Branch validation: current=%s, expected=%s", currentBranch, expectedBranch)

	// Check if we're on the correct branch
	if currentBranch == expectedBranch {
		return true
	}

	// If we're in detached HEAD state, check if we're pointing to the correct commit
	if currentBranch == "HEAD" {
		currentCommit, err := g.GetCurrentCommit(repoPath)
		if err != nil {
			log.Infof("Failed to get current commit: %v", err)
			return false
		}

		expectedCommit, err := g.GetBranchCommit(repoPath, expectedBranch)
		if err != nil {
			log.Infof("Failed to get expected branch commit: %v", err)
			return false
		}

		log.Infof("Commit validation: current=%s, expected=%s", currentCommit, expectedCommit)
		return currentCommit == expectedCommit
	}

	return false
}

// ConfigureSafeDirectory configures Git safe directory
func (g *gitService) ConfigureSafeDirectory(repoPath string) error {
	cmd := exec.Command("git", "config", "--local", "--add", "safe.directory", repoPath)
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return GitError("config_safe_directory", repoPath, fmt.Errorf("%s: %w", string(output), err))
	}
	return nil
}

// ConfigurePullStrategy configures rebase as default pull strategy
func (g *gitService) ConfigurePullStrategy(repoPath string) error {
	cmd := exec.Command("git", "config", "--local", "pull.rebase", "true")
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to configure pull strategy: %w, output: %s", err, string(output))
	}
	return nil
}

// CreateAndCheckoutBranch creates and checks out a new branch
func (g *gitService) CreateAndCheckoutBranch(repoPath, branchName string) error {
	cmd := exec.Command("git", "checkout", "-b", branchName)
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create new branch %s: %w, output: %s", branchName, err, string(output))
	}
	return nil
}

// CheckoutBranch switches to an existing branch
func (g *gitService) CheckoutBranch(repoPath, branchName string) error {
	cmd := exec.Command("git", "checkout", branchName)
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to checkout branch %s: %w, output: %s", branchName, err, string(output))
	}
	return nil
}

// CreateTrackingBranch creates a tracking branch for a remote branch
func (g *gitService) CreateTrackingBranch(repoPath, branchName string) error {
	cmd := exec.Command("git", "checkout", "-b", branchName, fmt.Sprintf("origin/%s", branchName))
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create tracking branch %s: %w, output: %s", branchName, err, string(output))
	}
	return nil
}
