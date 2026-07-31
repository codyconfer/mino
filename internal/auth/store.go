package auth

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/codyconfer/sisyphus/sealed"

	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/log"
	"github.com/codyconfer/munin/plugin"
)

type Credential = plugin.Credential

type TokenStore = plugin.CredentialStore

type CredentialState int

const (
	CredMissing CredentialState = iota
	CredUnreadable
	CredUnavailable
	CredExpired
	CredValid
)

func (s CredentialState) String() string {
	switch s {
	case CredUnreadable:
		return "unreadable"
	case CredUnavailable:
		return "unavailable"
	case CredExpired:
		return "expired"
	case CredValid:
		return "valid"
	default:
		return "missing"
	}
}

func unreadableErr(service string, cause error) *errs.Error {
	return errs.Wrapf(errs.KindAuth, cause, "the stored %s credential cannot be decrypted with the current key", service).
		WithHint("the credential store was written with a different encryption key (a restored or copied tokens.duckdb, or a lost keyring entry): delete tokens.duckdb in the munin data directory, then run `munin login %s` again", service)
}

var credStore struct {
	mu  sync.Mutex
	err error
}

func noteCredentialStoreError(err error) {
	credStore.mu.Lock()
	credStore.err = err
	credStore.mu.Unlock()
}

func CredentialStoreError() error {
	credStore.mu.Lock()
	defer credStore.mu.Unlock()
	return credStore.err
}

func ClearCredentialStoreError() {
	credStore.mu.Lock()
	credStore.err = nil
	credStore.mu.Unlock()
}

func ReadCredential(store TokenStore, service string) (Credential, CredentialState, error) {
	if store == nil {
		return Credential{}, CredMissing, nil
	}
	c, ok, err := store.Get(context.Background(), service)
	if err != nil {
		if errors.Is(err, sealed.ErrUndecodable) {
			wrapped := unreadableErr(service, err)
			noteCredentialStoreError(wrapped)
			log.Errorf("%v", wrapped)
			return Credential{}, CredUnreadable, wrapped
		}
		log.Warnf("reading the stored %s credential: %v", service, err)
		return Credential{}, CredUnavailable, err
	}
	if !ok {
		return Credential{}, CredMissing, nil
	}
	if !c.Expiry.IsZero() && !c.Expiry.After(time.Now()) {
		return c, CredExpired, nil
	}
	return c, CredValid, nil
}

func getCred(store TokenStore, service string) (Credential, bool) {
	c, state, _ := ReadCredential(store, service)
	switch state {
	case CredValid, CredExpired:
		return c, true
	default:
		return Credential{}, false
	}
}
