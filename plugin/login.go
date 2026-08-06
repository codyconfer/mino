package plugin

import (
	"context"
	"io"
	"sort"
	"sync"
)

const KindLogin Kind = "login"

type LoginField struct {
	Key    string
	Label  string
	Secret bool
	Sealed bool
	Value  func(Host) string
}

type LoginProvider struct {
	PluginID string
	Key      string
	Label    string
	Signals  []string
	Fields   []LoginField
	Authed   func(Host) bool
	Login    func(ctx context.Context, h Host, creds map[string]string, w io.Writer) error
}

var (
	loginMu   sync.RWMutex
	loginByID = map[string]LoginProvider{}
)

func RegisterLoginProvider(p LoginProvider) {
	noteRegistrationCheckpoint(p.PluginID)
	if p.Key == "" {
		noteDiagnosticf(p.PluginID, KindLogin, "",
			"RegisterLoginProvider requires a provider key; provider skipped")
		return
	}
	if p.Login == nil {
		noteDiagnosticf(p.PluginID, KindLogin, p.Key,
			"login provider %q supplied no Login func; provider skipped", p.Key)
		return
	}
	loginMu.Lock()
	incumbent, dup := loginByID[p.Key]
	if !dup {
		loginByID[p.Key] = p
	}
	loginMu.Unlock()
	if dup {
		noteDiagnosticf(p.PluginID, KindLogin, p.Key,
			"login provider %q is already registered by %q; later provider skipped", p.Key, providerOwner(incumbent))
	}
}

func providerOwner(p LoginProvider) string {
	if p.PluginID != "" {
		return p.PluginID
	}
	return "an earlier registration"
}

func LoginProviders() []LoginProvider {
	loginMu.RLock()
	out := make([]LoginProvider, 0, len(loginByID))
	for _, p := range loginByID {
		out = append(out, p)
	}
	loginMu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func LookupLoginProvider(key string) (LoginProvider, bool) {
	loginMu.RLock()
	defer loginMu.RUnlock()
	p, ok := loginByID[key]
	return p, ok
}

func ResetLoginProviders() {
	loginMu.Lock()
	loginByID = map[string]LoginProvider{}
	loginMu.Unlock()
}
