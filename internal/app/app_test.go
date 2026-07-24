package app

import (
	"reflect"
	"testing"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/filter"
)

func TestVisibleNamesRespectActiveRole(t *testing.T) {
	directives := &config.Directives{
		Queries: map[string]config.Query{
			"incidents": {Name: "incidents"},
			"today":     {Name: "today"},
		},
		Filters: map[string]filter.Filter{
			"no-bots":   {Name: "no-bots"},
			"only-mine": {Name: "only-mine"},
		},
		Flights: map[string]config.Flight{
			"triage":  {Name: "triage"},
			"default": {Name: "default"},
		},
		Roles: map[string]config.RoleDef{
			"triage": {Name: "triage", Flights: []string{"triage"}, Queries: []string{"incidents"}, Filters: []string{"no-bots"}},
		},
	}

	a := &App{Cfg: &config.Config{Role: ""}, Directives: directives}
	if got := a.VisibleQueries(); !reflect.DeepEqual(got, []string{"incidents", "today"}) {
		t.Errorf("no role, queries = %v, want all", got)
	}
	if got := a.VisibleFlights(); !reflect.DeepEqual(got, []string{"default", "triage"}) {
		t.Errorf("no role, flights = %v, want all", got)
	}
	if got := a.VisibleFilters(); !reflect.DeepEqual(got, []string{"no-bots", "only-mine"}) {
		t.Errorf("no role, filters = %v, want all", got)
	}

	a.Cfg = &config.Config{Role: "triage"}
	if got := a.VisibleQueries(); !reflect.DeepEqual(got, []string{"incidents"}) {
		t.Errorf("triage, queries = %v, want [incidents]", got)
	}
	if got := a.VisibleFlights(); !reflect.DeepEqual(got, []string{"triage"}) {
		t.Errorf("triage, flights = %v, want [triage]", got)
	}
	if got := a.VisibleFilters(); !reflect.DeepEqual(got, []string{"no-bots"}) {
		t.Errorf("triage, filters = %v, want [no-bots]", got)
	}
}
