package github

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/ohzw/gh-tree/internal/model"
)

// Page sizes for the issue tree (docs/prd.md 9章). The first query fetches
// children and grandchildren together; the product of the two bounds the
// query cost, so grandchildren use a smaller page. Linked PRs are fetched 10
// per issue in the tree query and paged with follow-ups beyond that.
const (
	childPageSize      = 100
	grandchildPageSize = 20
	// maxQueryLevels caps how many sub-issue levels one query fetches.
	maxQueryLevels = 2
	// labelBatchSize is the nodes(ids:) limit.
	labelBatchSize = 100
)

// Linked PRs in the tree query carry no labels: labels on every linked PR of
// 2,000 possible grandchildren multiplies the query cost about sixfold, so
// they are fetched afterwards in nodes(ids:) batches (cost 1 per 100 PRs).
// ClosingIssues of linked PRs are not fetched; they point back into the tree.
const (
	treePRFragment    = `fragment TreePR on PullRequest{` + prCoreSelection + `}`
	treeIssueFragment = `fragment TreeIssue on Issue{id ` + issueRefSelection +
		` subIssuesSummary{total completed}` +
		` closedByPullRequestsReferences(first:10,userLinkedOnly:false,includeClosedPrs:true){pageInfo{hasNextPage endCursor} nodes{...TreePR}}}`
	treeFragments = treeIssueFragment + "\n" + treePRFragment

	// ancestorLevels is how many parents one query walks up. The level after
	// the last selects only the id, to tell whether more remain.
	ancestorLevels   = 5
	ancestorFragment = `fragment AncestorIssue on Issue{id ` + issueRefSelection + ` subIssuesSummary{total completed}}`
)

// ancestorSelection selects ancestorLevels levels of parent issues.
func ancestorSelection() string {
	return strings.Repeat("parent{...AncestorIssue ", ancestorLevels) + "parent{id}" + strings.Repeat("}", ancestorLevels)
}

type rawAncestor struct {
	ID string
	rawIssueRef
	SubIssuesSummary struct {
		Total     int
		Completed int
	}
	Parent *rawAncestor
}

type rawPRPage struct {
	PageInfo rawPageInfo
	Nodes    []*rawPR
}

type rawIssuePage struct {
	PageInfo rawPageInfo
	Nodes    []*rawIssue
}

type rawIssue struct {
	ID string
	rawIssueRef
	SubIssuesSummary struct {
		Total     int
		Completed int
	}
	ClosedByPullRequestsReferences rawPRPage
	// SubIssues is nil when the query did not select sub-issues.
	SubIssues *rawIssuePage
	// Parent is set only on the requested issue.
	Parent *rawAncestor
}

// subIssuesSelection selects levels of sub-issues. The first level pages
// with $after when withAfter is set.
func subIssuesSelection(levels int, withAfter bool) string {
	var b strings.Builder
	for level := range levels {
		first, after := grandchildPageSize, ""
		if level == 0 {
			first = childPageSize
			if withAfter {
				after = ",after:$after"
			}
		}
		fmt.Fprintf(&b, "subIssues(first:%d%s){pageInfo{hasNextPage endCursor} nodes{...TreeIssue ", first, after)
	}
	b.WriteString(strings.Repeat("}}", levels))
	return b.String()
}

// queryLevels is how many sub-issue levels to select when remaining levels
// are still wanted (remaining < 0 is unlimited).
func queryLevels(remaining int) int {
	if remaining < 0 || remaining > maxQueryLevels {
		return maxQueryLevels
	}
	return remaining
}

// below is the remaining depth one level further down.
func below(remaining int) int {
	if remaining < 0 {
		return remaining
	}
	return remaining - 1
}

// followUp is a query still needed to complete one issue: the next page of
// its sub-issues (or their first page when the parent query stopped above
// them), or the next page of its linked PRs.
type followUp struct {
	issue     *model.Issue
	id        string
	linkedPRs bool
	after     string
	// remaining is the number of sub-issue levels still wanted below issue.
	remaining int
}

type treeFetcher struct {
	c *Client
	// prIDs holds the node ids of issue.LinkedPRs, index for index.
	prIDs map[*model.Issue][]string
}

// IssueTree fetches the issue number and its sub-issues down to depth
// levels (depth < 0 is unlimited), with every page resolved. Sub-issues in
// other repositories are followed by node id.
func (c *Client) IssueTree(ctx context.Context, number int, depth int) (*model.Issue, error) {
	query := `query($owner:String!,$name:String!,$number:Int!){
  repository(owner:$owner,name:$name){issue(number:$number){...TreeIssue ` + subIssuesSelection(queryLevels(depth), false) + ` ` + ancestorSelection() + `}}
}
` + treeFragments + "\n" + ancestorFragment
	var resp struct {
		Repository *struct {
			Issue *rawIssue
		}
	}
	subject := fmt.Sprintf("issue #%d", number)
	if err := c.do(ctx, query, c.repoVars(map[string]any{"number": number}), &resp, subject); err != nil {
		return nil, err
	}
	if resp.Repository == nil {
		return nil, fmt.Errorf("repository %s not found", c.repo())
	}
	if resp.Repository.Issue == nil {
		return nil, fmt.Errorf("%s not found in %s", subject, c.repo())
	}

	f := &treeFetcher{c: c, prIDs: map[*model.Issue][]string{}}
	var todo []followUp
	root := f.mapIssue(resp.Repository.Issue, depth, &todo)
	for len(todo) > 0 {
		results := make([]*rawIssue, len(todo))
		err := parallel(ctx, len(todo), func(ctx context.Context, i int) error {
			var err error
			results[i], err = f.fetch(ctx, todo[i])
			return err
		})
		if err != nil {
			return nil, err
		}
		var next []followUp
		for i, fu := range todo {
			if fu.linkedPRs {
				f.addLinkedPRs(fu.issue, fu.id, results[i].ClosedByPullRequestsReferences, &next)
			} else {
				f.addSubIssues(fu.issue, fu.id, results[i].SubIssues, fu.remaining, &next)
			}
		}
		todo = next
	}
	if err := f.fillPRLabels(ctx); err != nil {
		return nil, err
	}
	ancestors, err := c.ancestors(ctx, resp.Repository.Issue.Parent)
	if err != nil {
		return nil, err
	}
	root.Ancestors = ancestors
	return root, nil
}

// ancestors walks the parent chain starting at p (nearest parent first) and
// returns it outermost first, fetching beyond each ancestorLevels window by
// node id.
func (c *Client) ancestors(ctx context.Context, p *rawAncestor) ([]*model.Issue, error) {
	var chain []*model.Issue
	for p != nil {
		for level := 0; p != nil && level < ancestorLevels; level++ {
			a := &model.Issue{IssueRef: p.rawIssueRef.toModel()}
			if s := p.SubIssuesSummary; s.Total > 0 {
				a.Progress = &model.Progress{Completed: s.Completed, Total: s.Total}
			}
			chain = append(chain, a)
			p = p.Parent
		}
		if p == nil {
			break
		}
		// p holds only the id of the next ancestor beyond this window.
		query := `query($id:ID!){node(id:$id){...AncestorIssue ` + ancestorSelection() + `}}
` + ancestorFragment
		var resp struct{ Node *rawAncestor }
		if err := c.do(ctx, query, map[string]any{"id": p.ID}, &resp, ""); err != nil {
			return nil, err
		}
		p = resp.Node
	}
	slices.Reverse(chain)
	return chain, nil
}

func (f *treeFetcher) mapIssue(r *rawIssue, remaining int, todo *[]followUp) *model.Issue {
	issue := &model.Issue{IssueRef: r.rawIssueRef.toModel()}
	if s := r.SubIssuesSummary; s.Total > 0 {
		issue.Progress = &model.Progress{Completed: s.Completed, Total: s.Total}
	}
	f.addLinkedPRs(issue, r.ID, r.ClosedByPullRequestsReferences, todo)
	switch {
	case r.SubIssues != nil:
		f.addSubIssues(issue, r.ID, r.SubIssues, remaining, todo)
	case remaining != 0 && r.SubIssuesSummary.Total > 0:
		*todo = append(*todo, followUp{issue: issue, id: r.ID, remaining: remaining})
	}
	return issue
}

func (f *treeFetcher) addSubIssues(issue *model.Issue, id string, page *rawIssuePage, remaining int, todo *[]followUp) {
	if page == nil {
		return
	}
	for _, child := range page.Nodes {
		if child != nil {
			issue.SubIssues = append(issue.SubIssues, f.mapIssue(child, below(remaining), todo))
		}
	}
	if page.PageInfo.HasNextPage {
		*todo = append(*todo, followUp{issue: issue, id: id, after: page.PageInfo.EndCursor, remaining: remaining})
	}
}

func (f *treeFetcher) addLinkedPRs(issue *model.Issue, id string, page rawPRPage, todo *[]followUp) {
	for _, pr := range page.Nodes {
		if pr != nil {
			issue.LinkedPRs = append(issue.LinkedPRs, pr.toModel())
			f.prIDs[issue] = append(f.prIDs[issue], pr.ID)
		}
	}
	if page.PageInfo.HasNextPage {
		*todo = append(*todo, followUp{issue: issue, id: id, linkedPRs: true, after: page.PageInfo.EndCursor})
	}
}

func (f *treeFetcher) fetch(ctx context.Context, fu followUp) (*rawIssue, error) {
	var query string
	if fu.linkedPRs {
		query = `query($id:ID!,$after:String){node(id:$id){...on Issue{
  closedByPullRequestsReferences(first:100,after:$after,userLinkedOnly:false,includeClosedPrs:true){pageInfo{hasNextPage endCursor} nodes{...TreePR}}
}}}
` + treePRFragment
	} else {
		query = `query($id:ID!,$after:String){node(id:$id){...on Issue{` +
			subIssuesSelection(queryLevels(fu.remaining), true) + `}}}
` + treeFragments
	}
	var after any
	if fu.after != "" {
		after = fu.after
	}
	var resp struct{ Node *rawIssue }
	if err := f.c.do(ctx, query, map[string]any{"id": fu.id, "after": after}, &resp, ""); err != nil {
		return nil, err
	}
	if resp.Node == nil {
		return nil, fmt.Errorf("issue %s#%d is no longer accessible", fu.issue.Repo, fu.issue.Number)
	}
	return resp.Node, nil
}

// fillPRLabels loads the labels of every linked PR in the tree.
func (f *treeFetcher) fillPRLabels(ctx context.Context) error {
	var ids []string
	for _, prIDs := range f.prIDs {
		ids = append(ids, prIDs...)
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)
	if len(ids) == 0 {
		return nil
	}

	const query = `query($ids:[ID!]!){nodes(ids:$ids){...on PullRequest{id ` + labelsSelection + `}}}`
	batches := slices.Collect(slices.Chunk(ids, labelBatchSize))
	labels := make(map[string][]string, len(ids))
	var mu sync.Mutex
	err := parallel(ctx, len(batches), func(ctx context.Context, i int) error {
		var resp struct {
			Nodes []*struct {
				ID     string
				Labels rawLabels
			}
		}
		if err := f.c.do(ctx, query, map[string]any{"ids": batches[i]}, &resp, ""); err != nil {
			return err
		}
		mu.Lock()
		defer mu.Unlock()
		for _, n := range resp.Nodes {
			if n != nil {
				labels[n.ID] = n.Labels.names()
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	for issue, prIDs := range f.prIDs {
		for i, id := range prIDs {
			issue.LinkedPRs[i].Labels = labels[id]
		}
	}
	return nil
}
