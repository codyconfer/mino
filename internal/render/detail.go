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

	"github.com/codyconfer/munin/internal/render/glyph"
	"github.com/codyconfer/munin/internal/signals"
)

type ItemRef struct {
	Signal string
	Item   signals.Item
	Meta   map[string]string
}

func ItemIndex(sections []signals.Section) map[string]ItemRef {
	index := map[string]ItemRef{}
	for _, s := range sections {
		for _, it := range s.Items {
			if it.URL == "" {
				continue
			}
			if _, seen := index[it.URL]; seen {
				continue
			}
			index[it.URL] = ItemRef{Signal: s.Signal, Item: it, Meta: s.Meta}
		}
	}
	return index
}

func ItemLabel(it signals.Item) string {
	kind := signals.Clean(it.Kind)
	if kind == "" {
		kind = "detail"
	}
	if n := urlTailNumber(it.URL); n != "" {
		return kind + " #" + n
	}
	return kind
}

func ItemScope(it signals.Item) string {
	sub := signals.Clean(it.Subtitle)
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
	return layout.NewFrame(f.BodyWidth() - 4)
}

func DetailPanel(f layout.Frame, ref ItemRef, d *signals.ItemDetail) string {
	ref, d = cleanRef(ref), cleanDetail(d)

	th := theme.Cur()
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
	icon := theme.SeverityStyle(sev).Render(glyph.Lead(glyph.For(sev)))
	out := []string{f.TitledBoxIcon(icon, head, lines...)}
	if d != nil {
		for _, s := range d.Sections {
			out = append(out, detailSection(f, cf, th, s))
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

func detailChips(th *theme.Theme, d *signals.ItemDetail) string {
	if d == nil || len(d.Chips) == 0 {
		return ""
	}
	parts := make([]string, 0, len(d.Chips))
	for _, c := range d.Chips {
		parts = append(parts, theme.SeverityStyle(c.Sev).Render(c.Label))
	}
	return strings.Join(parts, th.Dim.Render(" · "))
}

func detailRows(f layout.Frame, th *theme.Theme, ref ItemRef, d *signals.ItemDetail) []string {
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
	return rows
}

func gutter(f layout.Frame, th *theme.Theme, rows [][2]string) []string {
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
		label := layout.Fit(r[0], width)
		if pad := width - len([]rune(label)); pad > 0 {
			label += strings.Repeat(" ", pad)
		}
		out = append(out, th.Dim.Render(label)+"  "+layout.Fit(r[1], f.BodyWidth()-width-2))
	}
	return out
}

func detailSection(f, cf layout.Frame, th *theme.Theme, s signals.DetailSection) string {
	var lines []string
	if len(s.Rows) > 0 {
		lines = append(lines, gutter(cf, th, s.Rows)...)
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
