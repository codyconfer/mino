package views

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	vkdeck "github.com/codyconfer/viewkit/deck"
)

const ownerPollInterval = 2 * time.Second

type ownerTickMsg struct{}

type ownerWatchView struct {
	vkdeck.View
	alive func() bool
}

func WithOwnerWatch(inner vkdeck.View, alive func() bool) vkdeck.View {
	if alive == nil {
		return inner
	}
	return &ownerWatchView{View: inner, alive: alive}
}

func (v *ownerWatchView) Init() tea.Cmd {
	return tea.Batch(v.View.Init(), ownerTick())
}

func (v *ownerWatchView) Update(m *vkdeck.Model, msg tea.Msg) tea.Cmd {
	if _, ok := msg.(ownerTickMsg); ok {
		if !v.alive() {
			return tea.Quit
		}
		return ownerTick()
	}
	return v.View.Update(m, msg)
}

func ownerTick() tea.Cmd {
	return tea.Tick(ownerPollInterval, func(time.Time) tea.Msg { return ownerTickMsg{} })
}
