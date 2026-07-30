package cache

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/signals"
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
		TTL:     "1h",
		Signals: map[string]string{localSignal: "1ns"},
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
	for _, st := range stats {
		switch st.Label {
		case cacheableSignal:
			if st.Entries != 1 || st.Fresh != 1 {
				t.Errorf("%s entries=%d fresh=%d, want 1 and 1", st.Label, st.Entries, st.Fresh)
			}
		case localSignal:
			if st.Entries != 1 || st.Fresh != 0 {
				t.Errorf("%s entries=%d fresh=%d, want 1 and 0 under a 1ns ttl", st.Label, st.Entries, st.Fresh)
			}
		default:
			t.Errorf("unexpected namespace %q", st.Namespace)
		}
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
