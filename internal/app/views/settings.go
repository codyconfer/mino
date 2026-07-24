package views

import (
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/codyconfer/viewkit/forms"
	"github.com/codyconfer/viewkit/keys"
	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/theme"

	sconfig "github.com/codyconfer/sisyphus/config"
	"github.com/codyconfer/sisyphus/redact"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/deck"
	"github.com/codyconfer/munin/internal/keymap"
	"github.com/codyconfer/munin/internal/render"
)

func (k *Kit) Settings() deck.View {
	return deck.NewMenu("settings tools", k.setvCtx(),
		deck.MenuItem{Label: "Edit config", Desc: "output, audit, timeout, backup", Do: func(a *deck.State) tea.Cmd {
			return a.Push(k.setvEditConfigView())
		}},
		deck.MenuItem{Label: "Create config file", Desc: "write a default config.yaml", Do: k.setvCreateConfig},
		deck.MenuItem{Label: "Delete config file", Desc: "remove config.yaml/.yml/.json", Do: func(a *deck.State) tea.Cmd {
			return a.Push(k.setvDeleteConfirmView())
		}},
		deck.MenuItem{Label: "Overwrite DuckDB with file", Desc: "import on-disk config into DuckDB", Do: k.setvImportConfig},
		deck.MenuItem{Label: "Export DuckDB → files", Desc: "write DuckDB stores back to disk", Do: k.setvExportDirectives},
		deck.MenuItem{Label: "Show active config", Desc: "config stored in DuckDB", Do: func(a *deck.State) tea.Cmd {
			return a.Push(k.setvShowConfigView())
		}},
		deck.MenuItem{Label: "Appearance", Desc: "theme and key scheme", Do: func(a *deck.State) tea.Cmd {
			return a.Push(k.setvAppearanceView())
		}},
	)
}

func (k *Kit) setvCtx() [][2]string {
	return k.menuCtx()
}

func (k *Kit) setvRed(title, msg string) deck.View {
	return deck.NewMessage(title, theme.Cur().Cant.Render(msg), k.setvCtx())
}

func setvString(v any) string { s, _ := v.(string); return s }
func setvBool(v any) bool     { b, _ := v.(bool); return b }

func setvFirst(opts []string, cur string) []string {
	found := false
	rest := make([]string, 0, len(opts))
	for _, o := range opts {
		if o == cur {
			found = true
			continue
		}
		rest = append(rest, o)
	}
	if !found {
		return opts
	}
	return append([]string{cur}, rest...)
}

type setvEditForm struct {
	k    *Kit
	form *forms.Form
}

func (k *Kit) setvEditConfigView() deck.View {
	c := k.d.App.Cfg
	form := forms.NewForm(
		forms.Field{Key: "output", Label: "output", Kind: forms.FieldSelect, Options: setvFirst([]string{"terminal", "json"}, c.Output)},
		forms.Field{Key: "audit.enabled", Label: "audit.enabled", Kind: forms.FieldToggle, On: c.Audit.Enabled},
		forms.Field{Key: "timeout", Label: "timeout", Kind: forms.FieldText, Text: c.Timeout},
		forms.Field{Key: "backup.destination", Label: "backup.destination", Kind: forms.FieldSelect, Options: setvFirst([]string{"local", "gdrive"}, c.Backup.Destination)},
		forms.Field{Key: "backup.keep", Label: "backup.keep", Kind: forms.FieldText, Text: strconv.Itoa(c.Backup.Keep)},
		forms.Field{Key: "daemon.interval", Label: "daemon.interval", Kind: forms.FieldText, Text: c.Daemon.Interval},
		forms.Field{Key: "daemon.bell", Label: "daemon.bell", Kind: forms.FieldToggle, On: c.Daemon.Bell},
		forms.Field{Key: "daemon.desktop", Label: "daemon.desktop", Kind: forms.FieldToggle, On: c.Daemon.Desktop},
		forms.Field{Key: "daemon.tray", Label: "daemon.tray", Kind: forms.FieldToggle, On: c.Daemon.Tray},
		forms.Field{Key: "daemon.theme", Label: "daemon.theme", Kind: forms.FieldSelect, Options: setvFirst([]string{"dark", "light"}, c.Daemon.Theme)},
	)
	return &setvEditForm{k: k, form: form}
}

func (v *setvEditForm) Title() string        { return "edit config" }
func (v *setvEditForm) Init() tea.Cmd        { return nil }
func (v *setvEditForm) Context() [][2]string { return v.k.setvCtx() }
func (v *setvEditForm) Hints() [][2]string {
	return [][2]string{{"↑/↓", "field"}, {"←/→", "change"}, {"ctrl+s", "save"}}
}

func (v *setvEditForm) Body(width, _ int) string {
	return v.form.Render(layout.NewFrame(width), "edit config")
}

func (v *setvEditForm) Update(a *deck.State, msg tea.Msg) tea.Cmd {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil
	}
	act, ok := keymap.Form().Action(key.String())
	if !ok {
		if key.Type == tea.KeyRunes {
			v.form.Insert(string(key.Runes))
		}
		return nil
	}
	switch act {
	case keys.Cancel:
		return a.Pop()
	case keymap.Save:
		return v.submit(a)
	default:
		v.form.Handle(act)
	}
	return nil
}

func (v *setvEditForm) submit(a *deck.State) tea.Cmd {
	vals := v.form.Values()
	keep, _ := strconv.Atoi(setvString(vals["backup.keep"]))
	path, err := config.SetValues("", map[string]any{
		"output":             setvString(vals["output"]),
		"timeout":            setvString(vals["timeout"]),
		"audit.enabled":      setvBool(vals["audit.enabled"]),
		"backup.destination": setvString(vals["backup.destination"]),
		"backup.keep":        keep,
		"daemon.interval":    setvString(vals["daemon.interval"]),
		"daemon.bell":        setvBool(vals["daemon.bell"]),
		"daemon.desktop":     setvBool(vals["daemon.desktop"]),
		"daemon.tray":        setvBool(vals["daemon.tray"]),
		"daemon.theme":       setvString(vals["daemon.theme"]),
	})
	if err != nil {
		return a.Push(v.k.setvRed("edit config", err.Error()))
	}
	pop := a.Pop()
	push := a.Push(deck.NewMessage("edit config", "wrote "+path, v.k.setvCtx()))
	return tea.Batch(pop, push)
}

type setvAppearanceForm struct {
	k    *Kit
	form *forms.Form
}

func (k *Kit) setvAppearanceView() deck.View {
	gs := config.LoadGlobalSettings()
	th := gs.Theme
	if th == "" {
		th = render.DefaultThemeKey
	}
	ky := gs.Keys
	if ky == "" {
		ky = keymap.DefaultSchemeKey
	}
	form := forms.NewForm(
		forms.Field{Key: "theme", Label: "theme", Kind: forms.FieldSelect, Options: setvFirst(theme.Keys(), th)},
		forms.Field{Key: "keys", Label: "keys", Kind: forms.FieldSelect, Options: setvFirst(keys.Keys(), ky)},
	)
	return &setvAppearanceForm{k: k, form: form}
}

func (v *setvAppearanceForm) Title() string        { return "appearance" }
func (v *setvAppearanceForm) Init() tea.Cmd        { return nil }
func (v *setvAppearanceForm) Context() [][2]string { return v.k.setvCtx() }
func (v *setvAppearanceForm) Hints() [][2]string {
	return [][2]string{{"↑/↓", "field"}, {"←/→", "change"}, {"ctrl+s", "save"}}
}

func (v *setvAppearanceForm) Body(width, _ int) string {
	return v.form.Render(layout.NewFrame(width), "appearance")
}

func (v *setvAppearanceForm) Update(a *deck.State, msg tea.Msg) tea.Cmd {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil
	}
	act, ok := keymap.Form().Action(key.String())
	if !ok {
		return nil
	}
	switch act {
	case keys.Cancel:
		return a.Pop()
	case keymap.Save:
		return v.submit(a)
	default:
		v.form.Handle(act)
	}
	return nil
}

func (v *setvAppearanceForm) submit(a *deck.State) tea.Cmd {
	vals := v.form.Values()
	gs := config.LoadGlobalSettings()
	gs.Theme = setvString(vals["theme"])
	gs.Keys = setvString(vals["keys"])
	if err := config.SaveGlobalSettings(gs); err != nil {
		return a.Push(v.k.setvRed("appearance", err.Error()))
	}
	if t, ok := theme.Named(gs.Theme); ok {
		theme.Use(t)
	}
	if sc, ok := keys.Named(gs.Keys); ok {
		keys.Use(sc)
	}
	body := "theme: " + theme.DisplayName(gs.Theme) + "\nkeys:  " + keys.DisplayName(gs.Keys)
	pop := a.Pop()
	push := a.Push(deck.NewMessage("appearance", body, v.k.setvCtx()))
	return tea.Batch(pop, push)
}

func (k *Kit) setvCreateConfig(a *deck.State) tea.Cmd {
	home, raw, _, err := config.ReadConfigFile("")
	if err != nil {
		return a.Push(k.setvRed("create config", err.Error()))
	}
	if len(raw) > 0 {
		return a.Push(deck.NewMessage("create config", "config file already exists", k.setvCtx()))
	}
	path, err := sconfig.WriteConfigFile(home, []byte("output: terminal\naudit:\n  enabled: true\n"), "yaml")
	if err != nil {
		return a.Push(k.setvRed("create config", err.Error()))
	}
	return a.Push(deck.NewMessage("create config", "wrote "+path, k.setvCtx()))
}

func (k *Kit) setvDeleteConfirmView() deck.View {
	return deck.NewMenu("delete config?", k.setvCtx(),
		deck.MenuItem{Label: "No", Desc: "keep the config file", Do: func(a *deck.State) tea.Cmd {
			return a.Pop()
		}},
		deck.MenuItem{Label: "Yes, delete", Desc: "remove config.yaml/.yml/.json", Do: func(a *deck.State) tea.Cmd {
			removed := setvDeleteConfigFiles(k.d.App.Cfg.Home)
			body := "no config file found"
			if len(removed) > 0 {
				body = "removed:\n" + strings.Join(removed, "\n")
			}
			pop := a.Pop()
			push := a.Push(deck.NewMessage("delete config", body, k.setvCtx()))
			return tea.Batch(pop, push)
		}},
	)
}

func setvDeleteConfigFiles(home string) []string {
	removed, _ := sconfig.RemoveFiles(home, "config", nil)
	return removed
}

func (k *Kit) setvImportConfig(a *deck.State) tea.Cmd {
	mgr := k.d.App.Mgr
	if mgr == nil {
		return a.Push(k.setvRed("overwrite DuckDB", "config DB unavailable"))
	}
	_, raw, format, err := config.ReadConfigFile("")
	if err != nil {
		return a.Push(k.setvRed("overwrite DuckDB", err.Error()))
	}
	if len(raw) == 0 {
		return a.Push(k.setvRed("overwrite DuckDB", "no config file on disk"))
	}
	if err := mgr.DB().Import("config", raw, format); err != nil {
		return a.Push(k.setvRed("overwrite DuckDB", err.Error()))
	}
	return a.Push(deck.NewMessage("overwrite DuckDB", "imported config into DuckDB", k.setvCtx()))
}

func (k *Kit) setvExportDirectives(a *deck.State) tea.Cmd {
	written, err := k.d.ExportDirectives()
	if err != nil {
		return a.Push(k.setvRed("export DuckDB", err.Error()))
	}
	body := "nothing to export"
	if len(written) > 0 {
		body = "wrote:\n" + strings.Join(written, "\n")
	}
	return a.Push(deck.NewMessage("export DuckDB", body, k.setvCtx()))
}

func (k *Kit) setvShowConfigView() deck.View {
	return deck.NewContent("active config", k.setvCtx(), nil, func() string {
		mgr := k.d.App.Mgr
		if mgr == nil {
			return theme.Cur().Cant.Render("config DB unavailable")
		}
		cur, ok, err := mgr.DB().Current("config")
		if err != nil {
			return theme.Cur().Cant.Render("error: " + err.Error())
		}
		if !ok {
			return theme.Cur().Dim.Render("no stored config")
		}
		return layout.NewFrame(theme.BodyWidth).Panel("config ("+cur.Format+")", strings.Split(redact.Config([]byte(cur.Content), cur.Format), "\n")...)
	})
}
