package signals

import (
	"strings"
	"unicode/utf8"
)

func safeControl(r rune) bool {
	return r == '\n' || r == '\t'
}

func dangerousControl(r rune) bool {
	if safeControl(r) {
		return false
	}
	if r == utf8.RuneError {
		return true
	}
	return r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f)
}

func Clean(s string) string {
	if s == "" {
		return s
	}
	needs := false
	for _, r := range s {
		if dangerousControl(r) {
			needs = true
			break
		}
	}
	if !needs {
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
