package workspace

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/go-github/v58/github"
	"github.com/qiniu/codeagent/pkg/models"
	"github.com/qiniu/x/log"
)

// WorkspaceRepository manages workspace storage and retrieval
type WorkspaceRepository interface {
	Store(ws *models.Workspace) error
	GetByKey(key string) (*models.Workspace, bool)
	GetByPR(pr *github.PullRequest, aiModel string) (*models.Workspace, bool)
	GetAllByPR(pr *github.PullRequest) []*models.Workspace
	GetByIssue(issue *github.Issue, aiModel string) (*models.Workspace, bool)
	GetAllByIssue(issue *github.Issue) []*models.Workspace
	Remove(key string) bool
	RemoveByWorkspace(ws *models.Workspace) bool
	GetExpired(cleanupAfter time.Duration) []*models.Workspace
	Count() int
	GetAll() map[string]*models.Workspace
}

type workspaceRepository struct {
	// key: aimodel/org/repo/pr-number
	workspaces map[string]*models.Workspace
	mutex      sync.RWMutex
}

// NewWorkspaceRepository creates a new workspace repository
func NewWorkspaceRepository() WorkspaceRepository {
	return &workspaceRepository{
		workspaces: make(map[string]*models.Workspace),
	}
}

// Store stores a workspace in the repository
func (r *workspaceRepository) Store(ws *models.Workspace) error {
	if ws == nil {
		return fmt.Errorf("workspace cannot be nil")
	}

	var key string
	// Generate different keys for Issue vs PR workspaces
	if ws.Issue != nil && ws.PRNumber == 0 {
		// This is an Issue workspace
		key = generateWorkspaceKeyForIssue(ws.Org, ws.Repo, ws.Issue.GetNumber(), ws.AIModel)
	} else {
		// This is a PR workspace
		key = generateWorkspaceKey(ws.Org, ws.Repo, ws.PRNumber, ws.AIModel)
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()

	if _, exists := r.workspaces[key]; exists {
		log.Warnf("Workspace %s already exists, overwriting", key)
	}

	r.workspaces[key] = ws
	log.Infof("Stored workspace: %s, path: %s", key, ws.Path)
	return nil
}

// GetByKey retrieves a workspace by its key
func (r *workspaceRepository) GetByKey(key string) (*models.Workspace, bool) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	ws, exists := r.workspaces[key]
	return ws, exists
}

// GetByPR retrieves a workspace by PR and AI model
func (r *workspaceRepository) GetByPR(pr *github.PullRequest, aiModel string) (*models.Workspace, bool) {
	key := generateWorkspaceKey(
		pr.GetBase().GetRepo().GetOwner().GetLogin(),
		pr.GetBase().GetRepo().GetName(),
		pr.GetNumber(),
		aiModel)

	r.mutex.RLock()
	defer r.mutex.RUnlock()

	if ws, exists := r.workspaces[key]; exists {
		log.Infof("Found existing workspace for PR #%d with AI model %s: %s",
			pr.GetNumber(), aiModel, ws.Path)
		return ws, true
	}

	return nil, false
}

// GetAllByPR retrieves all workspaces for a specific PR (across all AI models)
func (r *workspaceRepository) GetAllByPR(pr *github.PullRequest) []*models.Workspace {
	orgRepoPath := fmt.Sprintf("%s/%s",
		pr.GetBase().GetRepo().GetOwner().GetLogin(),
		pr.GetBase().GetRepo().GetName())
	prNumber := pr.GetNumber()

	var workspaces []*models.Workspace

	r.mutex.RLock()
	defer r.mutex.RUnlock()

	// Iterate through all workspaces to find ones matching this PR
	for _, ws := range r.workspaces {
		if ws.PRNumber == prNumber &&
			fmt.Sprintf("%s/%s", ws.Org, ws.Repo) == orgRepoPath {
			workspaces = append(workspaces, ws)
		}
	}

	return workspaces
}

// GetByIssue retrieves a workspace by Issue and AI model
func (r *workspaceRepository) GetByIssue(issue *github.Issue, aiModel string) (*models.Workspace, bool) {
	// Extract org and repo from Issue URL
	org, repo, err := extractOrgRepoFromIssueURL(issue.GetHTMLURL())
	if err != nil {
		log.Errorf("Failed to extract org/repo from Issue URL: %v", err)
		return nil, false
	}

	key := generateWorkspaceKeyForIssue(org, repo, issue.GetNumber(), aiModel)

	r.mutex.RLock()
	defer r.mutex.RUnlock()

	if ws, exists := r.workspaces[key]; exists {
		log.Infof("Found existing workspace for Issue #%d with AI model %s: %s",
			issue.GetNumber(), aiModel, ws.Path)
		return ws, true
	}

	return nil, false
}

// GetAllByIssue retrieves all workspaces for a specific Issue (across all AI models)
func (r *workspaceRepository) GetAllByIssue(issue *github.Issue) []*models.Workspace {
	org, repo, err := extractOrgRepoFromIssueURL(issue.GetHTMLURL())
	if err != nil {
		log.Errorf("Failed to extract org/repo from Issue URL: %v", err)
		return nil
	}

	orgRepoPath := fmt.Sprintf("%s/%s", org, repo)
	issueNumber := issue.GetNumber()

	var workspaces []*models.Workspace

	r.mutex.RLock()
	defer r.mutex.RUnlock()

	// Iterate through all workspaces to find ones matching this Issue
	for _, ws := range r.workspaces {
		if ws.Issue != nil &&
			ws.Issue.GetNumber() == issueNumber &&
			fmt.Sprintf("%s/%s", ws.Org, ws.Repo) == orgRepoPath {
			workspaces = append(workspaces, ws)
		}
	}

	return workspaces
}

// Remove removes a workspace by key
func (r *workspaceRepository) Remove(key string) bool {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if _, exists := r.workspaces[key]; exists {
		delete(r.workspaces, key)
		log.Infof("Removed workspace from repository: %s", key)
		return true
	}

	return false
}

// RemoveByWorkspace removes a workspace by workspace object
func (r *workspaceRepository) RemoveByWorkspace(ws *models.Workspace) bool {
	if ws == nil {
		return false
	}

	var key string
	// Generate different keys for Issue vs PR workspaces
	if ws.Issue != nil && ws.PRNumber == 0 {
		// This is an Issue workspace
		key = generateWorkspaceKeyForIssue(ws.Org, ws.Repo, ws.Issue.GetNumber(), ws.AIModel)
	} else {
		// This is a PR workspace
		key = generateWorkspaceKey(ws.Org, ws.Repo, ws.PRNumber, ws.AIModel)
	}

	return r.Remove(key)
}

// GetExpired returns all workspaces that have expired based on the cleanup duration
func (r *workspaceRepository) GetExpired(cleanupAfter time.Duration) []*models.Workspace {
	expiredWorkspaces := []*models.Workspace{}
	now := time.Now()

	r.mutex.RLock()
	defer r.mutex.RUnlock()

	for _, ws := range r.workspaces {
		if now.Sub(ws.CreatedAt) > cleanupAfter {
			expiredWorkspaces = append(expiredWorkspaces, ws)
		}
	}

	return expiredWorkspaces
}

// Count returns the total number of workspaces
func (r *workspaceRepository) Count() int {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return len(r.workspaces)
}

// GetAll returns all workspaces (for recovery purposes)
func (r *workspaceRepository) GetAll() map[string]*models.Workspace {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	// Create a copy to prevent external modification
	result := make(map[string]*models.Workspace)
	for k, v := range r.workspaces {
		result[k] = v
	}

	return result
}

// generateWorkspaceKey generates a unique key for workspace storage
func generateWorkspaceKey(org, repo string, prNumber int, aiModel string) string {
	if aiModel == "" {
		return fmt.Sprintf("%s/%s/%d", org, repo, prNumber)
	}
	return fmt.Sprintf("%s/%s/%s/%d", aiModel, org, repo, prNumber)
}

// generateWorkspaceKeyForIssue generates a unique key for Issue workspace storage
// Uses "issue-" prefix to distinguish from PR workspaces
func generateWorkspaceKeyForIssue(org, repo string, issueNumber int, aiModel string) string {
	if aiModel == "" {
		return fmt.Sprintf("%s/%s/issue-%d", org, repo, issueNumber)
	}
	return fmt.Sprintf("%s/%s/%s/issue-%d", aiModel, org, repo, issueNumber)
}

// extractOrgRepoFromIssueURL extracts org and repo from Issue URL
func extractOrgRepoFromIssueURL(issueURL string) (org, repo string, err error) {
	// Issue URL format: https://github.com/owner/repo/issues/123
	if !strings.Contains(issueURL, "github.com") {
		return "", "", fmt.Errorf("invalid GitHub Issue URL: %s", issueURL)
	}

	parts := strings.Split(issueURL, "/")
	if len(parts) < 4 {
		return "", "", fmt.Errorf("invalid GitHub Issue URL format: %s", issueURL)
	}

	// Find github.com index and get owner/repo from there
	for i, part := range parts {
		if part == "github.com" && i+2 < len(parts) {
			return parts[i+1], parts[i+2], nil
		}
	}

	return "", "", fmt.Errorf("failed to extract org/repo from Issue URL: %s", issueURL)
}
