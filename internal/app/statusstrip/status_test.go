package statusstrip

import (
	"context"
	"testing"

	"github.com/codyconfer/munin/internal/app"
	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/deck"
	"github.com/codyconfer/munin/internal/plugin"
)

func TestProviderOmitsDisabledPluginAuthChips(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
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
	for _, wantGone := range []string{"github", "slack", "gmail"} {
		if hasName(names, wantGone) {
			t.Fatalf("disabled %q still in status chips: %v", wantGone, names)
		}
	}
	for _, want := range []string{"calendar", "docs", "drive", "tasks"} {
		if !hasName(names, want) {
			t.Fatalf("enabled %q missing from status chips: %v", want, names)
		}
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
