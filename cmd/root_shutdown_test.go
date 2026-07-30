package cmd

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/codyconfer/munin/internal/app"
	"github.com/codyconfer/munin/internal/audit"
	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/signals"
	"github.com/codyconfer/munin/internal/signals/cache"
)

func sharedForShutdown(t *testing.T) string {
	t.Helper()
	orig := shared
	t.Cleanup(func() { shared = orig })

	home := t.TempDir()
	au, err := audit.Open(context.Background(), config.DataPath(home, config.AuditDB))
	if err != nil {
		t.Fatalf("audit.Open: %v", err)
	}
	shared = &app.App{
		Cfg:        &config.Config{Home: home, Output: "terminal"},
		Directives: &config.Directives{},
		Audit:      au,
		Cache:      cache.New(home, config.CacheConfig{TTL: "1h"}, "fp", cache.ModeUse),
	}
	return home
}

func TestShutdownKeepsSharedApp(t *testing.T) {
	sharedForShutdown(t)

	Shutdown()

	if shared == nil {
		t.Fatal("Shutdown nilled shared; leaked bubbletea Cmd goroutines still dereference it")
	}
	if shared.Directives == nil || shared.Cfg == nil {
		t.Fatal("Shutdown emptied the app a leaked Cmd would read")
	}
	Shutdown()
}

func TestShutdownToleratesInFlightCmds(t *testing.T) {
	sharedForShutdown(t)

	var wg sync.WaitGroup
	start := make(chan struct{})
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			now := time.Now()
			secs := []signals.Section{{Signal: "github", Title: "in flight", Items: []signals.Item{{Title: "pr"}}}}
			for range 40 {
				if shared.Directives == nil {
					panic("shared.Directives went nil under a leaked Cmd")
				}
				fid := shared.Audit.StartFlight("leaked", shared.Cfg.Role)
				shared.Audit.RecordQuery(fid, "leaked", shared.Cfg.Role, now, time.Now(), secs)
				shared.Audit.FinishFlight(fid)
				shared.Cache.Put(context.Background(), "github:detail", "k", "{}", time.Now().Add(time.Minute))
			}
		}()
	}
	close(start)
	Shutdown()
	wg.Wait()

	if shared == nil {
		t.Fatal("Shutdown nilled shared out from under in-flight Cmds")
	}
}
