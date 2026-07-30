package auth

import (
	"context"
	"time"

	"github.com/codyconfer/munin/internal/log"
)

type Credential struct {
	AccessToken  string
	RefreshToken string
	Scope        string
	Expiry       time.Time
}

type TokenStore interface {
	Get(ctx context.Context, service string) (Credential, bool, error)
	Put(ctx context.Context, service string, c Credential) error
	Delete(ctx context.Context, service string) error
}

func getCred(store TokenStore, service string) (Credential, bool) {
	if store == nil {
		return Credential{}, false
	}
	c, ok, err := store.Get(context.Background(), service)
	if err != nil {
		log.Warnf("reading the stored %s credential: %v", service, err)
		return Credential{}, false
	}
	if !ok {
		return Credential{}, false
	}
	return c, true
}
