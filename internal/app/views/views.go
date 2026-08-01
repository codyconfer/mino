package views

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	vkdeck "github.com/codyconfer/viewkit/deck"
	"github.com/codyconfer/viewkit/keys"
	"github.com/codyconfer/viewkit/ui"

	"github.com/codyconfer/mino/internal/app"
	"github.com/codyconfer/mino/internal/app/pane"
	"github.com/codyconfer/mino/internal/app/verify"
	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/deck"
	"github.com/codyconfer/mino/internal/render"
	"github.com/codyconfer/mino/internal/render/glyph"
	"github.com/codyconfer/mino/internal/signals"
)

type Finding = verify.Finding

type Deps struct {
	App   *app.App
	Panes *pane.Manager
	// Scope is the deck's rendering context; nil falls back to the built-in
	// defaults. Updated in place when the settings view swaps appearance.
	Scope *ui.Scope

	FetchQuery         func(name string) []signals.Section
	FetchFlightAudited func(name string) []signals.Section
	FetchHomeFlight    func(name string) []signals.Section
	FetchAdhoc         func(q config.Query) []signals.Section
	FetchFlightQueries func(label string, queries []string) []signals.Section
	FetchDetail        func(signal string, it signals.Item) (*signals.ItemDetail, error)

	Verify func(kind string) []verify.Finding

	ExportDirectives func() ([]string, error)

	RenderReport func(fd config.FormatterDef, label string, sections []signals.Section) (string, error)
	CopyText     func(text string) error
	SaveReport   func(formatter, text string) (string, error)

	PreviewRole func(rd config.RoleDef, hold time.Duration, body func() app.RolePreviewStep) []app.RolePreviewStep
}

type Kit struct {
	d         Deps
	storeRev  string
	storeSeen bool
	histKnown bool
	histHas   bool
}

func New(d Deps) *Kit { return &Kit{d: d} }

// scope returns the kit's rendering scope, defaulting to the built-ins.
func (k *Kit) scope() *ui.Scope {
	if k.d.Scope != nil {
		return k.d.Scope
	}
	return ui.Default()
}

// modelScope returns a's rendering scope, tolerating nil models in tests.
func modelScope(a *vkdeck.Model) *ui.Scope {
	if s := a.UI(); s != nil {
		return s
	}
	return ui.Default()
}

func (k *Kit) menuCtx() []keys.Hint {
	if role := k.d.App.Role(); role != "" {
		return []keys.Hint{{Key: "role", Label: role}}
	}
	return nil
}

func (k *Kit) mainMenuItems() []vkdeck.MenuItem {
	return []vkdeck.MenuItem{
		{Label: "Directives", Desc: "flights, queries, roles, reports, history", Icon: glyph.Directives(), Hue: 0, OnSelect: func(a *vkdeck.Model) tea.Cmd {
			return a.Push(k.Directives())
		}},
		k.ntrMenuItem(),
		{Label: "DuckDB", Desc: "build, run, save, and manage SQL queries", Icon: glyph.Audit(), Hue: 4, OnSelect: func(a *vkdeck.Model) tea.Cmd {
			return a.Push(k.DuckDB())
		}},
		{Label: "Tooling", Desc: "accounts, plugins, settings", Icon: glyph.Settings(), Hue: 2, OnSelect: func(a *vkdeck.Model) tea.Cmd {
			return a.Push(k.Tooling())
		}},
		{Label: "Quit", Desc: "back to shell", Icon: glyph.Quit(), Hue: 3, OnSelect: func(*vkdeck.Model) tea.Cmd {
			return tea.Quit
		}},
	}
}

func (k *Kit) Directives() vkdeck.View {
	return vkdeck.NewMenu("directives", k.menuCtx(), k.directiveMenuItems()...)
}

func (k *Kit) directiveMenuItems() []vkdeck.MenuItem {
	items := []vkdeck.MenuItem{
		{Label: "Flights", Desc: "build, run, save, and manage flights", Icon: glyph.Flight(), Hue: 1, OnSelect: func(a *vkdeck.Model) tea.Cmd {
			return a.Push(k.Flights())
		}},
		{Label: "Queries", Desc: "build, run, save, and manage queries and filters", Icon: glyph.Builder(), Hue: 3, OnSelect: func(a *vkdeck.Model) tea.Cmd {
			return a.Push(k.Queries())
		}},
		{Label: "Roles", Desc: "build, dry-run, save, and manage roles", Icon: glyph.Role(), Hue: 4, OnSelect: func(a *vkdeck.Model) tea.Cmd {
			return a.Push(k.Roles())
		}},
		{Label: "Reports", Desc: "build, render, copy, save, and manage reports", Icon: glyph.Directives(), Hue: 2, OnSelect: func(a *vkdeck.Model) tea.Cmd {
			return a.Push(k.Reports())
		}},
	}
	if k.hasHistory() {
		items = append(items, vkdeck.MenuItem{Label: "History", Desc: "recall or drop past runs", Icon: glyph.History(), Hue: 6, OnSelect: func(a *vkdeck.Model) tea.Cmd {
			return a.Push(k.History())
		}})
	}
	return items
}

func (k *Kit) Tooling() vkdeck.View {
	return vkdeck.NewMenu("tooling", k.menuCtx(), k.toolingMenuItems()...)
}

func (k *Kit) toolingMenuItems() []vkdeck.MenuItem {
	return []vkdeck.MenuItem{
		{Label: "Accounts", Desc: "authenticate signal providers", Icon: glyph.Login(), Hue: 1, OnSelect: func(a *vkdeck.Model) tea.Cmd {
			return a.Push(k.Login())
		}},
		{Label: "Plugins", Desc: "install, enable/disable, or uninstall managed plugins", Icon: glyph.Plugins(), Hue: 0, OnSelect: func(a *vkdeck.Model) tea.Cmd {
			return a.Push(k.Plugins())
		}},
		{Label: "Settings", Desc: "config, import, export", Icon: glyph.Settings(), Hue: 5, OnSelect: func(a *vkdeck.Model) tea.Cmd {
			return a.Push(k.Settings())
		}},
	}
}

func (k *Kit) hasHistory() bool {
	if k.d.App.Audit == nil {
		return false
	}
	if k.histKnown {
		return k.histHas
	}
	return true
}

type historyProbedMsg struct{ has bool }

func (k *Kit) probeHistory() tea.Cmd {
	st := k.d.App.Audit
	if st == nil || k.histHas {
		return nil
	}
	return func() tea.Msg {
		runs, err := st.RecentEntries(1)
		if err != nil {
			return historyProbedMsg{has: true}
		}
		return historyProbedMsg{has: len(runs) > 0}
	}
}

func (k *Kit) forgetHistory() { k.histKnown, k.histHas = false, false }

func (k *Kit) MainMenu() vkdeck.View {
	return vkdeck.NewMenu("", k.menuCtx(), k.mainMenuItems()...)
}

func (k *Kit) Home() vkdeck.View {
	ctx := func() []keys.Hint {
		cues := k.menuCtx()
		if name := k.homeFlightName(); name != "" {
			cues = append(cues, keys.Hint{Key: "home", Label: name})
		}
		return cues
	}
	shell := deck.NewHome("home", ctx(), k.mainMenuItems(), k.homeFlightName, k.d.FetchHomeFlight, k.openDetail)
	return vkdeck.WithExtraHints(vkdeck.WithLiveContext(shell, ctx), k.hotkeyHints())
}

func (k *Kit) homeFlightName() string {
	role := k.d.App.Role()
	if role == "" {
		return ""
	}
	d := k.d.App.Dirs()
	if d == nil {
		return ""
	}
	rd, ok := d.Roles[role]
	if !ok || rd.Home == "" {
		return ""
	}
	if _, ok := d.Flights[rd.Home]; !ok {
		return ""
	}
	return rd.Home
}

func (k *Kit) FlightResults(name string) vkdeck.View {
	ctx := append(k.menuCtx(), keys.Hint{Key: "flight", Label: name})
	var held sectionHolder
	lst := deck.NewResults("flight: "+name, ctx, func() []signals.Section {
		sections := k.d.FetchFlightAudited(name)
		held.set(sections)
		return sections
	}, k.openDetail)
	return WithPaneSnapshot(lst, func() (pane.Snapshot, bool) {
		sections := held.get()
		if len(sections) == 0 {
			return pane.Snapshot{}, false
		}
		return pane.Snapshot{
			Kind:     pane.KindSections,
			Title:    "flight: " + name,
			Origin:   "flight:" + name,
			Sections: sections,
		}, true
	})
}

func (k *Kit) openDetail(a *vkdeck.Model, ref render.ItemRef) tea.Cmd {
	return a.Push(k.Detail(ref))
}
