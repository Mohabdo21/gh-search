package githubsearch

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
)

const pageSize = 100

type Repository struct {
	Host     string
	Owner    string
	Name     string
	FullName string
	URL      string
}

type SearchOptions struct {
	Keywords string
	State    string
	Sort     string
	Limit    int
}

type IssueResult struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	State     string    `json:"state"`
	CreatedAt time.Time `json:"created_at"`
	URL       string    `json:"url"`
}

type DiscussionResult struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	State     string    `json:"state"`
	CreatedAt time.Time `json:"created_at"`
	URL       string    `json:"url"`
}

type Client struct {
	gql *api.GraphQLClient
}

func NewClient(host string) (*Client, error) {
	client, err := api.NewGraphQLClient(api.ClientOptions{
		Host:    host,
		Timeout: 30 * time.Second,
	})
	if err != nil {
		return nil, err
	}

	return &Client{gql: client}, nil
}

func ParseRepositoryURL(raw string) (Repository, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Repository{}, errors.New("repository URL is required")
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return Repository{}, fmt.Errorf("parse repository URL: %w", err)
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return Repository{}, fmt.Errorf("invalid repository URL %q: expected http or https", raw)
	}
	if parsed.Host == "" {
		return Repository{}, fmt.Errorf("invalid repository URL %q: missing host", raw)
	}

	trimmedPath := strings.Trim(strings.TrimSuffix(parsed.Path, "/"), "/")
	parts := strings.Split(trimmedPath, "/")
	if len(parts) != 2 {
		return Repository{}, fmt.Errorf("invalid repository URL %q: expected /owner/repo path", raw)
	}

	owner := strings.TrimSpace(parts[0])
	repo := strings.TrimSuffix(strings.TrimSpace(parts[1]), ".git")
	if owner == "" || repo == "" {
		return Repository{}, fmt.Errorf("invalid repository URL %q: expected non-empty owner and repo", raw)
	}

	parsed.Path = path.Join(owner, repo)
	parsed.RawPath = ""
	parsed.Fragment = ""
	parsed.RawQuery = ""

	return Repository{
		Host:     parsed.Host,
		Owner:    owner,
		Name:     repo,
		FullName: owner + "/" + repo,
		URL:      parsed.String(),
	}, nil
}

func (c *Client) SearchIssues(ctx context.Context, repo Repository, opts SearchOptions) ([]IssueResult, error) {
	type issueNode struct {
		Number    int       `json:"number"`
		Title     string    `json:"title"`
		State     string    `json:"state"`
		CreatedAt time.Time `json:"createdAt"`
		URL       string    `json:"url"`
	}

	type response struct {
		Search struct {
			PageInfo struct {
				HasNextPage bool   `json:"hasNextPage"`
				EndCursor   string `json:"endCursor"`
			} `json:"pageInfo"`
			Nodes []issueNode `json:"nodes"`
		} `json:"search"`
	}

	query := buildIssueQuery(repo.FullName, opts)
	results := make([]IssueResult, 0, pageSize)
	var cursor *string

	for {
		first := nextPageSize(opts.Limit, len(results))
		if first == 0 {
			break
		}

		vars := map[string]any{
			"query": query,
			"first": first,
			"after": cursor,
		}

		var resp response
		if err := c.gql.DoWithContext(ctx, issueSearchQuery, vars, &resp); err != nil {
			return nil, wrapAPIError(err)
		}

		for _, node := range resp.Search.Nodes {
			results = append(results, IssueResult{
				Number:    node.Number,
				Title:     node.Title,
				State:     normalizeIssueState(node.State),
				CreatedAt: node.CreatedAt,
				URL:       node.URL,
			})
		}
		if opts.Limit > 0 && len(results) >= opts.Limit {
			break
		}

		if !resp.Search.PageInfo.HasNextPage || resp.Search.PageInfo.EndCursor == "" {
			break
		}

		next := resp.Search.PageInfo.EndCursor
		cursor = &next
	}

	return results, nil
}

func (c *Client) SearchDiscussions(ctx context.Context, repo Repository, opts SearchOptions) ([]DiscussionResult, error) {
	type discussionNode struct {
		Number     int       `json:"number"`
		Title      string    `json:"title"`
		Closed     bool      `json:"closed"`
		IsAnswered bool      `json:"isAnswered"`
		CreatedAt  time.Time `json:"createdAt"`
		URL        string    `json:"url"`
	}

	type response struct {
		Search struct {
			PageInfo struct {
				HasNextPage bool   `json:"hasNextPage"`
				EndCursor   string `json:"endCursor"`
			} `json:"pageInfo"`
			Nodes []discussionNode `json:"nodes"`
		} `json:"search"`
	}

	query := buildDiscussionQuery(repo.FullName, opts)
	results := make([]DiscussionResult, 0, pageSize)
	var cursor *string

	for {
		first := nextPageSize(opts.Limit, len(results))
		if first == 0 {
			break
		}

		vars := map[string]any{
			"query": query,
			"first": first,
			"after": cursor,
		}

		var resp response
		if err := c.gql.DoWithContext(ctx, discussionSearchQuery, vars, &resp); err != nil {
			return nil, wrapAPIError(err)
		}

		for _, node := range resp.Search.Nodes {
			results = append(results, DiscussionResult{
				Number:    node.Number,
				Title:     node.Title,
				State:     normalizeDiscussionState(node.Closed, node.IsAnswered),
				CreatedAt: node.CreatedAt,
				URL:       node.URL,
			})
		}
		if opts.Limit > 0 && len(results) >= opts.Limit {
			break
		}

		if !resp.Search.PageInfo.HasNextPage || resp.Search.PageInfo.EndCursor == "" {
			break
		}

		next := resp.Search.PageInfo.EndCursor
		cursor = &next
	}

	return results, nil
}

func buildIssueQuery(repo string, opts SearchOptions) string {
	parts := []string{"repo:" + repo, "is:issue"}
	if qualifier := issueStateQualifier(opts.State); qualifier != "" {
		parts = append(parts, qualifier)
	}
	if qualifier := sortQualifier(opts.Sort); qualifier != "" {
		parts = append(parts, qualifier)
	}
	parts = append(parts, opts.Keywords)
	return strings.Join(parts, " ")
}

func buildDiscussionQuery(repo string, opts SearchOptions) string {
	parts := []string{"repo:" + repo, "is:discussion"}
	if qualifier := discussionStateQualifier(opts.State); qualifier != "" {
		parts = append(parts, qualifier)
	}
	if qualifier := sortQualifier(opts.Sort); qualifier != "" {
		parts = append(parts, qualifier)
	}
	parts = append(parts, opts.Keywords)
	return strings.Join(parts, " ")
}

func nextPageSize(limit, current int) int {
	if limit <= 0 {
		return pageSize
	}
	remaining := limit - current
	if remaining <= 0 {
		return 0
	}
	if remaining < pageSize {
		return remaining
	}
	return pageSize
}

func sortQualifier(sort string) string {
	if strings.TrimSpace(sort) == "" {
		return ""
	}
	return "sort:" + sort
}

func issueStateQualifier(state string) string {
	switch state {
	case "open":
		return "state:open"
	case "closed":
		return "state:closed"
	default:
		return ""
	}
}

func discussionStateQualifier(state string) string {
	switch state {
	case "open":
		return "is:open"
	case "closed":
		return "is:closed"
	default:
		return ""
	}
}

func normalizeIssueState(state string) string {
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case "OPEN":
		return "Open"
	case "CLOSED":
		return "Closed"
	default:
		return strings.TrimSpace(state)
	}
}

func normalizeDiscussionState(closed, answered bool) string {
	if answered {
		return "Answered"
	}
	if closed {
		return "Closed"
	}
	return "Open"
}

func wrapAPIError(err error) error {
	var httpErr *api.HTTPError
	if errors.As(err, &httpErr) {
		if isRateLimit(httpErr.StatusCode, httpErr.Message, httpErr.Headers) {
			return fmt.Errorf("GitHub API rate limit exceeded%s", rateLimitResetSuffix(httpErr.Headers))
		}

		switch httpErr.StatusCode {
		case 401:
			return errors.New("GitHub authentication failed: run 'gh auth login' or set GH_TOKEN")
		case 403:
			return fmt.Errorf("GitHub API request was forbidden: %s", httpErr.Message)
		case 404:
			return errors.New("repository not found or not accessible with the current GitHub credentials")
		case 422:
			return fmt.Errorf("GitHub rejected the search query: %s", httpErr.Message)
		default:
			return fmt.Errorf("GitHub API error (%d): %s", httpErr.StatusCode, httpErr.Message)
		}
	}

	var gqlErr *api.GraphQLError
	if errors.As(err, &gqlErr) {
		if strings.Contains(strings.ToLower(gqlErr.Error()), "rate limit") {
			return errors.New("GitHub GraphQL rate limit exceeded")
		}
		return fmt.Errorf("GitHub GraphQL error: %w", gqlErr)
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return fmt.Errorf("network timeout while contacting GitHub: %w", err)
		}
		return fmt.Errorf("network error while contacting GitHub: %w", err)
	}

	return fmt.Errorf("GitHub request failed: %w", err)
}

func isRateLimit(statusCode int, message string, headers map[string][]string) bool {
	if statusCode == 429 {
		return true
	}
	if statusCode == 403 {
		remaining := firstHeader(headers, "X-Ratelimit-Remaining")
		if remaining == "0" {
			return true
		}
		if strings.Contains(strings.ToLower(message), "rate limit") {
			return true
		}
	}
	return false
}

func rateLimitResetSuffix(headers map[string][]string) string {
	reset := firstHeader(headers, "X-Ratelimit-Reset")
	if reset == "" {
		return ""
	}

	unixSeconds, err := strconv.ParseInt(reset, 10, 64)
	if err != nil {
		return ""
	}

	return " until " + time.Unix(unixSeconds, 0).UTC().Format(time.RFC3339)
}

func firstHeader(headers map[string][]string, key string) string {
	for headerKey, values := range headers {
		if strings.EqualFold(headerKey, key) && len(values) > 0 {
			return values[0]
		}
	}
	return ""
}

const issueSearchQuery = `
query($query: String!, $first: Int!, $after: String) {
	search(query: $query, type: ISSUE, first: $first, after: $after) {
		pageInfo {
			hasNextPage
			endCursor
		}
		nodes {
			... on Issue {
				number
				title
				state
				createdAt
				url
			}
		}
	}
}
`

const discussionSearchQuery = `
query($query: String!, $first: Int!, $after: String) {
	search(query: $query, type: DISCUSSION, first: $first, after: $after) {
		pageInfo {
			hasNextPage
			endCursor
		}
		nodes {
			... on Discussion {
				number
				title
				closed
				isAnswered
				createdAt
				url
			}
		}
	}
}
`
