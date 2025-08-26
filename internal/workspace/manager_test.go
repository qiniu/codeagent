package workspace

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/go-github/v58/github"
	"github.com/qiniu/codeagent/internal/config"
	"github.com/qiniu/codeagent/pkg/models"
)

func TestGenerateWorkspaceKey(t *testing.T) {
	tests := []struct {
		name     string
		org      string
		repo     string
		prNumber int
		aiModel  string
		expected string
	}{
		{
			name:     "Without AI model",
			org:      "org1",
			repo:     "repo1",
			prNumber: 123,
			aiModel:  "",
			expected: "org1/repo1/123",
		},
		{
			name:     "With AI model",
			org:      "org2",
			repo:     "repo2",
			prNumber: 456,
			aiModel:  "claude",
			expected: "claude/org2/repo2/456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateWorkspaceKey(tt.org, tt.repo, tt.prNumber, tt.aiModel)
			if result != tt.expected {
				t.Errorf("generateWorkspaceKey(%s, %s, %d, %s) = %s, want %s",
					tt.org, tt.repo, tt.prNumber, tt.aiModel, result, tt.expected)
			}
		})
	}
}

func TestExtractAIModelFromBranch(t *testing.T) {
	cfg := &config.Config{
		CodeProvider: "claude",
	}
	manager := NewManager(cfg)

	tests := []struct {
		name       string
		branchName string
		expected   string
	}{
		{
			name:       "Claude branch",
			branchName: "codeagent/claude/issue-123-456",
			expected:   "claude",
		},
		{
			name:       "Gemini branch",
			branchName: "codeagent/gemini/pr-789-101",
			expected:   "gemini",
		},
		{
			name:       "Invalid AI model",
			branchName: "codeagent/invalid/issue-123-456",
			expected:   "claude", // Falls back to config default
		},
		{
			name:       "Not a codeagent branch",
			branchName: "main",
			expected:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.ExtractAIModelFromBranch(tt.branchName)
			if result != tt.expected {
				t.Errorf("ExtractAIModelFromBranch(%s) = %s, want %s",
					tt.branchName, result, tt.expected)
			}
		})
	}
}

func TestIssueWorkspaceRepository(t *testing.T) {
	tests := []struct {
		name        string
		aiModel     string
		issueURL    string
		issueNumber int
		shouldFind  bool
	}{
		{
			name:        "Store and find Issue workspace",
			aiModel:     "claude",
			issueURL:    "https://github.com/testorg/testrepo/issues/123",
			issueNumber: 123,
			shouldFind:  true,
		},
		{
			name:        "Find Issue workspace with different AI model",
			aiModel:     "gemini",
			issueURL:    "https://github.com/testorg/testrepo/issues/123",
			issueNumber: 123,
			shouldFind:  false,
		},
	}

	// Create mock repository
	mockRepo := &mockWorkspaceRepository{
		workspaces: make(map[string]*models.Workspace),
	}

	// Create and store first workspace
	issue1 := &github.Issue{
		Number:  github.Int(123),
		HTMLURL: github.String("https://github.com/testorg/testrepo/issues/123"),
	}

	workspace1 := &models.Workspace{
		Org:     "testorg",
		Repo:    "testrepo",
		AIModel: "claude",
		Issue:   issue1,
		Path:    "/tmp/test-workspace-claude",
	}

	err := mockRepo.Store(workspace1)
	if err != nil {
		t.Fatalf("Failed to store workspace: %v", err)
	}

	// Run tests
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issue := &github.Issue{
				Number:  github.Int(tt.issueNumber),
				HTMLURL: github.String(tt.issueURL),
			}

			// Test GetByIssue method
			result, found := mockRepo.GetByIssue(issue, tt.aiModel)

			if tt.shouldFind {
				if !found {
					t.Error("Expected to find workspace but didn't")
					return
				}
				if result.AIModel != tt.aiModel {
					t.Errorf("Expected AI model %s, got %s", tt.aiModel, result.AIModel)
				}
				if result.Issue.GetNumber() != tt.issueNumber {
					t.Errorf("Expected issue number %d, got %d", tt.issueNumber, result.Issue.GetNumber())
				}
			} else {
				if found {
					t.Error("Expected not to find workspace but found one")
				}
			}
		})
	}

	// Test GetAllByIssue
	allWorkspaces := mockRepo.GetAllByIssue(issue1)
	if len(allWorkspaces) != 1 {
		t.Errorf("Expected 1 workspace for issue, got %d", len(allWorkspaces))
	}
}

func TestGenerateWorkspaceKeyForIssue(t *testing.T) {
	tests := []struct {
		name        string
		org         string
		repo        string
		issueNumber int
		aiModel     string
		expected    string
	}{
		{
			name:        "Without AI model",
			org:         "testorg",
			repo:        "testrepo",
			issueNumber: 123,
			aiModel:     "",
			expected:    "testorg/testrepo/issue-123",
		},
		{
			name:        "With AI model",
			org:         "testorg",
			repo:        "testrepo",
			issueNumber: 456,
			aiModel:     "claude",
			expected:    "claude/testorg/testrepo/issue-456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateWorkspaceKeyForIssue(tt.org, tt.repo, tt.issueNumber, tt.aiModel)
			if result != tt.expected {
				t.Errorf("generateWorkspaceKeyForIssue(%s, %s, %d, %s) = %s, want %s",
					tt.org, tt.repo, tt.issueNumber, tt.aiModel, result, tt.expected)
			}
		})
	}
}

func TestExtractOrgRepoFromIssueURL(t *testing.T) {
	tests := []struct {
		name         string
		issueURL     string
		expectedOrg  string
		expectedRepo string
		shouldError  bool
	}{
		{
			name:         "Valid GitHub Issue URL",
			issueURL:     "https://github.com/testorg/testrepo/issues/123",
			expectedOrg:  "testorg",
			expectedRepo: "testrepo",
			shouldError:  false,
		},
		{
			name:        "Invalid URL - not GitHub",
			issueURL:    "https://gitlab.com/testorg/testrepo/issues/123",
			shouldError: true,
		},
		{
			name:        "Invalid URL format",
			issueURL:    "https://github.com/testorg",
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			org, repo, err := extractOrgRepoFromIssueURL(tt.issueURL)

			if tt.shouldError {
				if err == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
				if org != tt.expectedOrg {
					t.Errorf("Expected org %s, got %s", tt.expectedOrg, org)
				}
				if repo != tt.expectedRepo {
					t.Errorf("Expected repo %s, got %s", tt.expectedRepo, repo)
				}
			}
		})
	}
}

// mockWorkspaceRepository is a simple mock for testing
type mockWorkspaceRepository struct {
	workspaces map[string]*models.Workspace
	mutex      sync.RWMutex
}

func (m *mockWorkspaceRepository) Store(ws *models.Workspace) error {
	var key string
	if ws.Issue != nil && ws.PRNumber == 0 {
		// Extract org and repo from Issue URL
		org, repo, err := extractOrgRepoFromIssueURL(ws.Issue.GetHTMLURL())
		if err != nil {
			return err
		}
		key = generateWorkspaceKeyForIssue(org, repo, ws.Issue.GetNumber(), ws.AIModel)
	} else {
		key = generateWorkspaceKey(ws.Org, ws.Repo, ws.PRNumber, ws.AIModel)
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.workspaces[key] = ws
	return nil
}

func (m *mockWorkspaceRepository) GetByKey(key string) (*models.Workspace, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	ws, exists := m.workspaces[key]
	return ws, exists
}

func (m *mockWorkspaceRepository) GetByPR(pr *github.PullRequest, aiModel string) (*models.Workspace, bool) {
	key := generateWorkspaceKey(
		pr.GetBase().GetRepo().GetOwner().GetLogin(),
		pr.GetBase().GetRepo().GetName(),
		pr.GetNumber(),
		aiModel)
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	ws, exists := m.workspaces[key]
	return ws, exists
}

func (m *mockWorkspaceRepository) GetAllByPR(pr *github.PullRequest) []*models.Workspace {
	var workspaces []*models.Workspace
	orgRepoPath := pr.GetBase().GetRepo().GetOwner().GetLogin() + "/" + pr.GetBase().GetRepo().GetName()
	prNumber := pr.GetNumber()

	m.mutex.RLock()
	defer m.mutex.RUnlock()
	for _, ws := range m.workspaces {
		if ws.PRNumber == prNumber && ws.Org+"/"+ws.Repo == orgRepoPath {
			workspaces = append(workspaces, ws)
		}
	}
	return workspaces
}

func (m *mockWorkspaceRepository) GetByIssue(issue *github.Issue, aiModel string) (*models.Workspace, bool) {
	org, repo, err := extractOrgRepoFromIssueURL(issue.GetHTMLURL())
	if err != nil {
		return nil, false
	}
	key := generateWorkspaceKeyForIssue(org, repo, issue.GetNumber(), aiModel)
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	ws, exists := m.workspaces[key]
	return ws, exists
}

func (m *mockWorkspaceRepository) GetAllByIssue(issue *github.Issue) []*models.Workspace {
	var workspaces []*models.Workspace
	issueNumber := issue.GetNumber()

	m.mutex.RLock()
	defer m.mutex.RUnlock()
	for _, ws := range m.workspaces {
		if ws.Issue != nil && ws.Issue.GetNumber() == issueNumber {
			workspaces = append(workspaces, ws)
		}
	}
	return workspaces
}

func (m *mockWorkspaceRepository) Remove(key string) bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if _, exists := m.workspaces[key]; exists {
		delete(m.workspaces, key)
		return true
	}
	return false
}

func (m *mockWorkspaceRepository) RemoveByWorkspace(ws *models.Workspace) bool {
	var key string
	if ws.Issue != nil && ws.PRNumber == 0 {
		// This is an Issue workspace
		org, repo, err := extractOrgRepoFromIssueURL(ws.Issue.GetHTMLURL())
		if err != nil {
			return false
		}
		key = generateWorkspaceKeyForIssue(org, repo, ws.Issue.GetNumber(), ws.AIModel)
	} else {
		// This is a PR workspace
		key = generateWorkspaceKey(ws.Org, ws.Repo, ws.PRNumber, ws.AIModel)
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()
	if _, exists := m.workspaces[key]; exists {
		delete(m.workspaces, key)
		return true
	}
	return false
}

func (m *mockWorkspaceRepository) GetExpired(cleanupAfter time.Duration) []*models.Workspace {
	var expiredWorkspaces []*models.Workspace
	now := time.Now()

	m.mutex.RLock()
	defer m.mutex.RUnlock()
	for _, ws := range m.workspaces {
		if now.Sub(ws.CreatedAt) > cleanupAfter {
			expiredWorkspaces = append(expiredWorkspaces, ws)
		}
	}
	return expiredWorkspaces
}

func (m *mockWorkspaceRepository) Count() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return len(m.workspaces)
}

func (m *mockWorkspaceRepository) GetAll() map[string]*models.Workspace {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	result := make(map[string]*models.Workspace)
	for k, v := range m.workspaces {
		result[k] = v
	}
	return result
}

// mockContainerService is a simple mock for testing
type mockContainerService struct{}

func (m *mockContainerService) CleanupWorkspaceContainers(ws *models.Workspace) error {
	return nil
}

func (m *mockContainerService) RemoveContainer(containerName string) error {
	return nil
}

func (m *mockContainerService) ContainerExists(containerName string) (bool, error) {
	return false, nil
}

func (m *mockContainerService) GenerateContainerNames(ws *models.Workspace) []string {
	return nil
}

// TestIssueWorkspaceLifecycle tests the complete lifecycle of Issue workspace management
func TestIssueWorkspaceLifecycle(t *testing.T) {
	// Setup test environment
	mockRepo := &mockWorkspaceRepository{
		workspaces: make(map[string]*models.Workspace),
	}

	cfg := &config.Config{
		CodeProvider: "claude",
	}

	manager := &Manager{
		config:           cfg,
		repository:       mockRepo,
		containerService: &mockContainerService{},
		gitService:       &mockGitService{},
	}

	// Test data
	issue := &github.Issue{
		Number:  github.Int(123),
		HTMLURL: github.String("https://github.com/testorg/testrepo/issues/123"),
	}

	t.Run("Create new Issue workspace", func(t *testing.T) {
		// Test workspace creation
		ws := manager.GetWorkspaceByIssue(issue, "claude")
		if ws != nil {
			t.Error("Expected no existing workspace but found one")
		}

		// Test workspace storage via repository
		testWs := &models.Workspace{
			Org:     "testorg",
			Repo:    "testrepo",
			AIModel: "claude",
			Issue:   issue,
			Path:    "/tmp/test-issue-workspace",
			Branch:  "codeagent/claude/issue-123-1234567890",
		}

		err := mockRepo.Store(testWs)
		if err != nil {
			t.Fatalf("Failed to store workspace: %v", err)
		}

		// Verify workspace can be retrieved
		retrieved := manager.GetWorkspaceByIssue(issue, "claude")
		if retrieved == nil {
			t.Error("Expected to find stored workspace but didn't")
		}

		if retrieved.AIModel != "claude" {
			t.Errorf("Expected AI model 'claude', got '%s'", retrieved.AIModel)
		}

		if retrieved.Issue.GetNumber() != 123 {
			t.Errorf("Expected issue number 123, got %d", retrieved.Issue.GetNumber())
		}
	})

	t.Run("AI model isolation", func(t *testing.T) {
		// Create workspace for different AI model
		testWsGemini := &models.Workspace{
			Org:     "testorg",
			Repo:    "testrepo",
			AIModel: "gemini",
			Issue:   issue,
			Path:    "/tmp/test-issue-workspace-gemini",
			Branch:  "codeagent/gemini/issue-123-1234567890",
		}

		err := mockRepo.Store(testWsGemini)
		if err != nil {
			t.Fatalf("Failed to store gemini workspace: %v", err)
		}

		// Verify both workspaces exist independently
		claudeWs := manager.GetWorkspaceByIssue(issue, "claude")
		geminiWs := manager.GetWorkspaceByIssue(issue, "gemini")

		if claudeWs == nil {
			t.Error("Claude workspace should exist")
		}
		if geminiWs == nil {
			t.Error("Gemini workspace should exist")
		}

		if claudeWs != nil && geminiWs != nil {
			if claudeWs.Path == geminiWs.Path {
				t.Error("Different AI models should have different workspace paths")
			}
		}

		// Note: GetAllWorkspacesByIssue method has been removed as it was only used in tests
	})

	t.Run("Workspace cleanup", func(t *testing.T) {
		// Get existing workspace
		ws := manager.GetWorkspaceByIssue(issue, "claude")
		if ws == nil {
			t.Fatal("Expected workspace to exist for cleanup test")
		}

		// For Issue workspace without SessionPath, cleanup will return false (design behavior)
		// But the workspace should still be removed from repository
		_ = manager.CleanupWorkspace(ws)

		// Issue workspace without SessionPath will return false, but should still cleanup
		// Let's verify the workspace is removed from repository
		wsAfterCleanup := manager.GetWorkspaceByIssue(issue, "claude")
		if wsAfterCleanup != nil {
			t.Error("Expected workspace to be removed from repository after cleanup")
		}

		// Now test with a workspace that has SessionPath
		wsWithSession := &models.Workspace{
			Org:         "testorg",
			Repo:        "testrepo",
			AIModel:     "claude",
			Issue:       issue,
			Path:        "/tmp/test-issue-workspace-with-session",
			SessionPath: "/tmp/test-session-path",
			Branch:      "codeagent/claude/issue-123-1234567890",
		}

		err := mockRepo.Store(wsWithSession)
		if err != nil {
			t.Fatalf("Failed to store workspace with session: %v", err)
		}

		// This should succeed since it has both Path and SessionPath
		successWithSession := manager.CleanupWorkspace(wsWithSession)
		if !successWithSession {
			t.Error("Expected cleanup to succeed for workspace with session path")
		}
	})
}

// TestPRWorkspaceLifecycle tests the complete lifecycle of PR workspace management
func TestPRWorkspaceLifecycle(t *testing.T) {
	// Setup test environment
	mockRepo := &mockWorkspaceRepository{
		workspaces: make(map[string]*models.Workspace),
	}

	cfg := &config.Config{
		CodeProvider: "claude",
	}

	manager := &Manager{
		config:           cfg,
		repository:       mockRepo,
		containerService: &mockContainerService{},
		gitService:       &mockGitService{},
	}

	// Create mock PR
	mockOwner := &github.User{Login: github.String("testorg")}
	mockRepo2 := &github.Repository{
		Name:  github.String("testrepo"),
		Owner: mockOwner,
	}
	mockBase := &github.PullRequestBranch{Repo: mockRepo2}
	mockHead := &github.PullRequestBranch{Ref: github.String("feature-branch")}

	pr := &github.PullRequest{
		Number: github.Int(456),
		Base:   mockBase,
		Head:   mockHead,
	}

	t.Run("Create and reuse PR workspace", func(t *testing.T) {
		// Initially no workspace should exist
		ws := manager.GetWorkspaceByPR(pr, "claude")
		if ws != nil {
			t.Error("Expected no existing workspace but found one")
		}

		// Create test workspace
		testWs := &models.Workspace{
			Org:         "testorg",
			Repo:        "testrepo",
			AIModel:     "claude",
			PRNumber:    456,
			PullRequest: pr,
			Path:        "/tmp/test-pr-workspace",
			SessionPath: "/tmp/test-pr-session",
			Branch:      "feature-branch",
		}

		err := mockRepo.Store(testWs)
		if err != nil {
			t.Fatalf("Failed to store PR workspace: %v", err)
		}

		// Test retrieval
		retrieved := manager.GetWorkspaceByPR(pr, "claude")
		if retrieved == nil {
			t.Error("Expected to find stored PR workspace")
		}

		if retrieved != nil && retrieved.PRNumber != 456 {
			t.Errorf("Expected PR number 456, got %d", retrieved.PRNumber)
		}
	})

	t.Run("PR AI model isolation", func(t *testing.T) {
		// Create workspace for different AI model
		testWsGemini := &models.Workspace{
			Org:         "testorg",
			Repo:        "testrepo",
			AIModel:     "gemini",
			PRNumber:    456,
			PullRequest: pr,
			Path:        "/tmp/test-pr-workspace-gemini",
			SessionPath: "/tmp/test-pr-session-gemini",
			Branch:      "feature-branch",
		}

		err := mockRepo.Store(testWsGemini)
		if err != nil {
			t.Fatalf("Failed to store gemini PR workspace: %v", err)
		}

		// Test that both workspaces exist independently
		allWorkspaces := manager.GetAllWorkspacesByPR(pr)
		if len(allWorkspaces) != 2 {
			t.Errorf("Expected 2 workspaces for PR, got %d", len(allWorkspaces))
		}
	})
}

// TestWorkspaceCleanupScenarios tests various cleanup scenarios
func TestWorkspaceCleanupScenarios(t *testing.T) {
	// Setup test environment
	mockRepo := &mockWorkspaceRepository{
		workspaces: make(map[string]*models.Workspace),
	}

	cfg := &config.Config{
		Workspace: config.WorkspaceConfig{
			CleanupAfter: 24 * time.Hour,
		},
	}

	manager := &Manager{
		config:           cfg,
		repository:       mockRepo,
		containerService: &mockContainerService{},
	}

	t.Run("Expired workspace cleanup", func(t *testing.T) {
		// Create expired workspace
		expiredTime := time.Now().Add(-25 * time.Hour)
		expiredWs := &models.Workspace{
			Org:       "testorg",
			Repo:      "testrepo",
			AIModel:   "claude",
			PRNumber:  789,
			Path:      "/tmp/expired-workspace",
			CreatedAt: expiredTime,
		}

		err := mockRepo.Store(expiredWs)
		if err != nil {
			t.Fatalf("Failed to store expired workspace: %v", err)
		}

		// Create fresh workspace
		freshWs := &models.Workspace{
			Org:       "testorg",
			Repo:      "testrepo",
			AIModel:   "gemini",
			PRNumber:  790,
			Path:      "/tmp/fresh-workspace",
			CreatedAt: time.Now(),
		}

		err = mockRepo.Store(freshWs)
		if err != nil {
			t.Fatalf("Failed to store fresh workspace: %v", err)
		}

		// Test GetExpiredWorkspaces
		expired := manager.GetExpiredWorkspaces()
		if len(expired) != 1 {
			t.Errorf("Expected 1 expired workspace, got %d", len(expired))
		}

		if len(expired) > 0 && expired[0].PRNumber != 789 {
			t.Errorf("Expected expired workspace PR number 789, got %d", expired[0].PRNumber)
		}
	})

	t.Run("Workspace count tracking", func(t *testing.T) {
		// Test workspace count
		count := manager.GetWorkspaceCount()
		if count != 2 { // From previous test: 1 expired + 1 fresh
			t.Errorf("Expected 2 workspaces, got %d", count)
		}

		// Cleanup one workspace
		workspaces := mockRepo.GetAll()
		for _, ws := range workspaces {
			manager.CleanupWorkspace(ws)
			break
		}

		// Check count after cleanup
		newCount := manager.GetWorkspaceCount()
		if newCount != 1 {
			t.Errorf("Expected 1 workspace after cleanup, got %d", newCount)
		}
	})
}

// mockGitService provides a mock implementation for testing
type mockGitService struct{}

func (m *mockGitService) CloneRepository(repoURL, clonePath, branch string, createNewBranch bool) error {
	return nil
}

func (m *mockGitService) GetRemoteURL(repoPath string) (string, error) {
	return "https://github.com/testorg/testrepo.git", nil
}

func (m *mockGitService) GetCurrentBranch(repoPath string) (string, error) {
	return "main", nil
}

func (m *mockGitService) GetCurrentCommit(repoPath string) (string, error) {
	return "abc123", nil
}

func (m *mockGitService) GetBranchCommit(repoPath, branch string) (string, error) {
	return "abc123", nil
}

func (m *mockGitService) ValidateBranch(repoPath, expectedBranch string) bool {
	// For testing, always return true unless path doesn't exist
	return repoPath != "" && expectedBranch != ""
}

func (m *mockGitService) ConfigureSafeDirectory(repoPath string) error {
	return nil
}

func (m *mockGitService) ConfigurePullStrategy(repoPath string) error {
	return nil
}

func (m *mockGitService) CreateAndCheckoutBranch(repoPath, branchName string) error {
	return nil
}

func (m *mockGitService) CheckoutBranch(repoPath, branchName string) error {
	return nil
}

func (m *mockGitService) CreateTrackingBranch(repoPath, branchName string) error {
	return nil
}

// TestIssueWorkspaceReuse tests the Issue workspace reuse mechanism
func TestIssueWorkspaceReuse(t *testing.T) {
	// Setup test environment
	mockRepo := &mockWorkspaceRepository{
		workspaces: make(map[string]*models.Workspace),
	}

	cfg := &config.Config{
		CodeProvider: "claude",
	}

	manager := &Manager{
		config:           cfg,
		repository:       mockRepo,
		containerService: &mockContainerService{},
		gitService:       &mockGitService{},
	}

	// Test data
	issue := &github.Issue{
		Number:  github.Int(999),
		HTMLURL: github.String("https://github.com/testorg/testrepo/issues/999"),
	}

	t.Run("Issue workspace retrieval pattern", func(t *testing.T) {
		// Test that retrieval works when workspace exists
		testWs := &models.Workspace{
			Org:     "testorg",
			Repo:    "testrepo",
			AIModel: "claude",
			Issue:   issue,
			Path:    "/tmp/test-reuse-workspace",
			Branch:  "codeagent/claude/issue-999-1234567890",
		}

		err := mockRepo.Store(testWs)
		if err != nil {
			t.Fatalf("Failed to store test workspace: %v", err)
		}

		// Now it should be retrievable
		retrieved := manager.GetWorkspaceByIssue(issue, "claude")
		if retrieved == nil {
			t.Error("Should be able to retrieve stored Issue workspace")
		}

		if retrieved != nil && retrieved.Issue.GetNumber() != 999 {
			t.Errorf("Retrieved workspace should have Issue #999, got #%d",
				retrieved.Issue.GetNumber())
		}
	})

	t.Run("Issue workspace key generation consistency", func(t *testing.T) {
		// Test that the same Issue + AI model generates the same key
		key1 := generateWorkspaceKeyForIssue("testorg", "testrepo", 999, "claude")
		key2 := generateWorkspaceKeyForIssue("testorg", "testrepo", 999, "claude")

		if key1 != key2 {
			t.Errorf("Keys should be identical: %s != %s", key1, key2)
		}

		// Test different AI models generate different keys
		keyGemini := generateWorkspaceKeyForIssue("testorg", "testrepo", 999, "gemini")
		if key1 == keyGemini {
			t.Error("Different AI models should generate different keys")
		}
	})

	t.Run("Issue to PR workspace transition logic", func(t *testing.T) {
		// Test the core logic of MoveIssueToPR without file system operations
		issueWs := &models.Workspace{
			Org:     "testorg",
			Repo:    "testrepo",
			AIModel: "claude",
			Issue:   issue,
			Path:    "/tmp/non-existent-path", // This path doesn't exist, so rename will fail
			Branch:  "codeagent/claude/issue-999-1234567890",
		}

		// MoveIssueToPR will fail due to file system operation, but that's expected in testing
		// We're mainly testing the workspace transition logic
		originalPRNumber := issueWs.PRNumber // Should be 0
		err := manager.MoveIssueToPR(issueWs, 555)

		// The method should fail due to file operations, but that's OK for testing
		if err == nil {
			// If somehow it succeeds (shouldn't in mock environment), verify the update
			if issueWs.PRNumber != 555 {
				t.Errorf("Expected PR number to be updated to 555, got %d", issueWs.PRNumber)
			}
		} else {
			// Expected failure due to file system operations in test
			// Verify that the workspace object wasn't modified on failure
			if issueWs.PRNumber != originalPRNumber {
				t.Errorf("Expected PR number to remain %d on failure, got %d",
					originalPRNumber, issueWs.PRNumber)
			}
		}
	})
}

// TestConcurrentWorkspaceOperations tests concurrent access to workspace operations
func TestConcurrentWorkspaceOperations(t *testing.T) {
	// Setup test environment
	mockRepo := &mockWorkspaceRepository{
		workspaces: make(map[string]*models.Workspace),
	}

	cfg := &config.Config{
		CodeProvider: "claude",
	}

	manager := &Manager{
		config:           cfg,
		repository:       mockRepo,
		containerService: &mockContainerService{},
		gitService:       &mockGitService{},
	}

	// Create test workspaces concurrently
	const numGoroutines = 10
	const numWorkspaces = 5

	t.Run("Concurrent workspace storage", func(t *testing.T) {
		done := make(chan bool, numGoroutines)

		// Launch multiple goroutines that store workspaces
		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer func() { done <- true }()

				for j := 0; j < numWorkspaces; j++ {
					ws := &models.Workspace{
						Org:       "testorg",
						Repo:      "testrepo",
						AIModel:   "claude",
						PRNumber:  (id * numWorkspaces) + j + 1,
						Path:      fmt.Sprintf("/tmp/concurrent-ws-%d-%d", id, j),
						CreatedAt: time.Now(),
					}

					err := mockRepo.Store(ws)
					if err != nil {
						t.Errorf("Goroutine %d failed to store workspace: %v", id, err)
					}
				}
			}(i)
		}

		// Wait for all goroutines to complete
		for i := 0; i < numGoroutines; i++ {
			<-done
		}

		// Verify all workspaces were stored
		expectedCount := numGoroutines * numWorkspaces
		actualCount := mockRepo.Count()
		if actualCount != expectedCount {
			t.Errorf("Expected %d workspaces, got %d", expectedCount, actualCount)
		}
	})

	t.Run("Concurrent workspace cleanup", func(t *testing.T) {
		// Get all workspaces
		allWorkspaces := mockRepo.GetAll()

		// Cleanup half of them concurrently
		workspaceList := make([]*models.Workspace, 0, len(allWorkspaces))
		for _, ws := range allWorkspaces {
			workspaceList = append(workspaceList, ws)
		}

		cleanupCount := len(workspaceList) / 2
		done := make(chan bool, cleanupCount)

		for i := 0; i < cleanupCount; i++ {
			go func(ws *models.Workspace) {
				defer func() { done <- true }()
				manager.CleanupWorkspace(ws)
			}(workspaceList[i])
		}

		// Wait for cleanup to complete
		for i := 0; i < cleanupCount; i++ {
			<-done
		}

		// Verify remaining count
		remainingCount := mockRepo.Count()
		expectedRemaining := len(workspaceList) - cleanupCount
		if remainingCount != expectedRemaining {
			t.Errorf("Expected %d remaining workspaces, got %d", expectedRemaining, remainingCount)
		}
	})
}

// TestWorkspaceStateConsistency tests workspace state consistency across operations
func TestWorkspaceStateConsistency(t *testing.T) {
	// Setup test environment
	mockRepo := &mockWorkspaceRepository{
		workspaces: make(map[string]*models.Workspace),
	}

	cfg := &config.Config{
		CodeProvider: "claude",
		Workspace: config.WorkspaceConfig{
			CleanupAfter: 1 * time.Hour,
		},
	}

	manager := &Manager{
		config:           cfg,
		repository:       mockRepo,
		containerService: &mockContainerService{},
		gitService:       &mockGitService{},
	}

	t.Run("Repository and manager state consistency", func(t *testing.T) {
		// Create workspace through manager
		ws := &models.Workspace{
			Org:       "testorg",
			Repo:      "testrepo",
			AIModel:   "claude",
			PRNumber:  123,
			Path:      "/tmp/consistency-test",
			CreatedAt: time.Now(),
		}

		err := mockRepo.Store(ws)
		if err != nil {
			t.Fatalf("Failed to store workspace: %v", err)
		}

		// Verify count consistency
		repoCount := mockRepo.Count()
		managerCount := manager.GetWorkspaceCount()
		if repoCount != managerCount {
			t.Errorf("Count mismatch: repository has %d, manager reports %d", repoCount, managerCount)
		}

		// Create PR object for testing
		mockOwner := &github.User{Login: github.String("testorg")}
		mockRepoObj := &github.Repository{
			Name:  github.String("testrepo"),
			Owner: mockOwner,
		}
		mockBase := &github.PullRequestBranch{Repo: mockRepoObj}

		pr := &github.PullRequest{
			Number: github.Int(123),
			Base:   mockBase,
		}

		// Test retrieval consistency
		retrievedWs := manager.GetWorkspaceByPR(pr, "claude")
		if retrievedWs == nil {
			t.Error("Manager should be able to retrieve workspace stored in repository")
		}

		if retrievedWs != nil && retrievedWs.PRNumber != ws.PRNumber {
			t.Errorf("Retrieved workspace PR number mismatch: expected %d, got %d",
				ws.PRNumber, retrievedWs.PRNumber)
		}
	})

	t.Run("Workspace key generation consistency", func(t *testing.T) {
		// Test that same parameters always generate same keys
		testCases := []struct {
			org     string
			repo    string
			number  int
			aiModel string
			keyType string
		}{
			{"org1", "repo1", 1, "claude", "PR"},
			{"org1", "repo1", 1, "", "PR"},
			{"org2", "repo2", 2, "gemini", "PR"},
			{"org1", "repo1", 1, "claude", "Issue"},
			{"org1", "repo1", 1, "", "Issue"},
		}

		for _, tc := range testCases {
			var key1, key2 string
			if tc.keyType == "Issue" {
				key1 = generateWorkspaceKeyForIssue(tc.org, tc.repo, tc.number, tc.aiModel)
				key2 = generateWorkspaceKeyForIssue(tc.org, tc.repo, tc.number, tc.aiModel)
			} else {
				key1 = generateWorkspaceKey(tc.org, tc.repo, tc.number, tc.aiModel)
				key2 = generateWorkspaceKey(tc.org, tc.repo, tc.number, tc.aiModel)
			}

			if key1 != key2 {
				t.Errorf("Key generation inconsistent for %+v: %s != %s", tc, key1, key2)
			}
		}
	})

	t.Run("Expired workspace detection consistency", func(t *testing.T) {
		now := time.Now()

		// Create fresh and expired workspaces
		freshWs := &models.Workspace{
			Org:       "testorg",
			Repo:      "testrepo",
			AIModel:   "claude",
			PRNumber:  200,
			Path:      "/tmp/fresh-ws",
			CreatedAt: now.Add(-30 * time.Minute), // 30 minutes ago
		}

		expiredWs := &models.Workspace{
			Org:       "testorg",
			Repo:      "testrepo",
			AIModel:   "gemini",
			PRNumber:  201,
			Path:      "/tmp/expired-ws",
			CreatedAt: now.Add(-90 * time.Minute), // 90 minutes ago
		}

		mockRepo.Store(freshWs)
		mockRepo.Store(expiredWs)

		// Get expired workspaces
		expired := manager.GetExpiredWorkspaces()
		if len(expired) != 1 {
			t.Errorf("Expected 1 expired workspace, got %d", len(expired))
		}

		if len(expired) > 0 && expired[0].PRNumber != 201 {
			t.Errorf("Expected expired workspace PR 201, got %d", expired[0].PRNumber)
		}
	})
}
