package views

import (
	tea "github.com/charmbracelet/bubbletea"

	vkdeck "github.com/codyconfer/viewkit/deck"

	"github.com/codyconfer/mino/internal/plugin/ntr"
	"github.com/codyconfer/mino/internal/render/glyph"
)

func (k *Kit) NTR() vkdeck.View {
	home := ""
	role := ""
	if k.d.App != nil && k.d.App.Cfg != nil {
		home = k.d.App.Cfg.Home
		role = k.d.App.Role()
	}
	return ntr.NewHomeView(home, role)
}

func (k *Kit) ntrMenuItem() vkdeck.MenuItem {
	desc := "notes, tasks"
	if ntr.RemindersUIVisible() {
		desc = "notes, tasks, reminders"
	}
	return vkdeck.MenuItem{
		Label: "Notes",
		Desc:  desc,
		Icon:  glyph.Notes(),
		Hue:   6,
		Do: func(a *vkdeck.Model) tea.Cmd {
			return a.Push(k.NTR())
		},
	}
}
