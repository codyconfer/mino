package views

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	vkdeck "github.com/codyconfer/viewkit/deck"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/deck"
	"github.com/codyconfer/munin/internal/signals"
)

func flightFor(t *testing.T, kit *Kit, name string) *flightView {
	t.Helper()
	var view any
	if name == "" {
		view = kit.FlightBuilder()
	} else {
		view = kit.FlightEditor(name)
	}
	v, ok := view.(*flightView)
	if !ok {
		t.Fatal("flight view has the wrong type")
	}
	return v
}

func (v *flightView) set(t *testing.T, key, val string) {
	t.Helper()
	for i := range v.Form().Fields {
		if v.Form().Fields[i].Key == key {
			v.Form().Fields[i].Text = val
			return
		}
	}
	t.Fatalf("flight form has no field %q", key)
}

func TestFlightsListPutsNewFirstAndOpensTheEditor(t *testing.T) {
	kit := testKit(t)
	kit.d.App.Cfg.Role = ""

	app := deck.New(kit.Flights())
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	body := app.View()
	newAt, flightAt := strings.Index(body, "New"), strings.Index(body, "default")
	if newAt < 0 || flightAt < 0 {
		t.Fatalf("flights list missing entries: %q", body)
	}
	if newAt > flightAt {
		t.Errorf("New should come before saved flights:\n%s", body)
	}
	if !strings.Contains(body, "q1") {
		t.Errorf("flight summary should list its queries: %q", body)
	}

	app = step(app, tea.KeyMsg{Type: tea.KeyDown})
	app, cmd := update(app, tea.KeyMsg{Type: tea.KeyEnter})
	for _, c := range flattenCmds(cmd) {
		if c != nil {
			app = step(app, c())
		}
	}
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	edit := app.View()
	if !strings.Contains(edit, "edit default") {
		t.Fatalf("selecting a flight did not open the editor: %q", edit)
	}
	if !strings.Contains(edit, "q1") {
		t.Errorf("editor did not prefill the flight queries: %q", edit)
	}
	for _, want := range []string{"ctrl+t validate", "ctrl+y yaml", "ctrl+x delete"} {
		if !strings.Contains(edit, want) {
			t.Errorf("flight editor hints missing %q: %q", want, edit)
		}
	}
}

func TestFlightEditorRejectsUnknownAndEmptyQueries(t *testing.T) {
	kit := testKit(t)

	v := flightFor(t, kit, "")
	v.set(t, "name", "empty")
	if _, err := v.flight(); err == nil {
		t.Error("a flight with no queries should fail")
	}

	v = flightFor(t, kit, "")
	v.set(t, "queries", "q1, nope")
	if _, err := v.flight(); err == nil {
		t.Error("a flight referencing an unknown query should fail")
	}
}

func TestFlightEditorRunsThroughFetchFlightQueries(t *testing.T) {
	kit := testKit(t)
	var gotLabel string
	var gotQueries []string
	kit.d.FetchFlightQueries = func(label string, queries []string) []signals.Section {
		gotLabel, gotQueries = label, queries
		return []signals.Section{{Signal: "github", Title: "t", Items: []signals.Item{{Title: "flight hit"}}}}
	}

	v := flightFor(t, kit, "default")
	app := deck.New(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	app, cmd := update(app, tea.KeyMsg{Type: tea.KeyCtrlR})
	for _, c := range flattenCmds(cmd) {
		if c != nil {
			app = step(app, c())
		}
	}
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})

	if gotLabel != "default" || len(gotQueries) != 1 || gotQueries[0] != "q1" {
		t.Fatalf("fetch got label=%q queries=%v", gotLabel, gotQueries)
	}
	if body := app.View(); !strings.Contains(body, "flight hit") {
		t.Fatalf("results panel missing the flight hit: %q", body)
	}
}

func TestFlightEditorSaveWritesFlightFile(t *testing.T) {
	kit := testKit(t)
	v := flightFor(t, kit, "")
	v.set(t, "queries", "q1")
	v.set(t, "name", "built-flight")

	app := deck.New(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	step(app, tea.KeyMsg{Type: tea.KeyCtrlS})

	path := filepath.Join(kit.d.App.Cfg.Home, config.DirFlights, "built-flight.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("save did not write the flight: %v (status %q)", err, v.Status())
	}
	body := string(data)
	for _, want := range []string{"name: built-flight", "q1"} {
		if !strings.Contains(body, want) {
			t.Errorf("saved flight missing %q:\n%s", want, body)
		}
	}

	blob, err := json.Marshal(map[string]string{"built-flight.yaml": body})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := config.ParseDirectives(blob)
	if err != nil {
		t.Fatalf("saved flight does not parse: %v", err)
	}
	flights := parsed.Flights
	if fl := flights["built-flight"]; len(fl.Queries) != 1 || fl.Queries[0] != "q1" {
		t.Errorf("parsed flight = %#v", fl)
	}
}

func TestFlightEditorSaveRejectsCollisionAndMissingName(t *testing.T) {
	kit := testKit(t)

	v := flightFor(t, kit, "")
	v.set(t, "queries", "q1")
	app := deck.New(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	step(app, tea.KeyMsg{Type: tea.KeyCtrlS})
	if !strings.Contains(v.Status(), "name is required") {
		t.Errorf("status = %q, want a name-required message", v.Status())
	}

	v = flightFor(t, kit, "")
	v.set(t, "queries", "q1")
	v.set(t, "name", "default")
	app = deck.New(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	step(app, tea.KeyMsg{Type: tea.KeyCtrlS})
	if !strings.Contains(v.Status(), "already exists") {
		t.Errorf("status = %q, want an already-exists message", v.Status())
	}
}

func TestFlightEditorValidateAndDelete(t *testing.T) {
	kit := testKit(t)
	fl := config.Flight{Name: "doomed", Queries: []string{"q1"}}
	if _, _, err := config.SaveDirective(nil, kit.d.App.Cfg.Home, "", config.TypeFlight, fl.Name, fl); err != nil {
		t.Fatal(err)
	}
	loadKitDirectives(t, kit)
	path := filepath.Join(kit.d.App.Cfg.Home, config.DirFlights, "doomed.yaml")

	v := flightFor(t, kit, "doomed")
	app := deck.New(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})

	app = step(app, tea.KeyMsg{Type: tea.KeyCtrlT})
	if len(v.Notice()) == 0 {
		t.Error("validate produced no findings panel")
	}

	app = step(app, tea.KeyMsg{Type: tea.KeyCtrlX})
	if !v.Confirming() {
		t.Fatal("delete did not raise a confirmation")
	}
	app = step(app, tea.KeyMsg{Type: tea.KeyLeft})
	app, cmd := update(app, tea.KeyMsg{Type: tea.KeyEnter})
	for _, c := range flattenCmds(cmd) {
		if c != nil {
			app = step(app, c())
		}
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("confirming delete left the flight file in place")
	}
}

func TestFlightEditorYAMLPreview(t *testing.T) {
	kit := testKit(t)
	v := flightFor(t, kit, "default")

	app := deck.New(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	if strings.Contains(app.View(), "queries:") {
		t.Fatal("yaml preview showing before it was toggled on")
	}
	app = step(app, tea.KeyMsg{Type: tea.KeyCtrlY})
	body := app.View()
	for _, want := range []string{"name: default", "queries:", "- q1"} {
		if !strings.Contains(body, want) {
			t.Errorf("yaml preview missing %q: %q", want, body)
		}
	}
}

func TestQueriesAndFlightsBothFilterByRole(t *testing.T) {
	kit := testKit(t)
	kit.d.App.Directives.Queries["hidden-q"] = config.Query{Name: "hidden-q", Signal: "github"}
	kit.d.App.Directives.Flights["hidden-f"] = config.Flight{Name: "hidden-f", Queries: []string{"q1"}}
	kit.d.App.Directives.Roles["triage"] = config.RoleDef{
		Name:    "triage",
		Flights: []string{"default"},
		Queries: []string{"q1", "f1"},
	}
	kit.d.App.Cfg.Role = "triage"

	render := func(v vkdeck.View) string {
		app := deck.New(v)
		app = step(app, tea.WindowSizeMsg{Width: 100, Height: 44})
		return app.View()
	}

	queries := render(kit.Queries())
	if strings.Contains(queries, "hidden-q") {
		t.Errorf("queries list showed a query outside the role:\n%s", queries)
	}
	if !strings.Contains(queries, "q1") {
		t.Errorf("queries list dropped a query the role allows:\n%s", queries)
	}

	flights := render(kit.Flights())
	if strings.Contains(flights, "hidden-f") {
		t.Errorf("flights list showed a flight outside the role:\n%s", flights)
	}
	if !strings.Contains(flights, "default") {
		t.Errorf("flights list dropped a flight the role allows:\n%s", flights)
	}
}

func TestNoRoleShowsEverythingOnBothSurfaces(t *testing.T) {
	kit := testKit(t)
	kit.d.App.Directives.Queries["extra-q"] = config.Query{Name: "extra-q", Signal: "github"}
	kit.d.App.Directives.Flights["extra-f"] = config.Flight{Name: "extra-f", Queries: []string{"q1"}}
	kit.d.App.Directives.Roles["triage"] = config.RoleDef{
		Name:    "triage",
		Flights: []string{"default"},
		Queries: []string{"q1"},
	}
	kit.d.App.Cfg.Role = ""

	render := func(v vkdeck.View) string {
		app := deck.New(v)
		app = step(app, tea.WindowSizeMsg{Width: 100, Height: 44})
		return app.View()
	}

	queries := render(kit.Queries())
	for _, want := range []string{"q1", "f1", "extra-q"} {
		if !strings.Contains(queries, want) {
			t.Errorf("no role should list every query, missing %q:\n%s", want, queries)
		}
	}
	flights := render(kit.Flights())
	for _, want := range []string{"default", "extra-f"} {
		if !strings.Contains(flights, want) {
			t.Errorf("no role should list every flight, missing %q:\n%s", want, flights)
		}
	}
}
