package views

import (
	"sync"

	vkdeck "github.com/codyconfer/viewkit/deck"

	"github.com/codyconfer/munin/internal/app/pane"
	"github.com/codyconfer/munin/internal/signals"
)

type PaneSource interface {
	PaneSnapshot() (pane.Snapshot, bool)
}

type paneSourceView struct {
	vkdeck.View
	snap func() (pane.Snapshot, bool)
}

func WithPaneSnapshot(inner vkdeck.View, snap func() (pane.Snapshot, bool)) vkdeck.View {
	if snap == nil {
		return inner
	}
	return &paneSourceView{View: inner, snap: snap}
}

func (v *paneSourceView) PaneSnapshot() (pane.Snapshot, bool) { return v.snap() }

type sectionHolder struct {
	mu       sync.Mutex
	sections []signals.Section
}

func (h *sectionHolder) set(s []signals.Section) {
	h.mu.Lock()
	h.sections = s
	h.mu.Unlock()
}

func (h *sectionHolder) get() []signals.Section {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.sections
}
