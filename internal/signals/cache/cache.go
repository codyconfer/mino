package cache

import (
	"context"
	"encoding/json"
	"maps"
	"strings"
	"sync"
	"time"

	"github.com/codyconfer/sisyphus/kv"

	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/log"
	"github.com/codyconfer/mino/internal/plugin"
	"github.com/codyconfer/mino/internal/signals"
)

const staleGrace = 24 * time.Hour

type Mode int

const (
	ModeUse Mode = iota
	ModeRefresh
	ModeOff
)

type Store struct {
	home      string
	ttl       time.Duration
	detailTTL time.Duration
	perSig    map[string]time.Duration
	mode      Mode
	fp        string

	mu     sync.Mutex
	kv     *kv.Store
	opened bool
	closed bool
}

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
	return &Store{home: home, ttl: ttl, detailTTL: parseDetailTTL(cfg.DetailTTL), perSig: per, mode: mode, fp: fingerprint}
}

func parseDetailTTL(raw string) time.Duration {
	if raw == "" {
		raw = config.DefaultDetailTTL
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		log.Debugf("cache: invalid detail ttl %q; detail caching disabled", raw)
		return 0
	}
	return d
}

func (s *Store) DetailTTL() time.Duration {
	if s == nil {
		return 0
	}
	return s.detailTTL
}

func (s *Store) open(ctx context.Context) *kv.Store {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		log.Debugf("cache: dropping work issued after close")
		return nil
	}
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
	s.closed = true
	if s.kv == nil {
		return nil
	}
	err := s.kv.Close()
	s.kv = nil
	return err
}

func (s *Store) Reads() bool { return s != nil && s.mode == ModeUse }

func (s *Store) Writes() bool { return s != nil && s.mode != ModeOff }

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
		age := now.Sub(prev.FetchedAt)
		if hasPrev && age < staleGrace {
			log.Debugf("cache: %s fetch failed (%v); serving results from %s ago", c.ns, err, age.Round(time.Second))
			return mark(prev.Sections, "stale", age), nil
		}
		if hasPrev {
			log.Debugf("cache: %s fetch failed (%v); cached copy is %s old, past the %s grace window",
				c.ns, err, age.Round(time.Second), staleGrace)
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
	if err := store.Put(ctx, ns, key, string(raw), p.FetchedAt.Add(ttl+staleGrace)); err != nil {
		log.Debugf("cache: write failed: %v", err)
	}
}

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
	WithHint("another mino process may hold the lock on .data/cache.duckdb")

type Stat struct {
	Namespace string
	Label     string
	Entries   int
	Expired   int
	Fresh     int
	Oldest    time.Time
	Newest    time.Time
}

func (s *Store) Stats(ctx context.Context) ([]Stat, error) {
	store := s.open(ctx)
	if store == nil {
		return nil, errUnavailable
	}
	rows, err := store.Stats(ctx, s.ttl)
	if err != nil {
		return nil, errs.Wrap(errs.KindStore, err, "summarize cache namespaces")
	}
	out := make([]Stat, 0, len(rows))
	idx := make(map[string]int, len(rows))
	windows := map[time.Duration][]string{}
	for _, r := range rows {
		if r.Entries == 0 {
			continue
		}
		st := Stat{
			Namespace: r.Namespace,
			Label:     r.Namespace,
			Entries:   int(r.Entries),
			Expired:   int(r.Expired),
			Oldest:    r.Oldest,
			Newest:    r.Newest,
		}
		if signal, isSignal := SignalOf(r.Namespace); !isSignal {
			st.Fresh = st.Entries
		} else {
			st.Label = signal
			if ttl := s.TTL(signal); ttl == s.ttl {
				st.Fresh = int(r.Fresh)
			} else if ttl > 0 {
				windows[ttl] = append(windows[ttl], r.Namespace)
			}
		}
		idx[r.Namespace] = len(out)
		out = append(out, st)
	}
	for window, names := range windows {
		rows, err := store.Stats(ctx, window)
		if err != nil {
			return nil, errs.Wrap(errs.KindStore, err, "summarize cache namespaces")
		}
		fresh := make(map[string]int64, len(rows))
		for _, r := range rows {
			fresh[r.Namespace] = r.Fresh
		}
		for _, ns := range names {
			out[idx[ns]].Fresh = int(fresh[ns])
		}
	}
	return out, nil
}

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

func (s *Store) ClearSignal(ctx context.Context, signal string) (int64, error) {
	store := s.open(ctx)
	if store == nil {
		return 0, errUnavailable
	}
	var total int64
	for _, ns := range s.signalNamespaces(ctx, store, signal) {
		n, err := store.Clear(ctx, ns)
		if err != nil {
			return total, errs.Wrap(errs.KindStore, err, "clear cache")
		}
		total += n
	}
	return total, nil
}

func (s *Store) signalNamespaces(ctx context.Context, store *kv.Store, signal string) []string {
	out := []string{Namespace(signal)}
	if signal == "" {
		return out
	}
	rows, err := store.Stats(ctx, 0)
	if err != nil {
		log.Debugf("cache: listing namespaces failed: %v", err)
		return out
	}
	prefix := signal + ":"
	for _, r := range rows {
		if _, isSignal := SignalOf(r.Namespace); isSignal {
			continue
		}
		if strings.HasPrefix(r.Namespace, prefix) {
			out = append(out, r.Namespace)
		}
	}
	return out
}
