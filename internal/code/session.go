package code

import (
	"fmt"
	"sync"

	"github.com/qiniu/codeagent/internal/config"
	"github.com/qiniu/codeagent/pkg/models"
)

type SessionManager struct {
	mu    sync.RWMutex
	codes map[string]Code
	cfg   *config.Config
}

func NewSessionManager(cfg *config.Config) *SessionManager {
	return &SessionManager{
		codes: make(map[string]Code),
		cfg:   cfg,
	}
}

// GetSession retrieves an existing Code session or creates a new one.
func (sm *SessionManager) GetSession(workspace *models.Workspace) (Code, error) {
	// Generate session key based on workspace type
	key := sm.generateSessionKey(workspace)
	
	sm.mu.RLock()
	c, ok := sm.codes[key]
	sm.mu.RUnlock()

	if ok {
		return c, nil
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Double-check if the code object was created by another goroutine while we were waiting for the write lock
	if code, ok := sm.codes[key]; ok {
		return code, nil
	}

	c, err := New(workspace, sm.cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create new code session: %w", err)
	}
	sm.codes[key] = c
	return c, nil
}

// generateSessionKey generates a unique session key based on workspace type
func (sm *SessionManager) generateSessionKey(workspace *models.Workspace) string {
	if workspace.PRNumber > 0 {
		// For PR workspaces: aimodel-org-repo-pr-number
		return fmt.Sprintf("%s-%s-%s-pr-%d", workspace.AIModel, workspace.Org, workspace.Repo, workspace.PRNumber)
	} else if workspace.Issue != nil {
		// For Issue workspaces: aimodel-org-repo-issue-number (no timestamp to ensure same session for same issue)
		return fmt.Sprintf("%s-%s-%s-issue-%d", workspace.AIModel, workspace.Org, workspace.Repo, workspace.Issue.GetNumber())
	} else {
		// Fallback for unknown workspace types
		return fmt.Sprintf("%s-%s-%s-workspace", workspace.AIModel, workspace.Org, workspace.Repo)
	}
}

// CloseSession closes and removes a Code session from the manager.
func (sm *SessionManager) CloseSession(workspace *models.Workspace) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	key := sm.generateSessionKey(workspace)

	if c, ok := sm.codes[key]; ok {
		delete(sm.codes, key)
		return c.Close()
	}
	return nil
}
