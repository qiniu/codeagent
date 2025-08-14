package context

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-github/v58/github"
	"github.com/qiniu/x/xlog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitHubContextInjector_validateTemplateData(t *testing.T) {
	injector := NewGitHubContextInjector()

	tests := []struct {
		name     string
		data     *GitHubTemplateData
		expected []string
	}{
		{
			name: "complete issue data",
			data: &GitHubTemplateData{
				GITHUB_REPOSITORY:   "owner/repo",
				GITHUB_EVENT_TYPE:   "issue_comment",
				GITHUB_TRIGGER_USER: "user1",
				GITHUB_IS_ISSUE:     true,
				GITHUB_ISSUE_AUTHOR: "author1",
				GITHUB_ISSUE_TITLE:  "Test Issue",
			},
			expected: make([]string, 0),
		},
		{
			name: "missing core fields",
			data: &GitHubTemplateData{
				GITHUB_IS_ISSUE: true,
			},
			expected: []string{"GITHUB_REPOSITORY", "GITHUB_EVENT_TYPE", "GITHUB_TRIGGER_USER", "GITHUB_ISSUE_AUTHOR", "GITHUB_ISSUE_TITLE"},
		},
		{
			name: "complete PR data",
			data: &GitHubTemplateData{
				GITHUB_REPOSITORY:   "owner/repo",
				GITHUB_EVENT_TYPE:   "pull_request",
				GITHUB_TRIGGER_USER: "user1",
				GITHUB_IS_PR:        true,
				GITHUB_PR_AUTHOR:    "author1",
				GITHUB_PR_TITLE:     "Test PR",
				GITHUB_BRANCH_NAME:  "feature-branch",
			},
			expected: make([]string, 0),
		},
		{
			name: "missing PR fields",
			data: &GitHubTemplateData{
				GITHUB_REPOSITORY:   "owner/repo",
				GITHUB_EVENT_TYPE:   "pull_request",
				GITHUB_TRIGGER_USER: "user1",
				GITHUB_IS_PR:        true,
			},
			expected: []string{"GITHUB_PR_AUTHOR", "GITHUB_PR_TITLE", "GITHUB_BRANCH_NAME"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			missing := injector.validateTemplateData(tt.data)
			assert.Equal(t, tt.expected, missing)
		})
	}
}

func TestGitHubContextInjector_extractRepoMetadata(t *testing.T) {
	injector := NewGitHubContextInjector()

	tests := []struct {
		name          string
		repository    string
		expectedOwner string
		expectedName  string
	}{
		{
			name:          "valid repository",
			repository:    "qiniu/codeagent",
			expectedOwner: "qiniu",
			expectedName:  "codeagent",
		},
		{
			name:          "repository with more parts",
			repository:    "github.com/qiniu/codeagent",
			expectedOwner: "github.com",
			expectedName:  "qiniu",
		},
		{
			name:          "single part repository",
			repository:    "codeagent",
			expectedOwner: "",
			expectedName:  "codeagent",
		},
		{
			name:          "empty repository",
			repository:    "",
			expectedOwner: "",
			expectedName:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, name := injector.extractRepoMetadata(tt.repository)
			assert.Equal(t, tt.expectedOwner, owner)
			assert.Equal(t, tt.expectedName, name)
		})
	}
}

func TestGitHubContextInjector_buildTemplateDataWithValidation(t *testing.T) {
	injector := NewGitHubContextInjector()

	tests := []struct {
		name        string
		event       *GitHubEvent
		expectError bool
		validate    func(t *testing.T, data *GitHubTemplateData)
	}{
		{
			name:        "nil event",
			event:       nil,
			expectError: true,
		},
		{
			name: "issue comment event",
			event: &GitHubEvent{
				Type:           "issue_comment",
				Repository:     "qiniu/codeagent",
				TriggerUser:    "user1",
				Action:         "created",
				TriggerComment: "/code implement feature",
				Issue: &github.Issue{
					Number: github.Int(123),
					Title:  github.String("Test Issue"),
					Body:   github.String("Issue body"),
					User:   &github.User{Login: github.String("issue_author")},
					Labels: []*github.Label{
						{Name: github.String("bug")},
						{Name: github.String("enhancement")},
					},
				},
				IssueComments: []string{"Comment 1", "Comment 2"},
			},
			expectError: false,
			validate: func(t *testing.T, data *GitHubTemplateData) {
				assert.Equal(t, "qiniu/codeagent", data.GITHUB_REPOSITORY)
				assert.Equal(t, "issue_comment", data.GITHUB_EVENT_TYPE)
				assert.Equal(t, "user1", data.GITHUB_TRIGGER_USER)
				assert.Equal(t, "created", data.GITHUB_EVENT_ACTION)
				assert.Equal(t, "/code implement feature", data.GITHUB_TRIGGER_COMMENT)
				assert.Equal(t, "qiniu", data.GITHUB_REPO_OWNER)
				assert.Equal(t, "codeagent", data.GITHUB_REPO_NAME)
				assert.True(t, data.GITHUB_IS_ISSUE)
				assert.False(t, data.GITHUB_IS_PR)
				assert.Equal(t, 123, data.GITHUB_ISSUE_NUMBER)
				assert.Equal(t, "Test Issue", data.GITHUB_ISSUE_TITLE)
				assert.Equal(t, "Issue body", data.GITHUB_ISSUE_BODY)
				assert.Equal(t, "issue_author", data.GITHUB_ISSUE_AUTHOR)
				assert.Equal(t, []string{"bug", "enhancement"}, data.GITHUB_ISSUE_LABELS)
				assert.Equal(t, []string{"Comment 1", "Comment 2"}, data.GITHUB_ISSUE_COMMENTS)
			},
		},
		{
			name: "pull request event",
			event: &GitHubEvent{
				Type:        "pull_request",
				Repository:  "qiniu/codeagent",
				TriggerUser: "user1",
				Action:      "opened",
				PullRequest: &github.PullRequest{
					Number: github.Int(456),
					Title:  github.String("Test PR"),
					Body:   github.String("PR body"),
					User:   &github.User{Login: github.String("pr_author")},
					Head:   &github.PullRequestBranch{Ref: github.String("feature-branch")},
					Base:   &github.PullRequestBranch{Ref: github.String("main")},
				},
				ChangedFiles:   []string{"file1.go", "file2.go"},
				PRComments:     []string{"PR Comment 1"},
				ReviewComments: []string{"Review Comment 1"},
			},
			expectError: false,
			validate: func(t *testing.T, data *GitHubTemplateData) {
				assert.True(t, data.GITHUB_IS_PR)
				assert.False(t, data.GITHUB_IS_ISSUE)
				assert.Equal(t, 456, data.GITHUB_PR_NUMBER)
				assert.Equal(t, "Test PR", data.GITHUB_PR_TITLE)
				assert.Equal(t, "PR body", data.GITHUB_PR_BODY)
				assert.Equal(t, "pr_author", data.GITHUB_PR_AUTHOR)
				assert.Equal(t, "feature-branch", data.GITHUB_BRANCH_NAME)
				assert.Equal(t, "main", data.GITHUB_BASE_BRANCH)
				assert.Equal(t, []string{"file1.go", "file2.go"}, data.GITHUB_CHANGED_FILES)
				assert.Equal(t, []string{"PR Comment 1"}, data.GITHUB_PR_COMMENTS)
				assert.Equal(t, []string{"Review Comment 1"}, data.GITHUB_REVIEW_COMMENTS)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := injector.buildTemplateDataWithValidation(tt.event)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, data)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, data)
				if tt.validate != nil {
					tt.validate(t, data)
				}
			}
		})
	}
}

func TestGitHubContextInjector_simpleVariableReplacement(t *testing.T) {
	injector := NewGitHubContextInjector()

	data := &GitHubTemplateData{
		GITHUB_REPOSITORY:      "qiniu/codeagent",
		GITHUB_EVENT_TYPE:      "issue_comment",
		GITHUB_TRIGGER_USER:    "user1",
		GITHUB_EVENT_ACTION:    "created",
		GITHUB_REPO_OWNER:      "qiniu",
		GITHUB_REPO_NAME:       "codeagent",
		GITHUB_ACTOR:           "actor1",
		GITHUB_ISSUE_NUMBER:    123,
		GITHUB_ISSUE_TITLE:     "Test Issue",
		GITHUB_ISSUE_BODY:      "Issue body",
		GITHUB_ISSUE_AUTHOR:    "issue_author",
		GITHUB_PR_NUMBER:       456,
		GITHUB_PR_TITLE:        "Test PR",
		GITHUB_PR_BODY:         "PR body",
		GITHUB_PR_AUTHOR:       "pr_author",
		GITHUB_BRANCH_NAME:     "feature-branch",
		GITHUB_BASE_BRANCH:     "main",
		GITHUB_TRIGGER_COMMENT: "/code implement feature",
	}

	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name:     "replace core variables",
			content:  "Repository: $GITHUB_REPOSITORY, Event: $GITHUB_EVENT_TYPE, User: $GITHUB_TRIGGER_USER",
			expected: "Repository: qiniu/codeagent, Event: issue_comment, User: user1",
		},
		{
			name:     "replace issue variables",
			content:  "Issue #$GITHUB_ISSUE_NUMBER: $GITHUB_ISSUE_TITLE by $GITHUB_ISSUE_AUTHOR",
			expected: "Issue #123: Test Issue by issue_author",
		},
		{
			name:     "replace PR variables",
			content:  "PR #$GITHUB_PR_NUMBER: $GITHUB_PR_TITLE by $GITHUB_PR_AUTHOR ($GITHUB_BRANCH_NAME -> $GITHUB_BASE_BRANCH)",
			expected: "PR #456: Test PR by pr_author (feature-branch -> main)",
		},
		{
			name:     "replace new variables",
			content:  "Action: $GITHUB_EVENT_ACTION, Owner: $GITHUB_REPO_OWNER, Name: $GITHUB_REPO_NAME, Actor: $GITHUB_ACTOR",
			expected: "Action: created, Owner: qiniu, Name: codeagent, Actor: actor1",
		},
		{
			name:     "replace trigger comment",
			content:  "Triggered by: $GITHUB_TRIGGER_COMMENT",
			expected: "Triggered by: /code implement feature",
		},
		{
			name:     "no replacements needed",
			content:  "This is just regular text without variables",
			expected: "This is just regular text without variables",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := injector.simpleVariableReplacement(tt.content, data)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGitHubContextInjector_InjectContextWithLogging(t *testing.T) {
	injector := NewGitHubContextInjector()

	// Create a test logger
	xl := xlog.New("test")

	event := &GitHubEvent{
		Type:           "issue_comment",
		Repository:     "qiniu/codeagent",
		TriggerUser:    "user1",
		Action:         "created",
		TriggerComment: "/code implement feature",
		Issue: &github.Issue{
			Number: github.Int(123),
			Title:  github.String("Test Issue"),
			Body:   github.String("Issue body"),
			User:   &github.User{Login: github.String("issue_author")},
		},
	}

	tests := []struct {
		name           string
		commandContent string
		expectedSubstr []string
	}{
		{
			name:           "simple variable replacement",
			commandContent: "Working on issue: $GITHUB_ISSUE_TITLE in repository $GITHUB_REPOSITORY",
			expectedSubstr: []string{"Working on issue: Test Issue in repository qiniu/codeagent"},
		},
		{
			name:           "template with Go template syntax",
			commandContent: "Issue #{{.GITHUB_ISSUE_NUMBER}}: {{.GITHUB_ISSUE_TITLE}} by {{.GITHUB_ISSUE_AUTHOR}}",
			expectedSubstr: []string{"Issue #123: Test Issue by issue_author"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			result, err := injector.InjectContextWithLogging(ctx, tt.commandContent, event, xl)

			assert.NoError(t, err)
			assert.NotEmpty(t, result)

			// Debug: print actual result
			t.Logf("Input: %s", tt.commandContent)
			t.Logf("Output: %s", result)

			for _, substr := range tt.expectedSubstr {
				assert.Contains(t, result, substr)
			}
		})
	}
}

func TestTemplateError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *TemplateError
		expected string
	}{
		{
			name: "error with variable",
			err: &TemplateError{
				Type:      "execute",
				Variable:  "GITHUB_ISSUE_TITLE",
				Cause:     assert.AnError,
				Timestamp: time.Now(),
			},
			expected: "template execute error: assert.AnError general error for testing (variable: GITHUB_ISSUE_TITLE)",
		},
		{
			name: "error without variable",
			err: &TemplateError{
				Type:      "parse",
				Cause:     assert.AnError,
				Timestamp: time.Now(),
			},
			expected: "template parse error: assert.AnError general error for testing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.err.Error()
			assert.Equal(t, tt.expected, result)
		})
	}
}
