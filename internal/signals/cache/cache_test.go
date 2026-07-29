package cache

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/plugin"
	"github.com/codyconfer/munin/internal/signals"
)

const (
	cacheableSignal = "cachetest"
	localSignal     = "cachetestlocal"
)

func init() {
	plugin.Register(plugin.Descriptor{
		ID: "munin.cachetest", Kind: plugin.KindSignal, Signal: cacheableSignal,
		Capabilities: []plugin.Capability{plugin.CapQuery, plugin.CapCacheable},
	})
	plugin.Register(plugin.Descriptor{
		ID: "munin.cachetestlocal", Kind: plugin.KindSignal, Signal: localSignal,
		Capabilities: []plugin.Capability{plugin.CapQuery},
	})
}

// fake counts calls so tests can tell a cache hit from a refetch.
type fake struct {
	calls    atomic.Int32
	title    string
	err      error
	failFrom int32
}

func (f *fake) Name() string { return cacheableSignal }

func (f *fake) Fetch(context.Context) ([]signals.Section, error) {
	n := f.calls.Add(1)
	if f.err != nil && n >= f.failFrom {
		return nil, f.err
	}
	return []signals.Section{{
		Signal: cacheableSignal,
		Title:  f.title,
		Items:  []signals.Item{{Title: "item"}},
	}}, nil
}

func newStore(t *testing.T, ttl string, mode Mode) *Store {
	t.Helper()
	s := New(t.TempDir(), config.CacheConfig{TTL: ttl}, "fp", mode)
	t.Cleanup(func() { s.Close() })
	return s
}

func fetch(t *testing.T, q signals.Signal) []signals.Section {
	t.Helper()
	secs, err := q.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	return secs
}

func TestHitAvoidsRefetch(t *testing.T) {
	s := newStore(t, "1h", ModeUse)
	f := &fake{title: "first"}
	q := s.Wrap(f, cacheableSignal, "", nil)

	if got := fetch(t, q)[0].Title; got != "first" {
		t.Fatalf("first fetch = %q", got)
	}
	f.title = "second"
	secs := fetch(t, q)
	if got := secs[0].Title; got != "first" {
		t.Errorf("second fetch = %q, want the cached %q", got, "first")
	}
	if n := f.calls.Load(); n != 1 {
		t.Errorf("inner Fetch called %d times, want 1", n)
	}
	if secs[0].Meta["cache"] != "hit" {
		t.Errorf("meta = %v, want cache=hit", secs[0].Meta)
	}
}

func TestExpiredEntryRefetches(t *testing.T) {
	s := newStore(t, "1ns", ModeUse)
	f := &fake{title: "first"}
	q := s.Wrap(f, cacheableSignal, "", nil)

	fetch(t, q)
	time.Sleep(time.Millisecond)
	f.title = "second"
	if got := fetch(t, q)[0].Title; got != "second" {
		t.Errorf("expired fetch = %q, want fresh %q", got, "second")
	}
	if n := f.calls.Load(); n != 2 {
		t.Errorf("inner Fetch called %d times, want 2", n)
	}
}

func TestKeyVariesByParamsRoleAndFingerprint(t *testing.T) {
	base := entryKey("s", "role", "fp", map[string]string{"a": "1"})
	cases := map[string]string{
		"different param value": entryKey("s", "role", "fp", map[string]string{"a": "2"}),
		"extra param":           entryKey("s", "role", "fp", map[string]string{"a": "1", "b": "2"}),
		"different role":        entryKey("s", "other", "fp", map[string]string{"a": "1"}),
		"different config":      entryKey("s", "role", "fp2", map[string]string{"a": "1"}),
		"different signal":      entryKey("t", "role", "fp", map[string]string{"a": "1"}),
	}
	for name, got := range cases {
		if got == base {
			t.Errorf("%s: key matched the base key %q", name, base)
		}
	}
	if again := entryKey("s", "role", "fp", map[string]string{"a": "1"}); again != base {
		t.Errorf("key not stable: %q vs %q", again, base)
	}
}

func TestFingerprintTracksSignalConfigOnly(t *testing.T) {
	cfg := config.Defaults()
	base := Fingerprint(cfg)

	cfg.Output, cfg.Role, cfg.Home, cfg.Timeout = "json", "work", "/tmp/x", "9s"
	cfg.Cache.TTL = "99m"
	if got := Fingerprint(cfg); got != base {
		t.Errorf("non-signal config changed the fingerprint: %q vs %q", got, base)
	}

	cfg.GitHub.Max = 999
	if got := Fingerprint(cfg); got == base {
		t.Error("github.max should change the fingerprint")
	}
}

func TestErrorIsNotCached(t *testing.T) {
	s := newStore(t, "1h", ModeUse)
	boom := errors.New("boom")
	f := &fake{title: "ok", err: boom, failFrom: 1}
	q := s.Wrap(f, cacheableSignal, "", nil)

	if _, err := q.Fetch(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("Fetch err = %v, want boom", err)
	}
	f.err = nil
	if got := fetch(t, q)[0].Title; got != "ok" {
		t.Errorf("after failure = %q, want a live fetch", got)
	}
}

func TestStaleFallbackOnError(t *testing.T) {
	s := newStore(t, "1ns", ModeUse)
	boom := errors.New("network down")
	f := &fake{title: "warm", err: boom, failFrom: 2}
	q := s.Wrap(f, cacheableSignal, "", nil)

	fetch(t, q) // populate
	time.Sleep(time.Millisecond)

	secs := fetch(t, q) // TTL expired, inner call fails
	if got := secs[0].Title; got != "warm" {
		t.Fatalf("stale fetch = %q, want the cached %q", got, "warm")
	}
	if secs[0].Meta["cache"] != "stale" {
		t.Errorf("meta = %v, want cache=stale", secs[0].Meta)
	}
	if secs[0].Meta["age"] == "" {
		t.Error("stale sections should carry an age")
	}
}

func TestStaleFallbackNeedsAnEntry(t *testing.T) {
	s := newStore(t, "1h", ModeUse)
	boom := errors.New("network down")
	q := s.Wrap(&fake{err: boom, failFrom: 1}, cacheableSignal, "", nil)
	if _, err := q.Fetch(context.Background()); !errors.Is(err, boom) {
		t.Errorf("Fetch err = %v, want boom passed through", err)
	}
}

func TestModeRefreshSkipsReadButWrites(t *testing.T) {
	home := t.TempDir()
	cfg := config.CacheConfig{TTL: "1h"}

	warm := New(home, cfg, "fp", ModeUse)
	f1 := &fake{title: "first"}
	fetch(t, warm.Wrap(f1, cacheableSignal, "", nil))
	warm.Close()

	refresh := New(home, cfg, "fp", ModeRefresh)
	f2 := &fake{title: "second"}
	if got := fetch(t, refresh.Wrap(f2, cacheableSignal, "", nil))[0].Title; got != "second" {
		t.Errorf("refresh fetch = %q, want a live %q", got, "second")
	}
	refresh.Close()

	after := New(home, cfg, "fp", ModeUse)
	defer after.Close()
	f3 := &fake{title: "third"}
	if got := fetch(t, after.Wrap(f3, cacheableSignal, "", nil))[0].Title; got != "second" {
		t.Errorf("after refresh = %q, want the stored %q", got, "second")
	}
	if n := f3.calls.Load(); n != 0 {
		t.Errorf("inner Fetch called %d times, want 0", n)
	}
}

func TestModeOffNeitherReadsNorWrites(t *testing.T) {
	home := t.TempDir()
	cfg := config.CacheConfig{TTL: "1h"}

	off := New(home, cfg, "fp", ModeOff)
	f := &fake{title: "first"}
	if wrapped := off.Wrap(f, cacheableSignal, "", nil); wrapped != signals.Signal(f) {
		t.Error("ModeOff should return the signal unwrapped")
	}
	fetch(t, off.Wrap(f, cacheableSignal, "", nil))
	off.Close()

	after := New(home, cfg, "fp", ModeUse)
	defer after.Close()
	f2 := &fake{title: "second"}
	if got := fetch(t, after.Wrap(f2, cacheableSignal, "", nil))[0].Title; got != "second" {
		t.Errorf("after ModeOff run = %q, want a live fetch; nothing should have been stored", got)
	}
}

func TestPassthrough(t *testing.T) {
	f := &fake{}
	tests := []struct {
		name  string
		store *Store
		sig   string
	}{
		{"nil store", nil, cacheableSignal},
		{"zero ttl", newStore(t, "0", ModeUse), cacheableSignal},
		{"empty ttl", newStore(t, "", ModeUse), cacheableSignal},
		{"invalid ttl", newStore(t, "nonsense", ModeUse), cacheableSignal},
		{"signal without CapCacheable", newStore(t, "1h", ModeUse), localSignal},
		{"unregistered signal", newStore(t, "1h", ModeUse), "nosuchsignal"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.store.Wrap(f, tc.sig, "", nil); got != signals.Signal(f) {
				t.Error("expected the signal back unwrapped")
			}
		})
	}
}

func TestPerSignalTTLOverridesCapability(t *testing.T) {
	home := t.TempDir()
	s := New(home, config.CacheConfig{
		TTL:     "1h",
		Signals: map[string]string{localSignal: "1h", cacheableSignal: "0"},
	}, "fp", ModeUse)
	defer s.Close()

	if s.TTL(localSignal) != time.Hour {
		t.Errorf("explicit ttl should cache a signal lacking CapCacheable, got %v", s.TTL(localSignal))
	}
	if s.TTL(cacheableSignal) != 0 {
		t.Errorf("explicit 0 should disable a CapCacheable signal, got %v", s.TTL(cacheableSignal))
	}
}

func TestDisabledCacheCreatesNoFile(t *testing.T) {
	home := t.TempDir()
	s := New(home, config.CacheConfig{TTL: "0"}, "fp", ModeUse)
	defer s.Close()
	fetch(t, s.Wrap(&fake{title: "a"}, cacheableSignal, "", nil))

	path := filepath.Join(home, config.DirData, config.CacheDB)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("cache.duckdb should not exist when caching is off; Stat = %v", err)
	}
}

func TestEnabledCacheCreatesFile(t *testing.T) {
	home := t.TempDir()
	s := New(home, config.CacheConfig{TTL: "1h"}, "fp", ModeUse)
	defer s.Close()
	fetch(t, s.Wrap(&fake{title: "a"}, cacheableSignal, "", nil))

	path := filepath.Join(home, config.DirData, config.CacheDB)
	if _, err := os.Stat(path); err != nil {
		t.Errorf("cache.duckdb should exist after a cached fetch: %v", err)
	}
}

func TestDetailTTL(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want time.Duration
	}{
		{"explicit", "90s", 90 * time.Second},
		{"empty falls back to the default", "", 5 * time.Minute},
		{"zero disables", "0", 0},
		{"invalid disables", "nonsense", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := New(t.TempDir(), config.CacheConfig{DetailTTL: c.raw}, "fp", ModeUse)
			defer s.Close()
			if got := s.DetailTTL(); got != c.want {
				t.Errorf("DetailTTL(%q) = %v, want %v", c.raw, got, c.want)
			}
		})
	}
	var nilStore *Store
	if got := nilStore.DetailTTL(); got != 0 {
		t.Errorf("nil DetailTTL = %v, want 0", got)
	}
}

func TestDetailTTLIsIndependentOfSignalTTL(t *testing.T) {
	s := New(t.TempDir(), config.CacheConfig{TTL: "0", DetailTTL: "5m"}, "fp", ModeUse)
	defer s.Close()
	if s.TTL(cacheableSignal) != 0 {
		t.Errorf("signal ttl = %v, want caching off", s.TTL(cacheableSignal))
	}
	if s.DetailTTL() != 5*time.Minute {
		t.Errorf("detail ttl = %v, want it unaffected by the signal ttl", s.DetailTTL())
	}
}

func TestDetailTTLDoesNotAffectTheFingerprint(t *testing.T) {
	cfg := config.Defaults()
	base := Fingerprint(cfg)
	cfg.Cache.DetailTTL = "99m"
	if got := Fingerprint(cfg); got != base {
		t.Errorf("detail ttl changed the fingerprint: %q vs %q", got, base)
	}
}

func TestReadsAndWritesFollowMode(t *testing.T) {
	cases := []struct {
		mode         Mode
		reads, wries bool
	}{
		{ModeUse, true, true},
		{ModeRefresh, false, true},
		{ModeOff, false, false},
	}
	for _, c := range cases {
		s := New(t.TempDir(), config.CacheConfig{TTL: "1h"}, "fp", c.mode)
		if s.Reads() != c.reads || s.Writes() != c.wries {
			t.Errorf("mode %v: reads=%v writes=%v, want %v/%v", c.mode, s.Reads(), s.Writes(), c.reads, c.wries)
		}
		s.Close()
	}
	var nilStore *Store
	if nilStore.Reads() || nilStore.Writes() {
		t.Error("a nil store should neither read nor write")
	}
}

func TestStatsAndClear(t *testing.T) {
	s := newStore(t, "1h", ModeUse)
	fetch(t, s.Wrap(&fake{title: "a"}, cacheableSignal, "", map[string]string{"q": "1"}))
	fetch(t, s.Wrap(&fake{title: "b"}, cacheableSignal, "", map[string]string{"q": "2"}))

	stats, err := s.Stats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 {
		t.Fatalf("stats = %+v, want one namespace", stats)
	}
	if stats[0].Label != cacheableSignal {
		t.Errorf("label = %q, want %q", stats[0].Label, cacheableSignal)
	}
	if stats[0].Entries != 2 || stats[0].Fresh != 2 {
		t.Errorf("entries=%d fresh=%d, want 2 and 2", stats[0].Entries, stats[0].Fresh)
	}

	n, err := s.Clear(context.Background(), Namespace(cacheableSignal))
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("Clear removed %d, want 2", n)
	}
	if stats, _ := s.Stats(context.Background()); len(stats) != 0 {
		t.Errorf("stats after clear = %+v, want empty", stats)
	}
}

func TestRosterCacheInterface(t *testing.T) {
	s := newStore(t, "1h", ModeUse)
	if _, ok := s.Get(context.Background(), "github:team", "acme/plat"); ok {
		t.Fatal("expected a miss")
	}
	s.Put(context.Background(), "github:team", "acme/plat", "alice\nbob", time.Now().Add(time.Hour))
	if v, ok := s.Get(context.Background(), "github:team", "acme/plat"); !ok || v != "alice\nbob" {
		t.Fatalf("Get = %q, %v", v, ok)
	}
}

func TestNilStoreIsUsableAsRosterCache(t *testing.T) {
	var s *Store
	if _, ok := s.Get(context.Background(), "ns", "k"); ok {
		t.Error("nil Get should miss")
	}
	s.Put(context.Background(), "ns", "k", "v", time.Now()) // must not panic
	if err := s.Close(); err != nil {
		t.Errorf("nil Close = %v", err)
	}
	if _, err := s.Stats(context.Background()); err == nil {
		t.Error("nil Stats should report unavailable")
	}
	if _, err := s.Clear(context.Background(), ""); err == nil {
		t.Error("nil Clear should report unavailable")
	}
}
