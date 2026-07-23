package views

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/sisyphus"

	"github.com/codyconfer/munin/internal/audit"
	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/tui"
)

type Finding struct {
	Name string
	Msg  string
	OK   bool
	Warn bool
}

type Deps struct {
	Home       func() string
	Role       func() string
	Directives func() *config.Directives
	Config     func() *config.Config
	Mgr        func() *sisyphus.Manager
	Audit      func() *audit.Store

	VisibleFlights func() []string

	RunQuery         func(name string) string
	RunFlight        func(name string) string
	RunFlightAudited func(name string) string

	Verify func(kind string) []Finding

	ExportDirectives func() ([]string, error)
}

type Kit struct {
	d Deps
}

func New(d Deps) *Kit { return &Kit{d: d} }

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func filepathBase(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}

func (k *Kit) menuCtx() [][2]string {
	return [][2]string{{"role", dash(k.d.Role())}, {"deck", filepathBase(k.d.Home())}}
}

func (k *Kit) MainMenu() tui.View {
	return tui.NewMenu("main menu", k.menuCtx(),
		tui.MenuItem{Label: "Run a flight", Desc: "aggregate saved queries", Do: func(a *tui.App) tea.Cmd {
			return a.Push(k.flightPicker())
		}},
		tui.MenuItem{Label: "History", Desc: "recall past runs", Do: func(a *tui.App) tea.Cmd {
			return a.Push(k.history())
		}},
		tui.MenuItem{Label: "Directives", Desc: "queries, filters, flights, roles", Do: func(a *tui.App) tea.Cmd {
			return a.Push(k.directivesMenu())
		}},
		tui.MenuItem{Label: "Audit query", Desc: "ad-hoc SQL over DuckDB", Do: func(a *tui.App) tea.Cmd {
			return a.Push(k.AuditQuery())
		}},
		tui.MenuItem{Label: "Settings", Desc: "config, DuckDB, export/import", Do: func(a *tui.App) tea.Cmd {
			return a.Push(k.Settings())
		}},
		tui.MenuItem{Label: "Quit", Desc: "back to shell", Do: func(*tui.App) tea.Cmd {
			return tea.Quit
		}},
	)
}

func (k *Kit) flightPicker() tui.View {
	var items []tui.MenuItem
	for _, n := range k.d.VisibleFlights() {
		n := n
		fl := k.d.Directives().Flights[n]
		items = append(items, tui.MenuItem{
			Label: n,
			Desc:  strings.Join(fl.Queries, ", "),
			Do:    func(a *tui.App) tea.Cmd { return a.Push(k.FlightResults(n)) },
		})
	}
	if len(items) == 0 {
		items = append(items, tui.MenuItem{Label: "(no flights visible)"})
	}
	return tui.NewMenu("flights", k.menuCtx(), items...)
}

func (k *Kit) FlightResults(name string) tui.View {
	ctx := [][2]string{{"role", dash(k.d.Role())}, {"flight", name}}
	return tui.NewContent("flight: "+name, ctx, nil, func() string {
		return k.d.RunFlightAudited(name)
	})
}

func (k *Kit) history() tui.View {
	return tui.NewContent("history", k.menuCtx(), nil, func() string {
		st := k.d.Audit()
		if st == nil {
			return theme.Cur().Dim.Render("audit disabled")
		}
		runs, err := st.RecentRuns(50)
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
			lines = append(lines, f.Spread(left, th.Dim.Render(r.Started.Format("01-02 15:04")+"  "+runStatus(r))))
		}
		return f.Panel("recent runs", lines...)
	})
}

func runStatus(r audit.FlightRow) string {
	if r.Error != "" {
		return "error: " + r.Error
	}
	return fmt.Sprintf("%d items", r.ItemCount)
}
