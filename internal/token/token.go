package token

import (
	"context"
	"errors"
	"sync"

	"github.com/codyconfer/sisyphus/sealed"

	"github.com/codyconfer/mino/internal/auth"
	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/log"
)

const (
	namespace      = "tokens"
	keyringService = "mino"
	keyName        = "mino-token-key"
)

var errUnavailable = errs.New(errs.KindStore, "token store unavailable")

type Store struct {
	path string
	opts sealed.Options

	mu     sync.Mutex
	s      *sealed.Store
	opened bool
	closed bool
}

func defaultOptions() sealed.Options {
	return sealed.Options{
		Namespace:      namespace,
		KeyringService: keyringService,
		KeyName:        keyName,
	}
}

// New returns a store that opens path on first use.
func New(path string) *Store {
	return &Store{path: path, opts: defaultOptions()}
}

func Open(ctx context.Context, path string) (*Store, error) {
	return open(ctx, path, defaultOptions())
}

func OpenWithKey(ctx context.Context, path string, keyProvider func(context.Context) ([]byte, error)) (*Store, error) {
	return open(ctx, path, sealed.Options{Namespace: namespace, KeyProvider: keyProvider})
}

func open(ctx context.Context, path string, opts sealed.Options) (*Store, error) {
	s, err := sealed.Open(ctx, path, opts)
	if err != nil {
		return nil, errs.Wrap(errs.KindStore, err, "open token store")
	}
	return &Store{path: path, opts: opts, s: s, opened: true}, nil
}

// sealedStore opens the backing store on first use. It returns a nil store
// when the store never opened, and errUnavailable once it has been closed.
func (s *Store) sealedStore(ctx context.Context) (*sealed.Store, error) {
	if s == nil {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		log.Debugf("tokens: dropping work issued after close")
		return nil, errUnavailable
	}
	if s.opened {
		return s.s, nil
	}
	s.opened = true
	if s.path == "" {
		return nil, nil
	}
	store, err := sealed.Open(ctx, s.path, s.opts)
	if err != nil {
		log.Debugf("token store unavailable: %v", err)
		return nil, nil
	}
	s.s = store
	return store, nil
}

func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	if s.s == nil {
		return nil
	}
	store := s.s
	s.s = nil
	if err := store.Close(); err != nil {
		return errs.Wrap(errs.KindStore, err, "close token store")
	}
	return nil
}

func (s *Store) Get(ctx context.Context, service string) (auth.Credential, bool, error) {
	st, err := s.sealedStore(ctx)
	if err != nil {
		return auth.Credential{}, false, err
	}
	if st == nil {
		return auth.Credential{}, false, nil
	}
	e, ok, err := st.Get(ctx, service)
	if err != nil {
		if errors.Is(err, sealed.ErrUndecodable) {
			return auth.Credential{}, false, errs.Wrapf(errs.KindAuth, err, "read %s token", service).
				WithHint("the token store cannot be decrypted with the current key: delete tokens.duckdb in the mino data directory and run `mino login %s` again", service)
		}
		if errors.Is(err, sealed.ErrUnavailable) {
			return auth.Credential{}, false, errUnavailable
		}
		return auth.Credential{}, ok, errs.Wrap(errs.KindStore, err, "read token")
	}
	if !ok {
		return auth.Credential{}, false, nil
	}
	return auth.Credential{
		AccessToken:  e.AccessToken,
		RefreshToken: e.RefreshToken,
		Scope:        e.Scope,
		Expiry:       e.Expiry,
	}, true, nil
}

func (s *Store) Put(ctx context.Context, service string, c auth.Credential) error {
	st, _ := s.sealedStore(ctx)
	if st == nil {
		return errUnavailable
	}
	err := st.Put(ctx, service, sealed.Entry{
		AccessToken:  c.AccessToken,
		RefreshToken: c.RefreshToken,
		Scope:        c.Scope,
		Expiry:       c.Expiry,
	})
	if err != nil {
		if errors.Is(err, sealed.ErrUnavailable) {
			return errUnavailable
		}
		return errs.Wrap(errs.KindStore, err, "write token")
	}
	return nil
}

func (s *Store) Delete(ctx context.Context, service string) error {
	st, _ := s.sealedStore(ctx)
	if st == nil {
		return errUnavailable
	}
	if err := st.Delete(ctx, service); err != nil {
		if errors.Is(err, sealed.ErrUnavailable) {
			return errUnavailable
		}
		return errs.Wrap(errs.KindStore, err, "delete token")
	}
	return nil
}
