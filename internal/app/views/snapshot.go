package views

import (
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	vkdeck "github.com/codyconfer/viewkit/deck"
	"github.com/codyconfer/viewkit/keys"
	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/mino/internal/app/pane"
	"github.com/codyconfer/mino/internal/keymap"
	"github.com/codyconfer/mino/internal/render"
)

const (
	snapshotPollInterval  = 500 * time.Millisecond
	snapshotChromeReserve = 7
)

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

	scroll layout.ScrollState
	total  int
	rows   int
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

func (v *SnapshotView) Update(_ *vkdeck.Model, msg tea.Msg) tea.Cmd {
	switch t := msg.(type) {
	case tea.WindowSizeMsg:
		v.rows = max(t.Height-snapshotChromeReserve, 1)
		v.scroll.Scroll(0, v.total, layout.ViewportContentRows(v.rows))
		return nil
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
		return v.handleKey(t)
	}
	return nil
}

func (v *SnapshotView) handleKey(k tea.KeyMsg) tea.Cmd {
	act, ok := keymap.ItemList().Action(k.String())
	if !ok {
		return nil
	}
	rows := layout.ViewportContentRows(v.rows)
	switch act {
	case keys.Up:
		v.scroll.Scroll(-1, v.total, rows)
	case keys.Down:
		v.scroll.Scroll(1, v.total, rows)
	case keys.PageUp:
		v.scroll.Scroll(-max(rows, 1), v.total, rows)
	case keys.PageDown:
		v.scroll.Scroll(max(rows, 1), v.total, rows)
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

func (v *SnapshotView) render(width int) string {
	th := theme.Cur()
	if v.loadErr != nil {
		return th.Cant.Render(fmt.Sprintf("snapshot unavailable: %v", v.loadErr))
	}
	switch v.snap.Kind {
	case pane.KindDetail:
		if v.snap.Detail == nil {
			return th.Dim.Render("no detail in snapshot")
		}
		return render.DetailPanel(layout.ScreenFrame(width), v.ref(), v.snap.Detail)
	default:
		if len(v.snap.Sections) == 0 {
			return th.Dim.Render("no results in snapshot")
		}
		return render.RenderTerminalString(v.snap.Sections)
	}
}

func (v *SnapshotView) Body(width, height int) string {
	body := v.render(width)
	v.total = layout.CountLines(body)
	v.rows = max(height, 1)
	return layout.Viewport(body, height, v.scroll.Offset)
}

func (v *SnapshotView) Hints() [][2]string {
	km := keymap.ItemList()
	hints := [][2]string{km.HintLabeled(keys.Up, "scroll"), km.HintLabeled(keys.PageUp, "page")}
	if v.url() != "" {
		hints = append(hints, km.HintLabeled(keys.Open, "open"))
	}
	return hints
}

func (v *SnapshotView) Context() [][2]string {
	var cues [][2]string
	if v.snap.Origin != "" {
		cues = append(cues, [2]string{"from", v.snap.Origin})
	}
	if !v.mod.IsZero() {
		cues = append(cues, [2]string{"updated", v.mod.Format("15:04:05")})
	}
	return cues
}
