package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/codyconfer/munin/internal/app"
	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/signals/cache"
)

func useCacheTestApp(t *testing.T) *cache.Store {
	t.Helper()
	orig := shared
	t.Cleanup(func() { shared = orig })
	home := t.TempDir()
	store := cache.New(home, config.CacheConfig{TTL: "1h"}, "fp", cache.ModeUse)
	t.Cleanup(func() { store.Close() })
	shared = &app.App{
		Cfg:        &config.Config{Home: home, Output: "terminal", Timeout: "5s"},
		Directives: &config.Directives{},
		Cache:      store,
	}
	return store
}

func runCache(t *testing.T, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	root := &cobra.Command{Use: "munin", SilenceUsage: true, SilenceErrors: true}
	root.AddCommand(newCacheCmd())
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"cache"}, args...))
	if err := root.Execute(); err != nil {
		t.Fatalf("cache %v: %v", args, err)
	}
	return out.String()
}

func TestCacheClearSignalDropsItsAuxiliaryNamespaces(t *testing.T) {
	store := useCacheTestApp(t)
	ctx := context.Background()
	store.Put(ctx, cache.Namespace("github"), "k", "{}", time.Now().Add(time.Hour))
	store.Put(ctx, "github:team", "acme/plat", "alice", time.Now().Add(24*time.Hour))
	store.Put(ctx, "github:detail", "acme/plat#1", "{}", time.Now().Add(5*time.Minute))
	store.Put(ctx, "slack:detail", "c1", "{}", time.Now().Add(5*time.Minute))

	runCache(t, "clear", "github")

	stats, err := store.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range stats {
		if strings.HasPrefix(s.Namespace, "github") {
			t.Errorf("%s still holds %d entry/entries after `cache clear github`; stats lists it, so clearing "+
				"the signal must clear it too", s.Namespace, s.Entries)
		}
	}
	var kept bool
	for _, s := range stats {
		if s.Namespace == "slack:detail" {
			kept = true
		}
	}
	if !kept {
		t.Errorf("clearing github also dropped slack:detail; stats = %+v", stats)
	}
}

func TestCacheClearNamespaceClearsOnlyThatNamespace(t *testing.T) {
	store := useCacheTestApp(t)
	ctx := context.Background()
	store.Put(ctx, cache.Namespace("github"), "k", "{}", time.Now().Add(time.Hour))
	store.Put(ctx, "github:team", "acme/plat", "alice", time.Now().Add(24*time.Hour))

	if out := runCache(t, "clear", "github:team"); !strings.Contains(out, "cleared 1 entry from github:team") {
		t.Errorf("output = %q, want one entry cleared from github:team", out)
	}
	if n, err := store.Clear(ctx, cache.Namespace("github")); err != nil || n != 1 {
		t.Errorf("signal:github = %d rows (err %v), want the 1 result row untouched", n, err)
	}
}

func TestCacheClearAllDropsEverything(t *testing.T) {
	store := useCacheTestApp(t)
	ctx := context.Background()
	store.Put(ctx, cache.Namespace("github"), "k", "{}", time.Now().Add(time.Hour))
	store.Put(ctx, "github:team", "acme/plat", "alice", time.Now().Add(24*time.Hour))

	if out := runCache(t, "clear"); !strings.Contains(out, "cleared 2 entries from cache") {
		t.Errorf("output = %q, want both entries cleared", out)
	}
	stats, err := store.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 0 {
		t.Errorf("stats after a full clear = %+v, want empty", stats)
	}
}

func TestCompleteCacheTargetsOffersWhatStatsShows(t *testing.T) {
	store := useCacheTestApp(t)
	ctx := context.Background()
	store.Put(ctx, "github:team", "acme/plat", "alice", time.Now().Add(24*time.Hour))
	store.Put(ctx, "github:detail", "acme/plat#1", "{}", time.Now().Add(5*time.Minute))

	names, _ := completeCacheTargets(&cobra.Command{}, nil, "")
	for _, want := range []string{"github", "github:team", "github:detail"} {
		if !containsCompletion(names, want) {
			t.Errorf("completions %v do not offer %q, which `cache stats` displays and `cache clear` accepts",
				names, want)
		}
	}
}

func containsCompletion(names []string, want string) bool {
	for _, n := range names {
		if n == want || strings.HasPrefix(n, want+"\t") {
			return true
		}
	}
	return false
}
