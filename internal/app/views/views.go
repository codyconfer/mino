package views

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/codyconfer/viewkit/theme"

	vkdeck "github.com/codyconfer/viewkit/deck"

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

func (k *Kit) mainMenuItems() []vkdeck.MenuItem {
	items := []vkdeck.MenuItem{
		{Label: "Take flight", Desc: "aggregate saved queries", Icon: glyph.Flight(), Hue: 0, Do: func(a *vkdeck.Model) tea.Cmd {
			return a.Push(k.flightPicker())
		}},
	}
	if k.hasHistory() {
		items = append(items, vkdeck.MenuItem{Label: "History", Desc: "recall past runs", Icon: glyph.History(), Hue: 6, Do: func(a *vkdeck.Model) tea.Cmd {
			return a.Push(k.History())
		}})
	}
	items = append(items,
		vkdeck.MenuItem{Label: "Directives", Desc: "queries, filters, flights, roles", Icon: glyph.Directives(), Hue: 2, Do: func(a *vkdeck.Model) tea.Cmd {
			return a.Push(k.directivesMenu())
		}},
		k.ntrMenuItem(),
		vkdeck.MenuItem{Label: "Query DuckDB", Desc: "ad-hoc SQL over DuckDB", Icon: glyph.Audit(), Hue: 4, Do: func(a *vkdeck.Model) tea.Cmd {
			return a.Push(k.AuditQuery())
		}},
		vkdeck.MenuItem{Label: "Login", Desc: "authenticate signal providers", Icon: glyph.Login(), Hue: 1, Do: func(a *vkdeck.Model) tea.Cmd {
			return a.Push(k.Login())
		}},
		vkdeck.MenuItem{Label: "Plugins", Desc: "install, enable/disable, or uninstall managed plugins", Icon: glyph.Plugins(), Hue: 8, Do: func(a *vkdeck.Model) tea.Cmd {
			return a.Push(k.Plugins())
		}},
		vkdeck.MenuItem{Label: "Settings", Desc: "config, DuckDB, export/import", Icon: glyph.Settings(), Hue: 5, Do: func(a *vkdeck.Model) tea.Cmd {
			return a.Push(k.Settings())
		}},
		vkdeck.MenuItem{Label: "Quit", Desc: "back to shell", Icon: glyph.Quit(), Hue: 3, Do: func(*vkdeck.Model) tea.Cmd {
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

func (k *Kit) MainMenu() vkdeck.View {
	return vkdeck.NewMenu("main menu", k.menuCtx(), k.mainMenuItems()...)
}

func (k *Kit) Home() vkdeck.View {
	name := k.homeFlightName()
	var shell *vkdeck.HomeShell
	if name == "" {
		shell = deck.NewHome("home", k.menuCtx(), k.mainMenuItems(), "", nil)
	} else {
		ctx := append(k.menuCtx(), [2]string{"home", name})
		shell = deck.NewHome("home", ctx, k.mainMenuItems(), name, func() []signals.Section {
			return k.d.FetchHomeFlight(name)
		})
	}
	return withHotkeyHints(shell, k.hotkeyHints())
}

// hintView appends footer hotkey cues without changing navigation.
type hintView struct {
	vkdeck.View
	extra [][2]string
}

func withHotkeyHints(inner vkdeck.View, extra [][2]string) vkdeck.View {
	if len(extra) == 0 {
		return inner
	}
	return &hintView{View: inner, extra: extra}
}

func (h *hintView) Hints() [][2]string {
	return append(append([][2]string{}, h.View.Hints()...), h.extra...)
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

func (k *Kit) flightPicker() vkdeck.View {
	var items []vkdeck.MenuItem
	for _, n := range k.d.App.VisibleFlights() {
		fl := k.d.App.Directives.Flights[n]
		items = append(items, vkdeck.MenuItem{
			Label: n,
			Desc:  strings.Join(fl.Queries, ", "),
			Do:    func(a *vkdeck.Model) tea.Cmd { return a.Push(k.FlightResults(n)) },
		})
	}
	if len(items) == 0 {
		items = append(items, vkdeck.MenuItem{Label: "(no flights visible)"})
	}
	return vkdeck.NewMenu("flights", k.menuCtx(), items...)
}

func (k *Kit) FlightResults(name string) vkdeck.View {
	ctx := append(k.menuCtx(), [2]string{"flight", name})
	return deck.NewResults("flight: "+name, ctx, func() []signals.Section {
		return k.d.FetchFlightAudited(name)
	})
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
	return deck.NewResults(title, ctx, func() []signals.Section {
		st := k.d.App.Audit
		if st == nil {
			return []signals.Section{{Signal: r.Name, Title: r.Name, Err: fmt.Errorf("audit disabled")}}
		}
		secs, err := st.Sections(id)
		if err != nil {
			return []signals.Section{{Signal: r.Name, Title: r.Name, Err: err}}
		}
		return secs
	})
}

func entryStatus(r audit.AuditRow) string {
	if r.Error != "" {
		return "error: " + r.Error
	}
	return fmt.Sprintf("%d items", r.ItemCount)
}
