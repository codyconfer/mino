package gitauth

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/codyconfer/mino/plugin"
)

const Default = "github"

type CredentialStore = plugin.CredentialStore

type SigningKeyKind string

const (
	SigningGPG SigningKeyKind = "openpgp"
	SigningSSH SigningKeyKind = "ssh"
)

type Identity interface {
	Token(ctx context.Context) (string, error)
	Origin() string
	Authenticated() bool
	ServiceIdentity() bool
	Trace() string
	Invalidate()
}

type Account struct {
	Login string
}

type RateLimit struct {
	Limit     int
	Remaining int
}

type AuthStatus struct {
	OK     bool
	Detail string
	Fix    []string
}

type KeyCheck struct {
	Registered bool
	// Identities are the provider-verified email identities bound to the key, when the
	// provider tracks any. Populated from the same fetch as Registered.
	Identities []string
	Err        error
	Fix        []string
}

type Finding struct {
	Name string
	Msg  string
	OK   bool
	Warn bool
}

type Provider interface {
	Name() string
	Label() string
	Host() string

	Resolve() (Identity, error)
	Status(ctx context.Context, id Identity) AuthStatus

	Account(ctx context.Context, id Identity) (Account, error)
	RateLimit(ctx context.Context, id Identity) (RateLimit, error)

	SigningKeyRegistered(ctx context.Context, id Identity, kind SigningKeyKind, key string) KeyCheck
	EmailVerified(ctx context.Context, id Identity, email string) (bool, error)

	UploadKeyFix(kind SigningKeyKind, key string) []string
	Findings(ctx context.Context, id Identity) []Finding
}

type Env struct {
	Setting func(key string) string
	Store   CredentialStore
	Role    string
}

func (e Env) Get(key string) string {
	if e.Setting == nil {
		return ""
	}
	return e.Setting(key)
}

type Factory func(Env) (Provider, error)

var (
	mu        sync.RWMutex
	factories = map[string]Factory{}
)

func Register(name string, f Factory) {
	if name == "" || f == nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	factories[name] = f
}

func New(name string, env Env) (Provider, error) {
	if name == "" {
		name = Default
	}
	mu.RLock()
	f, ok := factories[name]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown git provider %q (known: %v)", name, Names())
	}
	return f(env)
}

func Known(name string) bool {
	mu.RLock()
	defer mu.RUnlock()
	_, ok := factories[name]
	return ok
}

func Names() []string {
	mu.RLock()
	out := make([]string, 0, len(factories))
	for n := range factories {
		out = append(out, n)
	}
	mu.RUnlock()
	sort.Strings(out)
	return out
}
