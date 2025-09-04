package github

import (
	"context"
	"fmt"
	"net/http"

	"github.com/shurcooL/githubv4"
	"golang.org/x/oauth2"

	"github.com/qiniu/x/log"
)

// GraphQLClient wraps the GitHub GraphQL API client
type GraphQLClient struct {
	client *githubv4.Client
}

// NewGraphQLClient creates a new GraphQL client with authentication
func NewGraphQLClient(token string) *GraphQLClient {
	src := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	httpClient := oauth2.NewClient(context.Background(), src)

	return &GraphQLClient{
		client: githubv4.NewClient(httpClient),
	}
}

// NewGraphQLClientWithHTTPClient creates a new GraphQL client with custom HTTP client
func NewGraphQLClientWithHTTPClient(httpClient *http.Client) *GraphQLClient {
	return &GraphQLClient{
		client: githubv4.NewClient(httpClient),
	}
}

// GetPullRequestContext retrieves complete PR context using a single GraphQL query
func (gc *GraphQLClient) GetPullRequestContext(ctx context.Context, owner, repo string, number int) (*githubcontext.GraphQLPullRequestContext, error) {
	variables := githubcontext.GraphQLQueryVariables{
		Owner:  owner,
		Name:   repo,
		Number: number,
	}

	var query githubcontext.PullRequestContextQuery
	err := gc.client.Query(ctx, &query, variables)
	if err != nil {
		return nil, fmt.Errorf("failed to execute GraphQL query for PR %d: %w", number, err)
	}

	// Transform GraphQL response to our context structure
	prContext := &githubcontext.GraphQLPullRequestContext{
		Number:       query.Repository.PullRequest.Number,
		Title:        query.Repository.PullRequest.Title,
		Body:         query.Repository.PullRequest.Body,
		State:        query.Repository.PullRequest.State,
		Additions:    query.Repository.PullRequest.Additions,
		Deletions:    query.Repository.PullRequest.Deletions,
		Commits:      query.Repository.PullRequest.Commits.TotalCount,
		Author:       query.Repository.PullRequest.Author.Login,
		AuthorAvatar: query.Repository.PullRequest.Author.AvatarURL,
		BaseRef:      query.Repository.PullRequest.BaseRefName,
		HeadRef:      query.Repository.PullRequest.HeadRefName,
		RateLimit: githubcontext.RateLimitInfo{
			Limit:     query.RateLimit.Limit,
			Cost:      query.RateLimit.Cost,
			Remaining: query.RateLimit.Remaining,
			ResetAt:   query.RateLimit.ResetAt,
		},
	}

	// Transform file changes
	prContext.Files = make([]githubcontext.GraphQLFileChange, len(query.Repository.PullRequest.Files.Nodes))
	for i, file := range query.Repository.PullRequest.Files.Nodes {
		prContext.Files[i] = githubcontext.GraphQLFileChange{
			Path:       file.Path,
			Additions:  file.Additions,
			Deletions:  file.Deletions,
			ChangeType: file.ChangeType,
		}
	}

	// Transform comments
	prContext.Comments = make([]githubcontext.GraphQLComment, len(query.Repository.PullRequest.Comments.Nodes))
	for i, comment := range query.Repository.PullRequest.Comments.Nodes {
		prContext.Comments[i] = githubcontext.GraphQLComment{
			ID:        comment.ID,
			Body:      comment.Body,
			Author:    comment.Author.Login,
			CreatedAt: comment.CreatedAt,
			UpdatedAt: comment.UpdatedAt,
		}
	}

	// Transform reviews
	prContext.Reviews = make([]githubcontext.GraphQLReview, len(query.Repository.PullRequest.Reviews.Nodes))
	for i, review := range query.Repository.PullRequest.Reviews.Nodes {
		reviewComments := make([]githubcontext.GraphQLComment, len(review.Comments.Nodes))
		for j, comment := range review.Comments.Nodes {
			reviewComments[j] = githubcontext.GraphQLComment{
				ID:        comment.ID,
				Body:      comment.Body,
				Author:    comment.Author.Login,
				CreatedAt: comment.CreatedAt,
				Path:      comment.Path,
				Line:      comment.Line,
				DiffHunk:  comment.DiffHunk,
			}
		}

		prContext.Reviews[i] = githubcontext.GraphQLReview{
			ID:        review.ID,
			Body:      review.Body,
			State:     review.State,
			Author:    review.Author.Login,
			CreatedAt: review.CreatedAt,
			Comments:  reviewComments,
		}
	}

	log.Infof("GraphQL query for PR #%d completed - Cost: %d, Remaining: %d", 
		number, prContext.RateLimit.Cost, prContext.RateLimit.Remaining)

	return prContext, nil
}

// GetIssueContext retrieves complete Issue context using a single GraphQL query
func (gc *GraphQLClient) GetIssueContext(ctx context.Context, owner, repo string, number int) (*githubcontext.GraphQLIssueContext, error) {
	variables := githubcontext.GraphQLQueryVariables{
		Owner:  owner,
		Name:   repo,
		Number: number,
	}

	var query githubcontext.IssueContextQuery
	err := gc.client.Query(ctx, &query, variables)
	if err != nil {
		return nil, fmt.Errorf("failed to execute GraphQL query for Issue %d: %w", number, err)
	}

	// Transform GraphQL response to our context structure
	issueContext := &githubcontext.GraphQLIssueContext{
		Number:       query.Repository.Issue.Number,
		Title:        query.Repository.Issue.Title,
		Body:         query.Repository.Issue.Body,
		State:        query.Repository.Issue.State,
		Author:       query.Repository.Issue.Author.Login,
		AuthorAvatar: query.Repository.Issue.Author.AvatarURL,
		RateLimit: githubcontext.RateLimitInfo{
			Limit:     query.RateLimit.Limit,
			Cost:      query.RateLimit.Cost,
			Remaining: query.RateLimit.Remaining,
			ResetAt:   query.RateLimit.ResetAt,
		},
	}

	// Transform labels
	issueContext.Labels = make([]githubcontext.GraphQLLabel, len(query.Repository.Issue.Labels.Nodes))
	for i, label := range query.Repository.Issue.Labels.Nodes {
		issueContext.Labels[i] = githubcontext.GraphQLLabel{
			Name:  label.Name,
			Color: label.Color,
		}
	}

	// Transform comments
	issueContext.Comments = make([]githubcontext.GraphQLComment, len(query.Repository.Issue.Comments.Nodes))
	for i, comment := range query.Repository.Issue.Comments.Nodes {
		issueContext.Comments[i] = githubcontext.GraphQLComment{
			ID:        comment.ID,
			Body:      comment.Body,
			Author:    comment.Author.Login,
			CreatedAt: comment.CreatedAt,
		}
	}

	log.Infof("GraphQL query for Issue #%d completed - Cost: %d, Remaining: %d", 
		number, issueContext.RateLimit.Cost, issueContext.RateLimit.Remaining)

	return issueContext, nil
}

// GetRepositoryInfo retrieves basic repository information
func (gc *GraphQLClient) GetRepositoryInfo(ctx context.Context, owner, repo string) (string, error) {
	variables := map[string]interface{}{
		"owner": owner,
		"name":  repo,
	}

	var query githubcontext.RepositoryInfoQuery
	err := gc.client.Query(ctx, &query, variables)
	if err != nil {
		return "", fmt.Errorf("failed to execute GraphQL query for repository %s/%s: %w", owner, repo, err)
	}

	defaultBranch := query.Repository.DefaultBranchRef.Name
	if defaultBranch == "" {
		return "main", nil // fallback to main if no default branch found
	}

	log.Infof("GraphQL query for repository %s/%s completed - Default branch: %s, Cost: %d, Remaining: %d", 
		owner, repo, defaultBranch, query.RateLimit.Cost, query.RateLimit.Remaining)

	return defaultBranch, nil
}

// GetRateLimit returns the current rate limit status
func (gc *GraphQLClient) GetRateLimit(ctx context.Context) (*githubcontext.RateLimitInfo, error) {
	var query struct {
		RateLimit struct {
			Limit     int
			Cost      int
			Remaining int
			ResetAt   githubv4.DateTime
		}
	}

	err := gc.client.Query(ctx, &query, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to query rate limit: %w", err)
	}

	return &githubcontext.RateLimitInfo{
		Limit:     query.RateLimit.Limit,
		Cost:      query.RateLimit.Cost,
		Remaining: query.RateLimit.Remaining,
		ResetAt:   query.RateLimit.ResetAt.Time,
	}, nil
}