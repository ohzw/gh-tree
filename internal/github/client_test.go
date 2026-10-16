package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"

	"github.com/ohzw/gh-tree/internal/model"
)

type gqlRequest struct {
	Query     string
	Variables map[string]any
}

// fakeGraphQL is an http.RoundTripper answering GraphQL requests with handler.
type fakeGraphQL struct {
	handler  func(req gqlRequest) (status int, body any)
	mu       sync.Mutex
	requests []gqlRequest
}

func (f *fakeGraphQL) RoundTrip(r *http.Request) (*http.Response, error) {
	var req gqlRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, err
	}
	f.mu.Lock()
	f.requests = append(f.requests, req)
	f.mu.Unlock()
	status, body := f.handler(req)
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(raw)),
		Request:    r,
	}, nil
}

func newTestClient(t *testing.T, handler func(req gqlRequest) (int, any)) (*Client, *fakeGraphQL) {
	t.Helper()
	fake := &fakeGraphQL{handler: handler}
	gql, err := api.NewGraphQLClient(api.ClientOptions{Host: "github.com", AuthToken: "token", Transport: fake})
	if err != nil {
		t.Fatal(err)
	}
	c, err := newClient("octo/repo", gql)
	if err != nil {
		t.Fatal(err)
	}
	return c, fake
}

type obj = map[string]any

func data(d obj) obj { return obj{"data": d} }

func labels(names ...string) obj {
	nodes := []obj{}
	for _, n := range names {
		nodes = append(nodes, obj{"name": n})
	}
	return obj{"nodes": nodes}
}

func page(hasNext bool, cursor string, nodes ...obj) obj {
	if nodes == nil {
		nodes = []obj{}
	}
	return obj{"pageInfo": obj{"hasNextPage": hasNext, "endCursor": cursor}, "nodes": nodes}
}

func issueNode(id string, number int, repo string, total int) obj {
	return obj{
		"id": id, "number": number, "title": fmt.Sprintf("Issue %d", number),
		"url":                            fmt.Sprintf("https://github.com/%s/issues/%d", repo, number),
		"state":                          "OPEN",
		"stateReason":                    nil,
		"repository":                     obj{"nameWithOwner": repo},
		"labels":                         labels(),
		"subIssuesSummary":               obj{"total": total, "completed": 0},
		"closedByPullRequestsReferences": page(false, ""),
	}
}

func with(o obj, key string, value any) obj {
	o[key] = value
	return o
}

func TestPullRequestsMapsFields(t *testing.T) {
	created := time.Date(2026, 9, 6, 21, 14, 49, 0, time.UTC)
	c, _ := newTestClient(t, func(gqlRequest) (int, any) {
		return 200, data(obj{"repository": obj{"pullRequests": page(false, "",
			obj{
				"id": "PR_fork", "number": 12, "title": "Fork fix", "url": "https://github.com/octo/repo/pull/12",
				"state": "OPEN", "isDraft": true, "baseRefName": "dev", "headRefName": "dev",
				"isCrossRepository": true, "headRepositoryOwner": obj{"login": "forker"},
				"createdAt": created.Format(time.RFC3339), "repository": obj{"nameWithOwner": "octo/repo"},
				"labels": labels("bug", "ui"),
				"closingIssuesReferences": obj{"nodes": []any{
					obj{"number": 3, "title": "Crash", "url": "https://github.com/other/lib/issues/3", "state": "CLOSED",
						"stateReason": "COMPLETED", "repository": obj{"nameWithOwner": "other/lib"}, "labels": labels("p1")},
					nil,
				}},
			},
			obj{
				"id": "PR_same", "number": 13, "title": "Feature", "url": "https://github.com/octo/repo/pull/13",
				"state": "MERGED", "isDraft": false, "baseRefName": "main", "headRefName": "feature",
				"isCrossRepository": false, "headRepositoryOwner": obj{"login": "octo"},
				"createdAt": created.Format(time.RFC3339), "repository": obj{"nameWithOwner": "octo/repo"},
				"labels": labels(), "closingIssuesReferences": obj{"nodes": []any{}},
			},
		)}})
	})

	prs, err := c.PullRequests(context.Background(), []string{"OPEN"})
	if err != nil {
		t.Fatal(err)
	}
	want := []model.PR{
		{
			Repo: "octo/repo", Number: 12, Title: "Fork fix", URL: "https://github.com/octo/repo/pull/12",
			State: "OPEN", IsDraft: true, Base: "dev", Head: "dev", IsCrossRepository: true, HeadOwner: "forker",
			CreatedAt: created, Labels: []string{"bug", "ui"},
			ClosingIssues: []model.IssueRef{{
				Repo: "other/lib", Number: 3, Title: "Crash", URL: "https://github.com/other/lib/issues/3",
				State: "CLOSED", StateReason: "COMPLETED", Labels: []string{"p1"},
			}},
		},
		{
			Repo: "octo/repo", Number: 13, Title: "Feature", URL: "https://github.com/octo/repo/pull/13",
			State: "MERGED", Base: "main", Head: "feature", CreatedAt: created,
		},
	}
	if !reflect.DeepEqual(prs, want) {
		t.Errorf("PullRequests mismatch\n got: %+v\nwant: %+v", prs, want)
	}
}

func TestPullRequestsFollowsPages(t *testing.T) {
	pr := func(n int) obj {
		return obj{"id": fmt.Sprint(n), "number": n, "state": "OPEN", "baseRefName": "main", "headRefName": fmt.Sprint("b", n),
			"createdAt": "2026-01-01T00:00:00Z", "repository": obj{"nameWithOwner": "octo/repo"},
			"labels": labels(), "closingIssuesReferences": obj{"nodes": []any{}}}
	}
	c, fake := newTestClient(t, func(req gqlRequest) (int, any) {
		if req.Variables["after"] == nil {
			return 200, data(obj{"repository": obj{"pullRequests": page(true, "cursor1", pr(3), pr(2))}})
		}
		return 200, data(obj{"repository": obj{"pullRequests": page(false, "", pr(1))}})
	})

	prs, err := c.PullRequests(context.Background(), []string{"OPEN", "MERGED"})
	if err != nil {
		t.Fatal(err)
	}
	var got []int
	for _, p := range prs {
		got = append(got, p.Number)
	}
	if !reflect.DeepEqual(got, []int{3, 2, 1}) {
		t.Errorf("numbers = %v, want [3 2 1]", got)
	}
	if len(fake.requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(fake.requests))
	}
	second := fake.requests[1].Variables
	if second["after"] != "cursor1" {
		t.Errorf("second page after = %v, want cursor1", second["after"])
	}
	if !reflect.DeepEqual(second["states"], []any{"OPEN", "MERGED"}) {
		t.Errorf("states = %v", second["states"])
	}
}

// issueTreeHandler answers the root query with root and follow-ups from
// followUps keyed by "id|after" (after empty for a first page).
func issueTreeHandler(t *testing.T, root obj, followUps map[string]obj, prLabels map[string][]string) func(gqlRequest) (int, any) {
	return func(req gqlRequest) (int, any) {
		switch {
		case strings.Contains(req.Query, "nodes(ids:"):
			var nodes []obj
			for _, id := range req.Variables["ids"].([]any) {
				nodes = append(nodes, obj{"id": id, "labels": labels(prLabels[id.(string)]...)})
			}
			return 200, data(obj{"nodes": nodes})
		case strings.Contains(req.Query, "node(id:"):
			after, _ := req.Variables["after"].(string)
			key := fmt.Sprintf("%v|%s", req.Variables["id"], after)
			node, ok := followUps[key]
			if !ok {
				t.Errorf("unexpected follow-up %s", key)
				return 200, data(obj{"node": nil})
			}
			return 200, data(obj{"node": node})
		default:
			return 200, data(obj{"repository": obj{"issue": root}})
		}
	}
}

func numbers(issues []*model.Issue) []string {
	var out []string
	for _, i := range issues {
		out = append(out, fmt.Sprintf("%s#%d", i.Repo, i.Number))
	}
	return out
}

func TestIssueTreeMergesFollowUpPages(t *testing.T) {
	linked := func(id string, number int) obj {
		return obj{"id": id, "number": number, "state": "MERGED", "baseRefName": "main", "headRefName": "fix",
			"isCrossRepository": false, "headRepositoryOwner": obj{"login": "octo"},
			"createdAt": "2026-01-01T00:00:00Z", "repository": obj{"nameWithOwner": "octo/repo"}}
	}
	root := with(issueNode("R", 1, "octo/repo", 3), "subIssues", page(true, "r1",
		with(with(issueNode("A", 2, "octo/repo", 4), "subIssues", page(true, "a1",
			issueNode("A1", 3, "octo/repo", 1), // its children were beyond the first query
		)), "closedByPullRequestsReferences", page(true, "p1", linked("PR_1", 50))),
	))
	followUps := map[string]obj{
		"R|r1": {"subIssues": page(false, "",
			with(issueNode("B", 9, "other/lib", 0), "subIssues", page(false, "")))},
		"A|a1": {"subIssues": page(false, "",
			with(issueNode("A2", 4, "octo/repo", 0), "subIssues", page(false, "")),
			with(issueNode("A3", 5, "octo/repo", 0), "subIssues", page(false, "")))},
		"A1|": {"subIssues": page(false, "",
			with(issueNode("A1a", 6, "other/lib", 0), "subIssues", page(false, "")))},
		"A|p1": {"closedByPullRequestsReferences": page(false, "", linked("PR_2", 51))},
	}
	c, fake := newTestClient(t, issueTreeHandler(t, root, followUps,
		map[string][]string{"PR_1": {"bug"}, "PR_2": {"docs"}}))

	tree, err := c.IssueTree(context.Background(), 1, -1)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := numbers(tree.SubIssues), []string{"octo/repo#2", "other/lib#9"}; !reflect.DeepEqual(got, want) {
		t.Errorf("root sub-issues = %v, want %v", got, want)
	}
	a := tree.SubIssues[0]
	if got, want := numbers(a.SubIssues), []string{"octo/repo#3", "octo/repo#4", "octo/repo#5"}; !reflect.DeepEqual(got, want) {
		t.Errorf("A sub-issues = %v, want %v", got, want)
	}
	if got, want := numbers(a.SubIssues[0].SubIssues), []string{"other/lib#6"}; !reflect.DeepEqual(got, want) {
		t.Errorf("A1 sub-issues = %v, want %v", got, want)
	}
	if len(a.LinkedPRs) != 2 || a.LinkedPRs[0].Number != 50 || a.LinkedPRs[1].Number != 51 {
		t.Fatalf("A linked PRs = %+v", a.LinkedPRs)
	}
	if !reflect.DeepEqual(a.LinkedPRs[0].Labels, []string{"bug"}) || !reflect.DeepEqual(a.LinkedPRs[1].Labels, []string{"docs"}) {
		t.Errorf("linked PR labels = %v, %v", a.LinkedPRs[0].Labels, a.LinkedPRs[1].Labels)
	}
	if a.LinkedPRs[0].HeadOwner != "" {
		t.Errorf("same-repo linked PR HeadOwner = %q, want empty", a.LinkedPRs[0].HeadOwner)
	}
	if tree.Progress == nil || *tree.Progress != (model.Progress{Total: 3}) {
		t.Errorf("root progress = %+v", tree.Progress)
	}
	if b := tree.SubIssues[1]; b.Progress != nil {
		t.Errorf("issue without sub-issues has progress %+v", b.Progress)
	}
	// Root query, three sub-issue follow-ups, one linked-PR follow-up, one label batch.
	if len(fake.requests) != 6 {
		t.Errorf("requests = %d, want 6", len(fake.requests))
	}
}

func TestIssueTreeWalksAncestorsBeyondOneQuery(t *testing.T) {
	// Parents 101 (nearest) .. 107 (outermost). The tree query returns five
	// levels and the id of the sixth; a follow-up fetches the rest.
	ancestor := func(n int) obj { return issueNode(fmt.Sprint("a", n), n, "octo/repo", 1) }
	var chain obj
	for n := 105; n >= 101; n-- {
		a := ancestor(n)
		if chain != nil {
			a["parent"] = chain
		} else {
			a["parent"] = obj{"id": "a106"}
		}
		chain = a
	}
	root := with(issueNode("r", 1, "octo/repo", 0), "parent", chain)
	followUps := map[string]obj{
		"a106|": with(ancestor(106), "parent", with(ancestor(107), "parent", nil)),
	}
	c, _ := newTestClient(t, issueTreeHandler(t, root, followUps, nil))
	got, err := c.IssueTree(context.Background(), 1, -1)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"octo/repo#107", "octo/repo#106", "octo/repo#105", "octo/repo#104", "octo/repo#103", "octo/repo#102", "octo/repo#101"}
	if g := numbers(got.Ancestors); !slices.Equal(g, want) {
		t.Errorf("ancestors = %v, want %v", g, want)
	}
	if p := got.Ancestors[0].Progress; p == nil || p.Total != 1 {
		t.Errorf("outermost ancestor progress = %+v", p)
	}
}

func TestIssueTreeStopsAtDepth(t *testing.T) {
	tests := []struct {
		depth          int
		rootLevels     int
		wantFollowUps  int
		followUpLevels int
		wantDeepest    int
	}{
		{depth: 0, rootLevels: 0, wantDeepest: 0},
		{depth: 1, rootLevels: 1, wantDeepest: 1},
		{depth: 2, rootLevels: 2, wantDeepest: 2},
		{depth: 3, rootLevels: 2, wantFollowUps: 1, followUpLevels: 1, wantDeepest: 3},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint("depth ", tt.depth), func(t *testing.T) {
			// Every issue reports sub-issues; the server returns as many levels as the query selects.
			var levelResponse func(id string, levels int) obj
			levelResponse = func(id string, levels int) obj {
				n := issueNode(id, len(id), "octo/repo", 1)
				if levels > 0 {
					n["subIssues"] = page(false, "", levelResponse(id+"x", levels-1))
				}
				return n
			}
			c, fake := newTestClient(t, func(req gqlRequest) (int, any) {
				levels := strings.Count(req.Query, "subIssues(")
				if id, ok := req.Variables["id"].(string); ok {
					if levels != tt.followUpLevels {
						t.Errorf("follow-up selects %d levels, want %d", levels, tt.followUpLevels)
					}
					return 200, data(obj{"node": obj{"subIssues": page(false, "", levelResponse(id+"x", levels-1))}})
				}
				if levels != tt.rootLevels {
					t.Errorf("root query selects %d levels, want %d", levels, tt.rootLevels)
				}
				return 200, data(obj{"repository": obj{"issue": levelResponse("R", levels)}})
			})

			tree, err := c.IssueTree(context.Background(), 1, tt.depth)
			if err != nil {
				t.Fatal(err)
			}
			// No linked PRs, so every request after the root query is a sub-issue follow-up.
			if got := len(fake.requests) - 1; got != tt.wantFollowUps {
				t.Errorf("follow-ups = %d, want %d", got, tt.wantFollowUps)
			}
			deepest := 0
			for n := tree; len(n.SubIssues) > 0; n = n.SubIssues[0] {
				deepest++
			}
			if deepest != tt.wantDeepest {
				t.Errorf("tree depth = %d, want %d", deepest, tt.wantDeepest)
			}
		})
	}
}

func TestErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   any
		want   string
	}{
		{"unauthorized", 401, obj{"message": "Bad credentials"}, "gh auth refresh -s repo"},
		{"insufficient scopes", 200, obj{"data": nil, "errors": []obj{{"type": "INSUFFICIENT_SCOPES",
			"message": "Your token has not been granted the required scopes"}}}, "gh auth refresh -s repo"},
		{"issue not found", 200, obj{"data": obj{"repository": obj{"issue": nil}}, "errors": []obj{{"type": "NOT_FOUND",
			"path": []string{"repository", "issue"}, "message": "Could not resolve to an Issue with the number of 7."}}},
			"issue #7 not found in octo/repo"},
		{"repository not found", 200, obj{"data": obj{"repository": nil}, "errors": []obj{{"type": "NOT_FOUND",
			"path": []string{"repository"}, "message": "Could not resolve to a Repository"}}},
			"repository octo/repo not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newTestClient(t, func(gqlRequest) (int, any) { return tt.status, tt.body })
			_, err := c.IssueTree(context.Background(), 7, -1)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestKind(t *testing.T) {
	tests := []struct {
		typename any
		want     model.Kind
		wantErr  bool
	}{
		{typename: "Issue", want: model.KindIssue},
		{typename: "PullRequest", want: model.KindPR},
		{typename: nil, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.typename), func(t *testing.T) {
			c, _ := newTestClient(t, func(gqlRequest) (int, any) {
				var node any
				if tt.typename != nil {
					node = obj{"__typename": tt.typename}
				}
				return 200, data(obj{"repository": obj{"issueOrPullRequest": node}})
			})
			got, err := c.Kind(context.Background(), 5)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Errorf("Kind = %q, %v; want %q, error %v", got, err, tt.want, tt.wantErr)
			}
		})
	}
}

func TestDefaultBranch(t *testing.T) {
	tests := []struct {
		name string
		ref  any
		want string
	}{
		{"set", obj{"name": "trunk"}, "trunk"},
		{"empty repository", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newTestClient(t, func(gqlRequest) (int, any) {
				return 200, data(obj{"repository": obj{"defaultBranchRef": tt.ref}})
			})
			got, err := c.DefaultBranch(context.Background())
			if err != nil || got != tt.want {
				t.Errorf("DefaultBranch = %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}
