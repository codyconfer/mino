package render

import (
	"strings"
	"testing"

	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/munin/internal/render/glyph"
	"github.com/codyconfer/munin/internal/signals"
)

func chipPrefix() string { return glyph.Lead(glyph.Reply()) }

func TestLastCommentChip(t *testing.T) {
	glyph.SetMode(glyph.ModeNone)
	th := theme.Cur()

	cases := []struct {
		name string
		meta map[string]string
		want string
	}{
		{name: "no meta", meta: nil},
		{name: "team reply", meta: map[string]string{"last_comment_by": "alice", "last_comment_team": "true"}, want: "@alice ·us"},
		{name: "external reply", meta: map[string]string{"last_comment_by": "custuser", "last_comment_team": "false"}, want: "@custuser ·them"},
		{name: "no team configured", meta: map[string]string{"last_comment_by": "alice"}, want: "@alice"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := lastCommentChip(th, signals.Item{Meta: c.meta})
			if c.want == "" {
				if got != "" {
					t.Fatalf("chip = %q, want empty", got)
				}
				return
			}
			if !strings.Contains(got, c.want) {
				t.Fatalf("chip = %q, want it to contain %q", got, c.want)
			}
		})
	}
}

func TestLastCommentChipUsesSeverityTones(t *testing.T) {
	glyph.SetMode(glyph.ModeNone)
	th := theme.Cur()

	team := lastCommentChip(th, signals.Item{Meta: map[string]string{"last_comment_by": "alice", "last_comment_team": "true"}})
	external := lastCommentChip(th, signals.Item{Meta: map[string]string{"last_comment_by": "alice", "last_comment_team": "false"}})
	unknown := lastCommentChip(th, signals.Item{Meta: map[string]string{"last_comment_by": "alice"}})

	if team == external {
		t.Error("team and external chips should render differently")
	}
	if want := theme.SeverityStyle(glyph.KindPositive).Render(chipPrefix() + "@alice ·us"); team != want {
		t.Errorf("team chip = %q, want %q", team, want)
	}
	if want := theme.SeverityStyle(glyph.KindWarning).Render(chipPrefix() + "@alice ·them"); external != want {
		t.Errorf("external chip = %q, want %q", external, want)
	}
	if want := th.Dim.Render(chipPrefix() + "@alice"); unknown != want {
		t.Errorf("unknown chip = %q, want %q", unknown, want)
	}
}

func TestItemLinesIncludesLastCommentChip(t *testing.T) {
	glyph.SetMode(glyph.ModeNone)
	it := signals.Item{
		Kind:  "issue",
		Title: "panel crashes on load",
		Meta:  map[string]string{"author": "custuser", "last_comment_by": "alice", "last_comment_team": "true"},
	}
	lines := itemLines(layout.NewFrame(80), theme.Cur(), it)
	if len(lines) == 0 {
		t.Fatal("no lines rendered")
	}
	head := lines[0]
	if !strings.Contains(head, "@custuser") {
		t.Errorf("head %q lost the author chip", head)
	}
	if !strings.Contains(head, "@alice ·us") {
		t.Errorf("head %q missing the last-comment chip", head)
	}
}
