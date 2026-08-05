package github

import (
	"slices"
	"strings"
)

const scopeListPrefix = "following scopes: ["

// missingScopes pulls the scope names GitHub asked for out of an error message.
// GitHub repeats one sentence per offending field, e.g. "The 'number' field
// requires one of the following scopes: ['read:project'], but your token has
// only been granted the: ['repo'] scopes.", so the same scope shows up several
// times in one failure.
func missingScopes(msg string) []string {
	var out []string
	for rest := msg; ; {
		i := strings.Index(rest, scopeListPrefix)
		if i < 0 {
			break
		}
		rest = rest[i+len(scopeListPrefix):]
		list, after, ok := strings.Cut(rest, "]")
		if !ok {
			break
		}
		rest = after
		for _, raw := range strings.Split(list, ",") {
			if scope := strings.Trim(strings.TrimSpace(raw), "'\""); scope != "" && !slices.Contains(out, scope) {
				out = append(out, scope)
			}
		}
	}
	slices.Sort(out)
	return out
}

// scopeRefreshCommand is the `gh auth refresh` invocation that grants scopes on
// hostname (empty means github.com).
func scopeRefreshCommand(hostname string, scopes []string) string {
	var b strings.Builder
	b.WriteString("gh auth refresh")
	if hostname != "" && hostname != "github.com" {
		b.WriteString(" -h " + hostname)
	}
	for _, s := range scopes {
		b.WriteString(" -s " + s)
	}
	return b.String()
}

// scopeHint explains how to grant the scopes GitHub reported as missing. It
// falls back to fallback when the message named no scopes.
func scopeHint(hostname string, scopes []string, fallback string) string {
	if len(scopes) == 0 {
		return fallback
	}
	return "run `" + scopeRefreshCommand(hostname, scopes) +
		"` and retry, or re-run `mino login github`; on a GitHub App, add the matching installation permission"
}

// scopeSummary names the missing scopes for an error message.
func scopeSummary(scopes []string) string {
	if len(scopes) == 0 {
		return "your GitHub token is missing a required scope"
	}
	noun := "scope"
	if len(scopes) > 1 {
		noun = "scopes"
	}
	return "your GitHub token is missing the " + strings.Join(scopes, ", ") + " " + noun
}
