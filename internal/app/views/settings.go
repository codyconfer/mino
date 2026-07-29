package views

import (
	"context"
	"os/exec"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/codyconfer/viewkit/forms"
	"github.com/codyconfer/viewkit/keys"
	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/theme"

	sconfig "github.com/codyconfer/sisyphus/config"

	vkdeck "github.com/codyconfer/viewkit/deck"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/keymap"
	"github.com/codyconfer/munin/internal/plugin"
	"github.com/codyconfer/munin/internal/render"
)

func (k *Kit) Settings() vkdeck.View {
	return vkdeck.NewMenu("settings", k.setvCtx(), k.settingsMenuItems()...)
}

func (k *Kit) settingsMenuItems() []vkdeck.MenuItem {
	items := []vkdeck.MenuItem{
		{Label: "Edit config", Desc: "output, audit, timeout, backup", Do: func(a *vkdeck.Model) tea.Cmd {
			return a.Push(k.setvEditConfigView())
		}},
	}
	if k.setvHasConfigFile() {
		items = append(items,
			vkdeck.MenuItem{Label: "Delete config", Desc: "remove config.yaml/.yml/.json", Do: func(a *vkdeck.Model) tea.Cmd {
				return a.Push(k.setvDeleteConfirmView())
			}},
			vkdeck.MenuItem{Label: "Import config", Desc: "overwrite DuckDB with on-disk config", Do: func(a *vkdeck.Model) tea.Cmd {
				return a.Push(k.setvImportConfirmView())
			}},
		)
	} else {
		items = append(items, vkdeck.MenuItem{
			Label: "Create config",
			Desc:  "write a default config.yaml",
			Do:    k.setvCreateConfig,
		})
	}
	items = append(items,
		vkdeck.MenuItem{Label: "Export config", Desc: "write DuckDB stores back to disk", Do: func(a *vkdeck.Model) tea.Cmd {
			return a.Push(k.setvExportConfirmView())
		}},
		vkdeck.MenuItem{Label: "Open config in editor", Desc: "open on-disk config with $EDITOR", Do: k.setvOpenConfigInEditor},
		vkdeck.MenuItem{Label: "Appearance", Desc: "theme and key scheme", Do: func(a *vkdeck.Model) tea.Cmd {
			return a.Push(k.setvAppearanceView())
		}},
		vkdeck.MenuItem{Label: "Status bar", Desc: "hide or show chips; plugins stay enabled", Do: func(a *vkdeck.Model) tea.Cmd {
			return a.Push(k.setvStatusBarView())
		}},
	)
	return items
}

func (k *Kit) setvCtx() [][2]string {
	return k.menuCtx()
}

func (k *Kit) setvHome() string {
	if k.d.App != nil && k.d.App.Cfg != nil {
		return k.d.App.Cfg.Home
	}
	return ""
}

func (k *Kit) setvHasConfigFile() bool {
	_, raw, _, err := config.ReadConfigFile(k.setvHome())
	return err == nil && len(raw) > 0
}

func (k *Kit) setvFinish(a *vkdeck.Model, pops int, title, body string) tea.Cmd {
	for i := 0; i < pops; i++ {
		_ = a.Pop()
	}
	_ = a.Push(k.Settings())
	return a.Push(vkdeck.NewMessage(title, body, k.setvCtx()))
}

func (k *Kit) setvRed(title, msg string) vkdeck.View {
	return vkdeck.NewMessage(title, theme.Cur().Cant.Render(msg), k.setvCtx())
}

func (k *Kit) setvEditConfigView() vkdeck.View {
	c := k.d.App.Cfg
	fields := []forms.Field{
		{Key: "output", Label: "output", Kind: forms.FieldSelect, Options: forms.SelectFirst([]string{"terminal", "json"}, c.Output)},
		{Key: "audit.enabled", Label: "audit.enabled", Kind: forms.FieldToggle, On: c.Audit.Enabled},
		{Key: "timeout", Label: "timeout", Kind: forms.FieldText, Text: c.Timeout},
		{Key: "cache.ttl", Label: "cache.ttl", Kind: forms.FieldText, Text: c.Cache.TTL},
		{Key: "backup.destination", Label: "backup.destination", Kind: forms.FieldSelect, Options: forms.SelectFirst([]string{"local", "gdrive"}, c.Backup.Destination)},
		{Key: "backup.keep", Label: "backup.keep", Kind: forms.FieldText, Text: strconv.Itoa(c.Backup.Keep)},
		{Key: "daemon.interval", Label: "daemon.interval", Kind: forms.FieldText, Text: c.Daemon.Interval},
		{Key: "daemon.bell", Label: "daemon.bell", Kind: forms.FieldToggle, On: c.Daemon.Bell},
		{Key: "daemon.desktop", Label: "daemon.desktop", Kind: forms.FieldToggle, On: c.Daemon.Desktop},
		{Key: "daemon.theme", Label: "daemon.theme", Kind: forms.FieldSelect, Options: forms.SelectFirst([]string{"dark", "light"}, c.Daemon.Theme)},
	}
	fields = append(fields, sectionFields(c)...)
	return vkdeck.NewFormView(vkdeck.FormSpec{
		Title:       "edit config",
		Fields:      fields,
		Keys:        vkdeck.FormKeys{Map: keymap.Form(), Save: keymap.Save},
		ContextFunc: k.setvCtx,
		Hints:       [][2]string{{"↑/↓", "field"}, {"←/→", "change"}, {"ctrl+s", "save"}},
		OnSubmit:    k.setvSaveConfig,
	})
}

func (k *Kit) setvSaveConfig(a *vkdeck.Model, vals map[string]any) tea.Cmd {
	keep := forms.Int(vals, "backup.keep")
	set := map[string]any{
		"output":             forms.Str(vals, "output"),
		"timeout":            forms.Str(vals, "timeout"),
		"cache.ttl":          forms.Str(vals, "cache.ttl"),
		"audit.enabled":      forms.Bool(vals, "audit.enabled"),
		"backup.destination": forms.Str(vals, "backup.destination"),
		"backup.keep":        keep,
		"daemon.interval":    forms.Str(vals, "daemon.interval"),
		"daemon.bell":        forms.Bool(vals, "daemon.bell"),
		"daemon.desktop":     forms.Bool(vals, "daemon.desktop"),
		"daemon.theme":       forms.Str(vals, "daemon.theme"),
	}
	sectionValues(vals, set)
	path, err := config.SetValues(k.setvHome(), set)
	if err != nil {
		return a.Push(k.setvRed("edit config", err.Error()))
	}
	pop := a.Pop()
	push := a.Push(vkdeck.NewMessage("edit config", "wrote "+path, k.setvCtx()))
	return tea.Batch(pop, push)
}

func (k *Kit) setvAppearanceView() vkdeck.View {
	gs := config.LoadGlobalSettings()
	th := gs.Theme
	if th == "" {
		th = render.DefaultThemeKey
	}
	ky := gs.Keys
	if ky == "" {
		ky = keymap.DefaultSchemeKey
	}
	return vkdeck.NewFormView(vkdeck.FormSpec{
		Title: "appearance",
		Fields: []forms.Field{
			{Key: "theme", Label: "theme", Kind: forms.FieldSelect, Options: forms.SelectFirst(theme.Keys(), th)},
			{Key: "keys", Label: "keys", Kind: forms.FieldSelect, Options: forms.SelectFirst(keys.Keys(), ky)},
		},
		Keys:        vkdeck.FormKeys{Map: keymap.Form(), Save: keymap.Save},
		ContextFunc: k.setvCtx,
		Hints:       [][2]string{{"↑/↓", "field"}, {"←/→", "change"}, {"ctrl+s", "save"}},
		OnSubmit:    k.setvSaveAppearance,
	})
}

func (k *Kit) setvSaveAppearance(a *vkdeck.Model, vals map[string]any) tea.Cmd {
	gs := config.LoadGlobalSettings()
	gs.Theme = forms.Str(vals, "theme")
	gs.Keys = forms.Str(vals, "keys")
	if err := config.SaveGlobalSettings(gs); err != nil {
		return a.Push(k.setvRed("appearance", err.Error()))
	}
	if t, ok := theme.Named(gs.Theme); ok {
		theme.Use(t)
	}
	if sc, ok := keys.Named(gs.Keys); ok {
		keys.Use(sc)
	}
	body := "theme: " + theme.DisplayName(gs.Theme) + "\nkeys:  " + keys.DisplayName(gs.Keys)
	pop := a.Pop()
	push := a.Push(vkdeck.NewMessage("appearance", body, k.setvCtx()))
	return tea.Batch(pop, push)
}

type statusBarEntry struct{ id, label string }

var statusBarBuiltinEntries = []statusBarEntry{
	{"github", "github"},
	{"slack", "slack"},
	{"google", "google"},
}

func (k *Kit) setvStatusBarEntries() []statusBarEntry {
	entries := make([]statusBarEntry, 0, len(statusBarBuiltinEntries)+8)
	entries = append(entries, statusBarBuiltinEntries...)
	entries = append(entries, sectionStatusBarEntries()...)
	home, role := "", ""
	if k.d.App != nil && k.d.App.Cfg != nil {
		home, role = k.d.App.Cfg.Home, k.d.App.Cfg.Role
	}
	seen := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		seen[e.id] = struct{}{}
	}
	for _, id := range plugin.StatusContributionIDs() {
		if _, ok := seen[id]; ok {
			continue
		}
		label := id
		if c, ok := plugin.LookupStatusContribution(id, home, role); ok && c.Info != nil {
			if info := c.Info(); info != "" {
				label = info
			}
		}
		entries = append(entries, statusBarEntry{id, label})
		seen[id] = struct{}{}
	}
	return entries
}

func (k *Kit) setvStatusBarView() vkdeck.View {
	entries := k.setvStatusBarEntries()
	fields := make([]forms.Field, 0, len(entries))
	for _, e := range entries {
		fields = append(fields, forms.Field{
			Key:   e.id,
			Label: e.label,
			Kind:  forms.FieldToggle,
			On:    !config.StatusBarHidden(e.id),
		})
	}
	return vkdeck.NewFormView(vkdeck.FormSpec{
		Title:       "status bar",
		PanelTitle:  "status bar (show = visible chip)",
		Fields:      fields,
		Keys:        vkdeck.FormKeys{Map: keymap.Form(), Save: keymap.Save},
		ContextFunc: k.setvCtx,
		Hints:       [][2]string{{"↑/↓", "field"}, {"←/→", "show/hide"}, {"ctrl+s", "save"}},
		OnSubmit: func(a *vkdeck.Model, vals map[string]any) tea.Cmd {
			return k.setvSaveStatusBar(a, entries, vals)
		},
	})
}

func (k *Kit) setvSaveStatusBar(a *vkdeck.Model, entries []statusBarEntry, vals map[string]any) tea.Cmd {
	hidden := make([]string, 0, len(entries))
	shown := make([]string, 0, len(entries))
	for _, e := range entries {
		if forms.Bool(vals, e.id) {
			shown = append(shown, e.label)
			continue
		}
		hidden = append(hidden, e.id)
	}
	if err := config.SetHiddenStatusBar(hidden); err != nil {
		return a.Push(k.setvRed("status bar", err.Error()))
	}
	body := "all status chips shown"
	if len(hidden) > 0 {
		body = "hidden: " + strings.Join(hidden, ", ")
		if len(shown) > 0 {
			body += "\nshown:  " + strings.Join(shown, ", ")
		}
	}
	pop := a.Pop()
	push := a.Push(vkdeck.NewMessage("status bar", body, k.setvCtx()))
	return tea.Batch(pop, push, a.RefreshStatus())
}

func (k *Kit) setvCreateConfig(a *vkdeck.Model) tea.Cmd {
	home, raw, _, err := config.ReadConfigFile(k.setvHome())
	if err != nil {
		return a.Push(k.setvRed("create config", err.Error()))
	}
	if len(raw) > 0 {
		return a.Push(vkdeck.NewMessage("create config", "config file already exists", k.setvCtx()))
	}
	path, err := sconfig.WriteConfigFile(home, []byte("output: terminal\naudit:\n  enabled: true\n"), "yaml")
	if err != nil {
		return a.Push(k.setvRed("create config", err.Error()))
	}
	return k.setvFinish(a, 1, "create config", "wrote "+path)
}

func (k *Kit) setvDeleteConfirmView() vkdeck.View {
	return vkdeck.NewMenu("delete config?", k.setvCtx(),
		vkdeck.MenuItem{Label: "No, keep it", Desc: "keep the config file", Do: func(a *vkdeck.Model) tea.Cmd {
			return a.Pop()
		}},
		vkdeck.MenuItem{Label: "Yes, delete", Desc: "remove config.yaml/.yml/.json", Do: func(a *vkdeck.Model) tea.Cmd {
			removed := setvDeleteConfigFiles(k.setvHome())
			body := "no config file found"
			if len(removed) > 0 {
				body = "removed:\n" + strings.Join(removed, "\n")
			}
			return k.setvFinish(a, 2, "delete config", body)
		}},
	)
}

func setvDeleteConfigFiles(home string) []string {
	removed, _ := sconfig.RemoveFiles(home, "config", nil)
	return removed
}

func (k *Kit) setvImportConfirmView() vkdeck.View {
	return vkdeck.NewMenu("import config?", k.setvCtx(),
		vkdeck.MenuItem{Label: "No, cancel", Desc: "leave DuckDB unchanged", Do: func(a *vkdeck.Model) tea.Cmd {
			return a.Pop()
		}},
		vkdeck.MenuItem{Label: "Yes, import", Desc: "overwrite DuckDB with on-disk config", Do: k.setvImportConfig},
	)
}

func (k *Kit) setvImportConfig(a *vkdeck.Model) tea.Cmd {
	mgr := k.d.App.Mgr
	if mgr == nil {
		return a.Push(k.setvRed("import config", "config DB unavailable"))
	}
	_, raw, format, err := config.ReadConfigFile(k.setvHome())
	if err != nil {
		return a.Push(k.setvRed("import config", err.Error()))
	}
	if len(raw) == 0 {
		return a.Push(k.setvRed("import config", "no config file on disk"))
	}
	if err := mgr.DB().Import(context.Background(), "config", raw, format); err != nil {
		return a.Push(k.setvRed("import config", err.Error()))
	}
	return k.setvFinish(a, 2, "import config", "imported config into DuckDB")
}

func (k *Kit) setvExportConfirmView() vkdeck.View {
	return vkdeck.NewMenu("export config?", k.setvCtx(),
		vkdeck.MenuItem{Label: "No, cancel", Desc: "leave files unchanged", Do: func(a *vkdeck.Model) tea.Cmd {
			return a.Pop()
		}},
		vkdeck.MenuItem{Label: "Yes, export", Desc: "write DuckDB stores back to disk", Do: k.setvExportDirectives},
	)
}

func (k *Kit) setvExportDirectives(a *vkdeck.Model) tea.Cmd {
	written, err := k.d.ExportDirectives()
	if err != nil {
		return a.Push(k.setvRed("export config", err.Error()))
	}
	body := "nothing to export"
	if len(written) > 0 {
		body = "wrote:\n" + strings.Join(written, "\n")
	}
	return k.setvFinish(a, 2, "export config", body)
}

func (k *Kit) setvOpenConfigInEditor(a *vkdeck.Model) tea.Cmd {
	path, err := config.ConfigFilePath(k.setvHome())
	if err != nil {
		return a.Push(k.setvRed("open config", err.Error()))
	}
	cmd, _, err := config.EditorCmd(path)
	if err != nil {
		return a.Push(k.setvRed("open config", err.Error()))
	}
	return a.Push(&setvEditorView{k: k, path: path, cmd: cmd})
}

type setvEditorView struct {
	k    *Kit
	path string
	cmd  *exec.Cmd
}

type setvEditorDoneMsg struct {
	err  error
	path string
}

func (v *setvEditorView) Title() string        { return "open config" }
func (v *setvEditorView) Context() [][2]string { return v.k.setvCtx() }
func (v *setvEditorView) Hints() [][2]string   { return nil }

func (v *setvEditorView) Init() tea.Cmd {
	return tea.ExecProcess(v.cmd, func(err error) tea.Msg {
		return setvEditorDoneMsg{err: err, path: v.path}
	})
}

func (v *setvEditorView) Body(width, _ int) string {
	f := layout.NewFrame(width)
	return f.TitledBox("OPEN CONFIG", theme.Cur().Dim.Render("opening "+v.path+" in $EDITOR…"))
}

func (v *setvEditorView) Update(a *vkdeck.Model, msg tea.Msg) tea.Cmd {
	done, ok := msg.(setvEditorDoneMsg)
	if !ok {
		return nil
	}
	pop := a.Pop()
	var push tea.Cmd
	if done.err != nil {
		push = a.Push(v.k.setvRed("open config", done.err.Error()))
	} else {
		push = a.Push(vkdeck.NewMessage("open config", "opened "+done.path, v.k.setvCtx()))
	}
	return tea.Batch(pop, push)
}
