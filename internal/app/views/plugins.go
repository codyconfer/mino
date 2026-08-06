package views

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/codyconfer/viewkit/forms"
	"github.com/codyconfer/viewkit/keys"
	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/ui"

	vkdeck "github.com/codyconfer/viewkit/deck"

	"github.com/codyconfer/mino/internal/keymap"
	mnotify "github.com/codyconfer/mino/internal/notify"
	"github.com/codyconfer/mino/internal/plugin"
	"github.com/codyconfer/mino/internal/render/glyph"
	"github.com/codyconfer/mino/internal/signals/build"
)

type pluginsToggledMsg struct {
	id  string
	on  bool
	err error
}

type pluginsInstalledMsg struct {
	id      string
	written int
	skipped int
	err     error
}

type pluginsUninstalledMsg struct {
	id      string
	removed int
	kept    int
	err     error
}

type pluginRow struct {
	id      string
	enabled bool
	desc    string
	problem string
	orphan  bool
	hue     int
}

type pluginsPage struct {
	kit     *Kit
	rows    []pluginRow
	cursor  int
	toast   *vkdeck.Toaster
	confirm *forms.Confirm
	pending string
}

func (k *Kit) Plugins() vkdeck.View {
	_ = build.KnownSignals()
	plugin.LoadEnabled()
	page := &pluginsPage{kit: k, toast: vkdeck.NewToaster(4, 3*time.Second)}
	page.reload()
	return page
}

func (p *pluginsPage) reload() {
	listed := plugin.ListInstalled()
	known := make(map[string]bool, len(listed))
	for _, row := range listed {
		known[row.ID] = true
	}
	p.rows = make([]pluginRow, 0, len(listed))
	for i, row := range listed {
		d, _ := plugin.Lookup(row.ID)
		caps := make([]string, len(d.Capabilities))
		for j, c := range d.Capabilities {
			caps[j] = string(c)
		}
		state := "enabled"
		if !row.Enabled {
			state = "disabled"
		}
		if plugin.IsInternal(row.ID) {
			state = "built-in · " + state
		}
		desc := fmt.Sprintf("%s  kind=%s", state, d.Kind)
		if d.Signal != "" {
			desc += " signal=" + d.Signal
		}
		if len(caps) > 0 {
			desc += " caps=[" + strings.Join(caps, ",") + "]"
		}
		p.rows = append(p.rows, pluginRow{
			id:      row.ID,
			enabled: row.Enabled,
			desc:    desc,
			problem: diagnosticSummary(plugin.DiagnosticsFor(row.ID)),
			hue:     i,
		})
	}
	p.rows = append(p.rows, orphanDiagnosticRows(known, len(p.rows))...)
	if p.cursor >= len(p.rows) {
		p.cursor = max(len(p.rows)-1, 0)
	}
	if len(p.rows) > 0 && p.rows[p.cursor].orphan {
		p.cursor = p.nextSelectable(p.cursor, 1)
	}
}

func diagnosticSummary(diags []plugin.Diagnostic) string {
	if len(diags) == 0 {
		return ""
	}
	msgs := make([]string, 0, len(diags))
	for _, d := range diags {
		msg := d.Message
		if d.Kind != "" && d.Ref != "" {
			msg = fmt.Sprintf("%s %q: %s", d.Kind, d.Ref, d.Message)
		}
		msgs = append(msgs, msg)
	}
	return strings.Join(msgs, "; ")
}

func orphanDiagnosticRows(known map[string]bool, hue int) []pluginRow {
	grouped := map[string][]plugin.Diagnostic{}
	var order []string
	for _, d := range plugin.Diagnostics() {
		if known[d.PluginID] {
			continue
		}
		id := d.PluginID
		if id == "" {
			id = "<unidentified plugin>"
		}
		if _, seen := grouped[id]; !seen {
			order = append(order, id)
		}
		grouped[id] = append(grouped[id], d)
	}
	out := make([]pluginRow, 0, len(order))
	for i, id := range order {
		out = append(out, pluginRow{
			id:      id,
			desc:    "not registered",
			problem: diagnosticSummary(grouped[id]),
			orphan:  true,
			hue:     hue + i,
		})
	}
	return out
}

func (p *pluginsPage) nextSelectable(from, step int) int {
	for i := from + step; i >= 0 && i < len(p.rows); i += step {
		if !p.rows[i].orphan {
			return i
		}
	}
	for i := from; i >= 0 && i < len(p.rows); i -= step {
		if !p.rows[i].orphan {
			return i
		}
	}
	return from
}

func (p *pluginsPage) selected() (pluginRow, bool) {
	if len(p.rows) == 0 || p.cursor < 0 || p.cursor >= len(p.rows) {
		return pluginRow{}, false
	}
	row := p.rows[p.cursor]
	if row.orphan {
		return pluginRow{}, false
	}
	return row, true
}

func (p *pluginsPage) Title() string                       { return "plugins" }
func (p *pluginsPage) Context(scope *ui.Scope) []keys.Hint { return p.kit.menuCtx() }
func (p *pluginsPage) Init() tea.Cmd                       { return nil }

func (p *pluginsPage) Hints(scope *ui.Scope) []keys.Hint {
	if p.confirm != nil {
		return []keys.Hint{{Key: "←/→", Label: "choose"}, {Key: "enter", Label: "confirm"}}
	}
	hints := []keys.Hint{
		{Key: "↑/↓", Label: "move"},
		{Key: "enter/d", Label: "enable/disable"},
		{Key: "i", Label: "install…"},
	}
	if row, ok := p.selected(); ok && !plugin.IsInternal(row.id) {
		hints = append(hints, keys.Hint{Key: "u", Label: "uninstall"})
	}
	return append(hints, keys.Hint{Key: "esc", Label: "back"})
}

func (p *pluginsPage) Update(a *vkdeck.Model, msg tea.Msg) tea.Cmd {
	if cmd, handled := p.toast.Update(msg); handled {
		return cmd
	}
	if key, ok := msg.(tea.KeyMsg); ok && p.confirm != nil {
		return p.answer(a, key)
	}
	switch m := msg.(type) {
	case pluginsToggledMsg:
		if m.err != nil {
			return a.Push(vkdeck.NewMessage("plugins", modelScope(a).Theme.Cant.Render(m.err.Error()), p.kit.menuCtx()))
		}
		p.reload()
		return p.toast.Push(mnotify.PluginToggled(m.id, m.on))
	case pluginsInstalledMsg:
		if m.err != nil {
			return a.Push(vkdeck.NewMessage("plugins", modelScope(a).Theme.Cant.Render(m.err.Error()), p.kit.menuCtx()))
		}
		p.reload()
		return p.toast.Push(mnotify.PluginInstalled(m.id, m.written, m.skipped))
	case pluginsUninstalledMsg:
		if m.err != nil {
			return a.Push(vkdeck.NewMessage("plugins", modelScope(a).Theme.Cant.Render(m.err.Error()), p.kit.menuCtx()))
		}
		p.reload()
		return p.toast.Push(mnotify.PluginUninstalled(m.id, m.removed, m.kept))
	case tea.KeyMsg:
		act, ok := keymap.Plugins(modelScope(a).Keys).Action(m.String())
		if !ok {
			return nil
		}
		switch act {
		case keys.Up:
			if p.cursor > 0 {
				p.cursor = p.nextSelectable(p.cursor, -1)
			}
		case keys.Down:
			if p.cursor < len(p.rows)-1 {
				p.cursor = p.nextSelectable(p.cursor, 1)
			}
		case keys.Confirm:
			row, ok := p.selected()
			if !ok {
				return nil
			}
			id, on := row.id, !row.enabled
			return func() tea.Msg {
				err := plugin.SetEnabled(id, on)
				return pluginsToggledMsg{id: id, on: on, err: err}
			}
		case keymap.PluginInstall:
			return a.Push(p.kit.pluginsInstallPicker())
		case keymap.PluginUninstall:
			row, ok := p.selected()
			if !ok || plugin.IsInternal(row.id) {
				return nil
			}
			p.ask(row.id)
			return nil
		case keys.Cancel:
			return a.Pop()
		}
	}
	return nil
}

func (p *pluginsPage) ask(id string) {
	p.pending = id
	p.confirm = &forms.Confirm{
		Title:    "uninstall " + id + "?",
		Message:  "This disables the plugin and removes the seed files it still owns.",
		YesLabel: "Uninstall",
		NoLabel:  "Keep",
	}
}

func (p *pluginsPage) answer(a *vkdeck.Model, key tea.KeyMsg) tea.Cmd {
	act, ok := keymap.ConfirmMap(modelScope(a).Keys).Action(key.String())
	if !ok {
		return nil
	}
	switch p.confirm.Handle(act) {
	case forms.Submitted:
		yes, id := p.confirm.Yes, p.pending
		p.confirm, p.pending = nil, ""
		if !yes {
			return nil
		}
		return p.uninstall(id)
	case forms.Cancelled:
		p.confirm, p.pending = nil, ""
	}
	return nil
}

func (p *pluginsPage) uninstall(id string) tea.Cmd {
	home := p.kit.d.App.Cfg.Home
	app := p.kit.d.App
	return func() tea.Msg {
		res, err := plugin.Uninstall(home, id, plugin.UninstallOptions{})
		if err != nil {
			return pluginsUninstalledMsg{id: id, err: err}
		}
		if err := app.ReloadDirectives(); err != nil {
			return pluginsUninstalledMsg{id: id, removed: len(res.Removed), kept: len(res.Kept), err: err}
		}
		return pluginsUninstalledMsg{id: id, removed: len(res.Removed), kept: len(res.Kept)}
	}
}

func (k *Kit) pluginsInstallPicker() vkdeck.View {
	home := k.d.App.Cfg.Home
	app := k.d.App
	cands, err := plugin.ListInstallCandidates(home)
	if err != nil {
		return vkdeck.NewMessage("install plugin", k.scope().Theme.Cant.Render(err.Error()), k.menuCtx())
	}
	var items []vkdeck.MenuItem
	for _, c := range cands {
		c := c
		if !c.Installable {
			items = append(items, vkdeck.MenuItem{
				Label: c.Label,
				Desc:  c.Desc,
				OnSelect: func(a *vkdeck.Model) tea.Cmd {
					return a.Push(vkdeck.NewMessage("install plugin",
						modelScope(a).Theme.Cant.Render(c.Label+": "+c.Reason), k.menuCtx()))
				},
			})
			continue
		}
		items = append(items, vkdeck.MenuItem{
			Label: c.Label,
			Desc:  c.Desc,
			OnSelect: func(a *vkdeck.Model) tea.Cmd {
				resize := a.Pop()
				return tea.Batch(resize, func() tea.Msg {
					res, err := plugin.InstallCandidateEntry(home, c, plugin.InstallOptions{})
					if err != nil {
						return pluginsInstalledMsg{id: c.ID, err: err}
					}
					if err := app.ReloadDirectives(); err != nil {
						return pluginsInstalledMsg{id: c.ID, written: len(res.Written), skipped: len(res.Skipped), err: err}
					}
					return pluginsInstalledMsg{id: c.ID, written: len(res.Written), skipped: len(res.Skipped)}
				})
			},
		})
	}
	if len(items) == 0 {
		items = append(items, vkdeck.MenuItem{
			Label: "(no install candidates)",
			Desc:  "register plugins or add seed packs under .plugins/",
		})
	}
	ctx := append(k.menuCtx(), keys.Hint{Key: "dir", Label: plugin.PluginsDir(home)})
	return vkdeck.NewMenu("install plugin", ctx, items...)
}

func (p *pluginsPage) Body(f layout.Frame) string {
	th := f.Theme()
	outer := f
	width := f.Width
	f = f.Screen()
	if len(p.rows) == 0 {
		body := f.TitledBox(strings.ToUpper(p.Title()), th.Dim.Render("(no managed plugins — press i to install)"))
		return p.overlay(p.toast.Body(outer, body), width)
	}
	lines := make([]string, 0, len(p.rows))
	for i, row := range p.rows {
		cursor := "  "
		label := th.Val.Render(row.id)
		if i == p.cursor {
			cursor = th.Key.Render("▸ ")
			label = th.Key.Render(row.id)
		}
		var line string
		switch {
		case row.orphan:
			line = "  " + th.Cant.Render(glyph.Lead(glyph.Warn())+row.id)
		default:
			icon := glyph.Cross()
			if row.enabled {
				icon = glyph.Check()
			}
			line = cursor + th.Icon(icon, row.hue) + label
		}
		if row.desc != "" {
			line = f.Spread(line, th.Dim.Render(row.desc))
		}
		lines = append(lines, line)
		if row.problem != "" {
			lines = append(lines, "    "+th.Cant.Render(glyph.Lead(glyph.Warn())+row.problem))
		}
		if i == p.cursor && i < len(p.rows)-1 {
			lines = append(lines, "")
		}
	}
	body := f.TitledBox(strings.ToUpper(p.Title()), lines...)
	return p.overlay(p.toast.Body(outer, body), width)
}

func (p *pluginsPage) overlay(body string, width int) string {
	if p.confirm == nil {
		return body
	}
	return p.confirm.Overlay(body, layout.NewFrame(layout.DialogWidth(width)))
}
