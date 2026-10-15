package github

import (
	"context"
	"fmt"
	"time"

	"github.com/ohzw/gh-tree/internal/model"
)

const (
	labelsSelection   = `labels(first:20){nodes{name}}`
	issueRefSelection = `number title url state stateReason repository{nameWithOwner} ` + labelsSelection
	prCoreSelection   = `id number title url state isDraft baseRefName headRefName isCrossRepository headRepositoryOwner{login} createdAt repository{nameWithOwner}`

	prFullFragment = `fragment PRFull on PullRequest{` + prCoreSelection + ` ` + labelsSelection +
		` closingIssuesReferences(first:10){nodes{` + issueRefSelection + `}}}`
)

type rawLabels struct {
	Nodes []*struct{ Name string }
}

func (l rawLabels) names() []string {
	var out []string
	for _, n := range l.Nodes {
		if n != nil {
			out = append(out, n.Name)
		}
	}
	return out
}

type rawRepository struct{ NameWithOwner string }

type rawPageInfo struct {
	HasNextPage bool
	EndCursor   string
}

type rawIssueRef struct {
	Number      int
	Title       string
	URL         string
	State       string
	StateReason string
	Repository  rawRepository
	Labels      rawLabels
}

func (r rawIssueRef) toModel() model.IssueRef {
	return model.IssueRef{
		Repo:        r.Repository.NameWithOwner,
		Number:      r.Number,
		Title:       r.Title,
		URL:         r.URL,
		State:       r.State,
		StateReason: r.StateReason,
		Labels:      r.Labels.names(),
	}
}

type rawPR struct {
	ID                      string
	Number                  int
	Title                   string
	URL                     string
	State                   string
	IsDraft                 bool
	BaseRefName             string
	HeadRefName             string
	IsCrossRepository       bool
	HeadRepositoryOwner     *struct{ Login string }
	CreatedAt               time.Time
	Repository              rawRepository
	Labels                  rawLabels
	ClosingIssuesReferences struct {
		Nodes []*rawIssueRef
	}
}

func (r rawPR) toModel() model.PR {
	pr := model.PR{
		Repo:              r.Repository.NameWithOwner,
		Number:            r.Number,
		Title:             r.Title,
		URL:               r.URL,
		State:             r.State,
		IsDraft:           r.IsDraft,
		Base:              r.BaseRefName,
		Head:              r.HeadRefName,
		IsCrossRepository: r.IsCrossRepository,
		CreatedAt:         r.CreatedAt,
		Labels:            r.Labels.names(),
	}
	if r.IsCrossRepository && r.HeadRepositoryOwner != nil {
		pr.HeadOwner = r.HeadRepositoryOwner.Login
	}
	for _, issue := range r.ClosingIssuesReferences.Nodes {
		if issue != nil {
			pr.ClosingIssues = append(pr.ClosingIssues, issue.toModel())
		}
	}
	return pr
}

// PullRequests returns every pull request in one of states
// ("OPEN" | "MERGED" | "CLOSED"), newest first. Empty states means all.
func (c *Client) PullRequests(ctx context.Context, states []string) ([]model.PR, error) {
	const query = `query($owner:String!,$name:String!,$states:[PullRequestState!],$after:String){
  repository(owner:$owner,name:$name){
    pullRequests(states:$states,first:100,after:$after,orderBy:{field:CREATED_AT,direction:DESC}){
      pageInfo{hasNextPage endCursor}
      nodes{...PRFull}
    }
  }
}
` + prFullFragment
	var statesVar any
	if len(states) > 0 {
		statesVar = states
	}
	var prs []model.PR
	var after any
	for {
		var resp struct {
			Repository *struct {
				PullRequests struct {
					PageInfo rawPageInfo
					Nodes    []*rawPR
				}
			}
		}
		vars := c.repoVars(map[string]any{"states": statesVar, "after": after})
		if err := c.do(ctx, query, vars, &resp, ""); err != nil {
			return nil, err
		}
		if resp.Repository == nil {
			return nil, fmt.Errorf("repository %s not found", c.repo())
		}
		page := resp.Repository.PullRequests
		for _, pr := range page.Nodes {
			if pr != nil {
				prs = append(prs, pr.toModel())
			}
		}
		if !page.PageInfo.HasNextPage {
			return prs, nil
		}
		after = page.PageInfo.EndCursor
	}
}

// PullRequest returns one pull request.
func (c *Client) PullRequest(ctx context.Context, number int) (model.PR, error) {
	const query = `query($owner:String!,$name:String!,$number:Int!){
  repository(owner:$owner,name:$name){pullRequest(number:$number){...PRFull}}
}
` + prFullFragment
	var resp struct {
		Repository *struct {
			PullRequest *rawPR
		}
	}
	subject := fmt.Sprintf("pull request #%d", number)
	if err := c.do(ctx, query, c.repoVars(map[string]any{"number": number}), &resp, subject); err != nil {
		return model.PR{}, err
	}
	if resp.Repository == nil {
		return model.PR{}, fmt.Errorf("repository %s not found", c.repo())
	}
	if resp.Repository.PullRequest == nil {
		return model.PR{}, fmt.Errorf("%s not found in %s", subject, c.repo())
	}
	return resp.Repository.PullRequest.toModel(), nil
}
