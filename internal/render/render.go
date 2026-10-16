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
}

const (
	colGap = "  "
	// minCellWidth is the narrowest the wrapped main column may get before the
	// state and detail columns move onto their own lines.
	minCellWidth = 16
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
	kind   model.Kind
	prefix string // tree glyphs before the node
	cont   string // tree glyphs before the node's continuation lines
	main   []seg  // identifier, title, labels
	state  string
	tone   tone
	extra  []seg // progress or head → base
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
//  2. PR "head → base" moves to its own line under the PR, while issue
//     progress stays beside the state;
//  3. state and details all move to lines of their own under each node.
func Text(w io.Writer, roots []*model.Node, opt Options) error {
	var rows []row
	for _, root := range roots {
		rows = appendRows(rows, root, "", "", opt)
	}

	var mainW, stateW, extraW, progressW int
	for _, r := range rows {
		mainW = max(mainW, ansi.StringWidth(r.prefix)+segsWidth(r.main))
		stateW = max(stateW, ansi.StringWidth(r.state))
		extraW = max(extraW, segsWidth(r.extra))
		if r.kind == model.KindIssue {
			progressW = max(progressW, segsWidth(r.extra))
		}
	}
	// cellsFit caps the main column so that cols more columns fit beside it,
	// and reports whether every wrapped main cell stays at least minCellWidth.
	cellsFit := func(cols int) (int, bool) {
		avail := min(mainW, opt.Width-cols)
		for _, r := range rows {
			if avail-ansi.StringWidth(r.cont) < minCellWidth {
				return avail, false
			}
		}
		return avail, true
	}
	colsFor := func(extra int) int {
		c := len(colGap) + stateW
		if extra > 0 {
			c += len(colGap) + extra
		}
		return c
	}

	stacked := false
	inlineExtraW := extraW // extras wider than this go on their own line
	if opt.Width > 0 {
		if m, ok := cellsFit(colsFor(extraW)); ok {
			mainW = m
		} else if m, ok := cellsFit(colsFor(progressW)); ok {
			mainW, inlineExtraW = m, progressW
		} else {
			stacked = true
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
			b.WriteString(style.Render(s.text))
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

	for _, r := range rows {
		full := opt.Width - ansi.StringWidth(r.cont)
		switch {
		case opt.Width <= 0:
			writeColumns(&b, r, [][]seg{r.main}, mainW, stateW, glyph, paint)
		case stacked:
			writeLines(r.prefix, r.cont, wrap(r.main, opt.Width-ansi.StringWidth(r.prefix), full))
			var details []seg
			if r.state != "" {
				details = append(details, seg{text: r.state, tone: r.tone})
			}
			if len(r.extra) > 0 {
				if len(details) > 0 {
					details = append(details, seg{text: colGap})
				}
				details = append(details, r.extra...)
			}
			if len(details) > 0 {
				writeLines(r.cont, r.cont, wrap(details, full, full))
			}
		default:
			below := r.extra
			if segsWidth(r.extra) <= inlineExtraW {
				below = nil
			} else {
				r.extra = nil
			}
			lines := wrap(r.main, mainW-ansi.StringWidth(r.prefix), mainW-ansi.StringWidth(r.cont))
			writeColumns(&b, r, lines, mainW, stateW, glyph, paint)
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
		if i == 0 && (r.state != "" || len(r.extra) > 0) {
			b.WriteString(strings.Repeat(" ", max(mainW-ansi.StringWidth(r.prefix)-segsWidth(l), 0)))
			b.WriteString(colGap)
			paint([]seg{{text: r.state, tone: r.tone}})
			if len(r.extra) > 0 {
				b.WriteString(strings.Repeat(" ", stateW-ansi.StringWidth(r.state)))
				b.WriteString(colGap)
				paint(r.extra)
			}
		}
		b.WriteByte('\n')
	}
}

// wrap breaks segments into lines of at most first columns for the first
// line and rest columns after it. It breaks at spaces and between wide (CJK)
// characters, and splits a word only when it is longer than a whole line.
func wrap(ss []seg, first, rest int) [][]seg {
	lines := [][]seg{nil}
	limit, used := max(first, 1), 0
	newline := func() {
		cur := lines[len(lines)-1]
		if n := len(cur); n > 0 {
			cur[n-1].text = strings.TrimRight(cur[n-1].text, " ")
		}
		lines = append(lines, nil)
		limit, used = max(rest, 1), 0
	}
	add := func(text string, s seg) {
		cur := &lines[len(lines)-1]
		if n := len(*cur); n > 0 && (*cur)[n-1].tone == s.tone && (*cur)[n-1].bold == s.bold {
			(*cur)[n-1].text += text
		} else {
			*cur = append(*cur, seg{text: text, tone: s.tone, bold: s.bold})
		}
		used += ansi.StringWidth(text)
	}
	for _, s := range ss {
		for _, tok := range tokens(s.text) {
			if tok == " " {
				if used == 0 {
					continue
				}
				if used+1 > limit {
					newline()
					continue
				}
				add(tok, s)
				continue
			}
			if used > 0 && used+ansi.StringWidth(tok) > limit {
				newline()
			}
			for ansi.StringWidth(tok) > limit-used {
				head := ansi.Truncate(tok, limit-used, "")
				if head == "" {
					break
				}
				add(head, s)
				tok = ansi.TruncateLeft(tok, ansi.StringWidth(head), "")
				newline()
			}
			add(tok, s)
		}
	}
	return lines
}

// tokens splits s into single spaces, runs of narrow non-space characters,
// and single wide characters, which are the points wrap may break between.
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
		case unicode.IsSpace(r):
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

func appendRows(rows []row, n *model.Node, prefix, childPrefix string, opt Options) []row {
	r := nodeRow(n, opt)
	r.kind = n.Kind
	r.prefix = prefix
	// Summary lines for children that are not drawn, after the drawn ones.
	var summaries []string
	if n.HiddenClosed > 0 {
		summaries = append(summaries, fmt.Sprintf("… closed %d件", n.HiddenClosed))
	}
	if n.OmittedChildren > 0 {
		summaries = append(summaries, fmt.Sprintf("… 他 %d件", n.OmittedChildren))
	}
	r.cont = childPrefix + "   "
	if len(n.Children)+len(summaries) > 0 {
		r.cont = childPrefix + "│  "
	}
	rows = append(rows, r)

	last := len(n.Children) + len(summaries) - 1
	for i, c := range n.Children {
		if i == last {
			rows = appendRows(rows, c, childPrefix+"└─ ", childPrefix+"   ", opt)
		} else {
			rows = appendRows(rows, c, childPrefix+"├─ ", childPrefix+"│  ", opt)
		}
	}
	for i, s := range summaries {
		glyph, cont := "├─ ", "│  "
		if len(n.Children)+i == last {
			glyph, cont = "└─ ", "   "
		}
		rows = append(rows, row{
			prefix: childPrefix + glyph,
			cont:   childPrefix + cont,
			main:   []seg{{text: s, tone: toneGray}},
		})
	}
	return rows
}

// nodeRow sets the visual hierarchy: the number carries the state color and
// is the scanning anchor; open issue titles stay bright, closed issues and
// every PR row recede to gray so the work that remains stands out.
func nodeRow(n *model.Node, opt Options) row {
	repo := opt.Repo
	switch n.Kind {
	case model.KindIssue:
		state, t := issueState(n)
		titleTone := toneNone
		if n.State == "CLOSED" {
			titleTone = toneGray
		}
		r := row{
			main:  append([]seg{{text: ref(n, repo), tone: t, bold: true}, {text: title(n.Title), tone: titleTone, bold: n.Focus}}, labelSegs(n.Labels)...),
			state: state,
			tone:  t,
		}
		if n.Focus {
			r.main = append([]seg{{text: "▶ ", tone: t, bold: true}}, r.main...)
		}
		if p := n.Progress; p != nil {
			pt := toneBold
			if p.Completed == p.Total {
				pt = toneGreen
			}
			r.extra = []seg{{text: fmt.Sprintf("%d/%d", p.Completed, p.Total), tone: pt}}
		}
		return r
	case model.KindPR:
		state, t := prState(n)
		head := n.Head
		if n.HeadOwner != "" {
			head = n.HeadOwner + ":" + head
		}
		return row{
			main:  append([]seg{{text: "PR " + ref(n, repo), tone: t}, {text: title(n.Title), tone: toneGray}}, labelSegs(n.Labels)...),
			state: state,
			tone:  t,
			extra: []seg{{text: head + " → ", tone: toneGray}, {text: n.Base, tone: toneBranch}},
		}
	case model.KindBranch:
		r := row{main: append([]seg{{text: n.Name, tone: toneBranch, bold: true}}, labelSegs(n.Labels)...)}
		if n.Name != opt.DefaultBranch {
			r.state, r.tone = "（PR なし）", toneGray
		}
		return r
	default:
		return row{main: []seg{{text: string(n.Kind)}}}
	}
}

func ref(n *model.Node, repo string) string {
	num := "#" + strconv.Itoa(n.Number)
	if n.Repo != "" && repo != "" && !strings.EqualFold(n.Repo, repo) {
		return n.Repo + num
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

func labelSegs(ls []string) []seg {
	if len(ls) == 0 {
		return nil
	}
	return []seg{{text: " [" + strings.Join(ls, "] [") + "]", tone: toneLabel}}
}

func issueState(n *model.Node) (string, tone) {
	if n.State != "CLOSED" {
		if n.StateReason == "REOPENED" {
			return "● open (reopened)", toneGreen
		}
		return "● open", toneGreen
	}
	switch n.StateReason {
	case "NOT_PLANNED":
		return "⊘ closed (not planned)", toneGray
	case "DUPLICATE":
		return "⊘ closed (duplicate)", toneGray
	case "COMPLETED":
		return "✓ closed (completed)", tonePurple
	default:
		return "✓ closed", tonePurple
	}
}

func prState(n *model.Node) (string, tone) {
	switch n.State {
	case "MERGED":
		return "merged", tonePurple
	case "CLOSED":
		return "closed", toneRed
	default:
		if n.IsDraft {
			return "draft", toneGray
		}
		return "open", toneGreen
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
