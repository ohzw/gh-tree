// Package build assembles display trees from fetched issues and pull requests.
//
// Every function here is pure: no API calls, no I/O. The tree rules of
// docs/prd.md 8章 live in this package.
package build

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ohzw/gh-tree/internal/model"
)

type Options struct {
	Depth      int      // < 0 は無制限。根を 0 として数える。紐づいた PR / Issue の行も1段
	HideClosed bool     // closed の Sub-issue を親の HiddenClosed に畳む
	WithIssues bool     // PR の子として ClosingIssues を出す（--issues）
	Labels     []string // 節点に残すラベル。空なら Labels は空
	Ancestors  bool     // Issue tree の上に、指定した Issue の祖先の経路を出す。Depth は指定した Issue から数える
}

// within reports whether a node at level (root = 0) is shown.
func (o Options) within(level int) bool {
	return o.Depth < 0 || level <= o.Depth
}

// IssueTree builds the tree of root: each issue lists its linked PRs first,
// then its sub-issues in input order. With opt.Ancestors and a parent chain,
// the result is rooted at the outermost ancestor: each ancestor holds only
// the next issue on the path (the rest counted in OmittedChildren), and root
// is marked Focus.
func IssueTree(root *model.Issue, opt Options) *model.Node {
	if root == nil {
		return nil
	}
	lf := newLabelFilter(opt.Labels)
	n := issueTree(root, 0, opt, lf)
	if !opt.Ancestors || len(root.Ancestors) == 0 {
		return n
	}
	n.Focus = true
	for i := len(root.Ancestors) - 1; i >= 0; i-- {
		a := root.Ancestors[i]
		p := issueNode(a.IssueRef, lf)
		if a.Progress != nil {
			pr := *a.Progress
			p.Progress = &pr
			p.OmittedChildren = max(pr.Total-1, 0)
		}
		p.Children = []*model.Node{n}
		n = p
	}
	return n
}

func issueTree(is *model.Issue, level int, opt Options, lf labelFilter) *model.Node {
	n := issueNode(is.IssueRef, lf)
	if is.Progress != nil {
		p := *is.Progress
		n.Progress = &p
	}
	if !opt.within(level + 1) {
		return n
	}
	for _, pr := range is.LinkedPRs {
		n.Children = append(n.Children, prNode(pr, lf))
	}
	for _, sub := range is.SubIssues {
		if sub == nil {
			continue
		}
		if opt.HideClosed && strings.EqualFold(sub.State, "CLOSED") {
			n.HiddenClosed++
			continue
		}
		n.Children = append(n.Children, issueTree(sub, level+1, opt, lf))
	}
	return n
}

// PRForest returns every root of the PR tree: branch nodes sorted by name,
// then PRs cut out of a cycle sorted by number.
func PRForest(prs []model.PR, opt Options) []*model.Node {
	g := newGraph(prs)
	lf := newLabelFilter(opt.Labels)
	roots := make([]*model.Node, 0, len(g.branches)+len(g.prRoots))
	for _, name := range g.branchNames() {
		roots = append(roots, g.branchTree(name, opt, lf))
	}
	for _, i := range g.prRoots {
		roots = append(roots, g.prTree(i, 0, opt, lf))
	}
	return roots
}

// PRFocus は、根の branch から number の PR までの経路と、その PR より下の木を返す。
// Ancestors carry only the next node on the path: no sibling PRs and no issue rows.
func PRFocus(prs []model.PR, number int, opt Options) (*model.Node, error) {
	g := newGraph(prs)
	target := -1
	for i, pr := range g.prs {
		if pr.Number == number {
			target = i
			break
		}
	}
	if target < 0 {
		return nil, fmt.Errorf("PR #%d not found among the fetched pull requests", number)
	}
	lf := newLabelFilter(opt.Labels)

	// path holds PR indices from the topmost PR down to target.
	path := []int{target}
	for p := g.parent[target]; p >= 0; p = g.parent[p] {
		path = append([]int{p}, path...)
	}
	top := path[0]
	level := 0
	var root, tail *model.Node
	if !g.cut[top] {
		root = &model.Node{Kind: model.KindBranch, Name: g.prs[top].Base, Children: []*model.Node{}}
		tail = root
		level = 1
	}
	for k, i := range path {
		if !opt.within(level) {
			break
		}
		var n *model.Node
		if k == len(path)-1 {
			n = g.prTree(i, level, opt, lf)
		} else {
			n = prNode(g.prs[i], lf)
		}
		if tail == nil {
			root = n
		} else {
			tail.Children = append(tail.Children, n)
		}
		tail = n
		level++
	}
	return root, nil
}

// BranchTree は branch を根にした PR tree を返す。
func BranchTree(prs []model.PR, branch string, opt Options) (*model.Node, error) {
	g := newGraph(prs)
	if _, ok := g.branches[branch]; ok {
		return g.branchTree(branch, opt, newLabelFilter(opt.Labels)), nil
	}
	if i := g.headPR(branch); i >= 0 {
		n := g.prs[i].Number
		return nil, fmt.Errorf("branch %q is the head of PR #%d; use `gh tree pr %d`", branch, n, n)
	}
	return nil, fmt.Errorf("branch %q not found: no pull request targets it", branch)
}

// graph links PRs by base and head (docs/prd.md 8章).
type graph struct {
	prs []model.PR
	// parent is the index of the parent PR, or -1 when the PR hangs under its
	// base branch node or is a cycle-cut root.
	parent []int
	// children are PR indices under each PR, sorted by number.
	children [][]int
	// branches are PR indices under each branch node, sorted by number.
	branches map[string][]int
	// prRoots are PRs cut out of a cycle, sorted by number.
	prRoots []int
	cut     []bool
	// byHead indexes non-fork PRs by repo and head branch.
	byHead map[string][]int
}

func headKey(repo, head string) string { return repo + "\x00" + head }

func newGraph(prs []model.PR) *graph {
	g := &graph{
		prs:      prs,
		parent:   make([]int, len(prs)),
		children: make([][]int, len(prs)),
		branches: map[string][]int{},
		cut:      make([]bool, len(prs)),
		byHead:   map[string][]int{},
	}
	for i, pr := range prs {
		if !pr.IsCrossRepository {
			k := headKey(pr.Repo, pr.Head)
			g.byHead[k] = append(g.byHead[k], i)
		}
	}
	for i, pr := range prs {
		g.parent[i] = g.bestCandidate(g.byHead[headKey(pr.Repo, pr.Base)], i)
	}
	g.cutCycles()
	for i := range prs {
		switch {
		case g.cut[i]:
			g.prRoots = append(g.prRoots, i)
		case g.parent[i] >= 0:
			g.children[g.parent[i]] = append(g.children[g.parent[i]], i)
		default:
			g.branches[prs[i].Base] = append(g.branches[prs[i].Base], i)
		}
	}
	g.sortByNumber(g.prRoots)
	for _, c := range g.children {
		g.sortByNumber(c)
	}
	for _, c := range g.branches {
		g.sortByNumber(c)
	}
	return g
}

// bestCandidate picks the parent among candidates, skipping self: an open PR
// first, then the newest; ties fall to the higher number for determinism.
func (g *graph) bestCandidate(cands []int, self int) int {
	best := -1
	for _, c := range cands {
		if c == self {
			continue
		}
		if best < 0 || g.better(c, best) {
			best = c
		}
	}
	return best
}

func (g *graph) better(a, b int) bool {
	pa, pb := g.prs[a], g.prs[b]
	oa, ob := strings.EqualFold(pa.State, "OPEN"), strings.EqualFold(pb.State, "OPEN")
	if oa != ob {
		return oa
	}
	if !pa.CreatedAt.Equal(pb.CreatedAt) {
		return pa.CreatedAt.After(pb.CreatedAt)
	}
	return pa.Number > pb.Number
}

// cutCycles walks parent links from each PR in ascending number order. When a
// walk reaches a PR already on its own path, the link that closed the loop is
// cut and the PR owning it becomes a root.
func (g *graph) cutCycles() {
	const (
		unseen = iota
		onPath
		done
	)
	state := make([]int, len(g.prs))
	order := make([]int, len(g.prs))
	for i := range order {
		order[i] = i
	}
	g.sortByNumber(order)
	for _, start := range order {
		var path []int
		for i := start; i >= 0 && state[i] == unseen; {
			state[i] = onPath
			path = append(path, i)
			p := g.parent[i]
			if p >= 0 && state[p] == onPath {
				g.parent[i] = -1
				g.cut[i] = true
				break
			}
			i = p
		}
		for _, i := range path {
			state[i] = done
		}
	}
}

func (g *graph) sortByNumber(idx []int) {
	sort.SliceStable(idx, func(a, b int) bool { return g.prs[idx[a]].Number < g.prs[idx[b]].Number })
}

func (g *graph) branchNames() []string {
	names := make([]string, 0, len(g.branches))
	for name := range g.branches {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// headPR returns the PR that would be the parent of a PR based on branch, or -1.
func (g *graph) headPR(branch string) int {
	best := -1
	for _, cands := range g.byHead {
		for _, c := range cands {
			if g.prs[c].Head != branch {
				continue
			}
			if best < 0 || g.better(c, best) {
				best = c
			}
		}
	}
	return best
}

func (g *graph) branchTree(name string, opt Options, lf labelFilter) *model.Node {
	n := &model.Node{Kind: model.KindBranch, Name: name, Children: []*model.Node{}}
	if !opt.within(1) {
		return n
	}
	for _, i := range g.branches[name] {
		n.Children = append(n.Children, g.prTree(i, 1, opt, lf))
	}
	return n
}

// prTree builds the PR at index i and everything below it. Closing issues
// (--issues) come before child PRs, as linked rows do in the issue tree.
func (g *graph) prTree(i, level int, opt Options, lf labelFilter) *model.Node {
	n := prNode(g.prs[i], lf)
	if !opt.within(level + 1) {
		return n
	}
	if opt.WithIssues {
		for _, ref := range g.prs[i].ClosingIssues {
			n.Children = append(n.Children, issueNode(ref, lf))
		}
	}
	for _, c := range g.children[i] {
		n.Children = append(n.Children, g.prTree(c, level+1, opt, lf))
	}
	return n
}

func prNode(pr model.PR, lf labelFilter) *model.Node {
	n := &model.Node{
		Kind:     model.KindPR,
		Repo:     pr.Repo,
		Number:   pr.Number,
		Title:    pr.Title,
		URL:      pr.URL,
		State:    pr.State,
		IsDraft:  pr.IsDraft,
		Base:     pr.Base,
		Head:     pr.Head,
		Labels:   lf.apply(pr.Labels),
		Children: []*model.Node{},
	}
	if pr.IsCrossRepository {
		n.HeadOwner = pr.HeadOwner
	}
	return n
}

func issueNode(ref model.IssueRef, lf labelFilter) *model.Node {
	return &model.Node{
		Kind:        model.KindIssue,
		Repo:        ref.Repo,
		Number:      ref.Number,
		Title:       ref.Title,
		URL:         ref.URL,
		State:       ref.State,
		StateReason: ref.StateReason,
		Labels:      lf.apply(ref.Labels),
		Children:    []*model.Node{},
	}
}

// labelFilter keeps the labels selected with --labels, matched case-insensitively.
type labelFilter map[string]bool

func newLabelFilter(labels []string) labelFilter {
	lf := labelFilter{}
	for _, l := range labels {
		lf[strings.ToLower(l)] = true
	}
	return lf
}

func (lf labelFilter) apply(labels []string) []string {
	if len(lf) == 0 {
		return nil
	}
	var out []string
	for _, l := range labels {
		if lf[strings.ToLower(l)] {
			out = append(out, l)
		}
	}
	return out
}
