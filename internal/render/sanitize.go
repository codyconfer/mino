package render

import (
	"maps"
	"slices"

	"github.com/codyconfer/munin/internal/signals"
)

func cleanMeta(m map[string]string) map[string]string {
	out := m
	copied := false
	for k, v := range m {
		ck, cv := signals.Clean(k), signals.Clean(v)
		if ck == k && cv == v {
			continue
		}
		if !copied {
			out, copied = maps.Clone(m), true
		}
		if ck != k {
			delete(out, k)
		}
		out[ck] = cv
	}
	return out
}

func cleanLines(lines []string) []string {
	out := lines
	copied := false
	for i, l := range lines {
		cl := signals.Clean(l)
		if cl == l {
			continue
		}
		if !copied {
			out, copied = slices.Clone(lines), true
		}
		out[i] = cl
	}
	return out
}

func cleanRows(rows [][2]string) [][2]string {
	out := rows
	copied := false
	for i, r := range rows {
		ck, cv := signals.Clean(r[0]), signals.Clean(r[1])
		if ck == r[0] && cv == r[1] {
			continue
		}
		if !copied {
			out, copied = slices.Clone(rows), true
		}
		out[i] = [2]string{ck, cv}
	}
	return out
}

func cleanChips(chips []signals.Chip) []signals.Chip {
	out := chips
	copied := false
	for i, c := range chips {
		cl := signals.Clean(c.Label)
		if cl == c.Label {
			continue
		}
		if !copied {
			out, copied = slices.Clone(chips), true
		}
		out[i].Label = cl
	}
	return out
}

func cleanItem(it signals.Item) signals.Item {
	it.Kind = signals.Clean(it.Kind)
	it.Title = signals.Clean(it.Title)
	it.Subtitle = signals.Clean(it.Subtitle)
	it.Body = signals.Clean(it.Body)
	it.URL = signals.Clean(it.URL)
	it.Meta = cleanMeta(it.Meta)
	return it
}

func cleanRef(ref ItemRef) ItemRef {
	ref.Signal = signals.Clean(ref.Signal)
	ref.Item = cleanItem(ref.Item)
	ref.Meta = cleanMeta(ref.Meta)
	return ref
}

func cleanDetailSections(secs []signals.DetailSection) []signals.DetailSection {
	if len(secs) == 0 {
		return secs
	}
	out := slices.Clone(secs)
	for i, s := range out {
		out[i].Title = signals.Clean(s.Title)
		out[i].Icon = signals.Clean(s.Icon)
		out[i].Body = signals.Clean(s.Body)
		out[i].Rows = cleanRows(s.Rows)
		out[i].Lines = cleanLines(s.Lines)
		out[i].Meta = cleanMeta(s.Meta)
	}
	return out
}

func cleanDetail(d *signals.ItemDetail) *signals.ItemDetail {
	if d == nil {
		return nil
	}
	c := *d
	c.Kind = signals.Clean(c.Kind)
	c.Title = signals.Clean(c.Title)
	c.URL = signals.Clean(c.URL)
	c.Body = signals.Clean(c.Body)
	c.Chips = cleanChips(c.Chips)
	c.Rows = cleanRows(c.Rows)
	c.Sections = cleanDetailSections(c.Sections)
	return &c
}
