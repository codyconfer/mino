package format

import (
	"bytes"
	"reflect"
	"sort"
	"strings"
	"text/template"
	"time"
	"unicode"

	"github.com/codyconfer/viewkit/timefmt"

	"github.com/codyconfer/munin/internal/errs"
)

type Bucket struct {
	Key   string
	Items []Item
}

func Parse(name, src string) (*template.Template, error) {
	tmpl, err := template.New(name).Option("missingkey=zero").Funcs(FuncMap(time.Now)).Parse(src)
	if err != nil {
		return nil, errs.Wrapf(errs.KindConfig, err, "parsing formatter %q", name)
	}
	return tmpl, nil
}

func Render(name, src string, r Report) (string, error) {
	tmpl, err := Parse(name, src)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, r); err != nil {
		return "", errs.Wrapf(errs.KindConfig, err, "executing formatter %q", name)
	}
	return buf.String(), nil
}

func FuncMap(now func() time.Time) template.FuncMap {
	if now == nil {
		now = time.Now
	}
	return template.FuncMap{
		"now": now,
		"date": func(layout string, t time.Time) string {
			return t.Format(layout)
		},
		"rel": func(t time.Time) string {
			if t.IsZero() {
				return ""
			}
			return timefmt.RelAt(t, now())
		},
		"meta": func(key string, m map[string]string) string {
			return m[key]
		},
		"default": func(fallback, v string) string {
			if v == "" {
				return fallback
			}
			return v
		},
		"trim":     strings.TrimSpace,
		"upper":    strings.ToUpper,
		"lower":    strings.ToLower,
		"title":    titleCase,
		"join":     func(sep string, xs []string) string { return strings.Join(xs, sep) },
		"indent":   indent,
		"truncate": truncate,
		"count":    count,
		"limit": func(n int, items []Item) []Item {
			if n < 0 || n > len(items) {
				return items
			}
			return items[:n]
		},
		"byMeta":     byMeta,
		"withMeta":   withMeta,
		"sortByTime": sortByTime,
	}
}

func titleCase(s string) string {
	prev := ' '
	return strings.Map(func(r rune) rune {
		out := r
		if !unicode.IsLetter(prev) && !unicode.IsNumber(prev) {
			out = unicode.ToUpper(r)
		}
		prev = r
		return out
	}, s)
}

func indent(n int, s string) string {
	if n <= 0 {
		return s
	}
	pad := strings.Repeat(" ", n)
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = pad + l
	}
	return strings.Join(lines, "\n")
}

func truncate(n int, s string) string {
	if n < 0 {
		return s
	}
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	if n == 0 {
		return "…"
	}
	return string(rs[:n]) + "…"
}

func count(v any) int {
	if v == nil {
		return 0
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array, reflect.Map, reflect.String, reflect.Chan:
		return rv.Len()
	case reflect.Pointer, reflect.Interface:
		if rv.IsNil() {
			return 0
		}
		return count(rv.Elem().Interface())
	default:
		return 0
	}
}

func byMeta(key string, items []Item) []Bucket {
	seen := make(map[string]int, len(items))
	buckets := make([]Bucket, 0, len(items))
	for _, it := range items {
		k := it.Meta[key]
		idx, ok := seen[k]
		if !ok {
			idx = len(buckets)
			seen[k] = idx
			buckets = append(buckets, Bucket{Key: k})
		}
		buckets[idx].Items = append(buckets[idx].Items, it)
	}
	sort.Slice(buckets, func(i, j int) bool { return buckets[i].Key < buckets[j].Key })
	return buckets
}

func withMeta(key, val string, items []Item) []Item {
	out := make([]Item, 0, len(items))
	for _, it := range items {
		if it.Meta[key] == val {
			out = append(out, it)
		}
	}
	return out
}

func sortByTime(items []Item) []Item {
	out := make([]Item, len(items))
	copy(out, items)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Timestamp.After(out[j].Timestamp) })
	return out
}
