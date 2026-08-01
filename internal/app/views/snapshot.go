package views

import (
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	vkdeck "github.com/codyconfer/viewkit/deck"
	"github.com/codyconfer/viewkit/keys"
	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/ui"

	"github.com/codyconfer/mino/internal/app/pane"
	"github.com/codyconfer/mino/internal/keymap"
	"github.com/codyconfer/mino/internal/render"
)

const snapshotPollInterval = 500 * time.Millisecond

type snapshotLoadedMsg struct {
	snap pane.Snapshot
	mod  time.Time
	err  error
}

type snapshotPollMsg struct{}

type SnapshotView struct {
	path  string
	title string

	snap    pane.Snapshot
	mod     time.Time
	loadErr error

	scroll vkdeck.ScrollBody
}

func NewSnapshotView(path string) *SnapshotView {
	return &SnapshotView{path: path, title: "pane"}
}

func (v *SnapshotView) Title() string { return v.title }

func (v *SnapshotView) Init() tea.Cmd {
	return tea.Batch(v.loadCmd(), v.pollCmd())
}

func (v *SnapshotView) loadCmd() tea.Cmd {
	return func() tea.Msg {
		fi, err := os.Stat(v.path)
		if err != nil {
			return snapshotLoadedMsg{err: err}
		}
		snap, err := pane.ReadSnapshot(v.path)
		if err != nil {
			return snapshotLoadedMsg{err: err, mod: fi.ModTime()}
		}
		return snapshotLoadedMsg{snap: snap, mod: fi.ModTime()}
	}
}

func (v *SnapshotView) pollCmd() tea.Cmd {
	return tea.Tick(snapshotPollInterval, func(time.Time) tea.Msg { return snapshotPollMsg{} })
}

func (v *SnapshotView) Update(a *vkdeck.Model, msg tea.Msg) tea.Cmd {
	switch t := msg.(type) {
	case snapshotLoadedMsg:
		v.loadErr = t.err
		if t.err == nil {
			v.snap = t.snap
			if t.snap.Title != "" {
				v.title = t.snap.Title
			}
		}
		v.mod = t.mod
		return nil
	case snapshotPollMsg:
		fi, err := os.Stat(v.path)
		if err == nil && fi.ModTime().After(v.mod) {
			return tea.Batch(v.loadCmd(), v.pollCmd())
		}
		return v.pollCmd()
	case tea.KeyMsg:
		return v.handleKey(a, t)
	}
	return nil
}

func (v *SnapshotView) handleKey(a *vkdeck.Model, k tea.KeyMsg) tea.Cmd {
	act, ok := keymap.ItemList(modelScope(a).Keys).Action(k.String())
	if !ok {
		return nil
	}
	if v.scroll.Handle(act) {
		return nil
	}
	switch act {
	case keys.Open:
		if u := v.url(); u != "" {
			return openURL(u)
		}
	case keys.Cancel:
		return tea.Quit
	}
	return nil
}

func (v *SnapshotView) ref() render.ItemRef {
	r := render.ItemRef{Signal: v.snap.Signal, Meta: v.snap.Meta}
	if v.snap.Item != nil {
		r.Item = *v.snap.Item
	}
	return r
}

func (v *SnapshotView) url() string {
	if v.snap.Detail != nil && v.snap.Detail.URL != "" {
		return v.snap.Detail.URL
	}
	if v.snap.Item != nil {
		return v.snap.Item.URL
	}
	return ""
}

func (v *SnapshotView) render(f layout.Frame) string {
	th := f.Theme()
	if v.loadErr != nil {
		return th.Cant.Render(fmt.Sprintf("snapshot unavailable: %v", v.loadErr))
	}
	switch v.snap.Kind {
	case pane.KindDetail:
		if v.snap.Detail == nil {
			return th.Dim.Render("no detail in snapshot")
		}
		return render.DetailPanel(f.Screen(), v.ref(), v.snap.Detail)
	default:
		if len(v.snap.Sections) == 0 {
			return th.Dim.Italic(true).Render("no results in snapshot")
		}
		return render.RenderTerminalString(v.snap.Sections)
	}
}

func (v *SnapshotView) Body(f layout.Frame) string {
	return v.scroll.View(f, v.render(f), f.Height)
}

func (v *SnapshotView) Hints(scope *ui.Scope) []keys.Hint {
	km := keymap.ItemList(scope.Keys)
	hints := []keys.Hint{km.HintLabeled(keys.Up, "scroll"), km.HintLabeled(keys.PageUp, "page")}
	if v.url() != "" {
		hints = append(hints, km.HintLabeled(keys.Open, "open"))
	}
	return hints
}

func (v *SnapshotView) Context(scope *ui.Scope) []keys.Hint {
	var cues []keys.Hint
	if v.snap.Origin != "" {
		cues = append(cues, keys.Hint{Key: "from", Label: v.snap.Origin})
	}
	if !v.mod.IsZero() {
		cues = append(cues, keys.Hint{Key: "updated", Label: v.mod.Format("15:04:05")})
	}
	return cues
}
