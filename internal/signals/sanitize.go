package signals

import (
	"maps"
	"slices"
	"strings"
	"unicode/utf8"
)

const replacementRune = "\ufffd"

func safeControl(r rune) bool {
	return r == '\n' || r == '\t'
}

func bidiOverride(r rune) bool {
	return (r >= 0x202a && r <= 0x202e) || (r >= 0x2066 && r <= 0x2069)
}

func dangerousControl(r rune) bool {
	if safeControl(r) {
		return false
	}
	if bidiOverride(r) {
		return true
	}
	return r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f)
}

func breakControl(r rune) bool {
	return r == '\n' || r == '\r' || r == '\t'
}

func invalidByte(s string, i int, r rune) bool {
	return r == utf8.RuneError && !strings.HasPrefix(s[i:], replacementRune)
}

func needsClean(s string, collapse bool) bool {
	for i, r := range s {
		switch {
		case collapse && breakControl(r):
			return true
		case invalidByte(s, i, r):
			return true
		case r != utf8.RuneError && dangerousControl(r):
			return true
		}
	}
	return false
}

func scrub(s string, collapse bool) string {
	var b strings.Builder
	b.Grow(len(s))
	cr := false
	for i, r := range s {
		switch {
		case collapse && r == '\n':
			if !cr {
				b.WriteByte(' ')
			}
		case collapse && (r == '\r' || r == '\t'):
			b.WriteByte(' ')
		case invalidByte(s, i, r):
		case r != utf8.RuneError && dangerousControl(r):
		default:
			b.WriteRune(r)
		}
		cr = r == '\r'
	}
	return b.String()
}

func Clean(s string) string {
	if s == "" || !needsClean(s, false) {
		return s
	}
	return scrub(s, false)
}

func CleanLine(s string) string {
	if s == "" || !needsClean(s, true) {
		return s
	}
	return scrub(s, true)
}

func CleanMeta(m map[string]string) map[string]string {
	if len(m) == 0 {
		return m
	}
	dirty := false
	for k, v := range m {
		if CleanLine(k) != k || CleanLine(v) != v {
			dirty = true
			break
		}
	}
	if !dirty {
		return m
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		if CleanLine(k) == k {
			out[k] = CleanLine(v)
		}
	}
	for _, k := range slices.Sorted(maps.Keys(m)) {
		ck := CleanLine(k)
		if ck == k || ck == "" {
			continue
		}
		if _, taken := out[ck]; taken {
			continue
		}
		out[ck] = CleanLine(m[k])
	}
	return out
}

func CleanLines(lines []string) []string {
	out := lines
	copied := false
	for i, l := range lines {
		cl := CleanLine(l)
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

func CleanRows(rows [][2]string) [][2]string {
	out := rows
	copied := false
	for i, r := range rows {
		ck, cv := CleanLine(r[0]), CleanLine(r[1])
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

func CleanChips(chips []Chip) []Chip {
	out := chips
	copied := false
	for i, c := range chips {
		cl := CleanLine(c.Label)
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

func CleanItem(it Item) Item {
	it.Kind = CleanLine(it.Kind)
	it.Title = CleanLine(it.Title)
	it.Subtitle = CleanLine(it.Subtitle)
	it.URL = CleanLine(it.URL)
	it.Body = Clean(it.Body)
	it.Meta = CleanMeta(it.Meta)
	return it
}

func CleanItems(items []Item) []Item {
	if len(items) == 0 {
		return items
	}
	out := slices.Clone(items)
	for i := range out {
		out[i] = CleanItem(out[i])
	}
	return out
}

func CleanSection(s Section) Section {
	s.Signal = CleanLine(s.Signal)
	s.Title = CleanLine(s.Title)
	s.Meta = CleanMeta(s.Meta)
	s.Items = CleanItems(s.Items)
	return s
}

func CleanDetailSections(secs []DetailSection) []DetailSection {
	if len(secs) == 0 {
		return secs
	}
	out := slices.Clone(secs)
	for i, s := range out {
		out[i].Title = CleanLine(s.Title)
		out[i].Icon = CleanLine(s.Icon)
		out[i].Body = Clean(s.Body)
		out[i].Rows = CleanRows(s.Rows)
		out[i].Lines = CleanLines(s.Lines)
		out[i].Meta = CleanMeta(s.Meta)
	}
	return out
}

func CleanDetail(d *ItemDetail) *ItemDetail {
	if d == nil {
		return nil
	}
	c := *d
	c.Kind = CleanLine(c.Kind)
	c.Title = CleanLine(c.Title)
	c.URL = CleanLine(c.URL)
	c.Body = Clean(c.Body)
	c.Chips = CleanChips(c.Chips)
	c.Rows = CleanRows(c.Rows)
	c.Sections = CleanDetailSections(c.Sections)
	c.Meta = CleanMeta(c.Meta)
	return &c
}
