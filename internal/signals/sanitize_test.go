package signals

import (
	"maps"
	"strings"
	"testing"
)

func TestCleanStripsControlAndEscapes(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "hello world", "hello world"},
		{"esc-osc", "title\x1b]0;pwned\x07end", "title]0;pwnedend"},
		{"clear-screen", "a\x1b[2Jb", "a[2Jb"},
		{"carriage-return", "over\rwrite", "overwrite"},
		{"del-and-c1", "x\x7f\x9by", "xy"},
		{"keeps-tab-newline", "a\tb\nc", "a\tb\nc"},
		{"keeps-unicode", "café ☕ 日本", "café ☕ 日本"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Clean(tc.in); got != tc.want {
				t.Fatalf("Clean(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestCleanLineCollapsesEveryLineBreak(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "hello world", "hello world"},
		{"empty", "", ""},
		{"newline", "open\nFAKE", "open FAKE"},
		{"crlf", "open\r\nFAKE", "open FAKE"},
		{"carriage-return", "over\rwrite", "over write"},
		{"tab", "unit\tcolumn", "unit column"},
		{"blank-line", "a\n\nb", "a  b"},
		{"esc-osc", "title\x1b]0;pwned\x07end", "title]0;pwnedend"},
		{"del-and-c1", "x\x7f\u009b\u0085y", "xy"},
		{"keeps-unicode", "café ☕ 日本", "café ☕ 日本"},
		{"mixed", "a\x1b[2J\nb\tc\r\nd", "a[2J b c d"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CleanLine(tc.in)
			if got != tc.want {
				t.Fatalf("CleanLine(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if strings.ContainsAny(got, "\n\r\t") {
				t.Fatalf("CleanLine(%q) kept a line break: %q", tc.in, got)
			}
		})
	}
}

func TestCleanStripsBidiOverridesButKeepsMarksAndJoiners(t *testing.T) {
	stripped := map[string]string{
		"rlo": "safe\u202edetixe.exe",
		"lro": "safe\u202dtext",
		"rle": "safe\u202btext",
		"lre": "safe\u202atext",
		"pdf": "safe\u202ctext",
		"rli": "safe\u2067text",
		"lri": "safe\u2066text",
		"fsi": "safe\u2068text",
		"pdi": "safe\u2069text",
	}
	for name, in := range stripped {
		for _, fn := range []struct {
			label string
			clean func(string) string
		}{{"Clean", Clean}, {"CleanLine", CleanLine}} {
			got := fn.clean(in)
			if strings.ContainsAny(got, "\u202a\u202b\u202c\u202d\u202e\u2066\u2067\u2068\u2069") {
				t.Errorf("%s kept the %s override: %q", fn.label, name, got)
			}
			if !strings.HasPrefix(got, "safe") {
				t.Errorf("%s(%s) dropped visible text: %q", fn.label, name, got)
			}
		}
	}

	keep := map[string]string{
		"rlm":         "safe\u200fالعربية",
		"lrm":         "safe\u200etext",
		"zwj emoji":   "family \U0001f469\u200d\U0001f467",
		"zwnj":        "safe\u200ctext",
		"zwsp":        "safe\u200btext",
		"soft hyphen": "safe\u00adtext",
		"rtl text":    "مرحبا بالعالم",
		"replacement": "mojibake � here",
	}
	for name, in := range keep {
		if got := Clean(in); got != in {
			t.Errorf("Clean stripped legitimate %s: %q -> %q", name, in, got)
		}
		if got := CleanLine(in); got != in {
			t.Errorf("CleanLine stripped legitimate %s: %q -> %q", name, in, got)
		}
	}
}

func TestCleanDropsInvalidUTF8Bytes(t *testing.T) {
	in := "before\xffafter"
	for _, fn := range []struct {
		label string
		clean func(string) string
	}{{"Clean", Clean}, {"CleanLine", CleanLine}} {
		got := fn.clean(in)
		if got != "beforeafter" {
			t.Errorf("%s(%q) = %q, want %q", fn.label, in, got, "beforeafter")
		}
	}
}

func TestCleanMetaKeyCollisionIsFirstWriterWins(t *testing.T) {
	in := map[string]string{
		"author":        "benign",
		"author\x1b":    "ATTACKER",
		"author\x1b[2J": "ALSO ATTACKER",
		"state":         "open",
	}
	for range 50 {
		got := CleanMeta(maps.Clone(in))
		if got["author"] != "benign" {
			t.Fatalf("author = %q, want the benign value", got["author"])
		}
		if got["state"] != "open" {
			t.Fatalf("state = %q, want open", got["state"])
		}
		if len(got) != 3 {
			t.Fatalf("CleanMeta = %v, want author/state plus one cleaned key", got)
		}
		if got["author[2J"] != "ALSO ATTACKER" {
			t.Fatalf("cleaned key lost its value: %v", got)
		}
	}
}

func TestCleanMetaDropsKeysThatCleanToNothing(t *testing.T) {
	got := CleanMeta(map[string]string{"\x1b": "hidden", "ok": "yes"})
	if _, present := got[""]; present {
		t.Errorf("empty key survived: %v", got)
	}
	if got["ok"] != "yes" {
		t.Errorf("benign entry lost: %v", got)
	}
}

func TestCleanMetaKeepsBenignMapIdentity(t *testing.T) {
	in := map[string]string{"author": "cody", "age": "5m"}
	if got := CleanMeta(in); len(got) != len(in) || got["author"] != "cody" {
		t.Errorf("CleanMeta rewrote a benign map: %v", got)
	}
}

func TestCleanItemUsesLineRuleExceptForBody(t *testing.T) {
	in := Item{
		Kind:     "pr\nFAKE",
		Title:    "title\nFAKE",
		Subtitle: "sub\nFAKE",
		URL:      "https://x/1\nFAKE",
		Body:     "line one\nline two\twith tab",
		Meta:     map[string]string{"author\n": "ada\nFAKE"},
	}
	got := CleanItem(in)
	for name, v := range map[string]string{
		"Kind": got.Kind, "Title": got.Title, "Subtitle": got.Subtitle, "URL": got.URL,
		"Meta[author]": got.Meta["author"],
	} {
		if strings.ContainsAny(v, "\n\r\t") {
			t.Errorf("%s kept a line break: %q", name, v)
		}
	}
	if got.Body != in.Body {
		t.Errorf("Body = %q, want its newlines and tabs preserved", got.Body)
	}
	if in.Kind != "pr\nFAKE" || in.Meta["author\n"] != "ada\nFAKE" {
		t.Error("CleanItem mutated its input")
	}
}

func TestCleanSectionAndDetailDoNotMutateInput(t *testing.T) {
	sec := Section{
		Signal: "github\nFAKE",
		Title:  "Open PRs\nFAKE",
		Meta:   map[string]string{"age": "5m\nFAKE"},
		Items:  []Item{{Title: "title\nFAKE"}},
	}
	got := CleanSection(sec)
	if strings.Contains(got.Signal, "\n") || strings.Contains(got.Title, "\n") ||
		strings.Contains(got.Meta["age"], "\n") || strings.Contains(got.Items[0].Title, "\n") {
		t.Errorf("CleanSection left a newline: %+v", got)
	}
	if sec.Items[0].Title != "title\nFAKE" || sec.Meta["age"] != "5m\nFAKE" {
		t.Error("CleanSection mutated its input")
	}

	d := &ItemDetail{
		Kind:     "pr\nFAKE",
		Title:    "title\nFAKE",
		URL:      "https://x/1\nFAKE",
		Body:     "keep\nthis",
		Chips:    []Chip{{Label: "open\nFAKE"}},
		Rows:     [][2]string{{"repo\nFAKE", "acme\nFAKE"}},
		Sections: []DetailSection{{Title: "checks\nFAKE", Lines: []string{"one.go\nFAKE"}, Rows: [][2]string{{"lint\nFAKE", "fail\nFAKE"}}, Body: "keep\nthis", Meta: map[string]string{"k": "v\nFAKE"}}},
	}
	c := CleanDetail(d)
	for name, v := range map[string]string{
		"Kind": c.Kind, "Title": c.Title, "URL": c.URL, "Chip": c.Chips[0].Label,
		"Row key": c.Rows[0][0], "Row value": c.Rows[0][1],
		"Section title": c.Sections[0].Title, "Section line": c.Sections[0].Lines[0],
		"Section row": c.Sections[0].Rows[0][1], "Section meta": c.Sections[0].Meta["k"],
	} {
		if strings.ContainsAny(v, "\n\r\t") {
			t.Errorf("%s kept a line break: %q", name, v)
		}
	}
	if c.Body != "keep\nthis" || c.Sections[0].Body != "keep\nthis" {
		t.Errorf("bodies lost their newlines: %q / %q", c.Body, c.Sections[0].Body)
	}
	if d.Chips[0].Label != "open\nFAKE" || d.Sections[0].Lines[0] != "one.go\nFAKE" || d.Rows[0][0] != "repo\nFAKE" {
		t.Error("CleanDetail mutated its input")
	}
	if CleanDetail(nil) != nil {
		t.Error("CleanDetail(nil) must stay nil")
	}
}
