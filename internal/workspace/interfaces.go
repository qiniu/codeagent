package workspace

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/go-github/v58/github"
	"github.com/qiniu/codeagent/pkg/models"
)

// WorkspaceManager defines the interface for workspace management
type WorkspaceManager interface {
	// Basic workspace operations
	GetBaseDir() string
	GetWorkspaceCount() int
	RegisterWorkspace(ws *models.Workspace, pr *github.PullRequest)
	CleanupWorkspace(ws *models.Workspace) bool
	GetExpiredWorkspaces() []*models.Workspace

	// Workspace retrieval
	GetWorkspaceByPR(pr *github.PullRequest, aiModel string) *models.Workspace
	GetAllWorkspacesByPR(pr *github.PullRequest) []*models.Workspace
	GetWorkspaceByIssue(issue *github.Issue, aiModel string) *models.Workspace

	// Workspace creation
	CreateWorkspaceFromIssue(issue *github.Issue, aiModel string) *models.Workspace
	CreateWorkspaceFromPR(pr *github.PullRequest, aiModel string) *models.Workspace
	GetOrCreateWorkspaceForIssue(issue *github.Issue, aiModel string) *models.Workspace
	GetOrCreateWorkspaceForPR(pr *github.PullRequest, aiModel string) *models.Workspace

	// Workspace management
	CreateSessionPath(underPath, aiModel, repo string, prNumber int, suffix string) (string, error)
	MoveIssueToPR(ws *models.Workspace, prNumber int) error

	// Utility methods
	ExtractAIModelFromBranch(branchName string) string

	// Directory format delegation
	GenerateIssueDirName(aiModel, repo string, issueNumber int, timestamp int64) string
	GeneratePRDirName(aiModel, repo string, prNumber int, timestamp int64) string
	GenerateSessionDirName(aiModel, repo string, prNumber int, timestamp int64) string
	ParsePRDirName(dirName string) (*PRDirFormat, error)
	ExtractSuffixFromPRDir(aiModel, repo string, prNumber int, dirName string) string
	ExtractSuffixFromIssueDir(aiModel, repo string, issueNumber int, dirName string) string
}

// Ensure Manager implements the WorkspaceManager interface
var _ WorkspaceManager = (*Manager)(nil)

// MockWorkspaceManager provides a mock implementation for testing
type MockWorkspaceManager struct {
	BaseDir               string
	WorkspaceCount        int
	Workspaces            map[string]*models.Workspace
	ExpiredWorkspaces     []*models.Workspace
	RegisterWorkspaceFunc func(ws *models.Workspace, pr *github.PullRequest)
	CleanupWorkspaceFunc  func(ws *models.Workspace) bool
	CreateWorkspaceFunc   func() *models.Workspace
	CreateSessionPathFunc func(underPath, aiModel, repo string, prNumber int, suffix string) (string, error)
	MoveIssueToPRFunc     func(ws *models.Workspace, prNumber int) error
	ExtractAIModelFunc    func(branchName string) string
	DirectoryFormatFuncs  map[string]interface{}
}

// NewMockWorkspaceManager creates a new mock workspace manager
func NewMockWorkspaceManager() *MockWorkspaceManager {
	return &MockWorkspaceManager{
		Workspaces:           make(map[string]*models.Workspace),
		DirectoryFormatFuncs: make(map[string]interface{}),
	}
}

// GetBaseDir returns the base directory
func (m *MockWorkspaceManager) GetBaseDir() string {
	return m.BaseDir
}

// GetWorkspaceCount returns the workspace count
func (m *MockWorkspaceManager) GetWorkspaceCount() int {
	return m.WorkspaceCount
}

// RegisterWorkspace registers a workspace
func (m *MockWorkspaceManager) RegisterWorkspace(ws *models.Workspace, pr *github.PullRequest) {
	if m.RegisterWorkspaceFunc != nil {
		m.RegisterWorkspaceFunc(ws, pr)
		return
	}
	// Default implementation
	key := generateWorkspaceKey(ws.Org, ws.Repo, ws.PRNumber, ws.AIModel)
	m.Workspaces[key] = ws
}

// CleanupWorkspace cleans up a workspace
func (m *MockWorkspaceManager) CleanupWorkspace(ws *models.Workspace) bool {
	if m.CleanupWorkspaceFunc != nil {
		return m.CleanupWorkspaceFunc(ws)
	}
	// Default implementation
	key := generateWorkspaceKey(ws.Org, ws.Repo, ws.PRNumber, ws.AIModel)
	delete(m.Workspaces, key)
	return true
}

// GetExpiredWorkspaces returns expired workspaces
func (m *MockWorkspaceManager) GetExpiredWorkspaces() []*models.Workspace {
	return m.ExpiredWorkspaces
}

// GetWorkspaceByPR retrieves workspace by PR and optional AI model
func (m *MockWorkspaceManager) GetWorkspaceByPR(pr *github.PullRequest, aiModel string) *models.Workspace {
	key := generateWorkspaceKey(
		pr.GetBase().GetRepo().GetOwner().GetLogin(),
		pr.GetBase().GetRepo().GetName(),
		pr.GetNumber(),
		aiModel)
	return m.Workspaces[key]
}

// GetAllWorkspacesByPR gets all workspaces for a PR
func (m *MockWorkspaceManager) GetAllWorkspacesByPR(pr *github.PullRequest) []*models.Workspace {
	var workspaces []*models.Workspace
	orgRepoPath := pr.GetBase().GetRepo().GetOwner().GetLogin() + "/" + pr.GetBase().GetRepo().GetName()
	prNumber := pr.GetNumber()

	for _, ws := range m.Workspaces {
		if ws.PRNumber == prNumber && ws.Org+"/"+ws.Repo == orgRepoPath {
			workspaces = append(workspaces, ws)
		}
	}
	return workspaces
}

// Issue workspace methods (mock implementations)
func (m *MockWorkspaceManager) GetWorkspaceByIssue(issue *github.Issue, aiModel string) *models.Workspace {
	// Extract org and repo from Issue URL for key generation
	issueURL := issue.GetHTMLURL()
	if !strings.Contains(issueURL, "github.com") {
		return nil
	}

	parts := strings.Split(issueURL, "/")
	if len(parts) < 4 {
		return nil
	}

	var org, repo string
	for i, part := range parts {
		if part == "github.com" && i+2 < len(parts) {
			org = parts[i+1]
			repo = parts[i+2]
			break
		}
	}

	// Generate key using Issue number
	var key string
	if aiModel == "" {
		key = fmt.Sprintf("%s/%s/issue-%d", org, repo, issue.GetNumber())
	} else {
		key = fmt.Sprintf("%s/%s/%s/issue-%d", aiModel, org, repo, issue.GetNumber())
	}

	return m.Workspaces[key]
}

func (m *MockWorkspaceManager) CreateWorkspaceFromIssue(issue *github.Issue, aiModel string) *models.Workspace {
	if m.CreateWorkspaceFunc != nil {
		return m.CreateWorkspaceFunc()
	}
	ws := &models.Workspace{
		AIModel:   aiModel,
		Issue:     issue,
		CreatedAt: time.Now(),
	}
	return ws
}

func (m *MockWorkspaceManager) GetOrCreateWorkspaceForIssue(issue *github.Issue, aiModel string) *models.Workspace {
	ws := m.GetWorkspaceByIssue(issue, aiModel)
	if ws != nil {
		return ws
	}
	return m.CreateWorkspaceFromIssue(issue, aiModel)
}

// Workspace creation methods (mock implementations)
func (m *MockWorkspaceManager) CreateWorkspaceFromIssueWithAI(issue *github.Issue, aiModel string) *models.Workspace {
	if m.CreateWorkspaceFunc != nil {
		return m.CreateWorkspaceFunc()
	}
	return &models.Workspace{AIModel: aiModel, Issue: issue, CreatedAt: time.Now()}
}

func (m *MockWorkspaceManager) CreateWorkspaceFromPR(pr *github.PullRequest, aiModel string) *models.Workspace {
	if m.CreateWorkspaceFunc != nil {
		return m.CreateWorkspaceFunc()
	}
	ws := &models.Workspace{
		AIModel:     aiModel,
		PullRequest: pr,
		PRNumber:    pr.GetNumber(),
		Org:         pr.GetBase().GetRepo().GetOwner().GetLogin(),
		Repo:        pr.GetBase().GetRepo().GetName(),
		CreatedAt:   time.Now(),
	}
	m.RegisterWorkspace(ws, pr)
	return ws
}

// Legacy method for compatibility
func (m *MockWorkspaceManager) CreateWorkspaceFromPRWithAI(pr *github.PullRequest, aiModel string) *models.Workspace {
	return m.CreateWorkspaceFromPR(pr, aiModel)
}

func (m *MockWorkspaceManager) GetOrCreateWorkspaceForPR(pr *github.PullRequest, aiModel string) *models.Workspace {
	ws := m.GetWorkspaceByPR(pr, aiModel)
	if ws != nil {
		return ws
	}
	return m.CreateWorkspaceFromPR(pr, aiModel)
}

// Legacy method for compatibility
func (m *MockWorkspaceManager) GetOrCreateWorkspaceForPRWithAI(pr *github.PullRequest, aiModel string) *models.Workspace {
	return m.GetOrCreateWorkspaceForPR(pr, aiModel)
}

// Workspace management methods (mock implementations)
func (m *MockWorkspaceManager) CreateSessionPath(underPath, aiModel, repo string, prNumber int, suffix string) (string, error) {
	if m.CreateSessionPathFunc != nil {
		return m.CreateSessionPathFunc(underPath, aiModel, repo, prNumber, suffix)
	}
	return "/tmp/session", nil
}

func (m *MockWorkspaceManager) MoveIssueToPR(ws *models.Workspace, prNumber int) error {
	if m.MoveIssueToPRFunc != nil {
		return m.MoveIssueToPRFunc(ws, prNumber)
	}
	ws.PRNumber = prNumber
	return nil
}

// Utility methods (mock implementations)
func (m *MockWorkspaceManager) ExtractAIModelFromBranch(branchName string) string {
	if m.ExtractAIModelFunc != nil {
		return m.ExtractAIModelFunc(branchName)
	}
	return "claude"
}

// Directory format delegation (mock implementations)
func (m *MockWorkspaceManager) GenerateIssueDirName(aiModel, repo string, issueNumber int, timestamp int64) string {
	return "mock-issue-dir"
}

func (m *MockWorkspaceManager) GeneratePRDirName(aiModel, repo string, prNumber int, timestamp int64) string {
	return "mock-pr-dir"
}

func (m *MockWorkspaceManager) GenerateSessionDirName(aiModel, repo string, prNumber int, timestamp int64) string {
	return "mock-session-dir"
}

func (m *MockWorkspaceManager) ParsePRDirName(dirName string) (*PRDirFormat, error) {
	return &PRDirFormat{
		AIModel:   "claude",
		Repo:      "test-repo",
		PRNumber:  1,
		Timestamp: time.Now().Unix(),
	}, nil
}

func (m *MockWorkspaceManager) ExtractSuffixFromPRDir(aiModel, repo string, prNumber int, dirName string) string {
	return "mock-suffix"
}

func (m *MockWorkspaceManager) ExtractSuffixFromIssueDir(aiModel, repo string, issueNumber int, dirName string) string {
	return "mock-suffix"
}
