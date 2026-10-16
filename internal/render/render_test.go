package render

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/ohzw/gh-tree/internal/model"
)

const repo = "acme/app"

func issue(n int, title, state, reason string, children ...*model.Node) *model.Node {
	return &model.Node{Kind: model.KindIssue, Repo: repo, Number: n, Title: title, State: state, StateReason: reason, Children: children}
}

func pr(n int, title, state string, draft bool, head, base string, children ...*model.Node) *model.Node {
	return &model.Node{Kind: model.KindPR, Repo: repo, Number: n, Title: title, State: state, IsDraft: draft, Head: head, Base: base, Children: children}
}

func renderText(t *testing.T, roots []*model.Node, opt Options) string {
	t.Helper()
	var b bytes.Buffer
	if err := Text(&b, roots, opt); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func assertGolden(t *testing.T, got, want string) {
	t.Helper()
	want = strings.TrimPrefix(want, "\n")
	if got != want {
		t.Errorf("output mismatch\n--- got ---\n%s--- want ---\n%s", got, want)
	}
}

func TestTextIssueTree(t *testing.T) {
	root := issue(1244, "[Epic] Telemetryを判断起点で作り直す\r\n", "OPEN", "",
		issue(1245, "Bricks の Progression Funnel", "CLOSED", "COMPLETED",
			pr(1248, "Funnel", "MERGED", false, "feature/1245", "epic/1244"),
		),
		issue(1254, "削除と待機イベントの一本化", "OPEN", "REOPENED",
			pr(1257, "Unify delete", "OPEN", false, "feature/1254", "epic/1244"),
			pr(1250, "Old attempt", "CLOSED", false, "feature/1254-old", "epic/1244"),
		),
		&model.Node{Kind: model.KindIssue, Repo: "other/lib", Number: 7, Title: "Upstream fix", State: "CLOSED", StateReason: "NOT_PLANNED", Labels: []string{"bug", "p1"}},
		issue(1251, "KO エフェクトの装備の計測", "CLOSED", "DUPLICATE"),
	)
	root.Progress = &model.Progress{Completed: 1, Total: 4}
	root.HiddenClosed = 2

	got := renderText(t, []*model.Node{root}, Options{Repo: repo})
	assertGolden(t, got, `
#1244 [Epic] Telemetryを判断起点で作り直す  ● open 1/4
├─ #1245 Bricks の Progression Funnel       ✓ completed
│  └─ PR #1248 Funnel                       merged         feature/1245 → epic/1244
├─ #1254 削除と待機イベントの一本化         ● reopened
│  ├─ PR #1257 Unify delete                 open           feature/1254 → epic/1244
│  └─ PR #1250 Old attempt                  closed         feature/1254-old → epic/1244
├─ other/lib#7 Upstream fix [bug] [p1]      ⊘ not planned
├─ #1251 KO エフェクトの装備の計測          ⊘ duplicate
└─ … closed 2件
`)
}

func TestTextWrapsToWidth(t *testing.T) {
	tree := func() []*model.Node {
		root := issue(1244, "[Epic] Telemetryを判断起点で作り直す", "OPEN", "",
			issue(1254, "削除と待機イベントの一本化", "OPEN", "REOPENED",
				pr(1257, "Unify delete", "OPEN", false, "feature/1254-unify-delete-and-wait", "epic/1244"),
			),
			issue(9, "Wait event sampling", "CLOSED", "NOT_PLANNED"),
		)
		root.Progress = &model.Progress{Completed: 1, Total: 2}
		return []*model.Node{root}
	}
	full := renderText(t, tree(), Options{Repo: repo})
	if got := renderText(t, tree(), Options{Repo: repo, Width: 200}); got != full {
		t.Errorf("width 200 changed the full layout\n%s", got)
	}

	cases := []struct {
		width int
		want  string
	}{
		// The PR's head → base moves under it, indented past the PR's own
		// children.
		{72, `
#1244 [Epic] Telemetryを判断起点で作り直す  ● open 1/2
├─ #1254 削除と待機イベントの一本化         ● reopened
│  └─ PR #1257 Unify delete                 open
│          feature/1254-unify-delete-and-wait → epic/1244
└─ #9 Wait event sampling                   ⊘ not planned
`},
		// Titles also wrap inside their column, never leaving one CJK
		// character alone; head → base breaks before the arrow.
		{55, `
#1244 [Epic] Telemetryを判断起点で作り    ● open 1/2
│    直す
├─ #1254 削除と待機イベントの一本化       ● reopened
│  └─ PR #1257 Unify delete               open
│          feature/1254-unify-delete-and-wait
│          → epic/1244
└─ #9 Wait event sampling                 ⊘ not planned
`},
		// Rows too deep for a useful column become cards; rows that still
		// fit keep the column.
		{40, `
#1244 [Epic] Telemetryを   ● open 1/2
│    判断起点で作り直す
├─ #1254 削除と待機イベントの一本化
│  │    ● reopened
│  └─ PR #1257 Unify delete
│          open
│          feature/1254-unify-delete-
│          and-wait → epic/1244
└─ #9 Wait event sampling  ⊘ not planned
`},
		// Most rows are cards, so all are; long branch names break at
		// their seams.
		{36, `
#1244 [Epic] Telemetryを判断起点で作
│    り直す
│    ● open 1/2
├─ #1254 削除と待機イベントの一本化
│  │    ● reopened
│  └─ PR #1257 Unify delete
│          open
│          feature/1254-unify-
│          delete-and-wait
│          → epic/1244
└─ #9 Wait event sampling
        ⊘ not planned
`},
	}
	// content drops layout (whitespace and tree glyphs) and order, so only the
	// characters shown remain: narrowing must never drop any of them.
	strip := strings.NewReplacer(" ", "", "\n", "", "│", "", "├─", "", "└─", "")
	content := func(s string) string {
		rs := []rune(strip.Replace(s))
		slices.Sort(rs)
		return string(rs)
	}
	for _, c := range cases {
		got := renderText(t, tree(), Options{Repo: repo, Width: c.width})
		assertGolden(t, got, c.want)
		for _, line := range strings.Split(strings.TrimSuffix(got, "\n"), "\n") {
			if w := lipgloss.Width(line); w > c.width {
				t.Errorf("width %d: line is %d columns: %q", c.width, w, line)
			}
		}
		if content(got) != content(full) {
			t.Errorf("width %d dropped content\n%s", c.width, got)
		}
	}

	// An icon stays with its word at every width.
	for width := 24; width <= 80; width++ {
		roots := tree()
		roots[0].Labels = []string{"epic", "telemetry"}
		got := renderText(t, roots, Options{Repo: repo, Width: width, Icons: true})
		for _, line := range strings.Split(strings.TrimSuffix(got, "\n"), "\n") {
			if w := lipgloss.Width(line); w > width {
				t.Errorf("icons, width %d: line is %d columns: %q", width, w, line)
			}
			if rs := []rune(strings.TrimRight(line, " ")); len(rs) > 0 && rs[len(rs)-1] >= '\ue000' && rs[len(rs)-1] <= '\uf8ff' {
				t.Errorf("icons, width %d: line ends with an icon: %q", width, line)
			}
		}
	}
}

func TestTextIconsPerState(t *testing.T) {
	other := issue(7, "Upstream", "OPEN", "")
	other.Repo = "other/lib"
	otherPR := pr(8, "Upstream fix", "OPEN", false, "fix", "main")
	otherPR.Repo = "other/lib"
	progress := issue(1, "Epic", "OPEN", "")
	progress.Progress = &model.Progress{Completed: 1, Total: 3}
	labeled := issue(2, "Labeled", "OPEN", "")
	labeled.Labels = []string{"bug", "p1"}
	folded := issue(3, "Folded", "OPEN", "")
	folded.HiddenClosed, folded.OmittedChildren = 2, 4

	tests := []struct {
		name string
		node *model.Node
		want []string // substrings that must appear
	}{
		{"issue open", issue(1, "a", "OPEN", ""), []string{"\uf41b open"}},
		{"issue reopened", issue(1, "a", "OPEN", "REOPENED"), []string{"\uf41c reopened"}},
		{"issue completed", issue(1, "a", "CLOSED", "COMPLETED"), []string{"\uf41d completed"}},
		{"issue not planned", issue(1, "a", "CLOSED", "NOT_PLANNED"), []string{"\uf517 not planned"}},
		{"issue duplicate", issue(1, "a", "CLOSED", "DUPLICATE"), []string{"\uf4c4 duplicate"}},
		{"issue closed without a reason", issue(1, "a", "CLOSED", ""), []string{"\uf41d closed"}},
		{"pr open", pr(1, "a", "OPEN", false, "h", "main"), []string{"\uf407 #1", "open", "\uf418 h → main"}},
		{"pr draft", pr(1, "a", "OPEN", true, "h", "main"), []string{"\uf4dd #1", "draft"}},
		{"pr merged", pr(1, "a", "MERGED", false, "h", "main"), []string{"\uf419 #1", "merged"}},
		{"pr closed", pr(1, "a", "CLOSED", false, "h", "main"), []string{"\uf4dc #1", "closed"}},
		{"branch", &model.Node{Kind: model.KindBranch, Name: "epic/1"}, []string{"\uf418 epic/1"}},
		{"other repo issue", other, []string{"\uf401 other/lib#7"}},
		{"other repo pr has only the pr icon", otherPR, []string{"\uf407 other/lib#8"}},
		{"progress", progress, []string{"\uf4a0 1/3"}},
		{"labels", labeled, []string{"\uf412 bug \uf412 p1"}},
		{"folded children", folded, []string{"\uf41d closed 2件", "\uf51e 他 4件"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderText(t, []*model.Node{tt.node}, Options{Repo: repo, Icons: true})
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("missing %q in:\n%s", w, got)
				}
			}
			if strings.Contains(got, "PR #") || strings.Contains(got, "●") || strings.Contains(got, "[") {
				t.Errorf("plain symbols left in icon output:\n%s", got)
			}
		})
	}
}

func TestTextPRForestWithForkAndBranchRoots(t *testing.T) {
	fork := pr(1270, "Fork contribution", "OPEN", false, "patch-1", "release/2.4")
	fork.HeadOwner = "someone"
	roots := []*model.Node{
		{Kind: model.KindBranch, Name: "develop", Children: []*model.Node{
			pr(1260, "Release 2.4", "OPEN", false, "release/2.4", "develop", fork),
		}},
		{Kind: model.KindBranch, Name: "epic/#1244_telemetry-rebuild-with-a-very-long-name", Children: []*model.Node{
			pr(1256, "MatchBricksReward の送り元を戻す", "OPEN", true, "feature/#1255_match-bricks-reward", "epic/#1244_telemetry-rebuild-with-a-very-long-name"),
		}},
	}

	// A PR under its base (a branch node or the PR with that head) shows
	// only its head; the tree already shows the base.
	got := renderText(t, roots, Options{Repo: repo, DefaultBranch: "develop"})
	assertGolden(t, got, `
develop
└─ PR #1260 Release 2.4                             open         release/2.4
   └─ PR #1270 Fork contribution                    open         someone:patch-1
epic/#1244_telemetry-rebuild-with-a-very-long-name  （PR なし）
└─ PR #1256 MatchBricksReward の送り元を戻す        draft        feature/#1255_match-bricks-reward
`)
}

func TestTextPRUnderIssueShowsBase(t *testing.T) {
	roots := []*model.Node{issue(1, "Epic", "OPEN", "", pr(2, "Fix", "OPEN", false, "fix", "epic/1"))}
	got := renderText(t, roots, Options{Repo: repo})
	if !strings.Contains(got, "fix → epic/1") {
		t.Errorf("PR under an issue lost its base:\n%s", got)
	}
}

func TestTextNoColorHasNoEscapes(t *testing.T) {
	roots := []*model.Node{issue(1, "a", "OPEN", "", pr(2, "b", "MERGED", false, "h", "main"))}
	if got := renderText(t, roots, Options{Repo: repo}); strings.Contains(got, "\x1b[") {
		t.Errorf("Color=false output contains escape sequences: %q", got)
	}
	if got := renderText(t, roots, Options{Repo: repo, Color: true}); !strings.Contains(got, "\x1b[") {
		t.Errorf("Color=true output has no escape sequences: %q", got)
	}
}

func TestJSON(t *testing.T) {
	root := issue(1, "Root", "OPEN", "", pr(2, "Fix <thing>", "OPEN", false, "fix", "main"))
	root.Progress = &model.Progress{Completed: 0, Total: 0}
	var b bytes.Buffer
	if err := JSON(&b, repo, []*model.Node{root}); err != nil {
		t.Fatal(err)
	}
	assertGolden(t, b.String(), `
{
  "schemaVersion": 1,
  "repo": "acme/app",
  "roots": [
    {
      "kind": "issue",
      "repo": "acme/app",
      "number": 1,
      "title": "Root",
      "state": "OPEN",
      "progress": {
        "completed": 0,
        "total": 0
      },
      "children": [
        {
          "kind": "pr",
          "repo": "acme/app",
          "number": 2,
          "title": "Fix <thing>",
          "state": "OPEN",
          "base": "main",
          "head": "fix",
          "children": []
        }
      ]
    }
  ]
}
`)

	var doc model.Document
	if err := json.Unmarshal(b.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if root.Children[0].Children != nil {
		t.Error("JSON mutated the input tree")
	}
}

func TestJSONEmptyRoots(t *testing.T) {
	var b bytes.Buffer
	if err := JSON(&b, repo, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `"roots": []`) {
		t.Errorf("nil roots should encode as []: %s", b.String())
	}
}
