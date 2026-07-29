package render

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/munin/internal/render/glyph"
	"github.com/codyconfer/munin/internal/signals"
)

func chipPrefix() string { return glyph.Lead(glyph.Reply()) }

func ago(d time.Duration) string { return time.Now().Add(-d).Format(time.RFC3339) }

func TestLastCommentChip(t *testing.T) {
	glyph.SetMode(glyph.ModeNone)
	th := theme.Cur()
	threeDaysAgo := ago(72 * time.Hour)

	cases := []struct {
		name string
		meta map[string]string
		want string
	}{
		{name: "no meta", meta: nil},
		{
			name: "team reply",
			meta: map[string]string{"last_comment_by": "alice", "last_comment_team": "true", "last_comment_at": threeDaysAgo},
			want: "@alice ·team ·3d ago",
		},
		{
			name: "external reply",
			meta: map[string]string{"last_comment_by": "custuser", "last_comment_team": "false", "last_comment_at": threeDaysAgo},
			want: "@custuser ·3d ago",
		},
		{
			name: "no team configured",
			meta: map[string]string{"last_comment_by": "alice", "last_comment_at": threeDaysAgo},
			want: "@alice ·3d ago",
		},
		{
			name: "no comment time",
			meta: map[string]string{"last_comment_by": "alice", "last_comment_team": "true"},
			want: "@alice ·team",
		},
		{
			name: "unparseable comment time",
			meta: map[string]string{"last_comment_by": "alice", "last_comment_at": "yesterday"},
			want: "@alice",
		},
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
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	glyph.SetMode(glyph.ModeNone)
	th := theme.Cur()
	threeDaysAgo := ago(72 * time.Hour)

	team := lastCommentChip(th, signals.Item{Meta: map[string]string{"last_comment_by": "alice", "last_comment_team": "true", "last_comment_at": threeDaysAgo}})
	external := lastCommentChip(th, signals.Item{Meta: map[string]string{"last_comment_by": "alice", "last_comment_team": "false", "last_comment_at": threeDaysAgo}})
	unknown := lastCommentChip(th, signals.Item{Meta: map[string]string{"last_comment_by": "alice", "last_comment_at": threeDaysAgo}})

	if team == external {
		t.Error("team and external chips should render differently")
	}
	if external == unknown {
		t.Error("external and no-team chips carry the same text, so their tones must differ")
	}
	if want := theme.SeverityStyle(glyph.KindPositive).Render(chipPrefix() + "@alice ·team ·3d ago"); team != want {
		t.Errorf("team chip = %q, want %q", team, want)
	}
	if want := theme.SeverityStyle(glyph.KindWarning).Render(chipPrefix() + "@alice ·3d ago"); external != want {
		t.Errorf("external chip = %q, want %q", external, want)
	}
	if want := th.Dim.Render(chipPrefix() + "@alice ·3d ago"); unknown != want {
		t.Errorf("unknown chip = %q, want %q", unknown, want)
	}
}

func TestItemLinesIncludesLastCommentChip(t *testing.T) {
	glyph.SetMode(glyph.ModeNone)
	it := signals.Item{
		Kind:      "issue",
		Title:     "panel crashes on load",
		Timestamp: time.Now().Add(-5 * time.Hour),
		Meta: map[string]string{
			"author":            "custuser",
			"last_comment_by":   "alice",
			"last_comment_team": "true",
			"last_comment_at":   ago(72 * time.Hour),
		},
	}
	lines := itemLines(layout.NewFrame(80), theme.Cur(), it)
	if len(lines) == 0 {
		t.Fatal("no lines rendered")
	}
	head := lines[0]
	if !strings.Contains(head, "@custuser") {
		t.Errorf("head %q lost the author chip", head)
	}
	if !strings.Contains(head, "@alice ·team ·3d ago") {
		t.Errorf("head %q missing the last-comment chip", head)
	}
	if strings.Contains(head, "5h ago") {
		t.Errorf("head %q shows the item timestamp alongside the comment age", head)
	}
}

func TestItemLinesKeepsTimestampWithoutCommentAge(t *testing.T) {
	glyph.SetMode(glyph.ModeNone)
	it := signals.Item{
		Kind:      "issue",
		Title:     "panel crashes on load",
		Timestamp: time.Now().Add(-5 * time.Hour),
		Meta:      map[string]string{"last_comment_by": "alice", "last_comment_team": "true"},
	}
	lines := itemLines(layout.NewFrame(80), theme.Cur(), it)
	if len(lines) == 0 {
		t.Fatal("no lines rendered")
	}
	head := lines[0]
	if !strings.Contains(head, "@alice ·team") {
		t.Errorf("head %q missing the last-comment chip", head)
	}
	if !strings.Contains(head, "5h ago") {
		t.Errorf("head %q dropped the item timestamp", head)
	}
}
