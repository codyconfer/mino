package cache

import (
	"context"
	"encoding/json"
	"maps"
	"sync"
	"time"

	"github.com/codyconfer/sisyphus/kv"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/log"
	"github.com/codyconfer/munin/internal/plugin"
	"github.com/codyconfer/munin/internal/signals"
)

// staleGrace is how long past its TTL an entry survives so a failed fetch can fall back to it.
const staleGrace = 24 * time.Hour

type Mode int

const (
	ModeUse     Mode = iota // read and write
	ModeRefresh             // skip the read, still write
	ModeOff                 // do neither
)

// Store caches signal results in a DuckDB kv table. Every method is safe on a nil
// receiver and every failure degrades to a miss, so callers never have to guard.
type Store struct {
	home   string
	ttl    time.Duration
	perSig map[string]time.Duration
	mode   Mode
	fp     string

	mu     sync.Mutex
	kv     *kv.Store
	opened bool
}

// New builds a store without touching disk. The kv file is opened on first use, so a
// run that caches nothing never creates cache.duckdb.
func New(home string, cfg config.CacheConfig, fingerprint string, mode Mode) *Store {
	per := map[string]time.Duration{}
	for name, raw := range cfg.Signals {
		d, err := time.ParseDuration(raw)
		if err != nil {
			log.Debugf("cache: signal %q has invalid ttl %q; ignoring", name, raw)
			continue
		}
		per[name] = d
	}
	var ttl time.Duration
	if cfg.TTL != "" {
		d, err := time.ParseDuration(cfg.TTL)
		if err != nil {
			log.Debugf("cache: invalid ttl %q; caching disabled", cfg.TTL)
		} else {
			ttl = d
		}
	}
	return &Store{home: home, ttl: ttl, perSig: per, mode: mode, fp: fingerprint}
}

func (s *Store) open(ctx context.Context) *kv.Store {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.opened {
		return s.kv
	}
	s.opened = true
	if s.home == "" {
		return nil
	}
	store, err := kv.Open(ctx, config.DataPath(s.home, config.CacheDB))
	if err != nil {
		log.Debugf("cache: unavailable: %v", err)
		return nil
	}
	s.kv = store
	return store
}

func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.kv == nil {
		s.opened = false
		return nil
	}
	err := s.kv.Close()
	s.kv, s.opened = nil, false
	return err
}

// TTL reports how long results for a signal stay fresh. Zero means don't cache.
// An explicit cache.signals entry always wins, so "0" force-disables one signal and
// any other value force-enables one that doesn't advertise CapCacheable.
func (s *Store) TTL(signal string) time.Duration {
	if s == nil {
		return 0
	}
	if d, ok := s.perSig[signal]; ok {
		return d
	}
	if !plugin.HasCapability(signal, plugin.CapCacheable) {
		return 0
	}
	return s.ttl
}

// Wrap decorates a signal with read-through caching, or returns it untouched when
// this signal isn't cacheable.
func (s *Store) Wrap(q signals.Signal, signal, role string, params map[string]string) signals.Signal {
	if s == nil || q == nil || s.mode == ModeOff {
		return q
	}
	ttl := s.TTL(signal)
	if ttl <= 0 {
		return q
	}
	return &cached{
		store: s,
		inner: q,
		ttl:   ttl,
		ns:    Namespace(signal),
		key:   entryKey(signal, role, s.fp, params),
	}
}

type payload struct {
	FetchedAt time.Time         `json:"fetched_at"`
	Sections  []signals.Section `json:"sections"`
}

type cached struct {
	store *Store
	inner signals.Signal
	ttl   time.Duration
	ns    string
	key   string
}

func (c *cached) Name() string { return c.inner.Name() }

func (c *cached) Fetch(ctx context.Context) ([]signals.Section, error) {
	now := time.Now()
	prev, hasPrev := c.store.load(ctx, c.ns, c.key)

	if c.store.mode == ModeUse && hasPrev {
		if age := now.Sub(prev.FetchedAt); age < c.ttl {
			log.Debugf("cache: hit %s (age %s, ttl %s)", c.ns, age.Round(time.Second), c.ttl)
			return mark(prev.Sections, "hit", age), nil
		}
	}
	log.Debugf("cache: miss %s", c.ns)

	sections, err := c.inner.Fetch(ctx)
	if err != nil {
		if hasPrev {
			age := now.Sub(prev.FetchedAt)
			log.Debugf("cache: %s fetch failed (%v); serving results from %s ago", c.ns, err, age.Round(time.Second))
			return mark(prev.Sections, "stale", age), nil
		}
		return nil, err
	}
	c.store.save(ctx, c.ns, c.key, payload{FetchedAt: now, Sections: sections}, c.ttl)
	return sections, nil
}

func mark(sections []signals.Section, state string, age time.Duration) []signals.Section {
	out := make([]signals.Section, len(sections))
	copy(out, sections)
	for i := range out {
		meta := make(map[string]string, len(out[i].Meta)+2)
		maps.Copy(meta, out[i].Meta)
		meta["cache"] = state
		meta["age"] = age.Round(time.Second).String()
		out[i].Meta = meta
	}
	return out
}

func (s *Store) load(ctx context.Context, ns, key string) (payload, bool) {
	store := s.open(ctx)
	if store == nil {
		return payload{}, false
	}
	entry, found, err := store.Get(ctx, ns, key)
	if err != nil {
		log.Debugf("cache: read failed: %v", err)
		return payload{}, false
	}
	if !found {
		return payload{}, false
	}
	var p payload
	if err := json.Unmarshal([]byte(entry.Value), &p); err != nil {
		log.Debugf("cache: discarding unreadable entry %s/%s: %v", ns, key, err)
		return payload{}, false
	}
	return p, true
}

func (s *Store) save(ctx context.Context, ns, key string, p payload, ttl time.Duration) {
	store := s.open(ctx)
	if store == nil {
		return
	}
	raw, err := json.Marshal(p)
	if err != nil {
		log.Debugf("cache: encode failed: %v", err)
		return
	}
	// Keep the row past its TTL so a later failure can fall back to it. Freshness is
	// decided from FetchedAt, so changing cache.ttl takes effect on existing rows.
	grace := max(ttl, staleGrace)
	if err := store.Put(ctx, ns, key, string(raw), p.FetchedAt.Add(grace)); err != nil {
		log.Debugf("cache: write failed: %v", err)
	}
}

// Get and Put make Store a github.RosterCache, so the team roster shares this file
// instead of contending with the daemon over serve.duckdb.

func (s *Store) Get(ctx context.Context, namespace, key string) (string, bool) {
	store := s.open(ctx)
	if store == nil {
		return "", false
	}
	entry, found, err := store.Get(ctx, namespace, key)
	if err != nil {
		log.Debugf("cache: read failed: %v", err)
		return "", false
	}
	return entry.Value, found
}

func (s *Store) Put(ctx context.Context, namespace, key, value string, expiry time.Time) {
	store := s.open(ctx)
	if store == nil {
		return
	}
	if err := store.Put(ctx, namespace, key, value, expiry); err != nil {
		log.Debugf("cache: write failed: %v", err)
	}
}

var errUnavailable = errs.New(errs.KindStore, "cache store unavailable").
	WithHint("another munin process may hold the lock on .data/cache.duckdb")

type Stat struct {
	Namespace string
	Label     string
	Entries   int
	Fresh     int
	Oldest    time.Time
	Newest    time.Time
}

// Stats summarises what is currently cached. Unlike the read path this reports errors,
// because `munin cache stats` should say so rather than print an empty table.
func (s *Store) Stats(ctx context.Context) ([]Stat, error) {
	store := s.open(ctx)
	if store == nil {
		return nil, errUnavailable
	}
	names, err := store.Namespaces(ctx)
	if err != nil {
		return nil, errs.Wrap(errs.KindStore, err, "list cache namespaces")
	}
	now := time.Now()
	out := make([]Stat, 0, len(names))
	for _, ns := range names {
		entries, err := store.List(ctx, ns)
		if err != nil {
			return nil, errs.Wrapf(errs.KindStore, err, "list cache namespace %q", ns)
		}
		signal, isSignal := SignalOf(ns)
		st := Stat{Namespace: ns, Label: ns, Entries: len(entries)}
		if isSignal {
			st.Label = signal
		}
		ttl := s.TTL(signal)
		for _, e := range entries {
			var p payload
			at := e.Updated
			if isSignal && json.Unmarshal([]byte(e.Value), &p) == nil && !p.FetchedAt.IsZero() {
				at = p.FetchedAt
			}
			if ttl > 0 && now.Sub(at) < ttl {
				st.Fresh++
			}
			if st.Oldest.IsZero() || at.Before(st.Oldest) {
				st.Oldest = at
			}
			if at.After(st.Newest) {
				st.Newest = at
			}
		}
		out = append(out, st)
	}
	return out, nil
}

// Clear drops one namespace, or everything when namespace is empty.
func (s *Store) Clear(ctx context.Context, namespace string) (int64, error) {
	store := s.open(ctx)
	if store == nil {
		return 0, errUnavailable
	}
	n, err := store.Clear(ctx, namespace)
	if err != nil {
		return 0, errs.Wrap(errs.KindStore, err, "clear cache")
	}
	return n, nil
}
