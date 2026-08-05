package app

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/filter"
)

func TestVisibleNamesRespectActiveRole(t *testing.T) {
	directives := &config.Directives{
		Queries: map[string]config.Query{
			"incidents": {Name: "incidents", Signal: "github"},
			"today":     {Name: "today", Signal: "calendar"},
			"no-bots":   {Name: "no-bots", Rules: []filter.Rule{{Exclude: "bot$"}}},
			"only-mine": {Name: "only-mine", Rules: []filter.Rule{{Include: "me"}}},
		},
		Flights: map[string]config.Flight{
			"triage":  {Name: "triage"},
			"default": {Name: "default"},
		},
		Roles: map[string]config.RoleDef{
			"triage": {Name: "triage", Flights: []string{"triage"}, Queries: []string{"incidents", "no-bots"}},
		},
	}

	a := &App{Cfg: &config.Config{DefaultRole: ""}, Directives: directives}
	if got := a.VisibleQueries(); !reflect.DeepEqual(got, []string{"incidents", "no-bots", "only-mine", "today"}) {
		t.Errorf("no role, queries = %v, want all", got)
	}
	if got := a.VisibleFlights(); !reflect.DeepEqual(got, []string{"default", "triage"}) {
		t.Errorf("no role, flights = %v, want all", got)
	}
	if got := a.VisibleFilters(); !reflect.DeepEqual(got, []string{"no-bots", "only-mine"}) {
		t.Errorf("no role, filters = %v, want all", got)
	}

	a.UseRole("triage")
	if got := a.VisibleQueries(); !reflect.DeepEqual(got, []string{"incidents", "no-bots"}) {
		t.Errorf("triage, queries = %v, want [incidents no-bots]", got)
	}
	if got := a.VisibleFlights(); !reflect.DeepEqual(got, []string{"triage"}) {
		t.Errorf("triage, flights = %v, want [triage]", got)
	}
	if got := a.VisibleFilters(); !reflect.DeepEqual(got, []string{"no-bots"}) {
		t.Errorf("triage, filters = %v, want [no-bots]", got)
	}
}

func TestReloadDirectivesPicksUpNewFiles(t *testing.T) {
	home := t.TempDir()
	a := &App{
		Cfg: &config.Config{Home: home},
		Directives: &config.Directives{
			Queries: map[string]config.Query{},
			Flights: map[string]config.Flight{},
			Roles:   map[string]config.RoleDef{},
		},
	}
	closeDBs(t, a)
	qdir := filepath.Join(home, config.DirQueries, "gh")
	tdir := filepath.Join(home, "team")
	if err := os.MkdirAll(qdir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(tdir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(qdir, "ntr-list.yaml"), []byte("name: ntr-list\ntype: query\nsignal: ntr\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tdir, "ntr.yaml"), []byte("name: ntr\ntype: flight\nqueries: [ntr-list]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "oncall.yaml"), []byte("name: oncall\ntype: role\nflights: [ntr]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := a.ReloadDirectives(); err != nil {
		t.Fatal(err)
	}
	if _, ok := a.Directives.Queries["ntr-list"]; !ok {
		t.Fatalf("queries missing ntr-list: %v", a.Directives.QueryNames())
	}
	if _, ok := a.Directives.Flights["ntr"]; !ok {
		t.Fatalf("flights missing ntr: %v", a.Directives.FlightNames())
	}
	if _, ok := a.Directives.Roles["oncall"]; !ok {
		t.Fatalf("roles missing oncall: %v", a.Directives.RoleNames())
	}
	if got := a.Directives.Source(config.TypeFlight, "ntr"); got != "team/ntr.yaml" {
		t.Fatalf("Source(flight, ntr) = %q, want team/ntr.yaml", got)
	}
}

func TestDirsIsUsableWithoutDirectives(t *testing.T) {
	cases := map[string]*App{
		"nil app":            nil,
		"app no directives":  {Cfg: &config.Config{}},
		"app no cfg":         {},
		"thin no directives": {Cfg: &config.Config{}, thin: true},
	}
	for name, a := range cases {
		t.Run(name, func(t *testing.T) {
			d := a.Dirs()
			if d == nil {
				t.Fatal("Dirs() returned nil; every a.Dirs().Roles / .RoleNames() caller nil-derefs")
			}
			if got := d.RoleNames(); len(got) != 0 {
				t.Errorf("RoleNames() = %v, want empty", got)
			}
			if len(d.Roles) != 0 || len(d.Queries) != 0 || len(d.Flights) != 0 || len(d.Formatters) != 0 {
				t.Error("empty directives should carry no entries")
			}
			_ = d.QueryNames()
			_ = d.FlightNames()
			_ = d.FilterNames()
			_ = d.FormatterNames()
			_ = d.RunnableNames()
			_ = d.Source(config.TypeRole, "ops")
			_ = d.DocCount("ops.yaml")
			if _, ok := a.RoleDef("ops"); ok {
				t.Error("RoleDef found a role in empty directives")
			}
			_ = a.Access()
			if got := a.VisibleQueries(); got != nil {
				t.Errorf("VisibleQueries() = %v, want nil", got)
			}
			if got := a.VisibleFilters(); got != nil {
				t.Errorf("VisibleFilters() = %v, want nil", got)
			}
			if got := a.VisibleFlights(); got != nil {
				t.Errorf("VisibleFlights() = %v, want nil", got)
			}
			if got := a.VisibleFormatters(); got != nil {
				t.Errorf("VisibleFormatters() = %v, want nil", got)
			}
			if err := a.NotInRoleError("query", "incidents"); err == nil {
				t.Error("NotInRoleError returned nil")
			}
		})
	}
}
