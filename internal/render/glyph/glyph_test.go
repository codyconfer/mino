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
