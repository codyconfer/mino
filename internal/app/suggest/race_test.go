package suggest

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/codyconfer/mino/internal/app"
	"github.com/codyconfer/mino/internal/config"
)

const (
	suggestRaceReads   = 4000
	suggestRaceReloads = 200
)

func reloadableApp(t *testing.T) *app.App {
	t.Helper()
	home := t.TempDir()
	files := map[string]string{
		"ntr-list.yaml":   "name: ntr-list\ntype: query\nsignal: ntr\n",
		"ops-flight.yaml": "name: ops-flight\ntype: flight\nqueries: [ntr-list]\n",
		"ops.yaml":        "name: ops\ntype: role\nhome: ops-flight\nflights: [ops-flight]\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(home, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return &app.App{
		Cfg: &config.Config{Home: home},
		Directives: &config.Directives{
			Queries:    map[string]config.Query{},
			Flights:    map[string]config.Flight{},
			Roles:      map[string]config.RoleDef{},
			Formatters: map[string]config.FormatterDef{},
		},
	}
}

func TestRoleNamesRaceWithDirectiveReload(t *testing.T) {
	a := reloadableApp(t)

	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		for range suggestRaceReads {
			_ = RoleNames(a)
		}
	}()
	go func() {
		defer wg.Done()
		<-start
		for range suggestRaceReloads {
			if err := a.RefreshDirectives(config.ReconcileIgnore); err != nil {
				t.Errorf("RefreshDirectives: %v", err)
				return
			}
		}
	}()
	close(start)
	wg.Wait()
}

func TestRoleNamesToleratesNilDirectives(t *testing.T) {
	if got := RoleNames(&app.App{Cfg: &config.Config{}}); len(got) != 0 {
		t.Fatalf("RoleNames with no directives = %v, want empty", got)
	}
	if got := RoleNames(nil); len(got) != 0 {
		t.Fatalf("RoleNames(nil) = %v, want empty", got)
	}
}
