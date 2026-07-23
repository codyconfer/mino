package auth

import "time"

type Credential struct {
	AccessToken  string
	RefreshToken string
	Scope        string
	Expiry       time.Time
}

type TokenStore interface {
	Get(service string) (Credential, bool, error)
	Put(service string, c Credential) error
	Delete(service string) error
}

func getCred(store TokenStore, service string) (Credential, bool) {
	if store == nil {
		return Credential{}, false
	}
	c, ok, err := store.Get(service)
	if err != nil || !ok {
		return Credential{}, false
	}
	return c, true
}
