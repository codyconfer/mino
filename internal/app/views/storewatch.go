package views

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const storePollInterval = time.Second

type storeTickMsg struct{}

func StoreTick() tea.Cmd {
	return tea.Tick(storePollInterval, func(time.Time) tea.Msg { return storeTickMsg{} })
}

func (k *Kit) storeChanged() bool {
	rev, _ := k.d.App.StoreRevision()
	if !k.storeSeen {
		k.storeRev, k.storeSeen = rev, true
		return false
	}
	if rev == k.storeRev {
		return false
	}
	k.storeRev = rev
	return true
}
