package state

import (
	"context"
	"sync"
	"time"

	"github.com/codyconfer/sisyphus/kv"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/log"
)

const (
	nsRole    = "role"
	keyActive = "active"
)

type Store struct {
	path string

	mu     sync.Mutex
	kv     *kv.Store
	opened bool
	closed bool
}

func New(path string) *Store { return &Store{path: path} }

func Open(ctx context.Context, path string) (*Store, error) {
	s, err := kv.Open(ctx, path)
	if err != nil {
		return nil, errs.Wrap(errs.KindStore, err, "open state store")
	}
	return &Store{path: path, kv: s, opened: true}, nil
}

func (s *Store) store(ctx context.Context) *kv.Store {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		log.Debugf("state: dropping work issued after close")
		return nil
	}
	if s.opened {
		return s.kv
	}
	s.opened = true
	if s.path == "" {
		return nil
	}
	store, err := kv.Open(ctx, s.path)
	if err != nil {
		log.Debugf("state disabled: %v", err)
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
	store := s.kv
	s.kv = nil
	if err := store.Close(); err != nil {
		return errs.Wrap(errs.KindStore, err, "close state store")
	}
	return nil
}

func (s *Store) ActiveRole(ctx context.Context) (string, bool) {
	store := s.store(ctx)
	if store == nil {
		return "", false
	}
	e, found, err := store.Get(ctx, nsRole, keyActive)
	if err != nil {
		log.Debugf("state: reading active role: %v", err)
		return "", false
	}
	if !found {
		return "", false
	}
	return e.Value, true
}

func (s *Store) SetActiveRole(ctx context.Context, name string) error {
	store := s.store(ctx)
	if store == nil {
		return errs.New(errs.KindStore, "the state store is unavailable").
			WithHint("check that %s is writable", s.Path())
	}
	if err := store.Put(ctx, nsRole, keyActive, name, time.Time{}); err != nil {
		return errs.Wrap(errs.KindStore, err, "record the active role")
	}
	return nil
}

func (s *Store) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}
