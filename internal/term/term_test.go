package term

import "testing"

func TestShQuote(t *testing.T) {
	cases := map[string]string{
		"abc":            "abc",
		"/tmp/munin.bin": "/tmp/munin.bin",
		"a b":            "'a b'",
		"it's":           `'it'\''s'`,
		"":               "''",
	}
	for in, want := range cases {
		if got := shQuote(in); got != want {
			t.Errorf("shQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAppleEscape(t *testing.T) {
	if got := appleEscape(`say "hi"\n`); got != `say \"hi\"\\n` {
		t.Errorf("appleEscape = %q", got)
	}
}
