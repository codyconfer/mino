package views

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/deck"
	"github.com/codyconfer/munin/internal/keymap"
	"github.com/codyconfer/munin/internal/plugin"
	"github.com/codyconfer/munin/internal/role"
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
		{"alt+[", keymap.TargetRolePrev},
		{"alt+]", keymap.TargetRoleNext},
	} {
		got, ok := keymap.ResolveHotkey(kit.keybinds(), tc.key)
		if !ok || got != tc.want {
			t.Errorf("%s → %q,%v want %q", tc.key, got, ok, tc.want)
		}
	}
}

func TestHotkeyCyclesRoleDebounced(t *testing.T) {
	t.Cleanup(role.ClearStatusChips)
	var calls []string
	orig := role.Run
	role.Run = func(_, script string) error {
		calls = append(calls, script)
		return nil
	}
	t.Cleanup(func() { role.Run = orig })

	kit := testKit(t)
	kit.d.App.Cfg.Keybinds = config.DefaultKeybinds()
	kit.d.App.Directives.Roles = map[string]config.RoleDef{
		"ops": {Name: "ops", Hooks: config.RoleHooks{
			Enter: config.RoleShellHooks{Bash: "enter-ops", PowerShell: "enter-ops"},
			Exit:  config.RoleShellHooks{Bash: "exit-ops", PowerShell: "exit-ops"},
		}},
		"triage": {Name: "triage", Home: "default", Flights: []string{"default"}, Hooks: config.RoleHooks{
			Enter: config.RoleShellHooks{Bash: "enter-triage", PowerShell: "enter-triage"},
			Exit:  config.RoleShellHooks{Bash: "exit-triage", PowerShell: "exit-triage"},
		}},
		"weekly": {Name: "weekly", Hooks: config.RoleHooks{
			Enter: config.RoleShellHooks{Bash: "enter-weekly", PowerShell: "enter-weekly"},
		}},
	}
	kit.d.App.Cfg.Role = ""
	if err := kit.d.App.ActivateRole("ops"); err != nil {
		t.Fatal(err)
	}
	calls = nil

	host := deck.New(kit.Home(), deck.WithKeyHook(kit.KeyHook()), deck.WithMsgHook(kit.MsgHook()))
	host = step(host, tea.WindowSizeMsg{Width: 100, Height: 40})
	body := host.View()
	if !strings.Contains(body, "alt+]") || !strings.Contains(body, "next role") {
		t.Fatalf("home hints missing role cycle: %q", body)
	}

	var settleCmds []tea.Cmd
	for i := 0; i < 2; i++ {
		var cmd tea.Cmd
		host, cmd = update(host, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}, Alt: true})
		if cmd != nil {
			settleCmds = append(settleCmds, cmd)
		}
	}
	if kit.d.App.Cfg.Role != "weekly" {
		t.Fatalf("role after burst = %q, want weekly", kit.d.App.Cfg.Role)
	}
	if len(calls) != 0 {
		t.Fatalf("hooks during burst = %v", calls)
	}
	if ctx := kit.menuCtx(); len(ctx) == 0 || ctx[0][1] != "weekly" {
		t.Fatalf("menuCtx after burst = %v", ctx)
	}
	if len(settleCmds) < 2 {
		t.Fatalf("expected settle ticks, got %d", len(settleCmds))
	}

	for _, c := range settleCmds {
		host = step(host, c())
	}
	if len(calls) != 2 || calls[0] != "exit-ops" || calls[1] != "enter-weekly" {
		t.Fatalf("settle hooks = %v", calls)
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
