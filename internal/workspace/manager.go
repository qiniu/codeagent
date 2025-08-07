package workspace

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/qiniu/codeagent/internal/config"
	"github.com/qiniu/codeagent/pkg/models"

	"github.com/google/go-github/v58/github"
	"github.com/qiniu/x/log"
)

const (
	// BranchPrefix branch name prefix used to identify branches created by codeagent
	BranchPrefix = "codeagent"
)

func key(orgRepo string, prNumber int) string {
	return fmt.Sprintf("%s/%d", orgRepo, prNumber)
}

func keyWithAI(orgRepo string, prNumber int, aiModel string) string {
	if aiModel == "" {
		return fmt.Sprintf("%s/%d", orgRepo, prNumber)
	}
	return fmt.Sprintf("%s/%s/%d", aiModel, orgRepo, prNumber)
}

// Directory format related public methods

// GenerateIssueDirName generates Issue directory name
func (m *Manager) GenerateIssueDirName(aiModel, repo string, issueNumber int, timestamp int64) string {
	return m.dirFormatter.generateIssueDirName(aiModel, repo, issueNumber, timestamp)
}

// GeneratePRDirName generates PR directory name
func (m *Manager) GeneratePRDirName(aiModel, repo string, prNumber int, timestamp int64) string {
	return m.dirFormatter.generatePRDirName(aiModel, repo, prNumber, timestamp)
}

// GenerateSessionDirName generates Session directory name
func (m *Manager) GenerateSessionDirName(aiModel, repo string, prNumber int, timestamp int64) string {
	return m.dirFormatter.generateSessionDirName(aiModel, repo, prNumber, timestamp)
}

// ParsePRDirName parses PR directory name
func (m *Manager) ParsePRDirName(dirName string) (*PRDirFormat, error) {
	return m.dirFormatter.parsePRDirName(dirName)
}

// ExtractSuffixFromPRDir extracts suffix from PR directory name
func (m *Manager) ExtractSuffixFromPRDir(aiModel, repo string, prNumber int, dirName string) string {
	return m.dirFormatter.extractSuffixFromPRDir(aiModel, repo, prNumber, dirName)
}

// ExtractSuffixFromIssueDir extracts suffix from Issue directory name
func (m *Manager) ExtractSuffixFromIssueDir(aiModel, repo string, issueNumber int, dirName string) string {
	return m.dirFormatter.extractSuffixFromIssueDir(aiModel, repo, issueNumber, dirName)
}

// createSessionPath creates Session directory path
func (m *Manager) createSessionPath(underPath, aiModel, repo string, prNumber int, suffix string) string {
	return m.dirFormatter.createSessionPath(underPath, aiModel, repo, prNumber, suffix)
}

// createSessionPathWithTimestamp creates Session directory path with timestamp
func (m *Manager) createSessionPathWithTimestamp(underPath, aiModel, repo string, prNumber int, timestamp int64) string {
	return m.dirFormatter.createSessionPathWithTimestamp(underPath, aiModel, repo, prNumber, timestamp)
}

// ExtractAIModelFromBranch extracts AI model information from branch name
// Branch format: codeagent/{aimodel}/{type}-{number}-{timestamp}
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
		// Verify if it's a valid AI model
		if aiModel == "claude" || aiModel == "gemini" {
			return aiModel
		}
	}

	return ""
}

type Manager struct {
	baseDir string

	// key: aimodel/org/repo/pr-number
	workspaces map[string]*models.Workspace
	// key: org/repo
	repoManagers map[string]*RepoManager
	mutex        sync.RWMutex
	config       *config.Config

	// Directory format manager
	dirFormatter *dirFormatter
}

func NewManager(cfg *config.Config) *Manager {
	m := &Manager{
		baseDir:      cfg.Workspace.BaseDir,
		workspaces:   make(map[string]*models.Workspace),
		repoManagers: make(map[string]*RepoManager),
		config:       cfg,
		dirFormatter: newDirFormatter(),
	}

	// Recover existing workspaces at startup
	m.recoverExistingWorkspaces()

	return m
}

// CleanupWorkspace cleans up a single workspace, returns whether cleanup was successful
func (m *Manager) CleanupWorkspace(ws *models.Workspace) bool {
	if ws == nil || ws.Path == "" {
		return false
	}

	// Clean up memory mappings
	m.mutex.Lock()
	delete(m.workspaces, keyWithAI(fmt.Sprintf("%s/%s", ws.Org, ws.Repo), ws.PRNumber, ws.AIModel))
	m.mutex.Unlock()

	// Clean up physical workspace
	return m.cleanupWorkspaceWithWorktree(ws)
}

// cleanupWorkspaceWithWorktree cleans up worktree workspace, returns whether cleanup was successful
func (m *Manager) cleanupWorkspaceWithWorktree(ws *models.Workspace) bool {
	// Extract number from workspace path
	worktreeDir := filepath.Base(ws.Path)
	var entityNumber int

	// Extract number based on directory type
	if strings.Contains(worktreeDir, "-pr-") {
		entityNumber = m.extractPRNumberFromPRDir(worktreeDir)
	} else if strings.Contains(worktreeDir, "-issue-") {
		entityNumber = m.extractIssueNumberFromIssueDir(worktreeDir)
	}

	if entityNumber == 0 {
		log.Warnf("Could not extract entity number from workspace path: %s", ws.Path)
		return false
	}

	// Get repo manager (without lock to avoid deadlock)
	orgRepoPath := fmt.Sprintf("%s/%s", ws.Org, ws.Repo)
	var repoManager *RepoManager

	m.mutex.RLock()
	if rm, exists := m.repoManagers[orgRepoPath]; exists {
		repoManager = rm
	}
	m.mutex.RUnlock()

	if repoManager == nil {
		log.Warnf("Repo manager not found for %s", orgRepoPath)
		// Even without repoManager, try to delete session directory
		if ws.SessionPath != "" {
			if err := os.RemoveAll(ws.SessionPath); err != nil {
				log.Errorf("Failed to remove session directory %s: %v", ws.SessionPath, err)
			} else {
				log.Infof("Removed session directory: %s", ws.SessionPath)
			}
		}
		return false
	}

	// Remove worktree
	worktreeRemoved := false
	if err := repoManager.RemoveWorktreeWithAI(entityNumber, ws.AIModel); err != nil {
		log.Errorf("Failed to remove worktree for entity #%d with AI model %s: %v", entityNumber, ws.AIModel, err)
	} else {
		worktreeRemoved = true
		log.Infof("Successfully removed worktree for entity #%d with AI model %s", entityNumber, ws.AIModel)
	}

	// Delete session directory
	sessionRemoved := false
	if ws.SessionPath != "" {
		if err := os.RemoveAll(ws.SessionPath); err != nil {
			log.Errorf("Failed to remove session directory %s: %v", ws.SessionPath, err)
		} else {
			sessionRemoved = true
			log.Infof("Successfully removed session directory: %s", ws.SessionPath)
		}
	}

	// Only return true if both worktree and session are cleaned successfully
	return worktreeRemoved && sessionRemoved
}

// PrepareFromEvent prepares workspace from complete IssueCommentEvent
func (m *Manager) PrepareFromEvent(event *github.IssueCommentEvent) models.Workspace {
	// Issue events themselves don't create workspaces, need to create PR first
	log.Infof("Issue comment event for Issue #%d, but workspace should be created after PR is created", event.Issue.GetNumber())

	// Return empty workspace, indicating PR needs to be created first
	return models.Workspace{
		Issue: event.Issue,
	}
}

// recoverExistingWorkspaces scans directory names to recover existing workspaces
func (m *Manager) recoverExistingWorkspaces() {
	log.Infof("Starting to recover existing workspaces by scanning directory names from %s", m.baseDir)

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

		// Scan all directories to find workspaces matching the format
		for _, entry := range orgEntries {
			if !entry.IsDir() {
				continue
			}

			dirName := entry.Name()
			dirPath := filepath.Join(orgPath, dirName)

			// Check if it's a PR workspace directory: {repo}-pr-{pr-number}-{timestamp} or {aimodel}-{repo}-pr-{pr-number}-{timestamp}
			if !strings.Contains(dirName, "-pr-") {
				continue
			}

			parts := strings.Split(dirName, "-pr-")
			if len(parts) < 2 {
				log.Warnf("Invalid PR workspace directory name: %s", dirName)
				continue
			}

			// Extract repository name and AI model - require directory must contain aimodel information
			var repoName, aiModel string
			if strings.Contains(parts[0], "-") {
				// Contains AI model: aimodel-repo
				aiModelParts := strings.Split(parts[0], "-")
				if len(aiModelParts) == 2 {
					aiModel = aiModelParts[0]
					repoName = aiModelParts[1]
				} else {
					log.Errorf("Invalid PR workspace directory name with AI model: %s, skipping", dirName)
					continue
				}
			} else {
				// Directory without AI model, skip directly
				log.Errorf("PR workspace directory must contain AI model info: %s, skipping", dirName)
				continue
			}

			// Extract PR number
			numberParts := strings.Split(parts[1], "-")
			if len(numberParts) < 2 {
				log.Warnf("Invalid PR workspace directory name: %s", dirName)
				continue
			}

			if prNumber, err := strconv.Atoi(numberParts[0]); err == nil && prNumber > 0 {
				// Find corresponding repository directory
				repoPath := filepath.Join(orgPath, repoName)
				if _, err := os.Stat(filepath.Join(repoPath, ".git")); err == nil {
					// Get remote repository URL
					remoteURL, err := m.getRemoteURL(repoPath)
					if err != nil {
						log.Warnf("Failed to get remote URL for %s: %v", repoPath, err)
						continue
					}

					// Create repository manager
					orgRepoPath := fmt.Sprintf("%s/%s", org, repoName)
					m.mutex.Lock()
					if m.repoManagers[orgRepoPath] == nil {
						repoManager := NewRepoManager(repoPath, remoteURL)
						// Recover worktrees
						if err := repoManager.RestoreWorktrees(); err != nil {
							log.Warnf("Failed to restore worktrees for %s: %v", orgRepoPath, err)
						}
						m.repoManagers[orgRepoPath] = repoManager
						log.Infof("Created repo manager for %s", orgRepoPath)
					}
					m.mutex.Unlock()

					// Recover PR workspace
					if err := m.recoverPRWorkspace(org, repoName, dirPath, remoteURL, prNumber, aiModel); err != nil {
						log.Errorf("Failed to recover PR workspace %s: %v", dirName, err)
					} else {
						recoveredCount++
					}
				}
			}

		}
	}

	log.Infof("Workspace recovery completed. Recovered %d workspaces", recoveredCount)
}

// recoverPRWorkspace recovers a single PR workspace
func (m *Manager) recoverPRWorkspace(org, repo, worktreePath, remoteURL string, prNumber int, aiModel string) error {
	// Extract PR information from worktree path
	worktreeDir := filepath.Base(worktreePath)
	var timestamp string

	if aiModel != "" {
		// With AI model: aimodel-repo-pr-number-timestamp
		timestamp = strings.TrimPrefix(worktreeDir, aiModel+"-"+repo+"-pr-"+strconv.Itoa(prNumber)+"-")
	} else {
		// Without AI model: repo-pr-number-timestamp
		timestamp = strings.TrimPrefix(worktreeDir, repo+"-pr-"+strconv.Itoa(prNumber)+"-")
	}

	// Convert timestamp string to time
	var createdAt time.Time
	if timestampInt, err := strconv.ParseInt(timestamp, 10, 64); err == nil {
		createdAt = time.Unix(timestampInt, 0)
	} else {
		log.Errorf("Failed to parse timestamp %s, using current time: %v", timestamp, err)
		return fmt.Errorf("failed to parse timestamp %s", timestamp)
	}

	// Create corresponding session directory (same level as repo)
	// session directory format: {aiModel}-{repo}-session-{prNumber}-{timestamp}
	timestampInt, _ := strconv.ParseInt(timestamp, 10, 64)
	sessionPath := m.createSessionPathWithTimestamp(m.baseDir, aiModel, repo, prNumber, timestampInt)

	// Recover workspace object
	ws := &models.Workspace{
		Org:         org,
		Repo:        repo,
		AIModel:     aiModel,
		Path:        worktreePath,
		PRNumber:    prNumber,
		SessionPath: sessionPath,
		Repository:  remoteURL,
		CreatedAt:   createdAt,
	}

	// Register to memory mapping
	orgRepoPath := fmt.Sprintf("%s/%s", org, repo)
	prKey := keyWithAI(orgRepoPath, prNumber, aiModel)
	m.mutex.Lock()
	m.workspaces[prKey] = ws
	m.mutex.Unlock()

	log.Infof("Recovered PR workspace: %v", ws)
	return nil
}

// getRemoteURL gets remote repository URL
func (m *Manager) getRemoteURL(repoPath string) (string, error) {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// getOrCreateRepoManager gets or creates repository manager
func (m *Manager) getOrCreateRepoManager(org, repo string) *RepoManager {
	orgRepo := fmt.Sprintf("%s/%s", org, repo)
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Check if already exists
	if repoManager, exists := m.repoManagers[orgRepo]; exists {
		return repoManager
	}

	// Create new repository manager
	repoPath := filepath.Join(m.baseDir, orgRepo)
	repoManager := NewRepoManager(repoPath, fmt.Sprintf("https://github.com/%s/%s.git", org, repo))
	m.repoManagers[orgRepo] = repoManager

	return repoManager
}

// extractOrgRepoPath extracts org/repo path from repository URL
func (m *Manager) extractOrgRepoPath(repoURL string) string {
	// Remove .git suffix
	repoURL = strings.TrimSuffix(repoURL, ".git")

	// Handle SSH URL format: git@github.com:org/repo
	if strings.HasPrefix(repoURL, "git@") {
		// Split git@github.com:org/repo
		parts := strings.Split(repoURL, ":")
		if len(parts) == 2 {
			// Split org/repo
			repoParts := strings.Split(parts[1], "/")
			if len(repoParts) >= 2 {
				return fmt.Sprintf("%s/%s", repoParts[0], repoParts[1])
			}
		}
		return "unknown/unknown"
	}

	// Handle HTTPS URL format: https://github.com/org/repo
	parts := strings.Split(repoURL, "/")
	if len(parts) >= 2 {
		// Extract org/repo from https://github.com/org/repo
		return fmt.Sprintf("%s/%s", parts[len(parts)-2], parts[len(parts)-1])
	}

	return "unknown/unknown"
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

// RegisterWorkspace registers workspace
func (m *Manager) RegisterWorkspace(ws *models.Workspace, pr *github.PullRequest) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	prKey := keyWithAI(fmt.Sprintf("%s/%s", ws.Org, ws.Repo), pr.GetNumber(), ws.AIModel)
	if _, exists := m.workspaces[prKey]; exists {
		// This shouldn't error, because one PR can only have one workspace
		log.Errorf("Workspace %s already registered", prKey)
		return
	}

	m.workspaces[prKey] = ws
	log.Infof("Registered workspace: %s, %s", prKey, ws.Path)
}

// GetWorkspaceCount gets current workspace count
func (m *Manager) GetWorkspaceCount() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return len(m.workspaces)
}

// GetRepoManagerCount gets repository manager count
func (m *Manager) GetRepoManagerCount() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return len(m.repoManagers)
}

// GetWorktreeCount gets total worktree count
func (m *Manager) GetWorktreeCount() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	total := 0
	for _, repoManager := range m.repoManagers {
		total += repoManager.GetWorktreeCount()
	}
	return total
}

func (m *Manager) CreateSessionPath(underPath, aiModel, repo string, prNumber int, suffix string) (string, error) {
	// session directory format: {aiModel}-{repo}-session-{prNumber}-{timestamp}
	// Keep only timestamp part to avoid duplicate information
	sessionPath := m.createSessionPath(underPath, aiModel, repo, prNumber, suffix)
	if err := os.MkdirAll(sessionPath, 0755); err != nil {
		log.Errorf("Failed to create session directory: %v", err)
		return "", err
	}
	return sessionPath, nil
}

// CreateWorkspaceFromIssue creates workspace from Issue
func (m *Manager) CreateWorkspaceFromIssue(issue *github.Issue) *models.Workspace {
	return m.CreateWorkspaceFromIssueWithAI(issue, "")
}

// CreateWorkspaceFromIssueWithAI creates workspace from Issue with AI model support
func (m *Manager) CreateWorkspaceFromIssueWithAI(issue *github.Issue, aiModel string) *models.Workspace {
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

	// Generate Issue workspace directory name (same level as repo) with AI model information
	issueDir := m.GenerateIssueDirName(aiModel, repo, issue.GetNumber(), timestamp)

	// Get or create repository manager
	repoManager := m.getOrCreateRepoManager(org, repo)

	// Create worktree
	worktree, err := repoManager.CreateWorktreeWithName(issueDir, branchName, true)
	if err != nil {
		log.Errorf("Failed to create worktree for Issue #%d: %v", issue.GetNumber(), err)
		return nil
	}

	// Create workspace object
	ws := &models.Workspace{
		Org:     org,
		Repo:    repo,
		AIModel: aiModel,
		Path:    worktree.Worktree,
		// No session directory at this stage
		SessionPath: "",
		Repository:  repoURL,
		Branch:      worktree.Branch,
		CreatedAt:   time.Now(),
		Issue:       issue,
	}

	log.Infof("Created workspace from Issue #%d: %s", issue.GetNumber(), ws.Path)
	return ws
}

// MoveIssueToPR uses git worktree move to move Issue workspace to PR workspace
func (m *Manager) MoveIssueToPR(ws *models.Workspace, prNumber int) error {
	// Build new naming: aimodel-repo-issue-number-timestamp -> aimodel-repo-pr-number-timestamp
	oldPrefix := fmt.Sprintf("%s-%s-issue-%d-", ws.AIModel, ws.Repo, ws.Issue.GetNumber())
	issueSuffix := strings.TrimPrefix(filepath.Base(ws.Path), oldPrefix)
	newWorktreeName := fmt.Sprintf("%s-%s-pr-%d-%s", ws.AIModel, ws.Repo, prNumber, issueSuffix)

	newWorktreePath := filepath.Join(filepath.Dir(ws.Path), newWorktreeName)
	log.Infof("try to move workspace from %s to %s", ws.Path, newWorktreePath)

	// Get repository manager
	orgRepoPath := fmt.Sprintf("%s/%s", ws.Org, ws.Repo)
	repoManager := m.repoManagers[orgRepoPath]
	if repoManager == nil {
		return fmt.Errorf("repo manager not found for %s", orgRepoPath)
	}

	// Execute git worktree move command
	cmd := exec.Command("git", "worktree", "move", ws.Path, newWorktreePath)
	cmd.Dir = repoManager.GetRepoPath() // Execute in Git repository root directory

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Errorf("Failed to move worktree: %v, output: %s", err, string(output))
		return fmt.Errorf("failed to move worktree: %w, output: %s", err, string(output))
	}

	log.Infof("Successfully moved worktree: %s -> %s", ws.Path, newWorktreeName)

	// Update workspace path
	ws.Path = newWorktreePath

	// After moving, register worktree to memory
	worktree := &WorktreeInfo{
		Worktree: ws.Path,
		Branch:   ws.Branch,
	}
	repoManager.RegisterWorktreeWithAI(prNumber, ws.AIModel, worktree)
	return nil
}

func (m *Manager) GetWorkspaceByPR(pr *github.PullRequest) *models.Workspace {
	return m.GetWorkspaceByPRAndAI(pr, "")
}

// GetAllWorkspacesByPR gets all workspaces for PR (all AI models)
func (m *Manager) GetAllWorkspacesByPR(pr *github.PullRequest) []*models.Workspace {
	orgRepoPath := fmt.Sprintf("%s/%s", pr.GetBase().GetRepo().GetOwner().GetLogin(), pr.GetBase().GetRepo().GetName())
	prNumber := pr.GetNumber()

	var workspaces []*models.Workspace

	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// Iterate through all workspaces and find those related to this PR
	for _, ws := range m.workspaces {
		// Check if this is a workspace for this PR
		if ws.PRNumber == prNumber &&
			fmt.Sprintf("%s/%s", ws.Org, ws.Repo) == orgRepoPath {
			workspaces = append(workspaces, ws)
		}
	}

	return workspaces
}

func (m *Manager) GetWorkspaceByPRAndAI(pr *github.PullRequest, aiModel string) *models.Workspace {
	orgRepoPath := fmt.Sprintf("%s/%s", pr.GetBase().GetRepo().GetOwner().GetLogin(), pr.GetBase().GetRepo().GetName())
	prKey := keyWithAI(orgRepoPath, pr.GetNumber(), aiModel)
	m.mutex.RLock()
	if ws, exists := m.workspaces[prKey]; exists {
		m.mutex.RUnlock()
		log.Infof("Found existing workspace for PR #%d with AI model %s: %s", pr.GetNumber(), aiModel, ws.Path)
		return ws
	}
	m.mutex.RUnlock()
	return nil
}

// CreateWorkspaceFromPR creates workspace from PR (directly contains PR number)
func (m *Manager) CreateWorkspaceFromPR(pr *github.PullRequest) *models.Workspace {
	return m.CreateWorkspaceFromPRWithAI(pr, "")
}

// CreateWorkspaceFromPRWithAI creates workspace from PR with AI model support
func (m *Manager) CreateWorkspaceFromPRWithAI(pr *github.PullRequest, aiModel string) *models.Workspace {
	log.Infof("Creating workspace from PR #%d with AI model: %s", pr.GetNumber(), aiModel)

	// Get repository URL
	repoURL := pr.GetBase().GetRepo().GetCloneURL()
	if repoURL == "" {
		log.Errorf("Failed to get repository URL for PR #%d", pr.GetNumber())
		return nil
	}

	// Get PR branch
	prBranch := pr.GetHead().GetRef()

	// Generate PR workspace directory name (same level as repo) with AI model information
	timestamp := time.Now().Unix()
	repo := pr.GetBase().GetRepo().GetName()
	prDir := m.GeneratePRDirName(aiModel, repo, pr.GetNumber(), timestamp)

	org := pr.GetBase().GetRepo().GetOwner().GetLogin()

	// Get or create repository manager
	repoManager := m.getOrCreateRepoManager(org, repo)

	// Create worktree (don't create new branch, switch to existing branch)
	worktree, err := repoManager.CreateWorktreeWithName(prDir, prBranch, false)
	if err != nil {
		log.Errorf("Failed to create worktree for PR #%d: %v", pr.GetNumber(), err)
		return nil
	}

	// Register worktree to memory
	repoManager.RegisterWorktreeWithAI(pr.GetNumber(), aiModel, worktree)

	// Create session directory
	// Extract suffix from PR directory name, supporting new directory format: {aiModel}-{repo}-pr-{prNumber}-{timestamp}
	prDirName := filepath.Base(worktree.Worktree)
	suffix := m.ExtractSuffixFromPRDir(aiModel, repo, pr.GetNumber(), prDirName)
	sessionPath, err := m.CreateSessionPath(filepath.Dir(repoManager.GetRepoPath()), aiModel, repo, pr.GetNumber(), suffix)
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
		Path:        worktree.Worktree,
		SessionPath: sessionPath,
		Repository:  repoURL,
		Branch:      worktree.Branch,
		PullRequest: pr,
		CreatedAt:   time.Now(),
	}

	// Register to memory mapping
	prKey := keyWithAI(fmt.Sprintf("%s/%s", org, repo), pr.GetNumber(), aiModel)
	m.mutex.Lock()
	m.workspaces[prKey] = ws
	m.mutex.Unlock()

	log.Infof("Created workspace from PR #%d: %s", pr.GetNumber(), ws.Path)
	return ws
}

// GetOrCreateWorkspaceForPR gets or creates workspace for PR
func (m *Manager) GetOrCreateWorkspaceForPR(pr *github.PullRequest) *models.Workspace {
	return m.GetOrCreateWorkspaceForPRWithAI(pr, "")
}

// GetOrCreateWorkspaceForPRWithAI gets or creates workspace for PR with AI model support
func (m *Manager) GetOrCreateWorkspaceForPRWithAI(pr *github.PullRequest, aiModel string) *models.Workspace {
	// 1. First try to get workspace for corresponding AI model from memory
	ws := m.GetWorkspaceByPRAndAI(pr, aiModel)
	if ws != nil {
		// Validate if workspace corresponds to correct PR branch
		if m.validateWorkspaceForPR(ws, pr) {
			return ws
		}
		// If validation fails, clean up old workspace
		log.Infof("Workspace validation failed for PR #%d with AI model %s, cleaning up old workspace", pr.GetNumber(), aiModel)
		m.CleanupWorkspace(ws)
	}

	// 2. Create new workspace
	log.Infof("Creating new workspace for PR #%d with AI model: %s", pr.GetNumber(), aiModel)
	return m.CreateWorkspaceFromPRWithAI(pr, aiModel)
}

// validateWorkspaceForPR validates if workspace corresponds to correct PR branch
func (m *Manager) validateWorkspaceForPR(ws *models.Workspace, pr *github.PullRequest) bool {
	// Check if workspace path exists
	if _, err := os.Stat(ws.Path); os.IsNotExist(err) {
		log.Infof("Workspace path does not exist: %s", ws.Path)
		return false
	}

	// Check if workspace is on correct branch
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = ws.Path
	output, err := cmd.Output()
	if err != nil {
		log.Infof("Failed to get current branch for workspace: %v", err)
		return false
	}

	currentBranch := strings.TrimSpace(string(output))
	expectedBranch := pr.GetHead().GetRef()

	log.Infof("Workspace branch validation: current=%s, expected=%s", currentBranch, expectedBranch)

	// Check if on correct branch or in detached HEAD state
	if currentBranch == expectedBranch {
		return true
	}

	// If detached HEAD, check if pointing to correct commit
	if currentBranch == "HEAD" {
		// Get current commit
		cmd = exec.Command("git", "rev-parse", "HEAD")
		cmd.Dir = ws.Path
		output, err = cmd.Output()
		if err != nil {
			log.Infof("Failed to get current commit for workspace: %v", err)
			return false
		}
		currentCommit := strings.TrimSpace(string(output))

		// Get latest commit of expected branch
		cmd = exec.Command("git", "rev-parse", fmt.Sprintf("origin/%s", expectedBranch))
		cmd.Dir = ws.Path
		output, err = cmd.Output()
		if err != nil {
			log.Infof("Failed to get expected branch commit: %v", err)
			return false
		}
		expectedCommit := strings.TrimSpace(string(output))

		log.Infof("Commit validation: current=%s, expected=%s", currentCommit, expectedCommit)
		return currentCommit == expectedCommit
	}

	return false
}

// extractPRNumberFromPRDir extracts PR number from PR directory name
func (m *Manager) extractPRNumberFromPRDir(prDir string) int {
	// PR directory format:
	// - {aiModel}-{repo}-pr-{number}-{timestamp} (with AI model)
	// - {repo}-pr-{number}-{timestamp} (without AI model)
	if strings.Contains(prDir, "-pr-") {
		parts := strings.Split(prDir, "-pr-")
		if len(parts) >= 2 {
			numberParts := strings.Split(parts[1], "-")
			if len(numberParts) >= 1 {
				if number, err := strconv.Atoi(numberParts[0]); err == nil {
					return number
				}
			}
		}
	}
	return 0
}

// extractIssueNumberFromIssueDir extracts Issue number from Issue directory name
func (m *Manager) extractIssueNumberFromIssueDir(issueDir string) int {
	// Issue directory format:
	// - {aiModel}-{repo}-issue-{number}-{timestamp} (with AI model)
	// - {repo}-issue-{number}-{timestamp} (without AI model)
	if strings.Contains(issueDir, "-issue-") {
		parts := strings.Split(issueDir, "-issue-")
		if len(parts) >= 2 {
			numberParts := strings.Split(parts[1], "-")
			if len(numberParts) >= 1 {
				if number, err := strconv.Atoi(numberParts[0]); err == nil {
					return number
				}
			}
		}
	}
	return 0
}

func (m *Manager) GetExpiredWorkspaces() []*models.Workspace {
	expiredWorkspaces := []*models.Workspace{}
	now := time.Now()
	m.mutex.RLock()
	for _, ws := range m.workspaces {
		if now.Sub(ws.CreatedAt) > m.config.Workspace.CleanupAfter {
			expiredWorkspaces = append(expiredWorkspaces, ws)
		}
	}
	m.mutex.RUnlock()

	return expiredWorkspaces
}
