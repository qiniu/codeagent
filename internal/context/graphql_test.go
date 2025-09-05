package context

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestGraphQLQueryStructures 测试GraphQL查询结构体的基本结构
func TestGraphQLQueryStructures(t *testing.T) {
	// 测试PR查询结构体
	var prQuery PullRequestContextQuery
	assert.NotNil(t, prQuery)

	// 测试Issue查询结构体
	var issueQuery IssueContextQuery
	assert.NotNil(t, issueQuery)

	// 测试结果数据结构
	prContext := &GraphQLPRContext{
		Repository: "test/repo",
		PR: GraphQLPullRequest{
			Number: 123,
			Title:  "Test PR",
		},
	}
	assert.Equal(t, "test/repo", prContext.Repository)
	assert.Equal(t, 123, prContext.PR.Number)
	assert.Equal(t, "Test PR", prContext.PR.Title)

	issueContext := &GraphQLIssueContext{
		Repository: "test/repo",
		Issue: GraphQLIssue{
			Number: 456,
			Title:  "Test Issue",
		},
	}
	assert.Equal(t, "test/repo", issueContext.Repository)
	assert.Equal(t, 456, issueContext.Issue.Number)
	assert.Equal(t, "Test Issue", issueContext.Issue.Title)
}

// TestRateLimitInfo 测试速率限制信息结构
func TestRateLimitInfo(t *testing.T) {
	rateLimitInfo := &RateLimitInfo{
		Limit:     5000,
		Cost:      10,
		Remaining: 4990,
	}

	assert.Equal(t, 5000, rateLimitInfo.Limit)
	assert.Equal(t, 10, rateLimitInfo.Cost)
	assert.Equal(t, 4990, rateLimitInfo.Remaining)
}

// TestGraphQLDataStructures 测试GraphQL数据结构的完整性
func TestGraphQLDataStructures(t *testing.T) {
	// 测试文件变更结构
	fileChange := GraphQLFileChange{
		Path:       "test.go",
		Additions:  10,
		Deletions:  5,
		ChangeType: "modified",
	}
	assert.Equal(t, "test.go", fileChange.Path)
	assert.Equal(t, 10, fileChange.Additions)
	assert.Equal(t, 5, fileChange.Deletions)
	assert.Equal(t, "modified", fileChange.ChangeType)

	// 测试用户结构
	user := GraphQLUser{
		Login:     "testuser",
		Name:      "Test User",
		AvatarURL: "https://avatar.url",
	}
	assert.Equal(t, "testuser", user.Login)
	assert.Equal(t, "Test User", user.Name)
	assert.Equal(t, "https://avatar.url", user.AvatarURL)

	// 测试标签结构
	label := GraphQLLabel{
		Name:  "bug",
		Color: "ff0000",
	}
	assert.Equal(t, "bug", label.Name)
	assert.Equal(t, "ff0000", label.Color)

	// 测试评论结构
	comment := GraphQLComment{
		ID:     "comment123",
		Body:   "This is a test comment",
		Author: user,
	}
	assert.Equal(t, "comment123", comment.ID)
	assert.Equal(t, "This is a test comment", comment.Body)
	assert.Equal(t, "testuser", comment.Author.Login)

	// 测试评审结构
	review := GraphQLReview{
		ID:     "review123",
		Body:   "This is a test review",
		State:  "APPROVED",
		Author: user,
	}
	assert.Equal(t, "review123", review.ID)
	assert.Equal(t, "This is a test review", review.Body)
	assert.Equal(t, "APPROVED", review.State)
	assert.Equal(t, "testuser", review.Author.Login)

	// 测试行级评论结构
	reviewComment := GraphQLReviewComment{
		ID:       "reviewcomment123",
		Body:     "This is a test review comment",
		Path:     "test.go",
		Line:     42,
		DiffHunk: "@@ -1,3 +1,4 @@",
		Author:   user,
	}
	assert.Equal(t, "reviewcomment123", reviewComment.ID)
	assert.Equal(t, "This is a test review comment", reviewComment.Body)
	assert.Equal(t, "test.go", reviewComment.Path)
	assert.Equal(t, 42, reviewComment.Line)
	assert.Equal(t, "@@ -1,3 +1,4 @@", reviewComment.DiffHunk)
	assert.Equal(t, "testuser", reviewComment.Author.Login)
}
