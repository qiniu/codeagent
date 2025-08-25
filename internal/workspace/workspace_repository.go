package workspace

import (
	"fmt"
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

	key := generateWorkspaceKey(ws.Org, ws.Repo, ws.PRNumber, ws.AIModel)
	
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

	key := generateWorkspaceKey(ws.Org, ws.Repo, ws.PRNumber, ws.AIModel)
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