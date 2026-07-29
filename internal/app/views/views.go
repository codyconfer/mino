package views

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/codyconfer/viewkit/theme"

	vkdeck "github.com/codyconfer/viewkit/deck"

	"github.com/codyconfer/munin/internal/app"
	"github.com/codyconfer/munin/internal/app/pane"
	"github.com/codyconfer/munin/internal/app/verify"
	"github.com/codyconfer/munin/internal/audit"
	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/deck"
	"github.com/codyconfer/munin/internal/render"
	"github.com/codyconfer/munin/internal/render/glyph"
	"github.com/codyconfer/munin/internal/signals"
)

type Finding = verify.Finding

type Deps struct {
	App   *app.App
	Panes *pane.Manager

	FetchQuery         func(name string) []signals.Section
	FetchFlightAudited func(name string) []signals.Section
	FetchHomeFlight    func(name string) []signals.Section
	FetchAdhoc         func(q config.Query) []signals.Section
	FetchFlightQueries func(label string, queries []string) []signals.Section
	FetchDetail        func(signal string, it signals.Item) (*signals.ItemDetail, error)

	Verify func(kind string) []verify.Finding

	ExportDirectives func() ([]string, error)

	FormatFlight   func(formatter, flight string) (string, error)
	FormatSections func(formatter, label string, sections []signals.Section) (string, error)
	CopyText       func(text string) error
	SaveReport     func(formatter, text string) (string, error)

	PreviewRole func(rd config.RoleDef, hold time.Duration, body func() app.RolePreviewStep) []app.RolePreviewStep
}

type Kit struct {
	d Deps
}

func New(d Deps) *Kit { return &Kit{d: d} }

func (k *Kit) menuCtx() [][2]string {
	if role := k.d.App.Cfg.Role; role != "" {
		return [][2]string{{"role", role}}
	}
	return nil
}

func (k *Kit) mainMenuItems() []vkdeck.MenuItem {
	return []vkdeck.MenuItem{
		{Label: "Fly", Desc: "flights, history, directives", Icon: glyph.Flight(), Hue: 0, Do: func(a *vkdeck.Model) tea.Cmd {
			return a.Push(k.Fly())
		}},
		k.ntrMenuItem(),
		{Label: "Query DuckDB", Desc: "ad-hoc SQL over DuckDB", Icon: glyph.Audit(), Hue: 4, Do: func(a *vkdeck.Model) tea.Cmd {
			return a.Push(k.AuditQuery())
		}},
		{Label: "Tooling", Desc: "accounts, plugins, settings", Icon: glyph.Settings(), Hue: 2, Do: func(a *vkdeck.Model) tea.Cmd {
			return a.Push(k.Tooling())
		}},
		{Label: "Quit", Desc: "back to shell", Icon: glyph.Quit(), Hue: 3, Do: func(*vkdeck.Model) tea.Cmd {
			return tea.Quit
		}},
	}
}

func (k *Kit) Fly() vkdeck.View {
	return vkdeck.NewMenu("fly", k.menuCtx(), k.flyMenuItems()...)
}

func (k *Kit) flyMenuItems() []vkdeck.MenuItem {
	items := []vkdeck.MenuItem{
		{Label: "Flights", Desc: "build, run, save, and manage flights", Icon: glyph.Flight(), Hue: 1, Do: func(a *vkdeck.Model) tea.Cmd {
			return a.Push(k.Flights())
		}},
	}
	if k.hasHistory() {
		items = append(items, vkdeck.MenuItem{Label: "History", Desc: "recall past runs", Icon: glyph.History(), Hue: 6, Do: func(a *vkdeck.Model) tea.Cmd {
			return a.Push(k.History())
		}})
	}
	items = append(items,
		vkdeck.MenuItem{Label: "Queries", Desc: "build, run, save, and manage queries and filters", Icon: glyph.Builder(), Hue: 3, Do: func(a *vkdeck.Model) tea.Cmd {
			return a.Push(k.Queries())
		}},
		vkdeck.MenuItem{Label: "Formatters", Desc: "build, render, save, and manage formatters", Icon: glyph.Directives(), Hue: 2, Do: func(a *vkdeck.Model) tea.Cmd {
			return a.Push(k.FormatterDirectives())
		}},
		vkdeck.MenuItem{Label: "Reports", Desc: "run a formatter over a flight and copy or save the report", Icon: glyph.Directives(), Hue: 5, Do: func(a *vkdeck.Model) tea.Cmd {
			return a.Push(k.Formatters())
		}},
		vkdeck.MenuItem{Label: "Roles", Desc: "build, dry-run, save, and manage roles", Icon: glyph.Role(), Hue: 4, Do: func(a *vkdeck.Model) tea.Cmd {
			return a.Push(k.Roles())
		}},
	)
	return items
}

func (k *Kit) Tooling() vkdeck.View {
	return vkdeck.NewMenu("tooling", k.menuCtx(), k.toolingMenuItems()...)
}

func (k *Kit) toolingMenuItems() []vkdeck.MenuItem {
	return []vkdeck.MenuItem{
		{Label: "Accounts", Desc: "authenticate signal providers", Icon: glyph.Login(), Hue: 1, Do: func(a *vkdeck.Model) tea.Cmd {
			return a.Push(k.Login())
		}},
		{Label: "Plugins", Desc: "install, enable/disable, or uninstall managed plugins", Icon: glyph.Plugins(), Hue: 0, Do: func(a *vkdeck.Model) tea.Cmd {
			return a.Push(k.Plugins())
		}},
		{Label: "Settings", Desc: "config, import, export", Icon: glyph.Settings(), Hue: 5, Do: func(a *vkdeck.Model) tea.Cmd {
			return a.Push(k.Settings())
		}},
	}
}

func (k *Kit) hasHistory() bool {
	st := k.d.App.Audit
	if st == nil {
		return false
	}
	runs, err := st.RecentEntries(1)
	if err != nil {
		return true
	}
	return len(runs) > 0
}

func (k *Kit) MainMenu() vkdeck.View {
	return vkdeck.NewMenu("", k.menuCtx(), k.mainMenuItems()...)
}

func (k *Kit) Home() vkdeck.View {
	name := k.homeFlightName()
	var shell *vkdeck.HomeShell
	if name == "" {
		shell = deck.NewHome("home", k.menuCtx(), k.mainMenuItems(), "", nil, nil)
	} else {
		ctx := append(k.menuCtx(), [2]string{"home", name})
		shell = deck.NewHome("home", ctx, k.mainMenuItems(), name, func() []signals.Section {
			return k.d.FetchHomeFlight(name)
		}, k.openDetail)
	}
	return vkdeck.WithExtraHints(vkdeck.WithLiveContext(shell, k.menuCtx), k.hotkeyHints())
}

func (k *Kit) homeFlightName() string {
	role := k.d.App.Cfg.Role
	if role == "" {
		return ""
	}
	rd, ok := k.d.App.Directives.Roles[role]
	if !ok || rd.Home == "" {
		return ""
	}
	if _, ok := k.d.App.Directives.Flights[rd.Home]; !ok {
		return ""
	}
	return rd.Home
}

func (k *Kit) FlightResults(name string) vkdeck.View {
	ctx := append(k.menuCtx(), [2]string{"flight", name})
	var held sectionHolder
	lst := deck.NewResults("flight: "+name, name, ctx, func() []signals.Section {
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

func (k *Kit) History() vkdeck.View {
	st := k.d.App.Audit
	if st == nil {
		return vkdeck.NewScroll("history", k.menuCtx(), nil, func() string {
			return theme.Cur().Dim.Render("audit disabled")
		})
	}
	runs, err := st.RecentEntries(50)
	if err != nil {
		return vkdeck.NewScroll("history", k.menuCtx(), nil, func() string {
			return theme.Cur().Cant.Render("error: " + err.Error())
		})
	}
	if len(runs) == 0 {
		return vkdeck.NewScroll("history", k.menuCtx(), nil, func() string {
			return theme.Cur().Dim.Render("no recorded runs")
		})
	}
	items := make([]vkdeck.MenuItem, 0, len(runs))
	for _, r := range runs {
		r := r
		items = append(items, vkdeck.MenuItem{
			Label: fmt.Sprintf("#%-4d %-6s %s", r.ID, r.Kind, r.Name),
			Desc:  r.Started.Format("01-02 15:04") + "  " + entryStatus(r),
			Do: func(a *vkdeck.Model) tea.Cmd {
				return a.Push(k.historyResults(r))
			},
		})
	}
	return vkdeck.NewMenu("history", k.menuCtx(), items...)
}

func (k *Kit) historyResults(r audit.AuditRow) vkdeck.View {
	title := r.Kind + ": " + r.Name
	if r.Kind == "flight" {
		title = "flight: " + r.Name
	}
	ctx := append(k.menuCtx(), [2]string{r.Kind, r.Name})
	id := r.ID
	return deck.NewResults(title, r.Name, ctx, func() []signals.Section {
		st := k.d.App.Audit
		if st == nil {
			return []signals.Section{{Signal: r.Name, Title: r.Name, Err: fmt.Errorf("audit disabled")}}
		}
		secs, err := st.Sections(id)
		if err != nil {
			return []signals.Section{{Signal: r.Name, Title: r.Name, Err: err}}
		}
		return secs
	}, k.openDetail)
}

func entryStatus(r audit.AuditRow) string {
	if r.Error != "" {
		return "error: " + r.Error
	}
	return fmt.Sprintf("%d items", r.ItemCount)
}
