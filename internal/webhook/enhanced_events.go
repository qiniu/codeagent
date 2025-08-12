package webhook

import (
	"github.com/google/go-github/v58/github"
	"github.com/qiniu/codeagent/internal/content"
)

// EnhancedIssueCommentEvent wraps GitHub issue comment event with rich content processing
type EnhancedIssueCommentEvent struct {
	*github.IssueCommentEvent
	CommentRichContent *content.RichContent `json:"comment_rich_content,omitempty"`
	IssueRichContent   *content.RichContent `json:"issue_rich_content,omitempty"`
	FormattedComment   string               `json:"formatted_comment,omitempty"`
	FormattedIssue     string               `json:"formatted_issue,omitempty"`
}

// EnhancedPullRequestReviewCommentEvent wraps GitHub PR review comment event with rich content
type EnhancedPullRequestReviewCommentEvent struct {
	*github.PullRequestReviewCommentEvent
	CommentRichContent *content.RichContent `json:"comment_rich_content,omitempty"`
	FormattedComment   string               `json:"formatted_comment,omitempty"`
}

// EnhancedPullRequestReviewEvent wraps GitHub PR review event with rich content
type EnhancedPullRequestReviewEvent struct {
	*github.PullRequestReviewEvent
	ReviewRichContent *content.RichContent `json:"review_rich_content,omitempty"`
	FormattedReview   string               `json:"formatted_review,omitempty"`
}

// GetEnhancedCommentBody returns the formatted comment if available, otherwise raw comment
func (e *EnhancedIssueCommentEvent) GetEnhancedCommentBody() string {
	if e.FormattedComment != "" {
		return e.FormattedComment
	}
	return e.Comment.GetBody()
}

// GetEnhancedIssueBody returns the formatted issue body if available, otherwise raw issue body
func (e *EnhancedIssueCommentEvent) GetEnhancedIssueBody() string {
	if e.FormattedIssue != "" {
		return e.FormattedIssue
	}
	return e.Issue.GetBody()
}

// GetEnhancedCommentBody returns the formatted comment if available, otherwise raw comment
func (e *EnhancedPullRequestReviewCommentEvent) GetEnhancedCommentBody() string {
	if e.FormattedComment != "" {
		return e.FormattedComment
	}
	return e.Comment.GetBody()
}

// GetEnhancedReviewBody returns the formatted review body if available, otherwise raw review body
func (e *EnhancedPullRequestReviewEvent) GetEnhancedReviewBody() string {
	if e.FormattedReview != "" {
		return e.FormattedReview
	}
	return e.Review.GetBody()
}