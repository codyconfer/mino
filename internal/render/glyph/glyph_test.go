package glyph

import (
	"testing"

	vk "github.com/codyconfer/viewkit/glyph"
)

func TestForTool(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"github", GitHub()},
		{"GitHub", GitHub()},
		{"gitlab", GitLab()},
		{"GitLab", GitLab()},
		{"slack", Slack()},
		{"google", Google()},
		{"Google", Google()},
		{"notes", Notes()},
		{"ntr", Notes()},
		{"calendar", ""},
		{"gmail", ""},
		{"daemon", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := ForTool(tc.name); got != tc.want {
			t.Errorf("ForTool(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestForToolResolvesRegistryID(t *testing.T) {
	vk.Register("test.role.status.glyph", vk.Variants{Nerd: "N", Uni: "U", ASCII: "A"})
	if got := ForTool("test.role.status.glyph"); got != vk.ResolveID("test.role.status.glyph") || got == "" {
		t.Fatalf("ForTool registry = %q", got)
	}
	if got := ForTool("totally-unknown-glyph-xyz"); got != "" {
		t.Fatalf("unknown = %q, want empty (plain-text fallback)", got)
	}
}

func TestGitLabHasItsOwnMark(t *testing.T) {
	if GitLab() == "" {
		t.Fatal("GitLab() is empty; viewkit ships no gitlab mark, so mino must register one")
	}
	if GitLab() == GitHub() {
		t.Error("GitLab() and GitHub() render the same glyph; the status strip could not tell the " +
			"two forges apart")
	}
	prev := vk.CurrentMode()
	t.Cleanup(func() { SetMode(prev) })
	for _, mode := range []Mode{ModeNerd, ModeUnicode, ModeNone} {
		SetMode(mode)
		if ForTool("gitlab") == "" {
			t.Errorf("ForTool(gitlab) is empty in mode %v; every mode needs a fallback or the "+
				"status strip renders a hole", mode)
		}
	}
}
