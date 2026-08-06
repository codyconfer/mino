package gcx

import (
	"context"
	"testing"

	"github.com/codyconfer/mino/plugin"
)

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

type fakeHost struct {
	settings map[string]any
	store    memStore
}

func newFakeHost(settings map[string]any) *fakeHost {
	if settings == nil {
		settings = map[string]any{}
	}
	return &fakeHost{settings: settings, store: memStore{}}
}

func (h *fakeHost) Home() string { return "/tmp/mino-test" }

func (h *fakeHost) Role() string { return "test" }

func (h *fakeHost) Settings(ns string) map[string]any {
	if ns != SignalName {
		return nil
	}
	return h.settings
}

func (h *fakeHost) Credentials() plugin.CredentialStore { return h.store }

type fakeBuildContext struct {
	*fakeHost
	params map[string]string
}

func newFakeBC(params map[string]string, settings map[string]any) *fakeBuildContext {
	if params == nil {
		params = map[string]string{}
	}
	return &fakeBuildContext{fakeHost: newFakeHost(settings), params: params}
}

func (b *fakeBuildContext) Params() map[string]string { return b.params }

func (b *fakeBuildContext) GetToken(ctx context.Context, service string) (string, string, bool, error) {
	c, ok, err := b.store.Get(ctx, service)
	return c.AccessToken, c.Scope, ok, err
}

// clearEnv keeps the token env override out of a test's way.
func clearEnv(t *testing.T) {
	t.Helper()
	t.Setenv(DefaultTokenEnv, "")
}

// pinStack sets the shared context provider and restores it afterwards.
func pinStack(t *testing.T, name string) {
	t.Helper()
	prev := shared.cur
	shared.cur = name
	t.Cleanup(func() { shared.cur = prev })
}

func hintOf(err error) string {
	if e, ok := err.(*plugin.Error); ok {
		return e.Hint()
	}
	return ""
}
