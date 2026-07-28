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

func TestPluginsListsInternalAsBuiltIn(t *testing.T) {
	pluginsTestEnv(t)
	kit := testKit(t)
	id := "munin.ntr"
	if _, ok := plugin.Lookup(id); !ok {
		t.Skip("munin.ntr not linked")
	}
	page := kit.Plugins().(*pluginsPage)
	if len(page.rows) == 0 {
		t.Fatal("expected built-in plugins listed without install")
	}
	found := false
	for _, row := range page.rows {
		if row.id != id {
			continue
		}
		found = true
		if !row.enabled {
			t.Fatal("expected internal plugin enabled by default")
		}
		if !strings.Contains(row.desc, "built-in") {
			t.Fatalf("internal row missing built-in state: %q", row.desc)
		}
		if strings.Contains(row.desc, "not installed") {
			t.Fatalf("internal row must not show not installed: %q", row.desc)
		}
	}
	if !found {
		t.Fatalf("missing internal %s in rows: %+v", id, page.rows)
	}
	body := pluginsAnsi.ReplaceAllString(page.Body(100, 40), "")
	if !strings.Contains(body, id) || !strings.Contains(body, "built-in") {
		t.Fatalf("body missing built-in internal plugin:\n%s", body)
	}
	cands, err := plugin.ListInstallCandidates(kit.d.App.Cfg.Home)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cands {
		if c.ID == id && c.Source != "local" {
			t.Fatalf("internal plugin offered as install candidate: %+v", c)
		}
	}
	for i, row := range page.rows {
		if row.id == id {
			page.cursor = i
			break
		}
	}
	joined := fmt.Sprint(page.Hints())
	if strings.Contains(joined, "uninstall") {
		t.Fatalf("uninstall hint should be hidden for internal: %v", page.Hints())
	}
}

func TestPluginsListsInternalFirst(t *testing.T) {
	pluginsTestEnv(t)
	kit := testKit(t)
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

func TestPluginsUninstallRemovesExternalFromListAndReinstallRestores(t *testing.T) {
	pluginsTestEnv(t)
	kit := testKit(t)
	id := "test.views.uninstall"
	if _, ok := plugin.Lookup(id); !ok {
		plugin.Register(plugin.Descriptor{
			ID: id, Kind: plugin.KindSignal, Signal: "testviewsuninstall",
			Capabilities: []plugin.Capability{plugin.CapQuery},
		})
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
	joined := fmt.Sprint(page.Hints())
	if !strings.Contains(joined, "uninstall") {
		t.Fatalf("uninstall hint missing for external: %v", page.Hints())
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
	for _, row := range page.rows {
		if row.id == id {
			t.Fatalf("external still listed after uninstall: %+v", page.rows)
		}
	}

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

func TestPluginsUninstallIgnoredForInternal(t *testing.T) {
	pluginsTestEnv(t)
	kit := testKit(t)
	id := "munin.ntr"
	if _, ok := plugin.Lookup(id); !ok {
		t.Skip("munin.ntr not linked")
	}
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
	if cmd != nil {
		t.Fatal("uninstall must be a no-op for built-in plugins")
	}
	if !plugin.Installed(id) || !plugin.Enabled(id) {
		t.Fatalf("internal state changed: installed=%v enabled=%v", plugin.Installed(id), plugin.Enabled(id))
	}
	_ = a
}

func TestPluginsTogglePersists(t *testing.T) {
	pluginsTestEnv(t)
	kit := testKit(t)
	id := "munin.demo"
	if _, ok := plugin.Lookup(id); !ok {
		t.Skip("munin.demo not linked")
	}

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
	id := "test.views.install"
	if _, ok := plugin.Lookup(id); !ok {
		plugin.Register(plugin.Descriptor{
			ID: id, Kind: plugin.KindSignal, Signal: "testviewsinstall",
			Capabilities: []plugin.Capability{plugin.CapQuery},
		})
	}
	plugin.RegisterSeeds(id, []plugin.FileSeed{
		{RelPath: "queries/test-views-install.yaml", Content: []byte("name: test-views-install\nsignal: testviewsinstall\n")},
	})
	t.Cleanup(func() { plugin.RegisterSeeds(id, nil) })

	page := kit.Plugins().(*pluginsPage)
	for _, row := range page.rows {
		if row.id == id {
			t.Fatalf("external should not be listed before install: %+v", page.rows)
		}
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
	if strings.Contains(pickerBody, "munin.ntr") {
		t.Fatalf("built-in munin.ntr must not appear in install picker:\n%s", pickerBody)
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
		t.Fatalf("%s missing from install candidates: %+v", id, cands)
	}
	res, err := plugin.InstallCandidateEntry(home, cand, plugin.InstallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.ReloadDirectives(); err != nil {
		t.Fatal(err)
	}
	_ = step(a, pluginsInstalledMsg{id: id, written: len(res.Written), skipped: len(res.Skipped)})

	if _, err := os.Stat(filepath.Join(home, "queries", "test-views-install.yaml")); err != nil {
		t.Fatalf("seed missing: %v", err)
	}
	if _, ok := kit.d.App.Directives.Queries["test-views-install"]; !ok {
		t.Fatalf("ReloadDirectives missed query: %v", kit.d.App.Directives.QueryNames())
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
	cands, err = plugin.ListInstallCandidates(home)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cands {
		if c.ID == id && c.Source != "local" {
			t.Fatalf("installed plugin still in registry candidates: %+v", c)
		}
	}
	for i, row := range page.rows {
		if row.id == id {
			page.cursor = i
			break
		}
	}
	hints := page.Hints()
	joined := fmt.Sprint(hints)
	if !strings.Contains(joined, "install") || !strings.Contains(joined, "uninstall") {
		t.Fatalf("hints missing install/uninstall for external: %v", hints)
	}
	if !strings.Contains(joined, "enable/disable") {
		t.Fatalf("hints missing enable/disable: %v", hints)
	}
}
