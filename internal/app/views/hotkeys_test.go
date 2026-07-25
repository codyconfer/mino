package views

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/deck"
	"github.com/codyconfer/munin/internal/keymap"
	"github.com/codyconfer/munin/internal/plugin"
	pub "github.com/codyconfer/munin/plugin"
)

func TestHotkeyOpensNewNoteFromHome(t *testing.T) {
	kit := testKit(t)
	kit.d.App.Cfg.Keybinds = config.DefaultKeybinds()

	app := deck.New(kit.Home(), deck.WithKeyHook(kit.KeyHook()))
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	body := app.View()
	if !strings.Contains(body, "alt+n") {
		t.Fatalf("home hints missing alt+n: %q", body)
	}

	app, cmd := update(app, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}, Alt: true})
	if cmd != nil {
		for _, c := range flattenCmds(cmd) {
			if c == nil {
				continue
			}
			app = step(app, c())
		}
	}
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	got := app.View()
	if !strings.Contains(got, "new note") {
		t.Fatalf("alt+n did not open new note form: %q", got)
	}
}

func TestHotkeyOpensFlight(t *testing.T) {
	kit := testKit(t)
	kit.d.App.Cfg.Keybinds = map[string]string{"alt+f": "default"}

	app := deck.New(kit.Home(), deck.WithKeyHook(kit.KeyHook()))
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	app, cmd := update(app, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}, Alt: true})
	if cmd != nil {
		for _, c := range flattenCmds(cmd) {
			if c == nil {
				continue
			}
			app = step(app, c())
		}
	}
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	got := app.View()
	if !strings.Contains(got, "flight: default") && !strings.Contains(got, "flight:default") {
		// Title is "flight: default" from FlightResults
		if !strings.Contains(strings.ToLower(got), "default") {
			t.Fatalf("alt+f did not open flight: %q", got)
		}
	}
}

func TestHotkeyResolveTargets(t *testing.T) {
	kit := testKit(t)
	kit.d.App.Cfg.Keybinds = config.DefaultKeybinds()
	for _, tc := range []struct {
		key  string
		want string
	}{
		{"alt+n", keymap.TargetNoteNew},
		{"alt+r", keymap.TargetRemindNew},
		{"alt+t", keymap.TargetTaskNew},
	} {
		got, ok := keymap.ResolveHotkey(kit.keybinds(), tc.key)
		if !ok || got != tc.want {
			t.Errorf("%s → %q,%v want %q", tc.key, got, ok, tc.want)
		}
	}
}

func TestRemindHotkeyHiddenWithoutService(t *testing.T) {
	pub.SetServiceAttachedFunc(func() bool { return false })
	t.Cleanup(func() { pub.SetServiceAttachedFunc(plugin.ServiceAttached) })

	kit := testKit(t)
	kit.d.App.Cfg.Keybinds = config.DefaultKeybinds()
	for _, h := range kit.hotkeyHints() {
		if h[1] == "new reminder" {
			t.Fatalf("reminder hint present while detached: %v", kit.hotkeyHints())
		}
	}

	app := deck.New(kit.Home(), deck.WithKeyHook(kit.KeyHook()))
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	app, cmd := update(app, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}, Alt: true})
	if cmd != nil {
		for _, c := range flattenCmds(cmd) {
			if c == nil {
				continue
			}
			app = step(app, c())
		}
	}
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	got := app.View()
	if strings.Contains(got, "new reminder") {
		t.Fatalf("alt+r opened reminder form while detached: %q", got)
	}
}

func TestRemindHotkeyWorksWithService(t *testing.T) {
	pub.SetServiceAttachedFunc(func() bool { return true })
	t.Cleanup(func() { pub.SetServiceAttachedFunc(plugin.ServiceAttached) })

	kit := testKit(t)
	kit.d.App.Cfg.Keybinds = config.DefaultKeybinds()
	found := false
	for _, h := range kit.hotkeyHints() {
		if h[1] == "new reminder" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("reminder hint missing while attached: %v", kit.hotkeyHints())
	}

	app := deck.New(kit.Home(), deck.WithKeyHook(kit.KeyHook()))
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	app, cmd := update(app, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}, Alt: true})
	if cmd != nil {
		for _, c := range flattenCmds(cmd) {
			if c == nil {
				continue
			}
			app = step(app, c())
		}
	}
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	got := app.View()
	if !strings.Contains(got, "new reminder") {
		t.Fatalf("alt+r did not open reminder form while attached: %q", got)
	}
}
