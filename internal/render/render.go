// Package render prints built trees as box-drawing text or as the JSON
// contract (docs/prd.md 7章, 10章).
package render

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/ohzw/gh-tree/internal/model"
)

type Options struct {
	Color bool
	// Repo is the "owner/name" being viewed. Issues and PRs of any other
	// repository are shown as "owner/repo#N".
	Repo string
	// DefaultBranch is the repository's default branch. Its branch node is
	// shown without the "（PR なし）" annotation.
	DefaultBranch string
	// Width is the terminal width in columns; 0 means unlimited (pipes).
	Width int
	// Icons draws Nerd Font (Octicons) icons instead of plain symbols.
	Icons bool
}

const (
	colGap = "  "
	// hang indents a node's own continuation lines (wrapped title, state and
	// details) past its children's text, so they never read as a child.
	hang = "  "
	// minCellWidth is the narrowest a row's wrapped main cell may get beside
	// the state column; a row that would wrap narrower is drawn as a card.
	minCellWidth = 20
	// iconSep joins an icon to its word, and a few words that belong
	// together. wrap never breaks there, and it is written as a plain space.
	iconSep = "\u00a0"
)

// tone is the color role of a cell: GitHub UI state colors plus the neutral
// roles that set the visual hierarchy (bold anchors, muted secondary text).
type tone int

const (
	toneNone tone = iota
	toneGreen
	tonePurple
	toneGray
	toneRed
	toneBold
	toneBranch
	toneLabel
	toneCount
)

// seg is a run of text drawn in one tone.
type seg struct {
	text string
	tone tone
	bold bool
}

// row is one node split into columns. Nothing is ever truncated: when the
// terminal is narrow, text wraps onto continuation lines instead.
type row struct {
	prefix string // tree glyphs before the node
	cont   string // tree glyphs and hang before the node's continuation lines
	main   []seg  // identifier, title, labels
	state  []seg  // state word, and an issue's progress
	extra  []seg  // a PR's head (→ base)
}

func segsWidth(ss []seg) int {
	w := 0
	for _, s := range ss {
		w += ansi.StringWidth(s.text)
	}
	return w
}

// Text writes roots as box-drawing trees with columns aligned across all
// roots. Nothing is truncated. With opt.Width > 0 it narrows in steps:
//  1. the main column (identifier, title, labels) wraps;
//  2. PR head → base moves to its own line under the PR;
//  3. a row whose main cell would get narrower than minCellWidth becomes a
//     card: its main text on full-width lines, then its state and details
//     on lines of their own. Other rows keep the columns.
func Text(w io.Writer, roots []*model.Node, opt Options) error {
	var rows []row
	for _, root := range roots {
		rows = appendRows(rows, root, nil, "", "", opt)
	}

	var mainW, stateW, extraW int
	for _, r := range rows {
		mainW = max(mainW, ansi.StringWidth(r.prefix)+segsWidth(r.main))
		stateW = max(stateW, segsWidth(r.state))
		extraW = max(extraW, segsWidth(r.extra))
	}
	colsFor := func(extra int) int {
		c := len(colGap) + stateW
		if extra > 0 {
			c += len(colGap) + extra
		}
		return c
	}
	// fits reports whether r can stay in the columns: its main text needs no
	// wrapping, or its wrapped lines keep minCellWidth.
	fits := func(r row, avail int) bool {
		return ansi.StringWidth(r.prefix)+segsWidth(r.main) <= avail || avail-ansi.StringWidth(r.cont) >= minCellWidth
	}

	inline := true // PR head → base beside the state
	card := make([]bool, len(rows))
	if opt.Width > 0 {
		avail := min(mainW, opt.Width-colsFor(extraW))
		for _, r := range rows {
			if !fits(r, avail) {
				inline = false
				break
			}
		}
		if !inline {
			avail = min(mainW, opt.Width-colsFor(0))
		}
		cards := 0
		for i, r := range rows {
			if card[i] = !fits(r, avail); card[i] {
				cards++
			}
		}
		// When most rows are cards, a column for the few left is noise.
		if cards*2 > len(rows) {
			for i := range card {
				card[i] = true
			}
		}
		// Cards leave the columns, so only column rows size the main column.
		mainW = 0
		for i, r := range rows {
			if !card[i] {
				mainW = max(mainW, min(avail, ansi.StringWidth(r.prefix)+segsWidth(r.main)))
			}
		}
	}

	st := newStyles(w, opt.Color)
	var b strings.Builder
	paint := func(ss []seg) {
		for _, s := range ss {
			if s.text == "" {
				continue
			}
			style := st.tones[s.tone]
			if s.bold {
				style = style.Bold(true)
			}
			b.WriteString(style.Render(strings.ReplaceAll(s.text, iconSep, " ")))
		}
	}
	glyph := func(g string) { paint([]seg{{text: g, tone: toneGray}}) }
	writeLines := func(first, rest string, lines [][]seg) {
		for i, l := range lines {
			if i == 0 {
				glyph(first)
			} else {
				glyph(rest)
			}
			paint(l)
			b.WriteByte('\n')
		}
	}

	for i, r := range rows {
		prefixW, contW := ansi.StringWidth(r.prefix), ansi.StringWidth(r.cont)
		full := opt.Width - contW
		switch {
		case opt.Width <= 0:
			writeColumns(&b, r, [][]seg{r.main}, mainW, stateW, glyph, paint)
		case card[i]:
			writeLines(r.prefix, r.cont, wrap(r.main, opt.Width-prefixW, full))
			for _, d := range [][]seg{r.state, r.extra} {
				if len(d) > 0 {
					writeLines(r.cont, r.cont, wrap(d, full, full))
				}
			}
		default:
			var below []seg
			if !inline {
				below, r.extra = r.extra, nil
			}
			writeColumns(&b, r, wrap(r.main, mainW-prefixW, mainW-contW), mainW, stateW, glyph, paint)
			if len(below) > 0 {
				writeLines(r.cont, r.cont, wrap(below, full, full))
			}
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// writeColumns writes the first main line with the state and extra columns
// beside it, then the remaining main lines under it.
func writeColumns(b *strings.Builder, r row, lines [][]seg, mainW, stateW int, glyph func(string), paint func([]seg)) {
	for i, l := range lines {
		if i == 0 {
			glyph(r.prefix)
		} else {
			glyph(r.cont)
		}
		paint(l)
		if i == 0 && (len(r.state) > 0 || len(r.extra) > 0) {
			b.WriteString(strings.Repeat(" ", max(mainW-ansi.StringWidth(r.prefix)-segsWidth(l), 0)))
			b.WriteString(colGap)
			paint(r.state)
			if len(r.extra) > 0 {
				b.WriteString(strings.Repeat(" ", stateW-segsWidth(r.state)))
				b.WriteString(colGap)
				paint(r.extra)
			}
		}
		b.WriteByte('\n')
	}
}

// unit is what wrap places on a line as a whole: a space, a single wide
// (CJK) character, or a word. A word is a run of narrow non-space
// characters and may span segs, so a change of color is not a break point.
type unit struct {
	parts       []seg
	space, wide bool
}

func (u unit) width() int { return segsWidth(u.parts) }

// split cuts u after at most n columns: after the last '/', '-' or '_' in
// them if that keeps at least half, so branch names break at their seams.
func (u unit) split(n int) (head, tail unit) {
	cut, w := n, 0
seams:
	for _, p := range u.parts {
		for _, r := range p.text {
			if w += ansi.StringWidth(string(r)); w > n {
				break seams
			}
			if strings.ContainsRune("/-_", r) && w*2 >= n {
				cut = w
			}
		}
	}
	n = cut
	for i, p := range u.parts {
		pw := ansi.StringWidth(p.text)
		if pw <= n {
			head.parts = append(head.parts, p)
			n -= pw
			continue
		}
		if h := ansi.Truncate(p.text, n, ""); h != "" {
			head.parts = append(head.parts, seg{text: h, tone: p.tone, bold: p.bold})
			p.text = ansi.TruncateLeft(p.text, ansi.StringWidth(h), "")
		}
		tail.parts = append([]seg{p}, u.parts[i+1:]...)
		return head, tail
	}
	return head, tail
}

// wrap breaks segments into lines of at most first columns for the first
// line and rest columns after it. It breaks at spaces and between wide (CJK)
// characters, and splits a word only when it is longer than a whole line.
// It does not leave a single wide character alone on the last line.
func wrap(ss []seg, first, rest int) [][]seg {
	var units []unit
	for _, s := range ss {
		for _, tok := range tokens(s.text) {
			p := seg{text: tok, tone: s.tone, bold: s.bold}
			n := len(units)
			switch {
			case tok == " ":
				units = append(units, unit{parts: []seg{p}, space: true})
			case ansi.StringWidth(tok) > 1 && utf8.RuneCountInString(tok) == 1:
				units = append(units, unit{parts: []seg{p}, wide: true})
			case n > 0 && !units[n-1].space && !units[n-1].wide:
				units[n-1].parts = append(units[n-1].parts, p)
			default:
				units = append(units, unit{parts: []seg{p}})
			}
		}
	}

	lines := [][]unit{nil}
	limit, used := max(first, 1), 0
	trim := func(l []unit) []unit {
		for len(l) > 0 && l[len(l)-1].space {
			l = l[:len(l)-1]
		}
		return l
	}
	newline := func() {
		lines[len(lines)-1] = trim(lines[len(lines)-1])
		lines = append(lines, nil)
		limit, used = max(rest, 1), 0
	}
	push := func(u unit) {
		lines[len(lines)-1] = append(lines[len(lines)-1], u)
		used += u.width()
	}
	for _, u := range units {
		if u.space {
			if used == 0 {
				continue
			}
			if used+1 > limit {
				newline()
				continue
			}
			push(u)
			continue
		}
		if used > 0 && used+u.width() > limit {
			newline()
		}
		for u.width() > limit-used {
			head, tail := u.split(limit - used)
			if len(head.parts) == 0 {
				break
			}
			push(head)
			newline()
			u = tail
		}
		push(u)
	}
	lines[len(lines)-1] = trim(lines[len(lines)-1])

	// Move the previous line's last wide character down to a lone one.
	if n := len(lines); n >= 2 && rest >= 4 {
		last, prev := lines[n-1], lines[n-2]
		if len(last) == 1 && last[0].wide && len(prev) > 1 && prev[len(prev)-1].wide {
			lines[n-2] = trim(prev[:len(prev)-1])
			lines[n-1] = []unit{prev[len(prev)-1], last[0]}
		}
	}

	out := make([][]seg, len(lines))
	for i, l := range lines {
		for _, u := range l {
			for _, p := range u.parts {
				if k := len(out[i]); k > 0 && out[i][k-1].tone == p.tone && out[i][k-1].bold == p.bold {
					out[i][k-1].text += p.text
				} else {
					out[i] = append(out[i], p)
				}
			}
		}
	}
	return out
}

// tokens splits s into single spaces, runs of narrow non-space characters,
// and single wide characters, which are the points wrap may break between.
// iconSep is not a space: it holds its neighbors together.
func tokens(s string) []string {
	var out []string
	var word strings.Builder
	flush := func() {
		if word.Len() > 0 {
			out = append(out, word.String())
			word.Reset()
		}
	}
	for _, r := range s {
		switch {
		case unicode.IsSpace(r) && string(r) != iconSep:
			flush()
			out = append(out, " ")
		case ansi.StringWidth(string(r)) > 1:
			flush()
			out = append(out, string(r))
		default:
			word.WriteRune(r)
		}
	}
	flush()
	return out
}

func appendRows(rows []row, n, parent *model.Node, prefix, childPrefix string, opt Options) []row {
	r := nodeRow(n, parent, opt)
	r.prefix = prefix
	g := glyphsFor(opt.Icons)
	// Summary lines for children that are not drawn, after the drawn ones.
	var summaries []string
	if n.HiddenClosed > 0 {
		summaries = append(summaries, withIcon(g.hiddenClosed, fmt.Sprintf("closed %d件", n.HiddenClosed)))
	}
	if n.OmittedChildren > 0 {
		summaries = append(summaries, withIcon(g.omitted, fmt.Sprintf("他 %d件", n.OmittedChildren)))
	}
	r.cont = childPrefix + "   " + hang
	if len(n.Children)+len(summaries) > 0 {
		r.cont = childPrefix + "│  " + hang
	}
	rows = append(rows, r)

	last := len(n.Children) + len(summaries) - 1
	for i, c := range n.Children {
		if i == last {
			rows = appendRows(rows, c, n, childPrefix+"└─ ", childPrefix+"   ", opt)
		} else {
			rows = appendRows(rows, c, n, childPrefix+"├─ ", childPrefix+"│  ", opt)
		}
	}
	for i, s := range summaries {
		glyph, cont := "├─ ", "│  "
		if len(n.Children)+i == last {
			glyph, cont = "└─ ", "   "
		}
		rows = append(rows, row{
			prefix: childPrefix + glyph,
			cont:   childPrefix + cont + "   " + hang,
			main:   []seg{{text: s, tone: toneGray}},
		})
	}
	return rows
}

// nodeRow sets the visual hierarchy: the number carries the state color and
// is the scanning anchor; open issue titles stay bright, closed issues and
// every PR row recede to gray so the work that remains stands out.
func nodeRow(n, parent *model.Node, opt Options) row {
	repo := opt.Repo
	g := glyphsFor(opt.Icons)
	switch n.Kind {
	case model.KindIssue:
		state, t := issueState(n, g)
		titleTone := toneNone
		if n.State == "CLOSED" {
			titleTone = toneGray
		}
		r := row{
			main:  append([]seg{{text: ref(n, repo, g.repo), tone: t, bold: true}, {text: title(n.Title), tone: titleTone, bold: n.Focus}}, labelSegs(n.Labels, g)...),
			state: []seg{{text: state, tone: t}},
		}
		if n.Focus {
			r.main = append([]seg{{text: "▶ ", tone: t, bold: true}}, r.main...)
		}
		if p := n.Progress; p != nil {
			pt := toneBold
			if p.Completed == p.Total {
				pt = toneGreen
			}
			r.state = append(r.state, seg{text: " " + withIcon(g.tasklist, fmt.Sprintf("%d/%d", p.Completed, p.Total)), tone: pt})
		}
		return r
	case model.KindPR:
		state, t, icon := prState(n, g)
		head := n.Head
		if n.HeadOwner != "" {
			head = n.HeadOwner + ":" + head
		}
		extra := []seg{{text: withIcon(g.branch, head), tone: toneGray}}
		if !parentIsBase(n, parent) {
			// "→ base" stays together; the line may break before the arrow.
			extra = []seg{{text: withIcon(g.branch, head) + " →" + iconSep, tone: toneGray}, {text: n.Base, tone: toneBranch}}
		}
		return row{
			// The PR icon already leads the row, so no repository icon.
			main:  append([]seg{{text: withIcon(icon, ref(n, repo, "")), tone: t}, {text: title(n.Title), tone: toneGray}}, labelSegs(n.Labels, g)...),
			state: []seg{{text: state, tone: t}},
			extra: extra,
		}
	case model.KindBranch:
		r := row{main: append([]seg{{text: withIcon(g.branch, n.Name), tone: toneBranch, bold: true}}, labelSegs(n.Labels, g)...)}
		if n.Name != opt.DefaultBranch {
			r.state = []seg{{text: "（PR なし）", tone: toneGray}}
		}
		return r
	default:
		return row{main: []seg{{text: string(n.Kind)}}}
	}
}

// parentIsBase reports whether a PR's parent node is its base branch: a
// branch node of that name, or a PR whose head is that branch. The tree
// already shows the base then, so the PR row shows only its head.
func parentIsBase(n, parent *model.Node) bool {
	if parent == nil {
		return false
	}
	switch parent.Kind {
	case model.KindBranch:
		return parent.Name == n.Base
	case model.KindPR:
		return parent.HeadOwner == "" && parent.Head == n.Base && strings.EqualFold(parent.Repo, n.Repo)
	default:
		return false
	}
}

// glyphs are the icons drawn in text output. An empty glyph draws nothing.
type glyphs struct {
	issueOpen, issueReopened, issueClosed, issueNotPlanned, issueDuplicate string
	prOpen, prDraft, prMerged, prClosed                                    string
	branch, repo, label, tasklist, hiddenClosed, omitted                   string
}

var (
	plainGlyphs = glyphs{
		issueOpen: "●", issueReopened: "●", issueClosed: "✓", issueNotPlanned: "⊘", issueDuplicate: "⊘",
		prOpen: "PR", prDraft: "PR", prMerged: "PR", prClosed: "PR",
		hiddenClosed: "…", omitted: "…",
	}
	// nerdGlyphs are GitHub's Octicons as packaged in Nerd Fonts (nf-oct-*).
	// Each is followed by a space so a font drawing it two cells wide
	// overlaps the space instead of shifting the columns.
	nerdGlyphs = glyphs{
		issueOpen:       "\uf41b", // oct-issue_opened
		issueReopened:   "\uf41c", // oct-issue_reopened
		issueClosed:     "\uf41d", // oct-issue_closed
		issueNotPlanned: "\uf517", // oct-skip
		issueDuplicate:  "\uf4c4", // oct-duplicate
		prOpen:          "\uf407", // oct-git_pull_request
		prDraft:         "\uf4dd", // oct-git_pull_request_draft
		prMerged:        "\uf419", // oct-git_merge
		prClosed:        "\uf4dc", // oct-git_pull_request_closed
		branch:          "\uf418", // oct-git_branch
		repo:            "\uf401", // oct-repo
		label:           "\uf412", // oct-tag
		tasklist:        "\uf4a0", // oct-tasklist
		hiddenClosed:    "\uf41d", // oct-issue_closed
		omitted:         "\uf51e", // oct-stack
	}
)

func glyphsFor(icons bool) glyphs {
	if icons {
		return nerdGlyphs
	}
	return plainGlyphs
}

func withIcon(icon, text string) string {
	if icon == "" {
		return text
	}
	return icon + iconSep + text
}

func ref(n *model.Node, repo, repoIcon string) string {
	num := "#" + strconv.Itoa(n.Number)
	if n.Repo != "" && repo != "" && !strings.EqualFold(n.Repo, repo) {
		return withIcon(repoIcon, n.Repo+num)
	}
	return num
}

// title collapses whitespace runs (titles can contain newlines).
func title(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return ""
	}
	return " " + s
}

func labelSegs(ls []string, g glyphs) []seg {
	if len(ls) == 0 {
		return nil
	}
	text := " [" + strings.Join(ls, "] [") + "]"
	if g.label != "" {
		text = ""
		for _, l := range ls {
			text += " " + withIcon(g.label, l)
		}
	}
	return []seg{{text: text, tone: toneLabel}}
}

// issueState returns the short state word: the icon carries open/closed, so
// the word names the reason, and plain "closed" means GitHub gave none.
func issueState(n *model.Node, g glyphs) (string, tone) {
	if n.State != "CLOSED" {
		if n.StateReason == "REOPENED" {
			return withIcon(g.issueReopened, "reopened"), toneGreen
		}
		return withIcon(g.issueOpen, "open"), toneGreen
	}
	switch n.StateReason {
	case "NOT_PLANNED":
		return withIcon(g.issueNotPlanned, "not"+iconSep+"planned"), toneGray
	case "DUPLICATE":
		return withIcon(g.issueDuplicate, "duplicate"), toneGray
	case "COMPLETED":
		return withIcon(g.issueClosed, "completed"), tonePurple
	default:
		return withIcon(g.issueClosed, "closed"), tonePurple
	}
}

// prState returns the state word, its tone, and the icon that leads the PR row.
func prState(n *model.Node, g glyphs) (string, tone, string) {
	switch n.State {
	case "MERGED":
		return "merged", tonePurple, g.prMerged
	case "CLOSED":
		return "closed", toneRed, g.prClosed
	default:
		if n.IsDraft {
			return "draft", toneGray, g.prDraft
		}
		return "open", toneGreen, g.prOpen
	}
}

type styles struct {
	tones [toneCount]lipgloss.Style
}

func newStyles(w io.Writer, color bool) styles {
	re := lipgloss.NewRenderer(w)
	if color {
		re.SetColorProfile(termenv.ANSI)
	} else {
		re.SetColorProfile(termenv.Ascii)
	}
	fg := func(c string) lipgloss.Style { return re.NewStyle().Foreground(lipgloss.Color(c)) }
	var s styles
	s.tones[toneNone] = re.NewStyle()
	s.tones[toneGreen] = fg("2")
	s.tones[tonePurple] = fg("5")
	s.tones[toneGray] = fg("8")
	s.tones[toneRed] = fg("1")
	s.tones[toneBold] = re.NewStyle().Bold(true)
	s.tones[toneBranch] = fg("4")
	s.tones[toneLabel] = fg("6")
	return s
}

// JSON writes the --json document. Nil children are written as [] so every
// node has the same shape.
func JSON(w io.Writer, repo string, roots []*model.Node) error {
	doc := model.Document{
		SchemaVersion: model.SchemaVersion,
		Repo:          repo,
		Roots:         normalize(roots),
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

func normalize(ns []*model.Node) []*model.Node {
	out := make([]*model.Node, len(ns))
	for i, n := range ns {
		c := *n
		c.Children = normalize(n.Children)
		out[i] = &c
	}
	return out
}
