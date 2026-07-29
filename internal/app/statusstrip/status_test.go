package statusstrip

import (
	"context"
	"testing"

	"github.com/codyconfer/munin/internal/app"
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
	info := Provider(a, nil)(context.Background())
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

func TestProviderOmitsDisabledPluginAuthChips(t *testing.T) {
	testenv.Isolate(t)
	plugin.RegisterBuiltins()
	plugin.LoadEnabled()

	for _, id := range []string{"munin.github", "munin.slack", "munin.gmail"} {
		if err := plugin.SetEnabled(id, false); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, id := range []string{"munin.github", "munin.slack", "munin.gmail"} {
			_ = plugin.SetEnabled(id, true)
		}
	})

	a := &app.App{Cfg: &config.Config{}}
	info := Provider(a, nil)(context.Background())

	if info.GitHubUser != "" {
		t.Fatalf("disabled github still set identity %q", info.GitHubUser)
	}
	names := serviceNames(info.Services)
	for _, wantGone := range []string{"github", "slack", "gmail", "calendar", "docs", "drive", "tasks"} {
		if hasName(names, wantGone) {
			t.Fatalf("disabled/collapsed %q still in status chips: %v", wantGone, names)
		}
	}
	var google *deck.ServiceStatus
	for i := range info.Services {
		if info.Services[i].Name == "google" {
			google = &info.Services[i]
			break
		}
	}
	if google == nil {
		t.Fatalf("expected collapsed google chip, got %v", names)
	}
	if google.ID != "google" {
		t.Fatalf("google chip ID = %q, want google", google.ID)
	}
	if google.Detail != "" {
		t.Fatalf("google detail = %q, want logo-only chip (no detail)", google.Detail)
	}
}

func TestProviderOmitsUninstalledDaemonChip(t *testing.T) {
	testenv.Isolate(t)

	a := &app.App{Cfg: &config.Config{}}
	daemon := func() (deck.ServiceStatus, bool) { return deck.ServiceStatus{}, false }
	info := Provider(a, daemon)(context.Background())

	if hasName(serviceNames(info.Services), "daemon") {
		t.Fatalf("uninstalled daemon still in status chips: %+v", info.Services)
	}
}

func TestProviderIncludesInstalledDaemonChip(t *testing.T) {
	testenv.Isolate(t)

	a := &app.App{Cfg: &config.Config{}}
	daemon := func() (deck.ServiceStatus, bool) {
		return deck.ServiceStatus{Name: "daemon", Detail: "stopped", Level: deck.StatusWarn}, true
	}
	info := Provider(a, daemon)(context.Background())

	found := false
	for _, s := range info.Services {
		if s.Name == "daemon" && s.Detail == "stopped" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("installed daemon chip missing: %+v", info.Services)
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
