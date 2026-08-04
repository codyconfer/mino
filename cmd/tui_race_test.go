package cmd

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/codyconfer/mino/internal/app"
	"github.com/codyconfer/mino/internal/config"
)

const (
	deckRaceReads   = 4000
	deckRaceReloads = 200
)

func sharedForDeckRace(t *testing.T) *app.App {
	t.Helper()
	orig := shared
	t.Cleanup(func() { shared = orig })

	home := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		path := filepath.Join(home, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("ntr-list.yaml", "name: ntr-list\ntype: query\nsignal: ntr\n")
	write("ops-flight.yaml", "name: ops-flight\ntype: flight\nqueries: [ntr-list]\n")
	write("ops.yaml", "name: ops\ntype: role\nhome: ops-flight\nflights: [ops-flight]\n")
	write("triage.yaml", "name: triage\ntype: role\nhome: ops-flight\nflights: [ops-flight]\n")

	shared = &app.App{
		Cfg: &config.Config{Home: home, Output: "terminal", DefaultRole: "ops"},
		Directives: &config.Directives{
			Queries: map[string]config.Query{"ntr-list": {Name: "ntr-list", Signal: "ntr"}},
			Flights: map[string]config.Flight{"ops-flight": {Name: "ops-flight", Queries: []string{"ntr-list"}}},
			Roles: map[string]config.RoleDef{
				"ops":    {Name: "ops", Home: "ops-flight"},
				"triage": {Name: "triage", Home: "ops-flight"},
			},
			Formatters: map[string]config.FormatterDef{},
		},
	}
	return shared
}

func TestDeckFlightNameRaceWithRoleCycleAndReload(t *testing.T) {
	a := sharedForDeckRace(t)

	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		<-start
		for range deckRaceReads {
			_ = deckFlightName()
			_ = verifyFindings("roles")
		}
	}()
	go func() {
		defer wg.Done()
		names := []string{"triage", "ops"}
		<-start
		for i := range deckRaceReads {
			a.BeginRoleCycle(names[i%len(names)])
		}
	}()
	go func() {
		defer wg.Done()
		<-start
		for range deckRaceReloads {
			if err := a.RefreshDirectives(config.ReconcileIgnore); err != nil {
				t.Errorf("RefreshDirectives: %v", err)
				return
			}
		}
	}()
	close(start)
	wg.Wait()
}
