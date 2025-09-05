package context

import (
	"time"

	"github.com/shurcooL/githubv4"
)

// GraphQL查询结构体定义，用于GitHub API v4

// PullRequestContextQuery PR完整上下文查询
type PullRequestContextQuery struct {
	Repository struct {
		DefaultBranchRef struct {
			Name githubv4.String
		}
		PullRequest struct {
			Number    githubv4.Int
			Title     githubv4.String
			Body      githubv4.String
			State     githubv4.PullRequestState
			Additions githubv4.Int
			Deletions githubv4.Int
			Commits   struct {
				TotalCount githubv4.Int
			}
			Author struct {
				Login     githubv4.String
				AvatarURL githubv4.String `graphql:"avatarUrl"`
			}
			BaseRefName githubv4.String
			HeadRefName githubv4.String

			// PR 文件变更
			Files struct {
				Nodes []struct {
					Path       githubv4.String
					Additions  githubv4.Int
					Deletions  githubv4.Int
					ChangeType githubv4.String
				}
			} `graphql:"files(first: 100)"`

			// Issue 评论 (PR也是一种Issue)
			Comments struct {
				Nodes []struct {
					ID        githubv4.String
					Body      githubv4.String
					CreatedAt githubv4.DateTime
					UpdatedAt githubv4.DateTime
					Author    struct {
						Login githubv4.String
						User  struct {
							Name githubv4.String
						} `graphql:"... on User"`
					}
				}
			} `graphql:"comments(first: 50, orderBy: {field: UPDATED_AT, direction: ASC})"`

			// 代码评审
			Reviews struct {
				Nodes []struct {
					ID        githubv4.String
					Body      githubv4.String
					State     githubv4.PullRequestReviewState
					CreatedAt githubv4.DateTime
					Author    struct {
						Login githubv4.String
					}
					// 评审中的行级评论
					Comments struct {
						Nodes []struct {
							ID       githubv4.String
							Body     githubv4.String
							Path     githubv4.String
							Line     githubv4.Int
							DiffHunk githubv4.String
							Author   struct {
								Login githubv4.String
							}
							CreatedAt githubv4.DateTime
						}
					} `graphql:"comments(first: 50)"`
				}
			} `graphql:"reviews(first: 20, states: [APPROVED, CHANGES_REQUESTED, COMMENTED])"`
		} `graphql:"pullRequest(number: $number)"`
	} `graphql:"repository(owner: $owner, name: $name)"`

	// 速率限制监控
	RateLimit struct {
		Limit     githubv4.Int
		Cost      githubv4.Int
		Remaining githubv4.Int
		ResetAt   githubv4.DateTime
	}
}

// IssueContextQuery Issue上下文查询
type IssueContextQuery struct {
	Repository struct {
		Issue struct {
			Number githubv4.Int
			Title  githubv4.String
			Body   githubv4.String
			State  githubv4.IssueState
			Author struct {
				Login     githubv4.String
				AvatarURL githubv4.String `graphql:"avatarUrl"`
			}
			Labels struct {
				Nodes []struct {
					Name  githubv4.String
					Color githubv4.String
				}
			} `graphql:"labels(first: 10)"`
			Comments struct {
				Nodes []struct {
					ID        githubv4.String
					Body      githubv4.String
					CreatedAt githubv4.DateTime
					Author    struct {
						Login githubv4.String
					}
				}
			} `graphql:"comments(first: 50, orderBy: {field: UPDATED_AT, direction: ASC})"`
		} `graphql:"issue(number: $number)"`
	} `graphql:"repository(owner: $owner, name: $name)"`

	// 速率限制监控
	RateLimit struct {
		Limit     githubv4.Int
		Cost      githubv4.Int
		Remaining githubv4.Int
		ResetAt   githubv4.DateTime
	}
}

// RateLimitInfo 速率限制信息
type RateLimitInfo struct {
	Limit     int
	Cost      int
	Remaining int
	ResetAt   time.Time
}

// GraphQLContextCollector GraphQL上下文收集器接口
type GraphQLContextCollector interface {
	// 使用GraphQL收集PR完整上下文
	CollectPRContextWithGraphQL(owner, repo string, prNumber int) (*GraphQLPRContext, *RateLimitInfo, error)

	// 使用GraphQL收集Issue完整上下文
	CollectIssueContextWithGraphQL(owner, repo string, issueNumber int) (*GraphQLIssueContext, *RateLimitInfo, error)
}

// GraphQLPRContext GraphQL PR上下文结果
type GraphQLPRContext struct {
	Repository     string
	DefaultBranch  string
	PR             GraphQLPullRequest
	Files          []GraphQLFileChange
	Comments       []GraphQLComment
	Reviews        []GraphQLReview
	ReviewComments []GraphQLReviewComment
}

// GraphQLIssueContext GraphQL Issue上下文结果
type GraphQLIssueContext struct {
	Repository string
	Issue      GraphQLIssue
	Comments   []GraphQLComment
}

// GraphQLPullRequest GraphQL PR信息
type GraphQLPullRequest struct {
	Number      int
	Title       string
	Body        string
	State       string
	Additions   int
	Deletions   int
	Commits     int
	Author      GraphQLUser
	BaseRefName string
	HeadRefName string
}

// GraphQLIssue GraphQL Issue信息
type GraphQLIssue struct {
	Number int
	Title  string
	Body   string
	State  string
	Author GraphQLUser
	Labels []GraphQLLabel
}

// GraphQLUser GraphQL用户信息
type GraphQLUser struct {
	Login     string
	Name      string
	AvatarURL string
}

// GraphQLLabel GraphQL标签信息
type GraphQLLabel struct {
	Name  string
	Color string
}

// GraphQLFileChange GraphQL文件变更信息
type GraphQLFileChange struct {
	Path       string
	Additions  int
	Deletions  int
	ChangeType string
}

// GraphQLComment GraphQL评论信息
type GraphQLComment struct {
	ID        string
	Body      string
	Author    GraphQLUser
	CreatedAt time.Time
	UpdatedAt time.Time
}

// GraphQLReview GraphQL评审信息
type GraphQLReview struct {
	ID        string
	Body      string
	State     string
	Author    GraphQLUser
	CreatedAt time.Time
	Comments  []GraphQLReviewComment
}

// GraphQLReviewComment GraphQL行级评论信息
type GraphQLReviewComment struct {
	ID        string
	Body      string
	Path      string
	Line      int
	DiffHunk  string
	Author    GraphQLUser
	CreatedAt time.Time
}
