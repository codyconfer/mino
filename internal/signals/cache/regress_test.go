package cache

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/signals"
)

func seedAged(t *testing.T, s *Store, age time.Duration, title string) {
	t.Helper()
	p := payload{
		FetchedAt: time.Now().Add(-age),
		Sections: []signals.Section{{
			Signal: cacheableSignal,
			Title:  title,
			Items:  []signals.Item{{Title: "item"}},
		}},
	}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	s.Put(context.Background(), Namespace(cacheableSignal),
		entryKey(cacheableSignal, "", "fp", nil), string(raw), time.Now().Add(time.Hour))
}

func TestCloseIsTerminal(t *testing.T) {
	home := t.TempDir()
	s := New(home, config.CacheConfig{TTL: "1h"}, "fp", ModeUse)
	q := s.Wrap(&fake{title: "warm"}, cacheableSignal, "", nil)
	fetch(t, q)

	data := filepath.Join(home, config.DirData)
	if _, err := os.Stat(data); err != nil {
		t.Fatalf("expected a cache db after a warm fetch: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := os.RemoveAll(data); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}

	fetch(t, q)

	if _, err := os.Stat(data); !os.IsNotExist(err) {
		t.Errorf("a save after Close reopened the cache db (Stat = %v); Close must be terminal", err)
	}
	if _, err := s.Stats(context.Background()); !errors.Is(err, errUnavailable) {
		t.Errorf("Stats after Close = %v, want the unavailable error", err)
	}
	if err := s.Close(); err != nil {
		t.Errorf("second Close = %v, want nil", err)
	}
}

func TestStaleFallbackStopsAtTheGraceWindow(t *testing.T) {
	boom := errors.New("bad credentials")
	cases := []struct {
		name    string
		ttl     string
		age     time.Duration
		wantErr bool
	}{
		{"inside the grace window", "1m", 12 * time.Hour, false},
		{"past the grace window", "1m", 25 * time.Hour, true},
		{"past the grace window under a month-long ttl", "720h", 31 * 24 * time.Hour, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newStore(t, tc.ttl, ModeUse)
			seedAged(t, s, tc.age, "ancient")
			q := s.Wrap(&fake{err: boom, failFrom: 1}, cacheableSignal, "", nil)

			secs, err := q.Fetch(context.Background())
			switch {
			case tc.wantErr && !errors.Is(err, boom):
				t.Errorf("Fetch on a %s-old entry = %v, want the fetch error surfaced", tc.age, err)
			case !tc.wantErr && err != nil:
				t.Errorf("Fetch on a %s-old entry = %v, want the cached copy served", tc.age, err)
			case !tc.wantErr && (len(secs) != 1 || secs[0].Meta["cache"] != "stale"):
				t.Errorf("sections = %+v, want one marked cache=stale", secs)
			}
		})
	}
}

func TestStaleFallbackStopsAtTheGraceWindowInRefreshMode(t *testing.T) {
	s := newStore(t, "720h", ModeRefresh)
	seedAged(t, s, 27*24*time.Hour, "ancient")
	boom := errors.New("bad credentials")
	q := s.Wrap(&fake{err: boom, failFrom: 1}, cacheableSignal, "", nil)

	if _, err := q.Fetch(context.Background()); !errors.Is(err, boom) {
		t.Errorf("Fetch on a 27-day-old entry = %v, want the fetch error surfaced", err)
	}
}

func TestStatsCountsNonSignalNamespaces(t *testing.T) {
	s := newStore(t, "1h", ModeUse)
	ctx := context.Background()
	s.Put(ctx, "github:detail", "acme/plat#1", "{}", time.Now().Add(5*time.Minute))
	s.Put(ctx, "github:team", "acme/plat", "alice\nbob", time.Now().Add(24*time.Hour))

	stats, err := s.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byNS := map[string]Stat{}
	for _, st := range stats {
		byNS[st.Namespace] = st
	}
	for _, ns := range []string{"github:detail", "github:team"} {
		st, ok := byNS[ns]
		if !ok {
			t.Errorf("%s missing from stats %+v", ns, stats)
			continue
		}
		if st.Entries != 1 {
			t.Errorf("%s entries = %d, want 1", ns, st.Entries)
		}
		if st.Fresh != 1 {
			t.Errorf("%s fresh = %d, want 1", ns, st.Fresh)
		}
	}
}

func TestStatsHonorsPerSignalTTL(t *testing.T) {
	s := New(t.TempDir(), config.CacheConfig{
		TTL:     "1ns",
		Signals: map[string]string{localSignal: "1h"},
	}, "fp", ModeUse)
	t.Cleanup(func() { s.Close() })
	ctx := context.Background()

	fetch(t, s.Wrap(&fake{title: "a"}, cacheableSignal, "", nil))
	fetch(t, s.Wrap(&fake{title: "b"}, localSignal, "", nil))
	time.Sleep(2 * time.Millisecond)

	stats, err := s.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, st := range stats {
		switch st.Label {
		case cacheableSignal:
			seen++
			if st.Entries != 1 || st.Fresh != 0 {
				t.Errorf("%s entries=%d fresh=%d, want 1 and 0 under the global 1ns ttl", st.Label, st.Entries, st.Fresh)
			}
		case localSignal:
			seen++
			if st.Entries != 1 || st.Fresh != 1 {
				t.Errorf("%s entries=%d fresh=%d, want 1 and 1: its own 1h ttl must be counted against its own "+
					"window, not the global one", st.Label, st.Entries, st.Fresh)
			}
		default:
			t.Errorf("unexpected namespace %q", st.Namespace)
		}
	}
	if seen != 2 {
		t.Errorf("stats reported %d of the 2 signal namespaces: %+v", seen, stats)
	}
}

func TestSaveKeepsRowsForTheFullStaleGrace(t *testing.T) {
	const ttl = 720 * time.Hour
	s := newStore(t, "720h", ModeUse)
	ctx := context.Background()
	fetch(t, s.Wrap(&fake{title: "a"}, cacheableSignal, "", nil))

	store := s.open(ctx)
	if store == nil {
		t.Fatal("cache store unavailable")
	}
	entry, found, err := store.Get(ctx, Namespace(cacheableSignal), entryKey(cacheableSignal, "", "fp", nil))
	if err != nil || !found {
		t.Fatalf("Get after a warm fetch: found=%v err=%v", found, err)
	}
	want := time.Now().Add(ttl + staleGrace)
	if d := entry.Expiry.Sub(want); d > time.Minute || d < -time.Minute {
		t.Errorf("row expiry = %s, want ~%s (ttl + the %s stale grace): a row that expires at the ttl is swept "+
			"before the stale window it exists to cover, so the fallback has nothing to serve",
			entry.Expiry.UTC(), want.UTC(), staleGrace)
	}
}

func TestClearSignalDropsAuxiliaryNamespaces(t *testing.T) {
	s := newStore(t, "1h", ModeUse)
	ctx := context.Background()
	fetch(t, s.Wrap(&fake{title: "a"}, cacheableSignal, "", nil))
	s.Put(ctx, cacheableSignal+":team", "acme/plat", "alice", time.Now().Add(24*time.Hour))
	s.Put(ctx, cacheableSignal+":detail", "acme/plat#1", "{}", time.Now().Add(5*time.Minute))
	s.Put(ctx, cacheableSignal+":detail", "acme/plat#2", "{}", time.Now().Add(-time.Hour))
	s.Put(ctx, "othersignal:team", "acme/plat", "bob", time.Now().Add(24*time.Hour))
	s.Put(ctx, Namespace(localSignal), "k", "{}", time.Now().Add(time.Hour))

	n, err := s.ClearSignal(ctx, cacheableSignal)
	if err != nil {
		t.Fatal(err)
	}
	if n != 4 {
		t.Errorf("ClearSignal removed %d rows, want 4 (1 result, 1 roster, 2 detail rows including the expired one)", n)
	}

	for _, ns := range []string{Namespace(cacheableSignal), cacheableSignal + ":team", cacheableSignal + ":detail"} {
		left, err := s.Clear(ctx, ns)
		if err != nil {
			t.Fatal(err)
		}
		if left != 0 {
			t.Errorf("%s still held %d row(s) after clearing the signal", ns, left)
		}
	}
	if left, err := s.Clear(ctx, "othersignal:team"); err != nil || left != 1 {
		t.Errorf("othersignal:team = %d rows (err %v), want 1 left untouched", left, err)
	}
	if left, err := s.Clear(ctx, Namespace(localSignal)); err != nil || left != 1 {
		t.Errorf("%s = %d rows (err %v), want 1 left untouched", Namespace(localSignal), left, err)
	}
}

func TestClearSignalOnAnUnavailableStore(t *testing.T) {
	var nilStore *Store
	if _, err := nilStore.ClearSignal(context.Background(), "github"); err == nil {
		t.Error("ClearSignal on a nil store should report unavailable")
	}
}

func TestStatsDoesNotWriteOrSweep(t *testing.T) {
	s := newStore(t, "1h", ModeUse)
	ctx := context.Background()
	s.Put(ctx, "github:detail", "live", "{}", time.Now().Add(5*time.Minute))
	s.Put(ctx, "github:detail", "expired", "{}", time.Now().Add(-time.Hour))

	stats, err := s.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var seen bool
	for _, st := range stats {
		if st.Namespace != "github:detail" {
			continue
		}
		seen = true
		if st.Entries != 1 || st.Expired != 1 {
			t.Errorf("github:detail entries=%d expired=%d, want 1 and 1", st.Entries, st.Expired)
		}
	}
	if !seen {
		t.Errorf("github:detail missing from stats %+v", stats)
	}

	n, err := s.Clear(ctx, "github:detail")
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("Clear removed %d rows, want 2: Stats must not sweep or otherwise write", n)
	}
}

func TestStatsOmitsFullyExpiredNamespaces(t *testing.T) {
	s := newStore(t, "1h", ModeUse)
	ctx := context.Background()
	s.Put(ctx, "github:team", "acme/plat", "alice", time.Now().Add(-time.Hour))

	stats, err := s.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 0 {
		t.Errorf("stats = %+v, want nothing reported for a namespace holding only expired rows", stats)
	}
}
