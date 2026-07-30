package token

import (
	"context"
	"errors"

	"github.com/codyconfer/sisyphus/sealed"

	"github.com/codyconfer/munin/internal/auth"
	"github.com/codyconfer/munin/internal/errs"
)

const (
	namespace      = "tokens"
	keyringService = "munin"
	keyName        = "munin-token-key"
)

var errUnavailable = errs.New(errs.KindStore, "token store unavailable")

type Store struct {
	s *sealed.Store
}

func Open(ctx context.Context, path string) (*Store, error) {
	s, err := sealed.Open(ctx, path, sealed.Options{
		Namespace:      namespace,
		KeyringService: keyringService,
		KeyName:        keyName,
	})
	if err != nil {
		return nil, errs.Wrap(errs.KindStore, err, "open token store")
	}
	return &Store{s: s}, nil
}

func OpenWithKey(ctx context.Context, path string, keyProvider func(context.Context) ([]byte, error)) (*Store, error) {
	s, err := sealed.Open(ctx, path, sealed.Options{
		Namespace:   namespace,
		KeyProvider: keyProvider,
	})
	if err != nil {
		return nil, errs.Wrap(errs.KindStore, err, "open token store")
	}
	return &Store{s: s}, nil
}

func (s *Store) Close() error {
	if s == nil || s.s == nil {
		return nil
	}
	if err := s.s.Close(); err != nil {
		return errs.Wrap(errs.KindStore, err, "close token store")
	}
	return nil
}

func (s *Store) Get(ctx context.Context, service string) (auth.Credential, bool, error) {
	if s == nil || s.s == nil {
		return auth.Credential{}, false, nil
	}
	e, ok, err := s.s.Get(ctx, service)
	if err != nil {
		if errors.Is(err, sealed.ErrUndecodable) {
			return auth.Credential{}, false, errs.Wrapf(errs.KindAuth, err, "read %s token", service).
				WithHint("the token store cannot be decrypted with the current key: delete tokens.duckdb in the munin data directory and run `munin login %s` again", service)
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
	if s == nil || s.s == nil {
		return errUnavailable
	}
	err := s.s.Put(ctx, service, sealed.Entry{
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
	if s == nil || s.s == nil {
		return errUnavailable
	}
	if err := s.s.Delete(ctx, service); err != nil {
		if errors.Is(err, sealed.ErrUnavailable) {
			return errUnavailable
		}
		return errs.Wrap(errs.KindStore, err, "delete token")
	}
	return nil
}
