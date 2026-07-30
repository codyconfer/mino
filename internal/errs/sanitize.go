package errs

import (
	"strings"
	"unicode/utf8"
)

const (
	maxExcerptRunes = 480
	maxExcerptBytes = maxExcerptRunes * 4
)

func dangerousControl(r rune) bool {
	if r == '\n' || r == '\t' {
		return false
	}
	if r == utf8.RuneError {
		return true
	}
	return r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f)
}

func Clean(s string) string {
	if s == "" || !strings.ContainsFunc(s, dangerousControl) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if dangerousControl(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func Excerpt(s string) string {
	if len(s) > maxExcerptBytes {
		s = s[:maxExcerptBytes]
	}
	s = strings.Join(strings.Fields(Clean(s)), " ")
	if utf8.RuneCountInString(s) <= maxExcerptRunes {
		return s
	}
	return string([]rune(s)[:maxExcerptRunes]) + "…"
}

func ExcerptBytes(b []byte) string {
	if len(b) > maxExcerptBytes {
		b = b[:maxExcerptBytes]
	}
	return Excerpt(string(b))
}
