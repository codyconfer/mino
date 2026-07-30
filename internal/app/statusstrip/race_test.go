package statusstrip

import (
	"context"
	"sync"
	"testing"

	"github.com/codyconfer/munin/internal/app"
	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/testenv"
)

const raceIterations = 2000

func roleCycleApp(t *testing.T) *app.App {
	t.Helper()
	return &app.App{
		Cfg: &config.Config{Home: t.TempDir(), Role: "ops"},
		Directives: &config.Directives{Roles: map[string]config.RoleDef{
			"ops":    {Name: "ops"},
			"triage": {Name: "triage"},
		}},
	}
}

func TestProviderRaceWithRoleCycle(t *testing.T) {
	testenv.Isolate(t)
	t.Cleanup(resetChips)
	resetChips()

	a := roleCycleApp(t)
	provider := Provider(a)

	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		ctx := context.Background()
		<-start
		for range raceIterations {
			_ = provider(ctx)
		}
	}()
	go func() {
		defer wg.Done()
		names := []string{"triage", "ops"}
		<-start
		for i := range raceIterations {
			a.BeginRoleCycle(names[i%len(names)])
		}
	}()
	close(start)
	wg.Wait()
}
