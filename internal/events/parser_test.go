package events

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/qiniu/codeagent/pkg/models"

	"github.com/google/go-github/v58/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventParser_ParseIssueCommentEvent(t *testing.T) {
	parser := NewEventParser()
	ctx := context.Background()

	// 创建测试数据
	event := &github.IssueCommentEvent{
		Action: github.String("created"),
		Repo: &github.Repository{
			FullName: github.String("test/repo"),
			Name:     github.String("repo"),
			Owner: &github.User{
				Login: github.String("test"),
			},
		},
		Sender: &github.User{
			Login: github.String("testuser"),
		},
		Issue: &github.Issue{
			Number: github.Int(123),
			Title:  github.String("Test Issue"),
		},
		Comment: &github.IssueComment{
			Body: github.String("/code -claude implement this feature"),
		},
	}

	// 序列化为JSON
	payload, err := json.Marshal(event)
	require.NoError(t, err)

	// 解析事件
	parsedCtx, err := parser.ParseWebhookEvent(ctx, "issue_comment", "test-delivery-id", payload)
	require.NoError(t, err)

	// 验证类型
	issueCommentCtx, ok := parsedCtx.(*models.IssueCommentContext)
	require.True(t, ok, "Expected IssueCommentContext")

	// 验证字段
	assert.Equal(t, models.EventIssueComment, issueCommentCtx.GetEventType())
	assert.Equal(t, "test/repo", issueCommentCtx.GetRepository().GetFullName())
	assert.Equal(t, "testuser", issueCommentCtx.GetSender().GetLogin())
	assert.Equal(t, "created", issueCommentCtx.GetEventAction())
	assert.Equal(t, "test-delivery-id", issueCommentCtx.GetDeliveryID())
	assert.Equal(t, 123, issueCommentCtx.Issue.GetNumber())
	assert.Equal(t, "/code -claude implement this feature", issueCommentCtx.Comment.GetBody())
	assert.False(t, issueCommentCtx.IsPRComment)
}

func TestEventParser_ParsePRCommentEvent(t *testing.T) {
	parser := NewEventParser()
	ctx := context.Background()

	// 创建测试数据 - PR评论（通过Issue.PullRequestLinks判断）
	event := &github.IssueCommentEvent{
		Action: github.String("created"),
		Repo: &github.Repository{
			FullName: github.String("test/repo"),
			Name:     github.String("repo"),
			Owner: &github.User{
				Login: github.String("test"),
			},
		},
		Sender: &github.User{
			Login: github.String("testuser"),
		},
		Issue: &github.Issue{
			Number: github.Int(123),
			Title:  github.String("Test PR"),
			PullRequestLinks: &github.PullRequestLinks{
				URL: github.String("https://api.github.com/repos/test/repo/pulls/123"),
			},
		},
		Comment: &github.IssueComment{
			Body: github.String("/continue -gemini fix the bug"),
		},
	}

	// 序列化为JSON
	payload, err := json.Marshal(event)
	require.NoError(t, err)

	// 解析事件
	parsedCtx, err := parser.ParseWebhookEvent(ctx, "issue_comment", "test-delivery-id", payload)
	require.NoError(t, err)

	// 验证类型
	issueCommentCtx, ok := parsedCtx.(*models.IssueCommentContext)
	require.True(t, ok, "Expected IssueCommentContext")

	// 验证这是PR评论
	assert.True(t, issueCommentCtx.IsPRComment)
}

func TestHasCommand(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected *models.CommandInfo
		hasCmd   bool
	}{
		{
			name:    "code command with claude",
			content: "/code -claude implement this feature",
			expected: &models.CommandInfo{
				Command: "/code",
				AIModel: "claude",
				Args:    "implement this feature",
				RawText: "/code -claude implement this feature",
			},
			hasCmd: true,
		},
		{
			name:    "continue command with gemini",
			content: "/continue -gemini fix the issue",
			expected: &models.CommandInfo{
				Command: "/continue",
				AIModel: "gemini",
				Args:    "fix the issue",
				RawText: "/continue -gemini fix the issue",
			},
			hasCmd: true,
		},
		{
			name:    "fix command without model",
			content: "/fix please resolve this",
			expected: &models.CommandInfo{
				Command: "/fix",
				AIModel: "",
				Args:    "please resolve this",
				RawText: "/fix please resolve this",
			},
			hasCmd: true,
		},
		{
			name:     "no command",
			content:  "just a regular comment",
			expected: nil,
			hasCmd:   false,
		},
		{
			name:     "command in middle",
			content:  "please /code this feature",
			expected: nil,
			hasCmd:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建测试上下文
			ctx := &models.IssueCommentContext{
				Comment: &github.IssueComment{
					Body: github.String(tt.content),
				},
			}

			cmdInfo, hasCmd := models.HasCommand(ctx)
			assert.Equal(t, tt.hasCmd, hasCmd)

			if tt.hasCmd {
				require.NotNil(t, cmdInfo)
				assert.Equal(t, tt.expected.Command, cmdInfo.Command)
				assert.Equal(t, tt.expected.AIModel, cmdInfo.AIModel)
				assert.Equal(t, tt.expected.Args, cmdInfo.Args)
				assert.Equal(t, tt.expected.RawText, cmdInfo.RawText)
			} else {
				assert.Nil(t, cmdInfo)
			}
		})
	}
}
