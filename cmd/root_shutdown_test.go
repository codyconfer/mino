package cmd

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/codyconfer/munin/internal/app"
	"github.com/codyconfer/munin/internal/audit"
	"github.com/codyconfer/munin/internal/auth"
	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/signals"
	"github.com/codyconfer/munin/internal/signals/cache"
	"github.com/codyconfer/munin/internal/testenv"
	"github.com/codyconfer/munin/internal/token"
)

func sharedForShutdown(t *testing.T) string {
	t.Helper()
	testenv.Isolate(t)
	orig := shared
	t.Cleanup(func() { shared = orig })

	home := t.TempDir()
	au, err := audit.Open(context.Background(), config.DataPath(home, config.AuditDB))
	if err != nil {
		t.Fatalf("audit.Open: %v", err)
	}
	ts, err := token.Open(context.Background(), config.DataPath(home, config.TokensDB))
	if err != nil {
		t.Fatalf("token.Open: %v", err)
	}
	mgr, err := config.OpenStore(context.Background(), home)
	if err != nil {
		t.Fatalf("config.OpenStore: %v", err)
	}
	shared = &app.App{
		Cfg:        &config.Config{Home: home, Output: "terminal"},
		Directives: &config.Directives{},
		Audit:      au,
		Tokens:     ts,
		Mgr:        mgr,
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
	if shared.Tokens == nil || shared.Mgr == nil {
		t.Fatal("Shutdown nilled the token store or config store a leaked Cmd would read")
	}
	if _, _, err := shared.Tokens.Get(context.Background(), "github"); err == nil {
		t.Log("token store still answers reads after Close")
	}
	_, _ = shared.StoreRevision()
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
			cred := auth.Credential{AccessToken: "t", Expiry: time.Now().Add(time.Hour)}
			for range 40 {
				if shared.Directives == nil {
					panic("shared.Directives went nil under a leaked Cmd")
				}
				fid := shared.Audit.StartFlight("leaked", shared.Role())
				shared.Audit.RecordQuery(fid, "leaked", shared.Role(), now, time.Now(), secs)
				shared.Audit.FinishFlight(fid)
				shared.Cache.Put(context.Background(), "github:detail", "k", "{}", time.Now().Add(time.Minute))
				_ = shared.Tokens.Put(context.Background(), "leaked", cred)
				_, _, _ = shared.Tokens.Get(context.Background(), "leaked")
				if shared.HasStore() {
					_, _ = shared.StoreRevision()
				}
			}
		}()
	}
	close(start)
	Shutdown()
	wg.Wait()

	if shared == nil {
		t.Fatal("Shutdown nilled shared out from under in-flight Cmds")
	}
	if shared.Tokens == nil || shared.Mgr == nil {
		t.Fatal("Shutdown nilled the token/config stores out from under in-flight Cmds")
	}
}
