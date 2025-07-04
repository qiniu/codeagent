package code

import (
	"fmt"
	"sync"

	"github.com/qbox/codeagent/internal/config"
	"github.com/qbox/codeagent/pkg/models"
)

type SessionManager struct {
	mu    sync.RWMutex
	codes map[int]Code
	cfg   *config.Config
}

func NewSessionManager(cfg *config.Config) *SessionManager {
	return &SessionManager{
		codes: make(map[int]Code),
		cfg:   cfg,
	}
}

// GetSession retrieves an existing Code session or creates a new one.
func (sm *SessionManager) GetSession(workspace *models.Workspace) (Code, error) {
	sm.mu.RLock()
	c, ok := sm.codes[workspace.PullRequest.GetNumber()]
	sm.mu.RUnlock()

	if ok {
		return c, nil
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Double-check if the code object was created by another goroutine while we were waiting for the write lock
	if c, ok := sm.codes[workspace.PullRequest.GetNumber()]; ok {
		return c, nil
	}

	c, err := New(workspace, sm.cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create new code session: %w", err)
	}
	sm.codes[workspace.PullRequest.GetNumber()] = c
	return c, nil
}

// CloseSession closes and removes a Code session from the manager.
func (sm *SessionManager) CloseSession(workspace *models.Workspace) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if c, ok := sm.codes[workspace.PullRequest.GetNumber()]; ok {
		delete(sm.codes, workspace.PullRequest.GetNumber())
		return c.Close()
	}
	return nil
}
