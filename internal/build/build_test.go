package build

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ohzw/gh-tree/internal/model"
)

const repo = "o/r"

var t0 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func pr(num int, head, base, state string, created int) model.PR {
	return model.PR{
		Repo: repo, Number: num, Title: fmt.Sprintf("PR %d", num), State: state,
		Head: head, Base: base, CreatedAt: t0.Add(time.Duration(created) * time.Hour),
	}
}

func fork(p model.PR, owner string) model.PR {
	p.IsCrossRepository = true
	p.HeadOwner = owner
	return p
}

func withIssues(p model.PR, nums ...int) model.PR {
	for _, n := range nums {
		p.ClosingIssues = append(p.ClosingIssues, model.IssueRef{Repo: repo, Number: n, State: "OPEN"})
	}
	return p
}

// outline renders nodes as one line per node, indented two spaces per level:
// "branch NAME", "pr #N", or "issue #N", plus "+hidden=N", "+omitted=N",
// "*focus", and "[labels]" when set.
func outline(roots ...*model.Node) string {
	var b strings.Builder
	var walk func(n *model.Node, depth int)
	walk = func(n *model.Node, depth int) {
		b.WriteString(strings.Repeat("  ", depth))
		switch n.Kind {
		case model.KindBranch:
			fmt.Fprintf(&b, "branch %s", n.Name)
		default:
			fmt.Fprintf(&b, "%s #%d", n.Kind, n.Number)
		}
		if n.HiddenClosed > 0 {
			fmt.Fprintf(&b, " +hidden=%d", n.HiddenClosed)
		}
		if n.OmittedChildren > 0 {
			fmt.Fprintf(&b, " +omitted=%d", n.OmittedChildren)
		}
		if n.Focus {
			b.WriteString(" *focus")
		}
		if len(n.Labels) > 0 {
			fmt.Fprintf(&b, " [%s]", strings.Join(n.Labels, ","))
		}
		b.WriteString("\n")
		for _, c := range n.Children {
			walk(c, depth+1)
		}
	}
	for _, r := range roots {
		walk(r, 0)
	}
	return b.String()
}

func lines(s ...string) string { return strings.Join(s, "\n") + "\n" }

func TestPRForest(t *testing.T) {
	all := Options{Depth: -1}
	tests := []struct {
		name string
		prs  []model.PR
		opt  Options
		want string
	}{
		{
			name: "open candidate preferred over newer merged one",
			prs: []model.PR{
				pr(1, "feat", "develop", "MERGED", 5),
				pr(2, "feat", "develop", "OPEN", 1),
				pr(3, "child", "feat", "OPEN", 6),
			},
			opt: all,
			want: lines(
				"branch develop",
				"  pr #1",
				"  pr #2",
				"    pr #3",
			),
		},
		{
			name: "newest candidate wins when none is open",
			prs: []model.PR{
				pr(1, "feat", "develop", "MERGED", 1),
				pr(2, "feat", "develop", "CLOSED", 3),
				pr(3, "feat", "develop", "MERGED", 2),
				pr(4, "child", "feat", "MERGED", 4),
			},
			opt: all,
			want: lines(
				"branch develop",
				"  pr #1",
				"  pr #2",
				"    pr #4",
				"  pr #3",
			),
		},
		{
			name: "fork head named like the base never becomes a parent",
			prs: []model.PR{
				fork(pr(181481, "dev", "dev", "OPEN", 1), "forestvogo"),
				pr(10, "fix-a", "dev", "OPEN", 2),
				fork(pr(11, "fix-b", "dev", "OPEN", 3), "someone"),
			},
			opt: all,
			want: lines(
				"branch dev",
				"  pr #10",
				"  pr #11",
				"  pr #181481",
			),
		},
		{
			name: "fork PR can be a child of a same-repo PR",
			prs: []model.PR{
				pr(1, "release", "main", "OPEN", 1),
				fork(pr(2, "release", "release", "OPEN", 2), "someone"),
			},
			opt: all,
			want: lines(
				"branch main",
				"  pr #1",
				"    pr #2",
			),
		},
		{
			name: "same head in another repo is not a candidate",
			prs: []model.PR{
				func() model.PR { p := pr(1, "feat", "main", "OPEN", 1); p.Repo = "other/r"; return p }(),
				pr(2, "child", "feat", "OPEN", 2),
			},
			opt: all,
			want: lines(
				"branch feat",
				"  pr #2",
				"branch main",
				"  pr #1",
			),
		},
		{
			name: "cycle is cut where it is found and that PR becomes a root",
			prs: []model.PR{
				pr(2, "b", "a", "OPEN", 2),
				pr(1, "a", "b", "OPEN", 1),
				pr(3, "c", "a", "OPEN", 3),
			},
			opt: all,
			want: lines(
				"pr #2",
				"  pr #1",
				"    pr #3",
			),
		},
		{
			name: "merged PR whose base branch was deleted hangs under a branch root",
			prs: []model.PR{
				pr(1, "gh-pr", "prototype", "MERGED", 1),
				pr(2, "x", "main", "MERGED", 2),
			},
			opt: all,
			want: lines(
				"branch main",
				"  pr #2",
				"branch prototype",
				"  pr #1",
			),
		},
		{
			name: "epic branch without a PR is an independent root",
			prs: []model.PR{
				pr(1260, "release/2.4", "develop", "OPEN", 1),
				pr(1257, "feature/1254", "epic/1244", "OPEN", 2),
				pr(1256, "feature/1255", "epic/1244", "OPEN", 3),
			},
			opt: all,
			want: lines(
				"branch develop",
				"  pr #1260",
				"branch epic/1244",
				"  pr #1256",
				"  pr #1257",
			),
		},
		{
			name: "closing issues come before child PRs and count as a level",
			prs: []model.PR{
				withIssues(pr(1, "epic", "main", "OPEN", 1), 100),
				withIssues(pr(2, "feat", "epic", "OPEN", 2), 101),
			},
			opt: Options{Depth: 2, WithIssues: true},
			want: lines(
				"branch main",
				"  pr #1",
				"    issue #100",
				"    pr #2",
			),
		},
		{
			name: "depth zero keeps only roots",
			prs:  []model.PR{pr(1, "a", "main", "OPEN", 1)},
			opt:  Options{Depth: 0},
			want: lines("branch main"),
		},
		{
			name: "labels filtered case-insensitively keeping original names",
			prs: []model.PR{
				func() model.PR {
					p := pr(1, "a", "main", "OPEN", 1)
					p.Labels = []string{"Bug", "wip", "UI"}
					return p
				}(),
			},
			opt: Options{Depth: -1, Labels: []string{"ui", "BUG"}},
			want: lines(
				"branch main",
				"  pr #1 [Bug,UI]",
			),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := outline(PRForest(tt.prs, tt.opt)...)
			if got != tt.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

func TestPRForestNodeFields(t *testing.T) {
	p := fork(pr(7, "dev", "main", "OPEN", 1), "someone")
	p.IsDraft = true
	p.Labels = []string{"x"}
	same := pr(8, "feat", "main", "OPEN", 2)
	same.HeadOwner = "o"
	roots := PRForest([]model.PR{p, same}, Options{Depth: -1})
	got := roots[0].Children
	if got[0].HeadOwner != "someone" || !got[0].IsDraft || got[0].Base != "main" || got[0].Head != "dev" {
		t.Errorf("fork PR node = %+v", got[0])
	}
	if got[0].Labels != nil {
		t.Errorf("labels without --labels = %v, want none", got[0].Labels)
	}
	if got[1].HeadOwner != "" {
		t.Errorf("same-repo PR HeadOwner = %q, want empty", got[1].HeadOwner)
	}
}

func TestPRFocus(t *testing.T) {
	prs := []model.PR{
		pr(1, "epic", "main", "OPEN", 1),
		pr(2, "feat-a", "epic", "OPEN", 2),
		pr(3, "feat-b", "epic", "OPEN", 3),
		pr(4, "sub", "feat-a", "OPEN", 4),
		pr(5, "other", "main", "OPEN", 5),
		withIssues(pr(6, "subsub", "sub", "OPEN", 6), 60),
		pr(7, "b", "a", "OPEN", 7),
		pr(8, "a", "b", "OPEN", 8),
	}
	tests := []struct {
		name   string
		number int
		opt    Options
		want   string
	}{
		{
			name:   "path to root without siblings plus full subtree",
			number: 2,
			opt:    Options{Depth: -1, WithIssues: true},
			want: lines(
				"branch main",
				"  pr #1",
				"    pr #2",
				"      pr #4",
				"        pr #6",
				"          issue #60",
			),
		},
		{
			name:   "depth counts from the root branch",
			number: 2,
			opt:    Options{Depth: 3},
			want: lines(
				"branch main",
				"  pr #1",
				"    pr #2",
				"      pr #4",
			),
		},
		{
			name:   "cycle-cut root has no branch above it",
			number: 7,
			opt:    Options{Depth: -1},
			want: lines(
				"pr #8",
				"  pr #7",
			),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n, err := PRFocus(prs, tt.number, tt.opt)
			if err != nil {
				t.Fatal(err)
			}
			if got := outline(n); got != tt.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
	if _, err := PRFocus(prs, 99, Options{Depth: -1}); err == nil {
		t.Error("PRFocus(99) succeeded, want not-found error")
	}
}

func TestBranchTree(t *testing.T) {
	prs := []model.PR{
		pr(1260, "release/2.4", "develop", "OPEN", 1),
		pr(1257, "feature/1254", "epic/1244", "OPEN", 2),
		pr(1256, "feature/1255", "epic/1244", "OPEN", 3),
		pr(1300, "stack-2", "feature/1254", "OPEN", 4),
		fork(pr(9, "fork-branch", "develop", "OPEN", 5), "someone"),
	}
	n, err := BranchTree(prs, "epic/1244", Options{Depth: -1})
	if err != nil {
		t.Fatal(err)
	}
	want := lines(
		"branch epic/1244",
		"  pr #1256",
		"  pr #1257",
		"    pr #1300",
	)
	if got := outline(n); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}

	errs := []struct {
		branch string
		want   string
	}{
		{"feature/1254", "gh tree pr 1257"},
		{"nope", "not found"},
		{"fork-branch", "not found"},
	}
	for _, e := range errs {
		_, err := BranchTree(prs, e.branch, Options{Depth: -1})
		if err == nil || !strings.Contains(err.Error(), e.want) {
			t.Errorf("BranchTree(%q) error = %v, want containing %q", e.branch, err, e.want)
		}
	}
}

func TestIssueTree(t *testing.T) {
	issue := func(num int, state string, subs ...*model.Issue) *model.Issue {
		return &model.Issue{IssueRef: model.IssueRef{Repo: repo, Number: num, State: state}, SubIssues: subs}
	}
	withPRs := func(is *model.Issue, nums ...int) *model.Issue {
		for _, n := range nums {
			is.LinkedPRs = append(is.LinkedPRs, pr(n, "h", "b", "OPEN", 0))
		}
		return is
	}
	root := func() *model.Issue {
		r := withPRs(issue(1244, "OPEN",
			withPRs(issue(1245, "CLOSED"), 1248),
			withPRs(issue(1254, "OPEN", issue(1300, "CLOSED"), issue(1301, "OPEN")), 1257),
			issue(1251, "CLOSED"),
		), 1290)
		r.Labels = []string{"Epic", "area/ui"}
		return r
	}

	tests := []struct {
		name string
		opt  Options
		want string
	}{
		{
			name: "linked PRs precede sub-issues in input order",
			opt:  Options{Depth: -1},
			want: lines(
				"issue #1244",
				"  pr #1290",
				"  issue #1245",
				"    pr #1248",
				"  issue #1254",
				"    pr #1257",
				"    issue #1300",
				"    issue #1301",
				"  issue #1251",
			),
		},
		{
			name: "depth counts PR rows as a level",
			opt:  Options{Depth: 1},
			want: lines(
				"issue #1244",
				"  pr #1290",
				"  issue #1245",
				"  issue #1254",
				"  issue #1251",
			),
		},
		{
			name: "depth zero is the root only",
			opt:  Options{Depth: 0},
			want: lines("issue #1244"),
		},
		{
			name: "hide closed folds closed sub-issues into the parent count",
			opt:  Options{Depth: -1, HideClosed: true},
			want: lines(
				"issue #1244 +hidden=2",
				"  pr #1290",
				"  issue #1254 +hidden=1",
				"    pr #1257",
				"    issue #1301",
			),
		},
		{
			name: "labels filtered case-insensitively",
			opt:  Options{Depth: 0, Labels: []string{"EPIC"}},
			want: lines("issue #1244 [Epic]"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := outline(IssueTree(root(), tt.opt)); got != tt.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

func TestIssueTreeProgressAndState(t *testing.T) {
	root := &model.Issue{
		IssueRef: model.IssueRef{Repo: repo, Number: 1, State: "OPEN", StateReason: "REOPENED"},
		Progress: &model.Progress{Completed: 1, Total: 4},
		SubIssues: []*model.Issue{
			{IssueRef: model.IssueRef{Repo: "x/y", Number: 2, State: "CLOSED", StateReason: "NOT_PLANNED"}},
		},
	}
	n := IssueTree(root, Options{Depth: -1})
	if n.Progress == nil || *n.Progress != (model.Progress{Completed: 1, Total: 4}) || n.StateReason != "REOPENED" {
		t.Errorf("root = %+v", n)
	}
	c := n.Children[0]
	if c.Progress != nil || c.Repo != "x/y" || c.StateReason != "NOT_PLANNED" || c.Children == nil {
		t.Errorf("child = %+v", c)
	}
}

func TestIssueTreeAncestors(t *testing.T) {
	ancestor := func(num, completed, total int) *model.Issue {
		return &model.Issue{IssueRef: model.IssueRef{Repo: repo, Number: num, State: "OPEN"}, Progress: &model.Progress{Completed: completed, Total: total}}
	}
	focus := func() *model.Issue {
		return &model.Issue{
			IssueRef:  model.IssueRef{Repo: repo, Number: 7, State: "OPEN"},
			LinkedPRs: []model.PR{pr(20, "h", "b", "OPEN", 0)},
			SubIssues: []*model.Issue{{IssueRef: model.IssueRef{Repo: repo, Number: 8, State: "OPEN"}}},
			Ancestors: []*model.Issue{ancestor(1, 2, 8), ancestor(3, 1, 1)},
		}
	}

	tests := []struct {
		name string
		opt  Options
		want string
	}{
		{
			name: "outermost ancestor is the root; off-path siblings are counted",
			opt:  Options{Depth: -1, Ancestors: true},
			want: lines(
				"issue #1 +omitted=7",
				"  issue #3",
				"    issue #7 *focus",
				"      pr #20",
				"      issue #8",
			),
		},
		{
			name: "depth counts from the requested issue, not the outermost ancestor",
			opt:  Options{Depth: 0, Ancestors: true},
			want: lines(
				"issue #1 +omitted=7",
				"  issue #3",
				"    issue #7 *focus",
			),
		},
		{
			name: "without Ancestors the requested issue is the root",
			opt:  Options{Depth: -1},
			want: lines(
				"issue #7",
				"  pr #20",
				"  issue #8",
			),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := outline(IssueTree(focus(), tt.opt)); got != tt.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}
