package statusstrip

import (
	"context"
	"sync"
	"testing"

	"github.com/codyconfer/mino/internal/app"
	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/testenv"
)

const raceIterations = 2000

func roleCycleApp(t *testing.T) *app.App {
	t.Helper()
	a := &app.App{
		Cfg: &config.Config{Home: t.TempDir(), DefaultRole: "ops"},
		Directives: &config.Directives{Roles: map[string]config.RoleDef{
			"ops":    {Name: "ops"},
			"triage": {Name: "triage"},
		}},
	}
	t.Cleanup(a.CloseDBs)
	return a
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
