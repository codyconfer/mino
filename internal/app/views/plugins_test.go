package views

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/codyconfer/munin/internal/deck"
	"github.com/codyconfer/munin/internal/plugin"
	"github.com/codyconfer/munin/internal/signals/build"
	"github.com/codyconfer/munin/internal/testenv"
)

var pluginsAnsi = regexp.MustCompile("\x1b\\[[0-9;]*m")

func pluginsTestEnv(t *testing.T) {
	t.Helper()
	testenv.Isolate(t)
	_ = build.KnownSignals()
	plugin.LoadEnabled()
}

func installTestPlugin(t *testing.T, home, id string) {
	t.Helper()
	if _, err := plugin.Install(home, id, plugin.InstallOptions{}); err != nil {
		t.Fatalf("install %s: %v", id, err)
	}
}

func TestPluginsMenuSmoke(t *testing.T) {
	pluginsTestEnv(t)
	kit := testKit(t)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("plugins menu panicked: %v", r)
		}
	}()
	a := deck.New(kit.Plugins())
	a = step(a, tea.WindowSizeMsg{Width: 100, Height: 40})
	body := pluginsAnsi.ReplaceAllString(a.View(), "")
	if !strings.Contains(body, "PLUGINS") {
		t.Fatalf("missing plugins title:\n%s", body)
	}
	a = step(a, tea.KeyMsg{Type: tea.KeyDown})
	_ = a.View()
	a = step(a, tea.KeyMsg{Type: tea.KeyEsc})
	_ = a.View()
}

func TestPluginsListsInstalledOnly(t *testing.T) {
	pluginsTestEnv(t)
	kit := testKit(t)
	page := kit.Plugins().(*pluginsPage)
	if len(page.rows) != 0 {
		t.Fatalf("expected empty installed list, got %d rows", len(page.rows))
	}
	body := pluginsAnsi.ReplaceAllString(page.Body(100, 40), "")
	if !strings.Contains(body, "no plugins installed") {
		t.Fatalf("empty body missing hint:\n%s", body)
	}

	id := "munin.ntr"
	if _, ok := plugin.Lookup(id); !ok {
		t.Skip("munin.ntr not linked")
	}
	installTestPlugin(t, kit.d.App.Cfg.Home, id)
	page.reload()
	if len(page.rows) == 0 {
		t.Fatal("expected installed plugin in list")
	}
	found := false
	for _, row := range page.rows {
		if row.id == id {
			found = true
			if !row.enabled {
				t.Fatal("expected enabled after install")
			}
		}
	}
	if !found {
		t.Fatalf("missing %s in rows: %+v", id, page.rows)
	}
	body = pluginsAnsi.ReplaceAllString(page.Body(100, 40), "")
	if !strings.Contains(body, id) || !strings.Contains(body, "kind=") {
		t.Fatalf("body missing installed plugin:\n%s", body)
	}
}

func TestPluginsListsInternalFirst(t *testing.T) {
	pluginsTestEnv(t)
	kit := testKit(t)
	home := kit.d.App.Cfg.Home
	for _, id := range []string{"munin.ntr", "munin.demo"} {
		if _, ok := plugin.Lookup(id); !ok {
			continue
		}
		installTestPlugin(t, home, id)
	}
	page := kit.Plugins().(*pluginsPage)
	if len(page.rows) == 0 {
		t.Fatal("expected installed plugins in list")
	}
	seenExternal := false
	var lastInternal, lastExternal string
	for _, row := range page.rows {
		if plugin.IsInternal(row.id) {
			if seenExternal {
				t.Fatalf("internal %q listed after external in plugins menu", row.id)
			}
			if lastInternal != "" && row.id < lastInternal {
				t.Fatalf("internal ids not alpha: %q before %q", lastInternal, row.id)
			}
			lastInternal = row.id
			continue
		}
		seenExternal = true
		if lastExternal != "" && row.id < lastExternal {
			t.Fatalf("external ids not alpha: %q before %q", lastExternal, row.id)
		}
		lastExternal = row.id
	}
}

func TestPluginsDisableKeepsRow(t *testing.T) {
	pluginsTestEnv(t)
	kit := testKit(t)
	id := "munin.ntr"
	if _, ok := plugin.Lookup(id); !ok {
		t.Skip("munin.ntr not linked")
	}
	installTestPlugin(t, kit.d.App.Cfg.Home, id)

	page := kit.Plugins().(*pluginsPage)
	page.cursor = 0
	for i, row := range page.rows {
		if row.id == id {
			page.cursor = i
			break
		}
	}

	a := deck.New(page)
	a = step(a, tea.WindowSizeMsg{Width: 100, Height: 40})
	a, cmd := update(a, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected toggle cmd")
	}
	for _, c := range flattenCmds(cmd) {
		if c == nil {
			continue
		}
		a = step(a, c())
	}

	if plugin.Enabled(id) {
		t.Fatal("expected disabled after toggle")
	}
	if !plugin.Installed(id) {
		t.Fatal("disable must keep plugin installed")
	}
	found := false
	for _, row := range page.rows {
		if row.id == id {
			found = true
			if row.enabled {
				t.Fatal("row should show disabled")
			}
		}
	}
	if !found {
		t.Fatal("disabled plugin missing from list")
	}
	body := pluginsAnsi.ReplaceAllString(a.View(), "")
	if !strings.Contains(body, "disabled") {
		t.Fatalf("view missing disabled:\n%s", body)
	}
	if !page.queue.Active() {
		t.Fatal("expected toast after toggle")
	}
}

func TestPluginsUninstallRemovesFromListAndReinstallRestores(t *testing.T) {
	pluginsTestEnv(t)
	kit := testKit(t)
	id := "munin.ntr"
	if _, ok := plugin.Lookup(id); !ok {
		t.Skip("munin.ntr not linked")
	}
	home := kit.d.App.Cfg.Home
	installTestPlugin(t, home, id)

	page := kit.Plugins().(*pluginsPage)
	for i, row := range page.rows {
		if row.id == id {
			page.cursor = i
			break
		}
	}

	a := deck.New(page)
	a = step(a, tea.WindowSizeMsg{Width: 100, Height: 40})
	a, cmd := update(a, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})
	if cmd == nil {
		t.Fatal("expected uninstall cmd")
	}
	for _, c := range flattenCmds(cmd) {
		if c == nil {
			continue
		}
		a = step(a, c())
	}

	if plugin.Installed(id) {
		t.Fatal("expected uninstalled")
	}
	if plugin.Enabled(id) {
		t.Fatal("expected disabled after uninstall")
	}
	if len(page.rows) != 0 {
		t.Fatalf("expected empty list after uninstall, got %+v", page.rows)
	}

	// Re-install via API (picker coverage is separate); list should regain the row.
	installTestPlugin(t, home, id)
	page.reload()
	found := false
	for _, row := range page.rows {
		if row.id == id && row.enabled {
			found = true
		}
	}
	if !found {
		t.Fatalf("re-install did not restore list row: %+v", page.rows)
	}
}

func TestPluginsTogglePersists(t *testing.T) {
	pluginsTestEnv(t)
	kit := testKit(t)
	id := "munin.demo"
	if _, ok := plugin.Lookup(id); !ok {
		t.Skip("munin.demo not linked")
	}
	installTestPlugin(t, kit.d.App.Cfg.Home, id)

	page := kit.Plugins().(*pluginsPage)
	for i, row := range page.rows {
		if row.id == id {
			page.cursor = i
			break
		}
	}
	was := page.rows[page.cursor].enabled

	a := deck.New(page)
	a = step(a, tea.WindowSizeMsg{Width: 100, Height: 40})
	a, cmd := update(a, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected toggle cmd")
	}
	for _, c := range flattenCmds(cmd) {
		if c == nil {
			continue
		}
		a = step(a, c())
	}

	if plugin.Enabled(id) == was {
		t.Fatalf("plugin %s enable state unchanged (was %v)", id, was)
	}
	wantState := "disabled"
	if !was {
		wantState = "enabled"
	}
	body := pluginsAnsi.ReplaceAllString(a.View(), "")
	if !strings.Contains(body, wantState) {
		t.Fatalf("view missing %q after toggle:\n%s", wantState, body)
	}
	if !plugin.Installed(id) {
		t.Fatal("toggle must not uninstall")
	}

	a, cmd = update(a, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	for _, c := range flattenCmds(cmd) {
		if c == nil {
			continue
		}
		a = step(a, c())
	}
	if plugin.Enabled(id) != was {
		t.Fatalf("plugin %s not restored to %v via d", id, was)
	}
}

func TestToolingMenuIncludesPluginsEntry(t *testing.T) {
	kit := testKit(t)
	labels := make([]string, 0, len(kit.toolingMenuItems()))
	var pluginsDesc string
	for _, it := range kit.toolingMenuItems() {
		labels = append(labels, it.Label)
		if it.Label == "Plugins" {
			pluginsDesc = it.Desc
		}
	}
	found := false
	for _, l := range labels {
		if l == "Plugins" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("tooling menu missing Plugins entry: %v", labels)
	}
	if !strings.Contains(pluginsDesc, "install") {
		t.Fatalf("Plugins desc should mention install: %q", pluginsDesc)
	}
	for _, it := range kit.mainMenuItems() {
		if it.Label == "Plugins" {
			t.Fatalf("main menu still exposes Plugins")
		}
	}
}

func TestPluginsInstallOpensPickerAndInstallAddsRow(t *testing.T) {
	pluginsTestEnv(t)
	kit := testKit(t)
	page := kit.Plugins().(*pluginsPage)
	if len(page.rows) != 0 {
		t.Fatalf("expected empty list before install, got %+v", page.rows)
	}

	a := deck.New(page)
	a = step(a, tea.WindowSizeMsg{Width: 100, Height: 40})
	a, cmd := update(a, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	if cmd == nil {
		t.Fatal("expected install picker push cmd")
	}
	for _, c := range flattenCmds(cmd) {
		if c == nil {
			continue
		}
		a = step(a, c())
	}
	pickerBody := pluginsAnsi.ReplaceAllString(a.View(), "")
	if !strings.Contains(strings.ToLower(pickerBody), "install") {
		t.Fatalf("expected install picker:\n%s", pickerBody)
	}

	id := "munin.ntr"
	if _, ok := plugin.Lookup(id); !ok {
		t.Skip("munin.ntr not linked")
	}
	home := kit.d.App.Cfg.Home
	app := kit.d.App
	cands, err := plugin.ListInstallCandidates(home)
	if err != nil {
		t.Fatal(err)
	}
	var cand plugin.InstallCandidate
	for _, c := range cands {
		if c.ID == id && c.Installable {
			cand = c
			break
		}
	}
	if cand.ID == "" {
		t.Fatalf("munin.ntr missing from install candidates: %+v", cands)
	}
	res, err := plugin.InstallCandidateEntry(home, cand, plugin.InstallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.ReloadDirectives(); err != nil {
		t.Fatal(err)
	}
	_ = step(a, pluginsInstalledMsg{id: id, written: len(res.Written), skipped: len(res.Skipped)})

	if _, err := os.Stat(filepath.Join(home, "queries", "ntr-list.yaml")); err != nil {
		t.Fatalf("ntr-list seed missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "flights", "ntr.yaml")); err != nil {
		t.Fatalf("ntr flight seed missing: %v", err)
	}
	if _, ok := kit.d.App.Directives.Queries["ntr-list"]; !ok {
		t.Fatalf("ReloadDirectives missed query ntr-list: %v", kit.d.App.Directives.QueryNames())
	}
	if !plugin.Installed(id) || !plugin.Enabled(id) {
		t.Fatalf("installed=%v enabled=%v", plugin.Installed(id), plugin.Enabled(id))
	}
	page.reload()
	found := false
	for _, row := range page.rows {
		if row.id == id {
			found = true
		}
	}
	if !found {
		t.Fatalf("installed plugin not listed: %+v", page.rows)
	}
	// After install, candidate should leave the picker set.
	cands, err = plugin.ListInstallCandidates(home)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cands {
		if c.ID == id && c.Source != "local" {
			t.Fatalf("installed plugin still in registry candidates: %+v", c)
		}
	}
	hints := page.Hints()
	joined := fmt.Sprint(hints)
	if !strings.Contains(joined, "install") || !strings.Contains(joined, "uninstall") {
		t.Fatalf("hints missing install/uninstall: %v", hints)
	}
	if !strings.Contains(joined, "enable/disable") {
		t.Fatalf("hints missing enable/disable: %v", hints)
	}
}
