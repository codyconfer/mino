package render

import (
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/panels"
	"github.com/codyconfer/viewkit/theme"
	"github.com/codyconfer/viewkit/timefmt"

	"github.com/codyconfer/mino/internal/render/glyph"
	"github.com/codyconfer/mino/internal/signals"
)

type ItemRef struct {
	Signal string
	Item   signals.Item
	Meta   map[string]string
}

func ItemLabel(it signals.Item) string {
	kind := signals.CleanLine(it.Kind)
	if kind == "" {
		kind = "detail"
	}
	if n := urlTailNumber(it.URL); n != "" {
		return kind + " #" + n
	}
	return kind
}

func ItemScope(it signals.Item) string {
	sub := signals.CleanLine(it.Subtitle)
	if sub == "" {
		return ""
	}
	return strings.TrimSpace(strings.Split(sub, "·")[0])
}

func urlTailNumber(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for _, seg := range slices.Backward(parts) {
		if _, err := strconv.Atoi(seg); err == nil {
			return seg
		}
	}
	return ""
}

var detailMetaRows = []struct {
	key   string
	label string
}{
	{"author", "author"},
	{"state", "state"},
	{"status", "status"},
	{"labels", "labels"},
	{"assignees", "assignees"},
	{"last_comment_by", "last reply"},
}

func contentFrame(f layout.Frame) layout.Frame {
	return f.WithWidth(f.BodyWidth() - 4)
}

func DetailPanel(f layout.Frame, ref ItemRef, d *signals.ItemDetail) string {
	return detailPanel(f, ref, d, -1)
}

func DetailPanelFrame(f layout.Frame, ref ItemRef, d *signals.ItemDetail, frame int) string {
	return detailPanel(f, ref, d, frame)
}

func DetailAnimates(ref ItemRef, d *signals.ItemDetail) bool {
	return ItemInProgress(ref.Item) || DetailHasInProgress(d)
}

func DetailHasInProgress(d *signals.ItemDetail) bool {
	if d == nil {
		return false
	}
	if d.Meta["in_progress"] == "true" {
		return true
	}
	for _, section := range d.Sections {
		if section.Meta["in_progress"] == "true" {
			return true
		}
	}
	return false
}

func detailPanel(f layout.Frame, ref ItemRef, d *signals.ItemDetail, frame int) string {
	ref, d = cleanRef(ref), signals.CleanDetail(d)

	th := f.Theme()
	cf := contentFrame(f)
	it := ref.Item

	kind := it.Kind
	title := it.Title
	if d != nil {
		if d.Kind != "" {
			kind = d.Kind
		}
		if d.Title != "" {
			title = d.Title
		}
	}

	head := kind
	if chips := detailChips(th, d); chips != "" {
		head += " · " + chips
	}
	head += staleCue(th, ref.Meta)

	lines := []string{th.Val.Render(title)}
	if sub := it.Subtitle; sub != "" {
		lines = append(lines, th.Dim.Render(sub))
	}
	if rows := detailRows(cf, th, ref, d); len(rows) > 0 {
		lines = append(lines, "")
		lines = append(lines, rows...)
	}
	body := detailBody(d, it)
	if body != "" {
		lines = append(lines, cf.Rule())
		lines = append(lines, layout.Lines(panels.Markdown(cf, body))...)
	}

	sev := detailSeverity(it, d)
	icon := th.SeverityStyle(sev).Render(glyph.Lead(glyph.ForIn(f.Glyphs(), sev)))
	if frame >= 0 && workflowInProgress(it) {
		icon = th.SeverityStyle(sev).Render(glyph.Lead(spinnerFrame(f.Glyphs(), frame)))
	}
	out := []string{f.TitledBoxIcon(icon, head, lines...)}
	if d != nil {
		for _, s := range d.Sections {
			out = append(out, detailSection(f, cf, th, s, frame))
		}
	}
	return strings.Join(out, "\n")
}

func detailSeverity(it signals.Item, d *signals.ItemDetail) glyph.Kind {
	if d != nil && len(d.Chips) > 0 {
		return d.Chips[0].Sev
	}
	return glyph.ClassifyItem(it)
}

func detailBody(d *signals.ItemDetail, it signals.Item) string {
	if d != nil && strings.TrimSpace(d.Body) != "" {
		return d.Body
	}
	return strings.TrimSpace(it.Body)
}

func detailChips(th theme.Theme, d *signals.ItemDetail) string {
	if d == nil || len(d.Chips) == 0 {
		return ""
	}
	parts := make([]string, 0, len(d.Chips))
	for _, c := range d.Chips {
		parts = append(parts, th.SeverityStyle(c.Sev).Render(c.Label))
	}
	return strings.Join(parts, th.Dim.Render(" · "))
}

func detailRows(f layout.Frame, th theme.Theme, ref ItemRef, d *signals.ItemDetail) []string {
	rows := localRows(ref)
	if d != nil && len(d.Rows) > 0 {
		rows = d.Rows
	}
	if !ref.Item.Timestamp.IsZero() {
		rows = append(rows, [2]string{"updated", timefmt.Rel(ref.Item.Timestamp)})
	}
	return gutter(f, th, rows)
}

func localRows(ref ItemRef) [][2]string {
	var rows [][2]string
	for _, r := range detailMetaRows {
		if v := ref.Item.Meta[r.key]; v != "" {
			rows = append(rows, [2]string{r.label, v})
		}
	}
	if ref.Item.Meta["draft"] == "true" {
		rows = append(rows, [2]string{"draft", "yes"})
	}
	if n := strings.TrimSpace(ref.Item.Meta[signals.MetaFiled]); n != "" && n != "0" {
		rows = append(rows, [2]string{"notes", n})
	}
	return rows
}

func gutter(f layout.Frame, th theme.Theme, rows [][2]string) []string {
	if len(rows) == 0 {
		return nil
	}
	cap := max(f.BodyWidth()/2, 8)
	width := 0
	for _, r := range rows {
		if n := len([]rune(r[0])); n > width {
			width = min(n, cap)
		}
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		if r[0] == "" && r[1] == "" {
			out = append(out, "")
			continue
		}
		label := layout.Fit(r[0], width)
		if pad := width - len([]rune(label)); pad > 0 {
			label += strings.Repeat(" ", pad)
		}
		out = append(out, th.Dim.Render(label)+"  "+layout.Fit(r[1], f.BodyWidth()-width-2))
	}
	return out
}

func detailSection(f, cf layout.Frame, th theme.Theme, s signals.DetailSection, frame int) string {
	var lines []string
	if len(s.Rows) > 0 {
		rows := s.Rows
		if workflowSection(s) {
			rows = workflowRows(cf.Glyphs(), th, rows, frame)
		}
		lines = append(lines, gutter(cf, th, rows)...)
	}
	for _, l := range s.Lines {
		lines = append(lines, th.Dim.Render(l))
	}
	if strings.TrimSpace(s.Body) != "" {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, layout.Lines(panels.Markdown(cf, s.Body))...)
	}
	return f.Panel(s.Title, lines...)
}

var detailLoadingFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

var asciiLoadingFrames = []string{"|", "/", "-", "\\"}

func spinnerFrame(g glyph.Set, frame int) string {
	frames := detailLoadingFrames
	if g.Mode() == glyph.ModeNone {
		frames = asciiLoadingFrames
	}
	if frame < 0 {
		frame = 0
	}
	return frames[frame%len(frames)]
}

func workflowSection(s signals.DetailSection) bool {
	return s.Meta["run_id"] != "" || s.Meta["state_rows"] == "true" || strings.HasPrefix(s.Title, "workflow")
}

const stepRowPrefix = "  ↳ "

func workflowRows(g glyph.Set, th theme.Theme, rows [][2]string, frame int) [][2]string {
	out := make([][2]string, 0, len(rows)*2)
	for _, r := range rows {
		if len(out) > 0 && !strings.HasPrefix(r[0], stepRowPrefix) {
			out = append(out, [2]string{})
		}
		out = append(out, [2]string{r[0], workflowStateCue(g, th, r[1], frame)})
	}
	return out
}

func workflowStateCue(g glyph.Set, th theme.Theme, state string, frame int) string {
	if strings.TrimSpace(state) == "" {
		return state
	}
	if strings.EqualFold(strings.TrimSpace(state), "pending") {
		return th.Dim.Render(state)
	}
	sev := signals.ClassifyState(state)
	mark := glyph.ForIn(g, sev)
	if strings.EqualFold(strings.TrimSpace(state), "in progress") {
		mark = "…"
		if frame >= 0 {
			mark = spinnerFrame(g, frame)
		}
	}
	if mark == "" {
		return th.SeverityStyle(sev).Render(state)
	}
	return th.SeverityStyle(sev).Render(mark + " " + state)
}
