package role

import (
	"errors"
	"strings"
	"testing"

	"github.com/codyconfer/munin/internal/config"
)

func TestTruncateStatus(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"  hi  ", "hi"},
		{"short", "short"},
		{"abcdefghijklmnopqrstuvwxyz", "abcdefghijklmnopqrst"},
		{"line1\nline2", "line1"},
		{"  padded line  \nmore", "padded line"},
		{"日本語テスト一二三四五六七八", "日本語テスト一二三四五六七八"}, // 14 runes
		{strings.Repeat("あ", 25), strings.Repeat("あ", 20)},
	}
	for _, tc := range cases {
		if got := TruncateStatus(tc.in); got != tc.want {
			t.Errorf("TruncateStatus(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	if n := len([]rune(TruncateStatus(strings.Repeat("x", 50)))); n != StatusTextMax {
		t.Fatalf("rune count = %d, want %d", n, StatusTextMax)
	}
}

func TestCollectStatusUsesCaptureAndTruncates(t *testing.T) {
	orig := Capture
	Capture = func(kind, script string) (string, error) {
		if !strings.Contains(script, "echo long") {
			t.Fatalf("unexpected script %q kind %s", script, kind)
		}
		return "abcdefghijklmnopqrstuvwxyz\nextra\n", nil
	}
	t.Cleanup(func() { Capture = orig })

	rd := config.RoleDef{
		Status: []config.RoleStatusBlock{
			{Glyph: "github", Bash: "echo long", PowerShell: "echo long"},
		},
	}
	chips, warns := CollectStatus(rd)
	if len(warns) != 0 {
		t.Fatalf("warnings = %v", warns)
	}
	if len(chips) != 1 {
		t.Fatalf("chips = %+v", chips)
	}
	if chips[0].Glyph != "github" || chips[0].Text != "abcdefghijklmnopqrst" || chips[0].Index != 0 {
		t.Fatalf("chip = %+v", chips[0])
	}
}

func TestCollectStatusSkipsFailures(t *testing.T) {
	orig := Capture
	var calls int
	Capture = func(kind, script string) (string, error) {
		calls++
		if strings.Contains(script, "fail") {
			return "", errors.New("boom")
		}
		return "ok-text", nil
	}
	t.Cleanup(func() { Capture = orig })

	rd := config.RoleDef{
		Status: []config.RoleStatusBlock{
			{Glyph: "github", Bash: "fail", PowerShell: "fail"},
			{Glyph: "slack", Bash: "ok", PowerShell: "ok"},
		},
	}
	chips, warns := CollectStatus(rd)
	if calls != 2 {
		t.Fatalf("calls = %d", calls)
	}
	if len(warns) != 1 || !strings.Contains(warns[0].Error(), "boom") {
		t.Fatalf("warnings = %v", warns)
	}
	if len(chips) != 1 || chips[0].Glyph != "slack" || chips[0].Text != "ok-text" || chips[0].Index != 1 {
		t.Fatalf("chips = %+v", chips)
	}
}

func TestCollectStatusSkipsEmptyBlocks(t *testing.T) {
	orig := Capture
	Capture = func(string, string) (string, error) {
		t.Fatal("should not capture empty blocks")
		return "", nil
	}
	t.Cleanup(func() { Capture = orig })

	chips, warns := CollectStatus(config.RoleDef{
		Status: []config.RoleStatusBlock{{Glyph: "github"}},
	})
	if len(chips) != 0 || len(warns) != 0 {
		t.Fatalf("chips=%v warns=%v", chips, warns)
	}
}

func TestStatusChipsSetClear(t *testing.T) {
	t.Cleanup(ClearStatusChips)
	ClearStatusChips()
	if got := StatusChips(); got != nil {
		t.Fatalf("empty = %v", got)
	}
	SetStatusChips([]Chip{{Glyph: "github", Text: "abc", Index: 0}})
	got := StatusChips()
	if len(got) != 1 || got[0].Text != "abc" {
		t.Fatalf("got = %+v", got)
	}
	got[0].Text = "mutated"
	if StatusChips()[0].Text != "abc" {
		t.Fatal("StatusChips should return a copy")
	}
	ClearStatusChips()
	if StatusChips() != nil {
		t.Fatal("expected clear")
	}
}
