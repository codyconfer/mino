package github

import (
	"reflect"
	"strings"
	"testing"
)

const ghScopeStderr = "Your token has not been granted the required scopes to execute this query. " +
	"The 'number' field requires one of the following scopes: ['read:project'], but your token has only been granted the: ['gist', 'read:org', 'repo'] scopes. " +
	"Your token has not been granted the required scopes to execute this query. " +
	"The 'name' field requires one of the following scopes: ['read:project'], but your token has only been granted the: ['gist', 'read:org', 'repo'] scopes."

func TestMissingScopesDedupesAndSorts(t *testing.T) {
	cases := []struct {
		name string
		msg  string
		want []string
	}{
		{"repeated field complaints", ghScopeStderr, []string{"read:project"}},
		{
			"several required scopes",
			"requires one of the following scopes: ['read:project', 'repo']",
			[]string{"read:project", "repo"},
		},
		{"no scope list", "Could not resolve to a ProjectV2 with the number 1085.", nil},
		{"unterminated list", "following scopes: ['read:project'", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := missingScopes(tc.msg); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("missingScopes = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMissingScopesIgnoresGrantedList(t *testing.T) {
	got := missingScopes(ghScopeStderr)
	for _, s := range got {
		if s == "repo" || s == "gist" {
			t.Fatalf("granted scope leaked into missing set: %v", got)
		}
	}
}

func TestScopeRefreshCommand(t *testing.T) {
	cases := []struct {
		hostname string
		scopes   []string
		want     string
	}{
		{"", []string{"read:project"}, "gh auth refresh -s read:project"},
		{"github.com", []string{"read:project"}, "gh auth refresh -s read:project"},
		{"ghe.example.com", []string{"read:project", "repo"}, "gh auth refresh -h ghe.example.com -s read:project -s repo"},
	}
	for _, tc := range cases {
		if got := scopeRefreshCommand(tc.hostname, tc.scopes); got != tc.want {
			t.Errorf("scopeRefreshCommand(%q, %v) = %q, want %q", tc.hostname, tc.scopes, got, tc.want)
		}
	}
}

func TestScopeHintFallsBackWithoutScopes(t *testing.T) {
	if got := scopeHint("", nil, projectScopeHint); got != projectScopeHint {
		t.Errorf("hint = %q, want the fallback", got)
	}
	hint := scopeHint("", []string{"read:project"}, projectScopeHint)
	if !strings.Contains(hint, "`gh auth refresh -s read:project`") {
		t.Errorf("hint = %q, want the refresh command", hint)
	}
}

func TestScopeSummary(t *testing.T) {
	cases := map[string][]string{
		"your GitHub token is missing a required scope":              nil,
		"your GitHub token is missing the read:project scope":        {"read:project"},
		"your GitHub token is missing the read:project, repo scopes": {"read:project", "repo"},
	}
	for want, scopes := range cases {
		if got := scopeSummary(scopes); got != want {
			t.Errorf("scopeSummary(%v) = %q, want %q", scopes, got, want)
		}
	}
}
