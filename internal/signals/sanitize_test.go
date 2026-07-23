package signals

import "testing"

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
