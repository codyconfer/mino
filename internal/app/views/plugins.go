package views

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/codyconfer/viewkit/keys"
	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/theme"

	vkdeck "github.com/codyconfer/viewkit/deck"

	"github.com/codyconfer/munin/internal/keymap"
	mnotify "github.com/codyconfer/munin/internal/notify"
	"github.com/codyconfer/munin/internal/plugin"
	"github.com/codyconfer/munin/internal/render/glyph"
	"github.com/codyconfer/munin/internal/signals/build"
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
	hue     int
}

type pluginsPage struct {
	kit    *Kit
	rows   []pluginRow
	cursor int
	toast  *vkdeck.Toaster
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
			hue:     i,
		})
	}
	if p.cursor >= len(p.rows) {
		p.cursor = max(len(p.rows)-1, 0)
	}
}

func (p *pluginsPage) Title() string        { return "plugins" }
func (p *pluginsPage) Context() [][2]string { return p.kit.menuCtx() }
func (p *pluginsPage) Init() tea.Cmd        { return nil }

func (p *pluginsPage) Hints() [][2]string {
	hints := [][2]string{
		{"↑/↓", "move"},
		{"enter/d", "enable/disable"},
		{"i", "install…"},
	}
	if len(p.rows) > 0 && !plugin.IsInternal(p.rows[p.cursor].id) {
		hints = append(hints, [2]string{"u", "uninstall"})
	}
	return append(hints, [2]string{"esc", "back"})
}

func (p *pluginsPage) Update(a *vkdeck.Model, msg tea.Msg) tea.Cmd {
	if cmd, handled := p.toast.Update(msg); handled {
		return cmd
	}
	switch m := msg.(type) {
	case pluginsToggledMsg:
		if m.err != nil {
			return a.Push(vkdeck.NewMessage("plugins", theme.Cur().Cant.Render(m.err.Error()), p.kit.menuCtx()))
		}
		p.reload()
		return p.toast.Push(mnotify.PluginToggled(m.id, m.on))
	case pluginsInstalledMsg:
		if m.err != nil {
			return a.Push(vkdeck.NewMessage("plugins", theme.Cur().Cant.Render(m.err.Error()), p.kit.menuCtx()))
		}
		p.reload()
		return p.toast.Push(mnotify.PluginInstalled(m.id, m.written, m.skipped))
	case pluginsUninstalledMsg:
		if m.err != nil {
			return a.Push(vkdeck.NewMessage("plugins", theme.Cur().Cant.Render(m.err.Error()), p.kit.menuCtx()))
		}
		p.reload()
		return p.toast.Push(mnotify.PluginUninstalled(m.id, m.removed, m.kept))
	case tea.KeyMsg:
		act, ok := keymap.Plugins().Action(m.String())
		if !ok {
			return nil
		}
		switch act {
		case keys.Up:
			if p.cursor > 0 {
				p.cursor--
			}
		case keys.Down:
			if p.cursor < len(p.rows)-1 {
				p.cursor++
			}
		case keys.Confirm:
			if len(p.rows) == 0 {
				return nil
			}
			row := p.rows[p.cursor]
			id, on := row.id, !row.enabled
			return func() tea.Msg {
				err := plugin.SetEnabled(id, on)
				return pluginsToggledMsg{id: id, on: on, err: err}
			}
		case keymap.PluginInstall:
			return a.Push(p.kit.pluginsInstallPicker())
		case keymap.PluginUninstall:
			if len(p.rows) == 0 {
				return nil
			}
			id := p.rows[p.cursor].id
			if plugin.IsInternal(id) {
				return nil
			}
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
		case keys.Cancel:
			return a.Pop()
		}
	}
	return nil
}

func (k *Kit) pluginsInstallPicker() vkdeck.View {
	home := k.d.App.Cfg.Home
	app := k.d.App
	cands, err := plugin.ListInstallCandidates(home)
	if err != nil {
		return vkdeck.NewMessage("install plugin", theme.Cur().Cant.Render(err.Error()), k.menuCtx())
	}
	var items []vkdeck.MenuItem
	for _, c := range cands {
		c := c
		if !c.Installable {
			items = append(items, vkdeck.MenuItem{
				Label: c.Label,
				Desc:  c.Desc,
				Do: func(a *vkdeck.Model) tea.Cmd {
					return a.Push(vkdeck.NewMessage("install plugin",
						theme.Cur().Cant.Render(c.Label+": "+c.Reason), k.menuCtx()))
				},
			})
			continue
		}
		items = append(items, vkdeck.MenuItem{
			Label: c.Label,
			Desc:  c.Desc,
			Do: func(a *vkdeck.Model) tea.Cmd {
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
	ctx := append(k.menuCtx(), [2]string{"dir", plugin.PluginsDir(home)})
	return vkdeck.NewMenu("install plugin", ctx, items...)
}

func (p *pluginsPage) Body(width, _ int) string {
	th := theme.Cur()
	f := layout.ScreenFrame(width)
	if len(p.rows) == 0 {
		body := f.TitledBox(strings.ToUpper(p.Title()), th.Dim.Render("(no managed plugins — press i to install)"))
		return p.toast.Body(body, width)
	}
	lines := make([]string, 0, len(p.rows))
	for i, row := range p.rows {
		cursor := "  "
		label := th.Val.Render(row.id)
		if i == p.cursor {
			cursor = th.Key.Render("▸ ")
			label = th.Key.Render(row.id)
		}
		icon := glyph.Cross()
		if row.enabled {
			icon = glyph.Check()
		}
		line := cursor + theme.Icon(icon, row.hue) + label
		if row.desc != "" {
			line = f.Spread(line, th.Dim.Render(row.desc))
		}
		lines = append(lines, line)
	}
	lines = layout.CursorRows(lines, p.cursor, 0)
	body := f.TitledBox(strings.ToUpper(p.Title()), lines...)
	return p.toast.Body(body, width)
}
