package tool

import (
	"os/exec"
	"regexp"
	"strings"
)

const ContextPrefix = "context."

var placeholder = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_.\-]+)\s*\}\}`)

func Expand(argv []string, lookup func(name string) string) []string {
	out := make([]string, 0, len(argv))
	for _, arg := range argv {
		expanded, ok := expandToken(arg, lookup)
		if !ok {
			continue
		}
		out = append(out, expanded)
	}
	return out
}

func expandToken(arg string, lookup func(string) string) (string, bool) {
	matches := placeholder.FindAllStringSubmatch(arg, -1)
	if len(matches) == 0 {
		return arg, true
	}
	for _, m := range matches {
		if resolve(m[1], lookup) == "" {
			return "", false
		}
	}
	return placeholder.ReplaceAllStringFunc(arg, func(raw string) string {
		m := placeholder.FindStringSubmatch(raw)
		return resolve(m[1], lookup)
	}), true
}

func resolve(name string, lookup func(string) string) string {
	if lookup == nil {
		return ""
	}
	name = strings.TrimSpace(name)
	if !strings.HasPrefix(name, ContextPrefix) {
		return ""
	}
	tool := strings.TrimPrefix(name, ContextPrefix)
	if tool == "" {
		return ""
	}
	return strings.TrimSpace(lookup(tool))
}

func Available(argv []string) bool {
	if len(argv) == 0 || strings.TrimSpace(argv[0]) == "" {
		return false
	}
	_, err := exec.LookPath(argv[0])
	return err == nil
}
