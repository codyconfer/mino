package views

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/munin/internal/app"
	"github.com/codyconfer/munin/internal/app/verify"
	"github.com/codyconfer/munin/internal/audit"
	"github.com/codyconfer/munin/internal/deck"
	"github.com/codyconfer/munin/internal/render/glyph"
	"github.com/codyconfer/munin/internal/signals"
)

type Finding = verify.Finding

type Deps struct {
	App *app.App

	FetchQuery         func(name string) []signals.Section
	FetchFlight        func(name string) []signals.Section
	FetchFlightAudited func(name string) []signals.Section
	FetchHomeFlight    func(name string) []signals.Section

	Verify func(kind string) []verify.Finding

	ExportDirectives func() ([]string, error)
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

func (k *Kit) mainMenuItems() []deck.MenuItem {
	items := []deck.MenuItem{
		{Label: "Take flight", Desc: "aggregate saved queries", Icon: glyph.Flight(), Hue: 0, Do: func(a *deck.State) tea.Cmd {
			return a.Push(k.flightPicker())
		}},
	}
	if k.hasHistory() {
		items = append(items, deck.MenuItem{Label: "History", Desc: "recall past runs", Icon: glyph.History(), Hue: 6, Do: func(a *deck.State) tea.Cmd {
			return a.Push(k.history())
		}})
	}
	items = append(items,
		deck.MenuItem{Label: "Directives", Desc: "queries, filters, flights, roles", Icon: glyph.Directives(), Hue: 2, Do: func(a *deck.State) tea.Cmd {
			return a.Push(k.directivesMenu())
		}},
		deck.MenuItem{Label: "Query DuckDB", Desc: "ad-hoc SQL over DuckDB", Icon: glyph.Audit(), Hue: 4, Do: func(a *deck.State) tea.Cmd {
			return a.Push(k.AuditQuery())
		}},
		deck.MenuItem{Label: "Login", Desc: "authenticate signal providers", Icon: glyph.Login(), Hue: 1, Do: func(a *deck.State) tea.Cmd {
			return a.Push(k.Login())
		}},
		deck.MenuItem{Label: "Settings", Desc: "config, DuckDB, export/import", Icon: glyph.Settings(), Hue: 5, Do: func(a *deck.State) tea.Cmd {
			return a.Push(k.Settings())
		}},
		deck.MenuItem{Label: "Quit", Desc: "back to shell", Icon: glyph.Quit(), Hue: 3, Do: func(*deck.State) tea.Cmd {
			return tea.Quit
		}},
	)
	return items
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

func (k *Kit) MainMenu() deck.View {
	return deck.NewMenu("main menu", k.menuCtx(), k.mainMenuItems()...)
}

func (k *Kit) Home() deck.View {
	name := k.homeFlightName()
	if name == "" {
		return deck.NewHome("home", k.menuCtx(), k.mainMenuItems(), "", nil)
	}
	ctx := append(k.menuCtx(), [2]string{"home", name})
	return deck.NewHome("home", ctx, k.mainMenuItems(), name, func() []signals.Section {
		return k.d.FetchHomeFlight(name)
	})
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

func (k *Kit) flightPicker() deck.View {
	var items []deck.MenuItem
	for _, n := range k.d.App.VisibleFlights() {
		fl := k.d.App.Directives.Flights[n]
		items = append(items, deck.MenuItem{
			Label: n,
			Desc:  strings.Join(fl.Queries, ", "),
			Do:    func(a *deck.State) tea.Cmd { return a.Push(k.FlightResults(n)) },
		})
	}
	if len(items) == 0 {
		items = append(items, deck.MenuItem{Label: "(no flights visible)"})
	}
	return deck.NewMenu("flights", k.menuCtx(), items...)
}

func (k *Kit) FlightResults(name string) deck.View {
	ctx := append(k.menuCtx(), [2]string{"flight", name})
	return deck.NewResults("flight: "+name, ctx, func() []signals.Section {
		return k.d.FetchFlightAudited(name)
	})
}

func (k *Kit) history() deck.View {
	return deck.NewContent("history", k.menuCtx(), nil, func() string {
		st := k.d.App.Audit
		if st == nil {
			return theme.Cur().Dim.Render("audit disabled")
		}
		runs, err := st.RecentEntries(50)
		if err != nil {
			return theme.Cur().Cant.Render("error: " + err.Error())
		}
		if len(runs) == 0 {
			return theme.Cur().Dim.Render("no recorded runs")
		}
		f := layout.NewFrame(theme.BodyWidth)
		th := theme.Cur()
		var lines []string
		for _, r := range runs {
			left := th.Val.Render(fmt.Sprintf("#%-4d %-6s %s", r.ID, r.Kind, r.Name))
			lines = append(lines, f.Spread(left, th.Dim.Render(r.Started.Format("01-02 15:04")+"  "+entryStatus(r))))
		}
		return f.Panel("recent runs", lines...)
	})
}

func entryStatus(r audit.AuditRow) string {
	if r.Error != "" {
		return "error: " + r.Error
	}
	return fmt.Sprintf("%d items", r.ItemCount)
}
