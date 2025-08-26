package workspace

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/qiniu/x/log"
)

// RepoCacheService manages local repository cache to avoid repeated remote clones
type RepoCacheService interface {
	GetOrCreateCachedRepo(repoURL, org, repo string) (string, error)
	GetOrCreateCachedRepoWithDefaultBranch(repoURL, org, repo, defaultBranch string) (string, error)
	UpdateCachedRepo(cachedRepoPath string) error
	UpdateCachedRepoWithDefaultBranch(cachedRepoPath, defaultBranch string) error
	CloneFromCache(cachedRepoPath, targetPath, branch, repoURL string, createNewBranch bool) error
	CachedRepoExists(org, repo string) bool
	GetCachedRepoPath(org, repo string) string
}

type repoCacheService struct {
	baseDir    string
	gitService GitService
	mutex      sync.RWMutex
	// Track which repos are currently being updated to avoid concurrent updates
	updating map[string]bool
}

// NewRepoCacheService creates a new repository cache service
func NewRepoCacheService(baseDir string, gitService GitService) RepoCacheService {
	return &repoCacheService{
		baseDir:    baseDir,
		gitService: gitService,
		updating:   make(map[string]bool),
	}
}

// GetOrCreateCachedRepo gets or creates a cached repository using fallback branch detection
func (r *repoCacheService) GetOrCreateCachedRepo(repoURL, org, repo string) (string, error) {
	return r.GetOrCreateCachedRepoWithDefaultBranch(repoURL, org, repo, "")
}

// GetOrCreateCachedRepoWithDefaultBranch gets or creates a cached repository with specified default branch
func (r *repoCacheService) GetOrCreateCachedRepoWithDefaultBranch(repoURL, org, repo, defaultBranch string) (string, error) {
	cachedRepoPath := r.GetCachedRepoPath(org, repo)

	// Check if cached repo already exists
	if r.CachedRepoExists(org, repo) {
		// Update the cached repo to get latest changes
		if err := r.UpdateCachedRepoWithDefaultBranch(cachedRepoPath, defaultBranch); err != nil {
			log.Warnf("Failed to update cached repo %s: %v", cachedRepoPath, err)
			// Continue with existing cached repo even if update fails
		}
		return cachedRepoPath, nil
	}

	// Clone repository to cache for the first time
	log.Infof("Cloning repository to cache: %s -> %s", repoURL, cachedRepoPath)

	// Create parent directory
	if err := os.MkdirAll(filepath.Dir(cachedRepoPath), 0755); err != nil {
		return "", DirectoryError("create_cache_dir", cachedRepoPath, err)
	}

	// Clone with full history (not shallow) for cache
	if err := r.gitService.CloneRepository(repoURL, cachedRepoPath, "", false); err != nil {
		// Clean up failed clone
		os.RemoveAll(cachedRepoPath)
		return "", GitError("cache_clone", cachedRepoPath, err)
	}

	log.Infof("Successfully cached repository: %s", cachedRepoPath)
	return cachedRepoPath, nil
}

// UpdateCachedRepo updates a cached repository with latest changes using fallback branch detection
func (r *repoCacheService) UpdateCachedRepo(cachedRepoPath string) error {
	return r.UpdateCachedRepoWithDefaultBranch(cachedRepoPath, "")
}

// UpdateCachedRepoWithDefaultBranch updates a cached repository with latest changes using specified default branch
func (r *repoCacheService) UpdateCachedRepoWithDefaultBranch(cachedRepoPath, defaultBranch string) error {
	repoKey := cachedRepoPath

	r.mutex.Lock()
	if r.updating[repoKey] {
		r.mutex.Unlock()
		log.Infof("Repository %s is already being updated, skipping", cachedRepoPath)
		return nil
	}
	r.updating[repoKey] = true
	r.mutex.Unlock()

	defer func() {
		r.mutex.Lock()
		delete(r.updating, repoKey)
		r.mutex.Unlock()
	}()

	log.Infof("Updating cached repository: %s", cachedRepoPath)

	// Git fetch to update all branches and tags
	if err := r.runGitCommand(cachedRepoPath, "fetch", "--all", "--prune"); err != nil {
		return GitError("fetch", cachedRepoPath, err)
	}

	// Update main/master branch
	if err := r.updateMainBranchWithDefault(cachedRepoPath, defaultBranch); err != nil {
		log.Warnf("Failed to update main branch in %s: %v", cachedRepoPath, err)
		// Don't fail the entire operation if main branch update fails
	}

	log.Infof("Successfully updated cached repository: %s", cachedRepoPath)
	return nil
}

// CloneFromCache clones from cached repository to target workspace
func (r *repoCacheService) CloneFromCache(cachedRepoPath, targetPath, branch, repoURL string, createNewBranch bool) error {
	log.Infof("Cloning from cache: %s -> %s, branch: %s, createNewBranch: %v",
		cachedRepoPath, targetPath, branch, createNewBranch)

	// Create parent directory
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return DirectoryError("clone_from_cache_prepare", targetPath, err)
	}

	// Clone from local cached repo (much faster than remote)
	var cloneArgs []string
	if createNewBranch {
		// Clone default branch, will create new branch later
		cloneArgs = []string{"clone", cachedRepoPath, targetPath}
	} else if branch != "" {
		// Try to clone specific branch directly
		cloneArgs = []string{"clone", "--branch", branch, cachedRepoPath, targetPath}
	} else {
		// Clone default branch
		cloneArgs = []string{"clone", cachedRepoPath, targetPath}
	}

	if err := r.runGitCommand("", cloneArgs...); err != nil {
		if !createNewBranch && branch != "" {
			// If specific branch clone failed, try default branch
			log.Warnf("Failed to clone branch %s, trying default branch", branch)
			cloneArgs = []string{"clone", cachedRepoPath, targetPath}
			if err := r.runGitCommand("", cloneArgs...); err != nil {
				return GitError("clone_from_cache", targetPath, err)
			}
		} else {
			return GitError("clone_from_cache", targetPath, err)
		}
	}

	// Configure Git settings
	if err := r.gitService.ConfigureSafeDirectory(targetPath); err != nil {
		log.Warnf("Failed to configure safe directory: %v", err)
	}

	if err := r.gitService.ConfigurePullStrategy(targetPath); err != nil {
		log.Warnf("Failed to configure pull strategy: %v", err)
	}

	// Handle branch operations
	if createNewBranch {
		if err := r.gitService.CreateAndCheckoutBranch(targetPath, branch); err != nil {
			return err
		}
		log.Infof("Created new branch: %s", branch)
	} else if branch != "" {
		// Verify we're on the correct branch or switch to it
		if !r.gitService.ValidateBranch(targetPath, branch) {
			if err := r.gitService.CheckoutBranch(targetPath, branch); err != nil {
				if err := r.gitService.CreateTrackingBranch(targetPath, branch); err != nil {
					log.Warnf("Failed to switch to branch %s: %v", branch, err)
				} else {
					log.Infof("Created tracking branch: %s", branch)
				}
			} else {
				log.Infof("Switched to existing branch: %s", branch)
			}
		}
	}

	// Fix remote origin URL to point to the original remote repository instead of local cache
	if repoURL != "" {
		if err := r.runGitCommand(targetPath, "remote", "set-url", "origin", repoURL); err != nil {
			log.Warnf("Failed to set remote origin URL to %s: %v", repoURL, err)
			// Don't fail the entire operation if remote URL setting fails
		} else {
			log.Infof("Set remote origin URL to: %s", repoURL)
		}
	}

	log.Infof("Successfully cloned from cache to: %s", targetPath)
	return nil
}

// CachedRepoExists checks if a cached repository exists
func (r *repoCacheService) CachedRepoExists(org, repo string) bool {
	cachedRepoPath := r.GetCachedRepoPath(org, repo)
	if _, err := os.Stat(filepath.Join(cachedRepoPath, ".git")); os.IsNotExist(err) {
		return false
	}
	return true
}

// GetCachedRepoPath returns the path where a cached repository should be stored
func (r *repoCacheService) GetCachedRepoPath(org, repo string) string {
	return filepath.Join(r.baseDir, "_cache", org, repo)
}

// runGitCommand runs a git command in the specified directory
func (r *repoCacheService) runGitCommand(workDir string, args ...string) error {
	cmd := exec.Command("git", args...)
	if workDir != "" {
		cmd.Dir = workDir
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git command failed: %s, output: %s", err, string(output))
	}
	return nil
}

// updateMainBranchWithDefault updates the main/master branch in cached repository
func (r *repoCacheService) updateMainBranchWithDefault(cachedRepoPath, defaultBranch string) error {
	// Try to find and update main branch, prioritizing provided default branch
	var mainBranches []string
	if defaultBranch != "" {
		mainBranches = []string{defaultBranch, "main", "master"}
	} else {
		mainBranches = []string{"main", "master"}
	}

	for _, mainBranch := range mainBranches {
		// Check if branch exists
		if err := r.runGitCommand(cachedRepoPath, "show-ref", "--verify", "--quiet", "refs/heads/"+mainBranch); err != nil {
			continue // Branch doesn't exist, try next
		}

		// Checkout to main branch
		if err := r.runGitCommand(cachedRepoPath, "checkout", mainBranch); err != nil {
			log.Warnf("Failed to checkout %s branch: %v", mainBranch, err)
			continue
		}

		// Pull latest changes
		if err := r.runGitCommand(cachedRepoPath, "pull", "--rebase"); err != nil {
			log.Warnf("Failed to pull %s branch: %v", mainBranch, err)
			continue
		}

		log.Infof("Successfully updated %s branch in cached repo", mainBranch)
		return nil
	}

	return fmt.Errorf("no main/master branch found to update")
}
