package render

import (
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/list"
	"github.com/codyconfer/viewkit/theme"
	"github.com/codyconfer/viewkit/timefmt"
	"github.com/codyconfer/viewkit/tree"
	"github.com/codyconfer/viewkit/ui"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/render/glyph"
	"github.com/codyconfer/mino/internal/signals"
)

type TerminalRenderer struct{ Root string }

func (tr *TerminalRenderer) Render(w io.Writer, sections []signals.Section) error {
	if _, err := io.WriteString(w, Panels(layout.FrameFor(w), tr.Root, sections)+"\n"); err != nil {
		return errs.Wrap(errs.KindInternal, err, "write terminal output")
	}
	return nil
}

func RenderTerminalStringTitled(root string, sections []signals.Section) string {
	return treeString(FlightTree(layout.DocumentFrame(), root, sections))
}

// RenderTerminalString renders sections without a trunk row, for views whose
// chrome already shows the title.
func RenderTerminalString(sections []signals.Section) string {
	return treeString(SectionRows(layout.DocumentFrame(), sections))
}

func treeString(rows []tree.Row) string {
	lines := make([]string, 0, len(rows))
	for _, r := range rows {
		lines = append(lines, r.Lines...)
	}
	return strings.Join(lines, "\n")
}

func sectionGlyph(g glyph.Set, s signals.Section) string {
	n := strings.ToLower(s.Signal + " " + s.Title)
	switch {
	case strings.Contains(n, "github"):
		return g.GitHub()
	case strings.Contains(n, "slack"):
		return g.Slack()
	case strings.Contains(n, "cal"), strings.Contains(n, "gmail"), strings.Contains(n, "mail"),
		strings.Contains(n, "doc"), strings.Contains(n, "drive"), strings.Contains(n, "task"),
		strings.Contains(n, "google"):
		return g.Google()
	}
	return g.Bullet()
}

const DefaultRoot = "results"

func rootLabel(root string) string {
	if strings.TrimSpace(root) == "" {
		return DefaultRoot
	}
	return root
}

func Panels(f layout.Frame, root string, sections []signals.Section) string {
	return treeString(FlightTree(f, rootLabel(root), sections))
}

func SectionItems(f layout.Frame, sections []signals.Section) []list.Item {
	rows := SectionRows(f, sections)
	items := make([]list.Item, 0, len(rows))
	for _, r := range rows {
		it := r.Item()
		if !r.Selectable {
			it.Block = layout.IndentLines(it.Block, 2)
		}
		items = append(items, it)
	}
	return items
}

type SectionResults struct {
	Sections []signals.Section
}

func (r SectionResults) Items(f layout.Frame) []list.Item {
	return SectionItems(f, r.Sections)
}

func (r SectionResults) Count() int {
	n := 0
	for _, sec := range r.Sections {
		if sec.Err != nil {
			n++
			continue
		}
		n += len(sec.Items)
	}
	return n
}

func (r SectionResults) Errored() int {
	n := 0
	for _, sec := range r.Sections {
		if sec.Err != nil {
			n++
		}
	}
	return n
}

func ItemRows(f layout.Frame, items []signals.Item) []list.Item {
	t := newItemTheme(f.Glyphs(), f.Theme())
	rows := make([]list.Item, 0, len(items))
	for _, it := range items {
		rows = append(rows, list.Item{
			Block:      strings.Join(itemLinesIn(f, t, it), "\n"),
			Key:        it.URL,
			Selectable: it.URL != "",
		})
	}
	return rows
}

// Success renders a success-colored check followed by msg in scope s.
func Success(s *ui.Scope, msg string) string { return s.Success(msg) }

// Bullet renders an accent-colored bullet followed by msg in scope s.
func Bullet(s *ui.Scope, msg string) string { return s.Bullet(msg) }

func lastCommentTime(it signals.Item) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339, it.Meta["last_comment_at"])
	if err != nil || t.IsZero() {
		return time.Time{}, false
	}
	return t, true
}

// sevTheme caches a theme's severity styles for one render pass, so the lookup
// does not repeat per item.
type sevTheme struct {
	th  theme.Theme
	sty [4]lipgloss.Style
}

func newSevTheme(th theme.Theme) sevTheme {
	t := sevTheme{th: th}
	for i := range t.sty {
		t.sty[i] = th.SeverityStyle(glyph.Kind(i))
	}
	return t
}

// style returns the cached style for k, resolving unknown kinds on the fly.
func (t sevTheme) style(k glyph.Kind) lipgloss.Style {
	if k >= 0 && int(k) < len(t.sty) {
		return t.sty[k]
	}
	return t.th.SeverityStyle(k)
}

// itemTheme adds the prerendered severity leads for one glyph set, so the item
// icon is styled once per pass instead of once per item.
type itemTheme struct {
	sevTheme
	g     glyph.Set
	icons [4]string
}

func newItemTheme(g glyph.Set, th theme.Theme) itemTheme {
	t := itemTheme{sevTheme: newSevTheme(th), g: g}
	for i := range t.icons {
		t.icons[i] = t.renderIcon(glyph.Kind(i))
	}
	return t
}

// icon returns the cached rendered lead for k.
func (t itemTheme) icon(k glyph.Kind) string {
	if k >= 0 && int(k) < len(t.icons) {
		return t.icons[k]
	}
	return t.renderIcon(k)
}

func (t itemTheme) renderIcon(k glyph.Kind) string {
	return t.style(k).Render(glyph.Lead(glyph.ForIn(t.g, k)))
}

func lastCommentChip(th theme.Theme, it signals.Item) string {
	at, ok := lastCommentTime(it)
	return commentChip(newSevTheme(th), it, at, ok)
}

// commentChip renders the last-comment cue; at and hasAge come from
// lastCommentTime, which the caller already needs.
func commentChip(t sevTheme, it signals.Item, at time.Time, hasAge bool) string {
	last := it.Meta["last_comment_by"]
	if last == "" {
		return ""
	}
	chip := glyph.Lead(glyph.Reply()) + "@" + last
	if it.Meta["last_comment_team"] == "true" {
		chip += " ·team"
	}
	if hasAge {
		chip += " ·" + timefmt.Rel(at)
	}
	switch it.Meta["last_comment_team"] {
	case "true":
		return t.style(glyph.KindPositive).Render(chip)
	case "false":
		return t.style(glyph.KindWarning).Render(chip)
	default:
		return t.th.Dim.Render(chip)
	}
}

func itemLines(f layout.Frame, th theme.Theme, it signals.Item) []string {
	return itemLinesIn(f, newItemTheme(f.Glyphs(), th), it)
}

func itemLinesIn(f layout.Frame, t itemTheme, it signals.Item) []string {
	it = signals.CleanItem(it)

	th := t.th
	head := t.icon(glyph.ClassifyItem(it)) + th.Val.Render(it.Title)
	if it.Subtitle != "" {
		head += "  " + th.Dim.Render(it.Subtitle)
	}
	if author := it.Meta["author"]; author != "" {
		head += "  " + th.Dim.Render("@"+author)
	}
	at, hasAge := lastCommentTime(it)
	tail := commentChip(t.sevTheme, it, at, hasAge)
	if !it.Timestamp.IsZero() && (tail == "" || !hasAge) {
		if tail != "" {
			tail += "  "
		}
		tail += th.Dim.Render(timefmt.Rel(it.Timestamp))
	}
	lines := make([]string, 0, 2)
	if tail != "" {
		lines = append(lines, f.Spread(head, tail))
	} else {
		lines = append(lines, head)
	}
	if it.URL != "" {
		lines = append(lines, th.Dim.Render(it.URL))
	}
	return lines
}
