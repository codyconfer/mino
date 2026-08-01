package filter

import (
	"math/rand"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/codyconfer/mino/internal/signals"
)

// advAlphabet holds every regexp metacharacter plus escapes, invalid UTF-8
// bytes, multi-byte runes and U+FFFD, so generated patterns/inputs cover the
// whole space the detection has to survive.
var advAlphabet = []string{
	`a`, `b`, `d`, `A`, `z`, `Z`, `p`, `Q`, `E`, `x`, `s`, `w`, `n`, `1`,
	`\`, `.`, `*`, `+`, `?`, `(`, `)`, `|`, `[`, `]`, `{`, `}`, `^`, `$`,
	`-`, `:`, `,`, `!`, `#`, `&`, `~`, `/`, `%`, `=`, `<`, `>`, `'`, `"`,
	" ", "\t", "\n", "\r", "\x00", "\x7f",
	"é", "日", "É", "K", "ſ", "�",
	"\xff", "\x80", "\xc3", "\xc0\xaf", "\xed\xa0\x80", "\xe4\xb8",
}

// advStrings enumerates every string of length 1..n over advAlphabet.
func advStrings(n int) []string {
	cur := []string{""}
	var out []string
	for range n {
		var next []string
		for _, p := range cur {
			for _, a := range advAlphabet {
				s := p + a
				next = append(next, s)
				out = append(out, s)
			}
		}
		cur = next
	}
	return out
}

// checkAgreement asserts strings.Contains(in, lit) == re.MatchString(in) for
// every pattern the detection accepted.
func checkAgreement(t *testing.T, pat string, re *regexp.Regexp, inputs []string) {
	t.Helper()
	lit := literalOf(pat)
	if lit == "" {
		return
	}
	if lit != pat {
		t.Fatalf("literalOf(%q) = %q, must return the pattern verbatim", pat, lit)
	}
	// Independent oracle: the regexp engine must itself consider the whole
	// pattern a literal. Guards against QuoteMeta's escape set ever drifting
	// away from regexp/syntax's metacharacter set.
	if prefix, complete := re.LiteralPrefix(); !complete || prefix != pat {
		t.Fatalf("literalOf(%q) fast-pathed but regexp says prefix=%q complete=%v", pat, prefix, complete)
	}
	for _, in := range inputs {
		if got, want := strings.Contains(in, lit), re.MatchString(in); got != want {
			t.Fatalf("pattern=%q input=%q: Contains=%v MatchString=%v", pat, in, got, want)
		}
	}
}

// curatedInputs are the hand-picked inputs every pattern is checked against.
var curatedInputs = []string{"", "abc", "a.c", "ABC", "aaa", "caf\xe9",
	"a\xffb", "\xef\xbf\xbd", "x�y", "^abc$", `a\b`, "incident", "Incident"}

// TestAdversarialLiteralAgreement is an exhaustive differential over every
// pattern of length 1..3 drawn from an alphabet of metacharacters, escapes and
// invalid UTF-8. Patterns of length 1..2 are crossed with every input of length
// 1..2; the 60x larger length-3 pattern space is crossed with inputs of length
// 1 plus the curated set, since the full cross product is ~224M comparisons and
// dominates the whole race-enabled suite for no extra signal.
func TestAdversarialLiteralAgreement(t *testing.T) {
	shallow := advStrings(2)
	pats := advStrings(3)
	// advStrings emits length 1, then 2, then 3, so the tail is exactly len 3.
	if len(pats) != len(shallow)+len(advAlphabet)*len(advAlphabet)*len(advAlphabet) {
		t.Fatalf("advStrings layout changed: len(3)=%d len(2)=%d", len(pats), len(shallow))
	}
	deep := pats[len(shallow):]
	fullInputs := append(advStrings(2), curatedInputs...)
	sampledInputs := append(advStrings(1), curatedInputs...)

	var fast int
	check := func(pats, inputs []string) {
		for _, pat := range pats {
			re, err := regexp.Compile(pat)
			if err != nil {
				continue // Compile() rejects these before any fast path is set.
			}
			if literalOf(pat) == "" {
				continue
			}
			fast++
			checkAgreement(t, pat, re, inputs)
		}
	}
	check(shallow, fullInputs)
	check(deep, sampledInputs)

	t.Logf("patterns=%d fast-pathed=%d inputs=%d/%d", len(pats), fast, len(fullInputs), len(sampledInputs))
	if fast == 0 {
		t.Fatal("no pattern reached the fast path; the generator is degenerate")
	}
}

// TestAdversarialRandomBytes throws random raw byte strings at both paths,
// including patterns and inputs that are not valid UTF-8.
func TestAdversarialRandomBytes(t *testing.T) {
	rng := rand.New(rand.NewSource(20260801))
	randStr := func(maxLen int) string {
		var b strings.Builder
		for range rng.Intn(maxLen) + 1 {
			switch rng.Intn(3) {
			case 0:
				b.WriteByte(byte(rng.Intn(256)))
			case 1:
				b.WriteString(advAlphabet[rng.Intn(len(advAlphabet))])
			default:
				b.WriteString("abc incident deploy"[rng.Intn(19):][:1])
			}
		}
		return b.String()
	}
	checked := 0
	for range 200000 {
		pat := randStr(6)
		if pat == "" {
			continue
		}
		re, err := regexp.Compile(pat)
		if err != nil || literalOf(pat) == "" {
			continue
		}
		for range 4 {
			in := randStr(12)
			if got, want := strings.Contains(in, pat), re.MatchString(in); got != want {
				t.Fatalf("pattern=%q (% x) input=%q (% x): Contains=%v MatchString=%v",
					pat, pat, in, in, got, want)
			}
			checked++
		}
	}
	t.Logf("random pairs checked on the fast path: %d", checked)
	if checked < 1000 {
		t.Fatalf("only %d random pairs reached the fast path", checked)
	}
}

// TestLiteralOfAgreesWithRegexpLiteralPrefix cross-checks the detection against
// the regexp engine's own literal analysis: anything we fast path must be a
// complete literal to regexp itself.
func TestLiteralOfAgreesWithRegexpLiteralPrefix(t *testing.T) {
	var missed []string
	for _, pat := range advStrings(2) {
		re, err := regexp.Compile(pat)
		if err != nil {
			continue
		}
		prefix, complete := re.LiteralPrefix()
		if literalOf(pat) != "" && (!complete || prefix != pat) {
			t.Errorf("literalOf(%q) fast-pathed but regexp says prefix=%q complete=%v", pat, prefix, complete)
		}
		// Reverse direction: complete literals we decline are perf misses only,
		// and are legitimate when the pattern carries U+FFFD or invalid bytes.
		if literalOf(pat) == "" && complete && prefix == pat && utf8.ValidString(pat) &&
			!strings.ContainsRune(pat, utf8.RuneError) {
			missed = append(missed, pat)
		}
	}
	if len(missed) > 0 {
		t.Logf("declined %d complete literals (perf miss, not a defect): %q", len(missed), missed[:min(len(missed), 20)])
	}
}

// TestAdversarialIncludeExcludeParity drives the real Compile/keeps path for
// both sides at once, so a fast include cannot disagree with a regexp exclude.
func TestAdversarialIncludeExcludeParity(t *testing.T) {
	pats := []string{
		"incident", "Incident", "a", " ", "-", "café", "日本", "a\nb", "\x00",
		"deploy ok", "bot", "d", "\\d", `\.`, ".", "^a", "a$", "(?i)a", "[a]",
		"a|b", "a*", "�", "x�y", "\xff", "abc",
	}
	// Inputs of length 1 plus realistic strings: literal-vs-regexp semantics are
	// covered exhaustively by TestAdversarialLiteralAgreement, so this test only
	// needs enough inputs to exercise the include/exclude interaction.
	inputs := append(advStrings(1), "incident", "Incident opened", "a-bot",
		"deploy ok", "caf\xe9", "a\xffb", "", "abc", "日本語")
	fields := []string{"body", "title", "subtitle", "meta.author", "nope"}
	for _, inc := range pats {
		for _, exc := range pats {
			for _, field := range fields {
				rule := Rule{Field: field, Include: inc, Exclude: exc}
				fast, err := Compile(Filter{Name: "adv", Rules: []Rule{rule}})
				if err != nil {
					continue // regexp rejects invalid-UTF-8 patterns before any fast path
				}
				slow := regexOnly(fast)
				for _, in := range inputs {
					for _, it := range []signals.Item{
						{Body: in}, {Title: in}, {Subtitle: in},
						{Meta: map[string]string{"author": in}},
					} {
						if got, want := fast.keeps(it), slow.keeps(it); got != want {
							t.Fatalf("field=%q include=%q exclude=%q item=%+v: fast=%v regexp=%v",
								field, inc, exc, it, got, want)
						}
					}
				}
			}
		}
	}
}

// FuzzLiteralFastPath lets the fuzzer search for a pattern/input pair where the
// fast path and the regexp path disagree.
func FuzzLiteralFastPath(f *testing.F) {
	seeds := [][2]string{
		{"incident", "sev2 incident"}, {"^abc", "abc"}, {"a.c", "abc"},
		{"(?i)abc", "ABC"}, {"�", "caf\xe9"}, {"\xff", "a\xffb"},
		{`\d`, "a1b"}, {"a", ""}, {"日本", "日本語"}, {"\x00", "a\x00b"},
		{"\xc3", "café"}, {"\xc0\xaf", "/"}, {"a\nb", "a\nb"},
	}
	for _, s := range seeds {
		f.Add(s[0], s[1])
	}
	f.Fuzz(func(t *testing.T, pat, in string) {
		if pat == "" {
			return
		}
		re, err := regexp.Compile(pat)
		if err != nil {
			return
		}
		lit := literalOf(pat)
		if lit == "" {
			return
		}
		if got, want := matches(re, lit, in), re.MatchString(in); got != want {
			t.Fatalf("pattern=%q (% x) input=%q (% x): fast=%v regexp=%v", pat, pat, in, in, got, want)
		}
	})
}

// TestAdversarialLongLiterals pushes patterns and inputs past the sizes where
// the regexp engine switches execution strategies (onepass, bitstate
// backtracker, NFA), where a naive substring equivalence could break down.
func TestAdversarialLongLiterals(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	body := func(n int) string {
		var b strings.Builder
		for range n {
			b.WriteByte(byte('a' + rng.Intn(4)))
		}
		return b.String()
	}
	for _, plen := range []int{1, 8, 64, 512, 4096} {
		pat := body(plen)
		re := regexp.MustCompile(pat)
		if literalOf(pat) != pat {
			t.Fatalf("plain ASCII pattern of length %d declined", plen)
		}
		for _, ilen := range []int{0, 1, plen - 1, plen, plen + 1, 100000} {
			if ilen < 0 {
				continue
			}
			for _, in := range []string{body(ilen), body(ilen) + pat, pat + body(ilen)} {
				if got, want := matches(re, pat, in), re.MatchString(in); got != want {
					t.Fatalf("len(pat)=%d len(in)=%d: fast=%v regexp=%v", plen, len(in), got, want)
				}
			}
		}
	}
}
