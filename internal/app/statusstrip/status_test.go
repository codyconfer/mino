package statusstrip

import (
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/codyconfer/sisyphus/sealed"
	vkglyph "github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/munin/internal/app"
	"github.com/codyconfer/munin/internal/auth"
	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/deck"
	"github.com/codyconfer/munin/internal/plugin"
	"github.com/codyconfer/munin/internal/role"
	"github.com/codyconfer/munin/internal/testenv"
)

func TestProviderIncludesRoleStatusChips(t *testing.T) {
	testenv.Isolate(t)
	t.Cleanup(role.ClearStatusChips)
	role.SetStatusChips([]role.Chip{
		{Glyph: "github", Text: "role-chip", Index: 0},
	})

	a := &app.App{Cfg: &config.Config{}}
	info := Provider(a)(context.Background())
	found := false
	for _, s := range info.Services {
		if s.ID == "role-status-0" && s.Name == "github" && s.Detail == "role-chip" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("role status chip missing: %+v", info.Services)
	}
}

func registerAcmeProvider(t *testing.T) {
	t.Helper()
	plugin.ResetLoginProviders()
	t.Cleanup(plugin.ResetLoginProviders)
	const id = "external.acme"
	if _, ok := plugin.Lookup(id); !ok {
		plugin.Register(plugin.Descriptor{
			ID:           id,
			Kind:         plugin.KindSignal,
			Signal:       "acmedocs",
			Capabilities: []plugin.Capability{plugin.CapQuery},
		})
	}
	plugin.RegisterLoginProvider(plugin.LoginProvider{
		PluginID: id,
		Key:      "acme",
		Label:    "Acme",
		Signals:  []string{"acmedocs"},
		Authed:   func(plugin.Host) bool { return true },
		Login:    func(context.Context, plugin.Host, map[string]string, io.Writer) error { return nil },
	})
}

func TestProviderChipsFollowContributedLoginProviders(t *testing.T) {
	testenv.Isolate(t)
	plugin.RegisterBuiltins()
	plugin.LoadEnabled()
	registerAcmeProvider(t)

	a := &app.App{Cfg: &config.Config{}}
	names := serviceNames(Provider(a)(context.Background()).Services)
	if !hasName(names, "acme") {
		t.Fatalf("contributed login provider missing from status chips: %v", names)
	}
}

func TestProviderOmitsDisabledPluginAuthChips(t *testing.T) {
	testenv.Isolate(t)
	plugin.RegisterBuiltins()
	plugin.LoadEnabled()
	registerAcmeProvider(t)

	for _, id := range []string{"munin.github", "external.acme"} {
		if err := plugin.SetEnabled(id, false); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, id := range []string{"munin.github", "external.acme"} {
			_ = plugin.SetEnabled(id, true)
		}
	})

	a := &app.App{Cfg: &config.Config{}}
	info := Provider(a)(context.Background())

	if info.GitHubUser != "" {
		t.Fatalf("disabled github still set identity %q", info.GitHubUser)
	}
	names := serviceNames(info.Services)
	for _, wantGone := range []string{"github", "acme"} {
		if hasName(names, wantGone) {
			t.Fatalf("disabled %q still in status chips: %v", wantGone, names)
		}
	}
}

func TestProviderOmitsChipThatReportsAbsent(t *testing.T) {
	testenv.Isolate(t)
	t.Cleanup(resetChips)
	RegisterChip(func() (deck.ServiceStatus, bool) { return deck.ServiceStatus{}, false })

	a := &app.App{Cfg: &config.Config{}}
	info := Provider(a)(context.Background())

	if hasName(serviceNames(info.Services), "daemon") {
		t.Fatalf("absent chip still in status chips: %+v", info.Services)
	}
}

func TestProviderIncludesRegisteredChip(t *testing.T) {
	testenv.Isolate(t)
	t.Cleanup(resetChips)
	RegisterChip(func() (deck.ServiceStatus, bool) {
		return deck.ServiceStatus{Name: "daemon", Detail: "stopped", Level: deck.StatusWarn}, true
	})

	a := &app.App{Cfg: &config.Config{}}
	info := Provider(a)(context.Background())

	found := false
	for _, s := range info.Services {
		if s.Name == "daemon" && s.Detail == "stopped" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("registered chip missing: %+v", info.Services)
	}
}

func TestProviderWithNoChipsRegistered(t *testing.T) {
	testenv.Isolate(t)
	t.Cleanup(resetChips)
	resetChips()

	a := &app.App{Cfg: &config.Config{}}
	info := Provider(a)(context.Background())

	if hasName(serviceNames(info.Services), "daemon") {
		t.Fatalf("daemon chip present with nothing registered: %+v", info.Services)
	}
}

func serviceNames(svcs []deck.ServiceStatus) []string {
	out := make([]string, 0, len(svcs))
	for _, s := range svcs {
		out = append(out, s.Name)
	}
	return out
}

func hasName(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}

type undecodableStore struct{}

func (undecodableStore) Get(context.Context, string) (auth.Credential, bool, error) {
	return auth.Credential{}, false, fmt.Errorf("read github token: %w", sealed.ErrUndecodable)
}

func (undecodableStore) Put(context.Context, string, auth.Credential) error { return nil }

func (undecodableStore) Delete(context.Context, string) error { return nil }

func TestProviderSurfacesAnUnreadableCredentialStore(t *testing.T) {
	testenv.Isolate(t)
	t.Cleanup(resetChips)
	resetChips()
	t.Cleanup(auth.ClearCredentialStoreError)
	auth.ClearCredentialStoreError()

	a := &app.App{Cfg: &config.Config{}}
	if hasName(serviceNames(Provider(a)(context.Background()).Services), "credentials") {
		t.Fatal("credentials chip present with a healthy store")
	}

	if _, state, _ := auth.ReadCredential(undecodableStore{}, "github"); state != auth.CredUnreadable {
		t.Fatalf("ReadCredential state = %v, want unreadable", state)
	}

	info := Provider(a)(context.Background())
	var chip *deck.ServiceStatus
	for i := range info.Services {
		if info.Services[i].ID == "credentials" {
			chip = &info.Services[i]
			break
		}
	}
	if chip == nil {
		t.Fatalf("status strip hides an undecryptable credential store, so munin still says 'not logged in': %v", serviceNames(info.Services))
	}
	if chip.Level != deck.StatusBad {
		t.Errorf("credentials chip level = %v, want bad", chip.Level)
	}
	if chip.Detail != auth.CredUnreadable.String() {
		t.Errorf("credentials chip detail = %q, want %q", chip.Detail, auth.CredUnreadable.String())
	}
}

func TestProviderHonoursTheDeckDeadlineForPluginStatus(t *testing.T) {
	testenv.Isolate(t)
	t.Cleanup(resetChips)
	resetChips()

	release := make(chan struct{})
	t.Cleanup(func() { close(release) })

	id := "test.statusstrip.block"
	if _, ok := plugin.Lookup(id); !ok {
		plugin.Register(plugin.Descriptor{
			ID:           id,
			Kind:         plugin.KindSignal,
			Signal:       "teststatusstripblock",
			Capabilities: []plugin.Capability{plugin.CapQuery},
		})
	}
	plugin.RegisterStatusContribution(id, func(_, _ string) vkglyph.StatusContribution {
		return vkglyph.StatusContribution{
			Info: func() string { return "blocker" },
			Status: func() (string, vkglyph.Severity) {
				<-release
				return "BLOCKED", vkglyph.SeverityNegative
			},
		}
	})

	const deadline = 150 * time.Millisecond
	a := &app.App{Cfg: &config.Config{Home: t.TempDir()}}
	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()

	start := time.Now()
	_ = Provider(a)(ctx)
	if d := time.Since(start); d > time.Second {
		t.Fatalf("status provider took %v with a %v deck deadline: the deadline is being discarded", d, deadline)
	}
}
