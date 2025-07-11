package code

import (
	"fmt"
	"sync"

	"github.com/qbox/codeagent/internal/config"
	"github.com/qbox/codeagent/pkg/models"
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
	key := fmt.Sprintf("%s-%d", workspace.Repository, workspace.PRNumber)
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

// CloseSession closes and removes a Code session from the manager.
func (sm *SessionManager) CloseSession(workspace *models.Workspace) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	key := fmt.Sprintf("%s-%d", workspace.Repository, workspace.PullRequest.GetNumber())

	if c, ok := sm.codes[key]; ok {
		delete(sm.codes, key)
		return c.Close()
	}
	return nil
}
