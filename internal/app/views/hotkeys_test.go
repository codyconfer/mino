package views

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	vkdeck "github.com/codyconfer/viewkit/deck"
	"github.com/codyconfer/viewkit/keys"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/deck"
	"github.com/codyconfer/munin/internal/keymap"
	"github.com/codyconfer/munin/internal/plugin"
	"github.com/codyconfer/munin/internal/role"
	"github.com/codyconfer/munin/internal/signals"
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
	if !strings.Contains(got, "build note") {
		t.Fatalf("alt+n did not open the note builder: %q", got)
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
		got, ok := keys.Resolve(kit.keybinds(), tc.key)
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

	var last tea.Cmd
	for _, c := range settleCmds {
		host, last = update(host, c())
	}
	if len(calls) != 0 {
		t.Fatalf("hooks must not run inside Update, they run via tea.ExecProcess: %v", calls)
	}
	if last == nil {
		t.Fatal("settling produced no command; the exit/enter hooks must be handed back as a tea.ExecProcess chain")
	}

	home := kit.d.App.Cfg.Home
	if got := role.LoadActive(home); got == "weekly" {
		t.Fatal("the role was committed before its hooks ran")
	}

	gen, changed := kit.d.App.BeginRoleCycle("triage")
	if !changed {
		t.Fatal("cycling back to triage reported no change")
	}
	settle, ok := kit.d.App.BeginRoleSettle(gen)
	if !ok {
		t.Fatal("BeginRoleSettle rejected a fresh generation")
	}
	if len(settle.Steps) == 0 {
		t.Fatal("the settle carries no hook steps")
	}
	if cmd := kit.runRoleHookStep(host, settle, 0); cmd == nil {
		t.Fatal("the first hook step produced no command")
	}
	if len(calls) != 0 {
		t.Fatalf("hooks ran without a process: %v", calls)
	}
	if got := role.LoadActive(home); got == "triage" {
		t.Fatalf("the settle committed before its last hook step: active=%q", got)
	}
	if cmd := kit.runRoleHookStep(host, settle, len(settle.Steps)); cmd == nil {
		t.Fatal("the end of the chain did not refresh the deck")
	}
	if got := role.LoadActive(home); got != "triage" {
		t.Fatalf("active role after the chain finished = %q, want triage", got)
	}
}

func TestHomeFlightRerunsOnReloadAndRoleCycle(t *testing.T) {
	t.Cleanup(role.ClearStatusChips)
	origRun := role.Run
	role.Run = func(string, string) error { return nil }
	t.Cleanup(func() { role.Run = origRun })

	var fetched []string
	kit := testKit(t)
	kit.d.FetchHomeFlight = func(name string) []signals.Section {
		fetched = append(fetched, name)
		return []signals.Section{{Signal: "github", Title: "from " + name}}
	}
	kit.d.App.Cfg.Keybinds = config.DefaultKeybinds()
	kit.d.App.Directives.Flights["weekly-check"] = config.Flight{Name: "weekly-check", Queries: []string{"q1"}}
	kit.d.App.Directives.Roles = map[string]config.RoleDef{
		"triage": {Name: "triage", Home: "default", Flights: []string{"default"}},
		"weekly": {Name: "weekly", Home: "weekly-check", Flights: []string{"weekly-check"}},
	}
	kit.d.App.Cfg.Role = ""
	if err := kit.d.App.ActivateRole("triage"); err != nil {
		t.Fatal(err)
	}

	home := kit.Home()
	host := deck.New(home, deck.WithKeyHook(kit.KeyHook()), deck.WithMsgHook(kit.MsgHook()))
	host = step(host, tea.WindowSizeMsg{Width: 120, Height: 40})
	host = settle(host, home.Init())
	if want := []string{"default"}; !equalStrings(fetched, want) {
		t.Fatalf("fetched after init = %v, want %v", fetched, want)
	}
	if got := host.View(); !strings.Contains(got, "home flight · default") {
		t.Fatalf("home label after init: %q", got)
	}

	host = settle(host, cmdOf(update(host, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})))
	if want := []string{"default", "default"}; !equalStrings(fetched, want) {
		t.Fatalf("fetched after reload key = %v, want %v", fetched, want)
	}

	host, cycle := update(host, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}, Alt: true})
	if kit.d.App.Cfg.Role != "weekly" {
		t.Fatalf("role after alt+] = %q, want weekly", kit.d.App.Cfg.Role)
	}
	host = settle(host, cycle)
	if want := []string{"default", "default", "weekly-check"}; !equalStrings(fetched, want) {
		t.Fatalf("fetched after role cycle = %v, want %v", fetched, want)
	}
	got := host.View()
	for _, want := range []string{"home flight · weekly-check", "from weekly-check"} {
		if !strings.Contains(got, want) {
			t.Errorf("home after role cycle missing %q\n%s", want, got)
		}
	}
}

func cmdOf(_ *vkdeck.Model, cmd tea.Cmd) tea.Cmd { return cmd }

func settle(a *vkdeck.Model, cmd tea.Cmd) *vkdeck.Model {
	return settleDepth(a, cmd, 8)
}

func settleDepth(a *vkdeck.Model, cmd tea.Cmd, depth int) *vkdeck.Model {
	if depth == 0 {
		return a
	}
	for _, c := range flattenCmds(cmd) {
		if c == nil {
			continue
		}
		msg := c()
		if msg == nil {
			continue
		}
		next, more := update(a, msg)
		a = settleDepth(next, more, depth-1)
	}
	return a
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
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
	if strings.Contains(got, "build reminder") {
		t.Fatalf("alt+r opened the reminder builder while detached: %q", got)
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
	if !strings.Contains(got, "build reminder") {
		t.Fatalf("alt+r did not open the reminder builder while attached: %q", got)
	}
}
