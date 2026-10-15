// Package github fetches issues and pull requests through the GitHub GraphQL API.
package github

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/cli/go-gh/v2/pkg/api"

	"github.com/ohzw/gh-tree/internal/model"
)

// maxConcurrent bounds parallel follow-up queries to stay clear of
// GitHub's secondary rate limits.
const maxConcurrent = 4

// Client fetches data for one repository.
type Client struct {
	gql   *api.GraphQLClient
	owner string
	name  string
}

// NewClient returns a client for repo ("owner/name") authenticated with gh.
func NewClient(repo string) (*Client, error) {
	gql, err := api.DefaultGraphQLClient()
	if err != nil {
		return nil, err
	}
	return newClient(repo, gql)
}

func newClient(repo string, gql *api.GraphQLClient) (*Client, error) {
	owner, name, ok := strings.Cut(repo, "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") {
		return nil, fmt.Errorf("invalid repository %q: want owner/name", repo)
	}
	return &Client{gql: gql, owner: owner, name: name}, nil
}

func (c *Client) repo() string { return c.owner + "/" + c.name }

func (c *Client) repoVars(extra map[string]any) map[string]any {
	vars := map[string]any{"owner": c.owner, "name": c.name}
	for k, v := range extra {
		vars[k] = v
	}
	return vars
}

// do runs query and translates API errors. subject names what the query
// looks up (e.g. "issue #3") for not-found messages; empty for follow-ups.
func (c *Client) do(ctx context.Context, query string, vars map[string]any, resp any, subject string) error {
	if err := c.gql.DoWithContext(ctx, query, vars, resp); err != nil {
		return c.translateError(err, subject)
	}
	return nil
}

const authHint = "run `gh auth refresh -s repo` to grant gh-tree the repo scope"

func (c *Client) translateError(err error, subject string) error {
	var httpErr *api.HTTPError
	if errors.As(err, &httpErr) && httpErr.StatusCode == 401 {
		return fmt.Errorf("GitHub authentication failed (HTTP 401): %s", authHint)
	}
	var gqlErr *api.GraphQLError
	if !errors.As(err, &gqlErr) {
		return err
	}
	for _, item := range gqlErr.Errors {
		if item.Type == "INSUFFICIENT_SCOPES" {
			return fmt.Errorf("GitHub token is missing a required scope: %s", authHint)
		}
	}
	for _, item := range gqlErr.Errors {
		if item.Type != "NOT_FOUND" {
			continue
		}
		if len(item.Path) == 1 && item.Path[0] == "repository" {
			return fmt.Errorf("repository %s not found", c.repo())
		}
		if subject != "" {
			return fmt.Errorf("%s not found in %s", subject, c.repo())
		}
	}
	return err
}

// Kind reports whether number is an issue or a pull request.
func (c *Client) Kind(ctx context.Context, number int) (model.Kind, error) {
	const query = `query($owner:String!,$name:String!,$number:Int!){
  repository(owner:$owner,name:$name){issueOrPullRequest(number:$number){__typename}}
}`
	var resp struct {
		Repository *struct {
			IssueOrPullRequest *struct {
				Typename string `json:"__typename"`
			}
		}
	}
	subject := fmt.Sprintf("issue or pull request #%d", number)
	if err := c.do(ctx, query, c.repoVars(map[string]any{"number": number}), &resp, subject); err != nil {
		return "", err
	}
	if resp.Repository == nil {
		return "", fmt.Errorf("repository %s not found", c.repo())
	}
	if resp.Repository.IssueOrPullRequest == nil {
		return "", fmt.Errorf("%s not found in %s", subject, c.repo())
	}
	switch t := resp.Repository.IssueOrPullRequest.Typename; t {
	case "Issue":
		return model.KindIssue, nil
	case "PullRequest":
		return model.KindPR, nil
	default:
		return "", fmt.Errorf("#%d in %s is a %s, not an issue or pull request", number, c.repo(), t)
	}
}

// DefaultBranch returns the name of the repository's default branch, or ""
// when the repository has none (it is empty).
func (c *Client) DefaultBranch(ctx context.Context) (string, error) {
	const query = `query($owner:String!,$name:String!){
  repository(owner:$owner,name:$name){defaultBranchRef{name}}
}`
	var resp struct {
		Repository *struct {
			DefaultBranchRef *struct{ Name string }
		}
	}
	if err := c.do(ctx, query, c.repoVars(nil), &resp, ""); err != nil {
		return "", err
	}
	if resp.Repository == nil {
		return "", fmt.Errorf("repository %s not found", c.repo())
	}
	if resp.Repository.DefaultBranchRef == nil {
		return "", nil
	}
	return resp.Repository.DefaultBranchRef.Name, nil
}

// parallel runs fn(i) for i in [0,n) with bounded concurrency and returns
// the first error. Remaining calls see a cancelled context after a failure.
func parallel(ctx context.Context, n int, fn func(ctx context.Context, i int) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	errs := make([]error, n)
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			if ctx.Err() != nil {
				return
			}
			if errs[i] = fn(ctx, i); errs[i] != nil {
				cancel()
			}
		})
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
	}
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return ctx.Err()
}
