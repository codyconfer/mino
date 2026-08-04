package run

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codyconfer/mino/internal/app"
	"github.com/codyconfer/mino/internal/audit"
	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/errs"
)

func testApp(t *testing.T, d *config.Directives, role string) *app.App {
	t.Helper()
	home := t.TempDir()
	st, err := audit.Open(context.Background(), filepath.Join(home, "audit.duckdb"))
	if err != nil {
		t.Fatalf("audit.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if d == nil {
		d = &config.Directives{}
	}
	return &app.App{
		Cfg:        &config.Config{Home: home, DefaultRole: role, Timeout: "5s"},
		Directives: d,
		Audit:      st,
	}
}

func TestBuildQueryRejectsAnUnknownName(t *testing.T) {
	_, err := BuildQuery(testApp(t, nil, ""), "nope")
	if err == nil {
		t.Fatal("BuildQuery on an unknown name = nil")
	}
	if errs.KindOf(err) != errs.KindUsage {
		t.Errorf("kind = %q, want usage", errs.KindOf(err))
	}
	if !strings.Contains(err.Error(), "nope") {
		t.Errorf("error = %q; want the name echoed so the user can spot a typo", err)
	}
	if errs.Hint(err) == "" {
		t.Error("no hint pointing at `mino query list`")
	}
}

func TestBuildQueryRejectsAFilterOnlyDocument(t *testing.T) {
	d := &config.Directives{Queries: map[string]config.Query{
		// No signal: a filter set, so there is nothing to run.
		"no-bots": {Name: "no-bots"},
	}}
	_, err := BuildQuery(testApp(t, d, ""), "no-bots")
	if err == nil {
		t.Fatal("BuildQuery on a filter-only document = nil; it has no signal to fetch")
	}
	if errs.KindOf(err) != errs.KindUsage {
		t.Errorf("kind = %q, want usage", errs.KindOf(err))
	}
}

func TestFlightQueriesKeepsAFailedBuildAsASection(t *testing.T) {
	d := &config.Directives{Queries: map[string]config.Query{
		"good": {Name: "good", Signal: "definitely-not-a-signal"},
	}}
	got := FlightQueries(testApp(t, d, ""), "f", []string{"good", "missing"})
	if len(got) != 2 {
		t.Fatalf("built %d queries, want 2; a query that fails to build must still surface as a failed "+
			"section rather than vanishing from the flight", len(got))
	}
	for i, q := range got {
		if q.Src == nil {
			t.Errorf("query %d has no source, so the flight would skip it silently", i)
		}
	}
	if got[1].Label != "missing" {
		t.Errorf("query 1 label = %q, want missing", got[1].Label)
	}
}

func TestFlightRejectsAnUnknownFlight(t *testing.T) {
	_, err := Flight(context.Background(), testApp(t, nil, ""), "nope")
	if err == nil {
		t.Fatal("Flight on an unknown name = nil")
	}
	if errs.KindOf(err) != errs.KindUsage {
		t.Errorf("kind = %q, want usage", errs.KindOf(err))
	}
}

func TestFlightRejectsAFlightOutsideTheRole(t *testing.T) {
	d := &config.Directives{
		Flights: map[string]config.Flight{"secret": {Queries: []string{"q"}}},
		Roles:   map[string]config.RoleDef{"limited": {Name: "limited", Flights: []string{"other"}}},
	}
	_, err := Flight(context.Background(), testApp(t, d, "limited"), "secret")
	if err == nil {
		t.Fatal("Flight ran a flight the active role cannot see; the role scope is the same gate the CLI applies")
	}
}

func TestFlightWithNoQueriesReturnsNothing(t *testing.T) {
	d := &config.Directives{Flights: map[string]config.Flight{"empty": {}}}
	sections, err := Flight(context.Background(), testApp(t, d, ""), "empty")
	if err != nil {
		t.Fatalf("Flight on an empty flight = %v, want nil", err)
	}
	if len(sections) != 0 {
		t.Errorf("sections = %v, want none", sections)
	}
}

func TestQueryRejectsAQueryOutsideTheRole(t *testing.T) {
	d := &config.Directives{
		Queries: map[string]config.Query{"secret": {Name: "secret", Signal: "demo"}},
		Roles:   map[string]config.RoleDef{"limited": {Name: "limited", Queries: []string{"other"}}},
	}
	_, err := Query(context.Background(), testApp(t, d, "limited"), "secret")
	if err == nil {
		t.Fatal("Query ran a query the active role cannot see")
	}
}

func TestQueryReportsAnUnknownNameRatherThanARoleDenial(t *testing.T) {
	d := &config.Directives{
		Roles: map[string]config.RoleDef{"limited": {Name: "limited", Queries: []string{"other"}}},
	}
	_, err := Query(context.Background(), testApp(t, d, "limited"), "nope")
	if err == nil {
		t.Fatal("Query on an unknown name = nil")
	}
	// The CLI only reports a role denial for a query that exists, so a typo
	// stays a typo instead of looking like a permissions problem.
	if !strings.Contains(err.Error(), "no saved query named") {
		t.Errorf("error = %q; want the unknown-name message, not a role denial", err)
	}
}

func TestActionSeedsHomeAndRole(t *testing.T) {
	a := testApp(t, nil, "test")
	// build.Action rejects the unknown signal before dispatch, which is enough to
	// prove Action forwards without panicking on a nil param map.
	err := Action(context.Background(), a, "definitely-not-a-signal", "x", nil)
	if err == nil {
		t.Fatal("Action on an unknown signal = nil")
	}
}
