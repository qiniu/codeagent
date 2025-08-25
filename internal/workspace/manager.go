package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/go-github/v58/github"
	"github.com/qiniu/codeagent/internal/config"
	"github.com/qiniu/codeagent/pkg/models"
	"github.com/qiniu/x/log"
)

const (
	// BranchPrefix branch name prefix, used to identify branches created by codeagent
	BranchPrefix = "codeagent"
)

// Manager manages workspace lifecycle
type Manager struct {
	baseDir string
	config  *config.Config

	// Service dependencies
	repository       WorkspaceRepository
	gitService       GitService
	containerService ContainerService
	dirFormatter     DirFormatter
	repoCacheService RepoCacheService
}

// NewManager creates a new workspace manager with service dependencies
func NewManager(cfg *config.Config) *Manager {
	gitService := NewGitService()
	m := &Manager{
		baseDir:          cfg.Workspace.BaseDir,
		config:           cfg,
		repository:       NewWorkspaceRepository(),
		gitService:       gitService,
		containerService: NewContainerService(),
		dirFormatter:     NewDirFormatter(),
		repoCacheService: NewRepoCacheService(cfg.Workspace.BaseDir, gitService),
	}

	// Recover existing workspaces on startup
	m.recoverExistingWorkspaces()

	return m
}

// GetBaseDir returns the base directory for workspaces
func (m *Manager) GetBaseDir() string {
	return m.baseDir
}

// GetWorkspaceCount returns the current number of workspaces
func (m *Manager) GetWorkspaceCount() int {
	return m.repository.Count()
}

// RegisterWorkspace registers a workspace in the repository
func (m *Manager) RegisterWorkspace(ws *models.Workspace, pr *github.PullRequest) {
	if err := m.repository.Store(ws); err != nil {
		log.Errorf("Failed to register workspace: %v", err)
	}
}

// GetWorkspaceByPR retrieves workspace by PR (with default AI model)
func (m *Manager) GetWorkspaceByPR(pr *github.PullRequest) *models.Workspace {
	return m.GetWorkspaceByPRAndAI(pr, "")
}

// GetWorkspaceByPRAndAI retrieves workspace by PR and AI model
func (m *Manager) GetWorkspaceByPRAndAI(pr *github.PullRequest, aiModel string) *models.Workspace {
	ws, exists := m.repository.GetByPR(pr, aiModel)
	if exists {
		return ws
	}
	return nil
}

// GetAllWorkspacesByPR gets all workspaces for a PR (all AI models)
func (m *Manager) GetAllWorkspacesByPR(pr *github.PullRequest) []*models.Workspace {
	return m.repository.GetAllByPR(pr)
}

// GetWorkspaceByIssue retrieves workspace by Issue (with default AI model)
func (m *Manager) GetWorkspaceByIssue(issue *github.Issue) *models.Workspace {
	return m.GetWorkspaceByIssueAndAI(issue, "")
}

// GetWorkspaceByIssueAndAI retrieves workspace by Issue and AI model
func (m *Manager) GetWorkspaceByIssueAndAI(issue *github.Issue, aiModel string) *models.Workspace {
	ws, exists := m.repository.GetByIssue(issue, aiModel)
	if exists {
		return ws
	}
	return nil
}

// GetAllWorkspacesByIssue gets all workspaces for an Issue (all AI models)
func (m *Manager) GetAllWorkspacesByIssue(issue *github.Issue) []*models.Workspace {
	return m.repository.GetAllByIssue(issue)
}

// CreateWorkspaceFromIssue creates workspace from Issue with AI model support
func (m *Manager) CreateWorkspaceFromIssue(issue *github.Issue, aiModel string) *models.Workspace {
	return m.CreateWorkspaceFromIssueWithDefaultBranch(issue, aiModel, "")
}

// CreateWorkspaceFromIssueWithDefaultBranch creates workspace from Issue with AI model and default branch support
func (m *Manager) CreateWorkspaceFromIssueWithDefaultBranch(issue *github.Issue, aiModel, defaultBranch string) *models.Workspace {
	log.Infof("Creating workspace from Issue #%d with AI model: %s", issue.GetNumber(), aiModel)

	// Extract repository information from Issue HTML URL
	repoURL, org, repo, err := m.extractRepoURLFromIssueURL(issue.GetHTMLURL())
	if err != nil {
		log.Errorf("Failed to extract repository URL from Issue URL: %s, %v", issue.GetHTMLURL(), err)
		return nil
	}

	// Generate branch name with AI model information
	timestamp := time.Now().Unix()
	var branchName string
	if aiModel != "" {
		branchName = fmt.Sprintf("%s/%s/issue-%d-%d", BranchPrefix, aiModel, issue.GetNumber(), timestamp)
	} else {
		branchName = fmt.Sprintf("%s/issue-%d-%d", BranchPrefix, issue.GetNumber(), timestamp)
	}

	// Generate Issue workspace directory name
	issueDir := m.dirFormatter.GenerateIssueDirName(aiModel, repo, issue.GetNumber(), timestamp)
	clonePath := filepath.Join(m.baseDir, org, issueDir)

	// Get or create cached repository, then clone from cache
	cachedRepoPath, err := m.repoCacheService.GetOrCreateCachedRepoWithDefaultBranch(repoURL, org, repo, defaultBranch)
	if err != nil {
		log.Errorf("Failed to get cached repository for Issue #%d: %v", issue.GetNumber(), err)
		return nil
	}

	// Clone from cache to workspace
	if err := m.repoCacheService.CloneFromCache(cachedRepoPath, clonePath, branchName, true); err != nil {
		log.Errorf("Failed to clone from cache for Issue #%d: %v", issue.GetNumber(), err)
		return nil
	}

	// Create workspace object
	ws := &models.Workspace{
		Org:         org,
		Repo:        repo,
		AIModel:     aiModel,
		Path:        clonePath,
		SessionPath: "", // No session path at this stage
		Repository:  repoURL,
		Branch:      branchName,
		CreatedAt:   time.Now(),
		Issue:       issue,
	}

	// Store in repository
	if err := m.repository.Store(ws); err != nil {
		log.Errorf("Failed to store workspace: %v", err)
	}

	log.Infof("Successfully created workspace from Issue #%d: %s", issue.GetNumber(), clonePath)
	return ws
}

// GetOrCreateWorkspaceForIssue gets or creates workspace for Issue with AI model
func (m *Manager) GetOrCreateWorkspaceForIssue(issue *github.Issue, aiModel string) *models.Workspace {
	return m.GetOrCreateWorkspaceForIssueWithDefaultBranch(issue, aiModel, "")
}

// GetOrCreateWorkspaceForIssueWithDefaultBranch gets or creates workspace for Issue with AI model and default branch support
func (m *Manager) GetOrCreateWorkspaceForIssueWithDefaultBranch(issue *github.Issue, aiModel, defaultBranch string) *models.Workspace {
	// Try to get existing workspace for the specific AI model
	ws := m.GetWorkspaceByIssueAndAI(issue, aiModel)
	if ws != nil {
		// Validate workspace for Issue
		if m.validateWorkspaceForIssue(ws, issue) {
			log.Infof("Reusing existing workspace for Issue #%d with AI model %s: %s",
				issue.GetNumber(), aiModel, ws.Path)
			return ws
		}
		// If validation fails, cleanup old workspace
		log.Infof("Workspace validation failed for Issue #%d with AI model %s, cleaning up",
			issue.GetNumber(), aiModel)
		m.CleanupWorkspace(ws)
	}

	// Create new workspace
	log.Infof("Creating new workspace for Issue #%d with AI model: %s", issue.GetNumber(), aiModel)
	return m.CreateWorkspaceFromIssueWithDefaultBranch(issue, aiModel, defaultBranch)
}

// CreateWorkspaceFromPR creates workspace from PR with AI model support
func (m *Manager) CreateWorkspaceFromPR(pr *github.PullRequest, aiModel string) *models.Workspace {
	return m.CreateWorkspaceFromPRWithDefaultBranch(pr, aiModel, "")
}

// CreateWorkspaceFromPRWithDefaultBranch creates workspace from PR with AI model and default branch support
func (m *Manager) CreateWorkspaceFromPRWithDefaultBranch(pr *github.PullRequest, aiModel, defaultBranch string) *models.Workspace {
	log.Infof("Creating workspace from PR #%d with AI model: %s", pr.GetNumber(), aiModel)

	// Get repository URL
	repoURL := pr.GetBase().GetRepo().GetCloneURL()
	if repoURL == "" {
		log.Errorf("Failed to get repository URL for PR #%d", pr.GetNumber())
		return nil
	}

	// Get PR branch
	prBranch := pr.GetHead().GetRef()

	// Generate PR workspace directory name with AI model information
	timestamp := time.Now().Unix()
	repo := pr.GetBase().GetRepo().GetName()
	prDir := m.dirFormatter.GeneratePRDirName(aiModel, repo, pr.GetNumber(), timestamp)

	org := pr.GetBase().GetRepo().GetOwner().GetLogin()
	clonePath := filepath.Join(m.baseDir, org, prDir)

	// Get or create cached repository, then clone from cache
	cachedRepoPath, err := m.repoCacheService.GetOrCreateCachedRepoWithDefaultBranch(repoURL, org, repo, defaultBranch)
	if err != nil {
		log.Errorf("Failed to get cached repository for PR #%d: %v", pr.GetNumber(), err)
		return nil
	}

	// Clone from cache to workspace (don't create new branch, switch to existing PR branch)
	if err := m.repoCacheService.CloneFromCache(cachedRepoPath, clonePath, prBranch, false); err != nil {
		log.Errorf("Failed to clone from cache for PR #%d: %v", pr.GetNumber(), err)
		return nil
	}

	// Create session directory
	suffix := m.dirFormatter.ExtractSuffixFromPRDir(aiModel, repo, pr.GetNumber(), prDir)
	sessionPath, err := m.CreateSessionPath(filepath.Join(m.baseDir, org), aiModel, repo, pr.GetNumber(), suffix)
	if err != nil {
		log.Errorf("Failed to create session directory: %v", err)
		return nil
	}

	// Create workspace object
	ws := &models.Workspace{
		Org:         org,
		Repo:        repo,
		AIModel:     aiModel,
		PRNumber:    pr.GetNumber(),
		Path:        clonePath,
		SessionPath: sessionPath,
		Repository:  repoURL,
		Branch:      prBranch,
		PullRequest: pr,
		CreatedAt:   time.Now(),
	}

	// Store in repository
	if err := m.repository.Store(ws); err != nil {
		log.Errorf("Failed to store workspace: %v", err)
	}

	log.Infof("Created workspace from PR #%d: %s", pr.GetNumber(), ws.Path)
	return ws
}

// GetOrCreateWorkspaceForPR gets or creates workspace for PR with AI model
func (m *Manager) GetOrCreateWorkspaceForPR(pr *github.PullRequest, aiModel string) *models.Workspace {
	return m.GetOrCreateWorkspaceForPRWithDefaultBranch(pr, aiModel, "")
}

// GetOrCreateWorkspaceForPRWithDefaultBranch gets or creates workspace for PR with AI model and default branch support
func (m *Manager) GetOrCreateWorkspaceForPRWithDefaultBranch(pr *github.PullRequest, aiModel, defaultBranch string) *models.Workspace {
	// Try to get existing workspace for the specific AI model
	ws := m.GetWorkspaceByPRAndAI(pr, aiModel)
	if ws != nil {
		// Validate workspace for PR
		if m.validateWorkspaceForPR(ws, pr) {
			return ws
		}
		// If validation fails, cleanup old workspace
		log.Infof("Workspace validation failed for PR #%d with AI model %s, cleaning up", pr.GetNumber(), aiModel)
		m.CleanupWorkspace(ws)
	}

	// Create new workspace
	log.Infof("Creating new workspace for PR #%d with AI model: %s", pr.GetNumber(), aiModel)
	return m.CreateWorkspaceFromPRWithDefaultBranch(pr, aiModel, defaultBranch)
}

// CreateSessionPath creates a session directory
func (m *Manager) CreateSessionPath(underPath, aiModel, repo string, prNumber int, suffix string) (string, error) {
	sessionPath := m.dirFormatter.CreateSessionPath(underPath, aiModel, repo, prNumber, suffix)
	if err := os.MkdirAll(sessionPath, 0755); err != nil {
		log.Errorf("Failed to create session directory: %v", err)
		return "", err
	}
	return sessionPath, nil
}

// MoveIssueToPR moves Issue workspace to PR workspace (using directory rename)
func (m *Manager) MoveIssueToPR(ws *models.Workspace, prNumber int) error {
	// Build new naming: aimodel__repo__issue__number__timestamp -> aimodel__repo__pr__number__timestamp
	oldPrefix := fmt.Sprintf("%s__%s__issue__%d__", ws.AIModel, ws.Repo, ws.Issue.GetNumber())
	issueSuffix := strings.TrimPrefix(filepath.Base(ws.Path), oldPrefix)
	newCloneName := fmt.Sprintf("%s__%s__pr__%d__%s", ws.AIModel, ws.Repo, prNumber, issueSuffix)

	newClonePath := filepath.Join(filepath.Dir(ws.Path), newCloneName)
	log.Infof("Moving workspace from %s to %s", ws.Path, newClonePath)

	// Rename directory (simple filesystem operation)
	if err := os.Rename(ws.Path, newClonePath); err != nil {
		log.Errorf("Failed to rename workspace directory: %v", err)
		return fmt.Errorf("failed to rename workspace directory: %w", err)
	}

	log.Infof("Successfully moved workspace: %s -> %s", ws.Path, newClonePath)

	// Update workspace paths
	ws.Path = newClonePath
	ws.PRNumber = prNumber

	return nil
}

// CleanupWorkspace cleans up a single workspace
func (m *Manager) CleanupWorkspace(ws *models.Workspace) bool {
	if ws == nil || ws.Path == "" {
		return false
	}

	// Remove from repository
	m.repository.RemoveByWorkspace(ws)

	// Clean up physical workspace
	return m.cleanupPhysicalWorkspace(ws)
}

// GetExpiredWorkspaces returns expired workspaces based on cleanup policy
func (m *Manager) GetExpiredWorkspaces() []*models.Workspace {
	return m.repository.GetExpired(m.config.Workspace.CleanupAfter)
}

// PrepareFromEvent prepares workspace from Issue comment event
func (m *Manager) PrepareFromEvent(event *github.IssueCommentEvent) models.Workspace {
	// Issue event itself doesn't create workspace, need to create PR first
	log.Infof("Issue comment event for Issue #%d, workspace should be created after PR is created", event.Issue.GetNumber())

	// Return empty workspace indicating PR needs to be created first
	return models.Workspace{
		Issue: event.Issue,
	}
}

// ExtractAIModelFromBranch extracts AI model information from branch name
func (m *Manager) ExtractAIModelFromBranch(branchName string) string {
	// Check if it's a codeagent branch
	if !strings.HasPrefix(branchName, BranchPrefix+"/") {
		return ""
	}

	// Remove codeagent/ prefix
	branchWithoutPrefix := strings.TrimPrefix(branchName, BranchPrefix+"/")

	// Split to get aimodel part
	parts := strings.Split(branchWithoutPrefix, "/")
	if len(parts) >= 2 {
		aiModel := parts[0]
		// Validate if it's a valid AI model
		if aiModel == "claude" || aiModel == "gemini" {
			return aiModel
		}
	}

	return m.config.CodeProvider
}

// Directory format delegation methods
func (m *Manager) GenerateIssueDirName(aiModel, repo string, issueNumber int, timestamp int64) string {
	return m.dirFormatter.GenerateIssueDirName(aiModel, repo, issueNumber, timestamp)
}

func (m *Manager) GeneratePRDirName(aiModel, repo string, prNumber int, timestamp int64) string {
	return m.dirFormatter.GeneratePRDirName(aiModel, repo, prNumber, timestamp)
}

func (m *Manager) GenerateSessionDirName(aiModel, repo string, prNumber int, timestamp int64) string {
	return m.dirFormatter.GenerateSessionDirName(aiModel, repo, prNumber, timestamp)
}

func (m *Manager) ParsePRDirName(dirName string) (*PRDirFormat, error) {
	return m.dirFormatter.ParsePRDirName(dirName)
}

func (m *Manager) ExtractSuffixFromPRDir(aiModel, repo string, prNumber int, dirName string) string {
	return m.dirFormatter.ExtractSuffixFromPRDir(aiModel, repo, prNumber, dirName)
}

func (m *Manager) ExtractSuffixFromIssueDir(aiModel, repo string, issueNumber int, dirName string) string {
	return m.dirFormatter.ExtractSuffixFromIssueDir(aiModel, repo, issueNumber, dirName)
}

// Private helper methods

// cleanupPhysicalWorkspace cleans up the physical workspace directories and containers
func (m *Manager) cleanupPhysicalWorkspace(ws *models.Workspace) bool {
	cloneRemoved := false
	sessionRemoved := false

	// Remove cloned repository directory
	if ws.Path != "" {
		if err := m.cleanupClonedRepository(ws.Path); err != nil {
			log.Errorf("Failed to remove cloned repository %s: %v", ws.Path, err)
		} else {
			cloneRemoved = true
			log.Infof("Successfully removed cloned repository: %s", ws.Path)
		}
	}

	// Remove session directory
	if ws.SessionPath != "" {
		if err := os.RemoveAll(ws.SessionPath); err != nil {
			log.Errorf("Failed to remove session directory %s: %v", ws.SessionPath, err)
		} else {
			sessionRemoved = true
			log.Infof("Successfully removed session directory: %s", ws.SessionPath)
		}
	}

	// Clean up related Docker containers
	if err := m.containerService.CleanupWorkspaceContainers(ws); err != nil {
		log.Warnf("Failed to cleanup containers for workspace %s: %v", ws.Path, err)
	}

	// Only return true if both clone and session are cleaned successfully
	return cloneRemoved && sessionRemoved
}

// cleanupClonedRepository removes a cloned repository directory
func (m *Manager) cleanupClonedRepository(clonePath string) error {
	if clonePath == "" {
		return nil
	}

	log.Infof("Cleaning up cloned repository: %s", clonePath)

	// Remove the entire directory
	if err := os.RemoveAll(clonePath); err != nil {
		return fmt.Errorf("failed to remove cloned repository directory: %w", err)
	}

	log.Infof("Successfully removed cloned repository: %s", clonePath)
	return nil
}

// validateWorkspaceForPR validates workspace for PR branch alignment
func (m *Manager) validateWorkspaceForPR(ws *models.Workspace, pr *github.PullRequest) bool {
	// Check if workspace path exists
	if _, err := os.Stat(ws.Path); os.IsNotExist(err) {
		log.Infof("Workspace path does not exist: %s", ws.Path)
		return false
	}

	// Check if workspace is on correct branch
	expectedBranch := pr.GetHead().GetRef()
	return m.gitService.ValidateBranch(ws.Path, expectedBranch)
}

// validateWorkspaceForIssue validates workspace for Issue
func (m *Manager) validateWorkspaceForIssue(ws *models.Workspace, issue *github.Issue) bool {
	// Check if workspace path exists
	if _, err := os.Stat(ws.Path); os.IsNotExist(err) {
		log.Infof("Workspace path does not exist: %s", ws.Path)
		return false
	}

	// For Issue workspace, check if workspace is on the correct branch
	// Issue workspace should be on its own branch created for the issue
	if ws.Branch == "" {
		log.Infof("Workspace branch is empty: %s", ws.Path)
		return false
	}

	// Validate the branch exists in the workspace
	return m.gitService.ValidateBranch(ws.Path, ws.Branch)
}

// extractRepoURLFromIssueURL extracts repository URL from Issue URL
func (m *Manager) extractRepoURLFromIssueURL(issueURL string) (url, org, repo string, err error) {
	// Issue URL format: https://github.com/owner/repo/issues/123
	if strings.Contains(issueURL, "github.com") {
		parts := strings.Split(issueURL, "/")
		if len(parts) >= 4 {
			org = parts[len(parts)-4]
			repo = parts[len(parts)-3]
			url = fmt.Sprintf("https://github.com/%s/%s.git", org, repo)
			return
		}
	}
	return "", "", "", fmt.Errorf("failed to extract repository URL from Issue URL: %s", issueURL)
}

// recoverExistingWorkspaces scans directories and recovers existing cloned workspaces
func (m *Manager) recoverExistingWorkspaces() {
	log.Infof("Starting to recover existing workspaces from %s", m.baseDir)

	entries, err := os.ReadDir(m.baseDir)
	if err != nil && !os.IsNotExist(err) {
		log.Errorf("Failed to read base directory: %v", err)
		return
	}

	recoveredCount := 0
	for _, orgEntry := range entries {
		if !orgEntry.IsDir() {
			continue
		}

		// Organization
		org := orgEntry.Name()
		orgPath := filepath.Join(m.baseDir, org)

		// Read all directories under organization
		orgEntries, err := os.ReadDir(orgPath)
		if err != nil {
			log.Warnf("Failed to read org directory %s: %v", orgPath, err)
			continue
		}

		// Scan all directories to find workspace-formatted ones
		for _, entry := range orgEntries {
			if !entry.IsDir() {
				continue
			}

			dirName := entry.Name()
			dirPath := filepath.Join(orgPath, dirName)

			// Check if it's a PR workspace directory: contains "__pr__"
			if !strings.Contains(dirName, "__pr__") {
				continue
			}

			// Check if it's a valid git repository (cloned workspace should contain complete .git directory)
			if _, err := os.Stat(filepath.Join(dirPath, ".git")); os.IsNotExist(err) {
				log.Warnf("Directory %s does not contain .git, skipping", dirPath)
				continue
			}

			// Parse directory name using formatter
			prFormat, err := m.dirFormatter.ParsePRDirName(dirName)
			if err != nil {
				log.Errorf("Failed to parse PR directory name %s: %v, skipping", dirName, err)
				continue
			}

			aiModel := prFormat.AIModel
			repoName := prFormat.Repo
			prNumber := prFormat.PRNumber

			// Get remote repository URL
			remoteURL, err := m.gitService.GetRemoteURL(dirPath)
			if err != nil {
				log.Warnf("Failed to get remote URL for %s: %v", dirPath, err)
				continue
			}

			// Recover PR workspace
			if err := m.recoverPRWorkspaceFromClone(org, repoName, dirPath, remoteURL, prNumber, aiModel, prFormat.Timestamp); err != nil {
				log.Errorf("Failed to recover PR workspace %s: %v", dirName, err)
			} else {
				recoveredCount++
			}
		}
	}

	log.Infof("Workspace recovery completed. Recovered %d workspaces", recoveredCount)
}

// recoverPRWorkspaceFromClone recovers a single PR workspace from clone
func (m *Manager) recoverPRWorkspaceFromClone(org, repo, clonePath, remoteURL string, prNumber int, aiModel string, timestamp int64) error {
	// Create corresponding session directory (same level as clone directory)
	sessionPath := m.dirFormatter.CreateSessionPathWithTimestamp(m.baseDir, aiModel, repo, prNumber, timestamp)

	// Get current branch information
	currentBranch, err := m.gitService.GetCurrentBranch(clonePath)
	if err != nil {
		log.Warnf("Failed to get current branch for %s: %v", clonePath, err)
		currentBranch = ""
	}

	// Recover workspace object
	ws := &models.Workspace{
		Org:         org,
		Repo:        repo,
		AIModel:     aiModel,
		Path:        clonePath,
		PRNumber:    prNumber,
		SessionPath: sessionPath,
		Repository:  remoteURL,
		Branch:      currentBranch,
		CreatedAt:   time.Unix(timestamp, 0),
	}

	// Store in repository
	if err := m.repository.Store(ws); err != nil {
		return fmt.Errorf("failed to store recovered workspace: %w", err)
	}

	log.Infof("Recovered PR workspace from clone: %v", ws)
	return nil
}
