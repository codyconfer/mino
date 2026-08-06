package slack

import (
	"context"

	"github.com/codyconfer/mino/plugin"
)

type testHost struct {
	params   map[string]string
	settings map[string]any
	creds    memStore
}

func newTestHost(params map[string]string, settings map[string]any) testHost {
	if params == nil {
		params = map[string]string{}
	}
	if settings == nil {
		settings = map[string]any{}
	}
	return testHost{params: params, settings: settings, creds: memStore{}}
}

func (h testHost) Params() map[string]string { return h.params }

func (h testHost) Home() string { return "" }

func (h testHost) Role() string { return "" }

func (h testHost) Settings(ns string) map[string]any {
	if ns != SignalName {
		return nil
	}
	return h.settings
}

func (h testHost) Credentials() plugin.CredentialStore { return h.creds }

type memStore map[string]plugin.Credential

func (m memStore) Get(_ context.Context, service string) (plugin.Credential, bool, error) {
	c, ok := m[service]
	return c, ok, nil
}

func (m memStore) Put(_ context.Context, service string, c plugin.Credential) error {
	m[service] = c
	return nil
}

func (m memStore) Delete(_ context.Context, service string) error {
	delete(m, service)
	return nil
}
