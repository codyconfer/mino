package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/codyconfer/munin/internal/app"
	"github.com/codyconfer/munin/internal/config"
)

func useServeTestApp(t *testing.T, configuredInterval string) {
	t.Helper()
	orig := shared
	t.Cleanup(func() { shared = orig })
	cfg := config.Defaults()
	cfg.Home = t.TempDir()
	cfg.Daemon.Interval = configuredInterval
	shared = &app.App{Cfg: cfg, Directives: &config.Directives{}}
}

func runServe(t *testing.T, args ...string) error {
	t.Helper()
	var out bytes.Buffer
	root := &cobra.Command{Use: "munin", SilenceUsage: true, SilenceErrors: true}
	root.AddCommand(newServeCmd())
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"serve"}, args...))
	return root.Execute()
}

func TestServeRefusesAHotPollIntervalFlag(t *testing.T) {
	useServeTestApp(t, "")
	for _, raw := range []string{"0s", "100ms", "999ms"} {
		err := runServe(t, "--interval", raw)
		if err == nil {
			t.Errorf("serve --interval %s = nil; nothing clamped the poll rate, so this hot-polled GitHub "+
				"straight into a rate limit", raw)
			continue
		}
		if !strings.Contains(err.Error(), "--interval") {
			t.Errorf("serve --interval %s error = %q; want the flag named so the user knows what to change",
				raw, err)
		}
	}
}

func TestServeRefusesAHotPollIntervalFromConfig(t *testing.T) {
	useServeTestApp(t, "100ms")
	err := runServe(t)
	if err == nil {
		t.Fatal("serve with daemon.interval=100ms = nil; the config path bypassed the flag, so the floor has " +
			"to be checked on the resolved value, not on the flag")
	}
	if !strings.Contains(err.Error(), "daemon.interval") {
		t.Errorf("error = %q; want daemon.interval named, since no flag was passed", err)
	}
}

func TestServeAcceptsAPollIntervalAtOrAboveTheFloor(t *testing.T) {
	useServeTestApp(t, "")
	for _, raw := range []string{"1s", "60s"} {
		err := runServe(t, "--interval", raw)
		if err != nil && strings.Contains(err.Error(), "poll interval") {
			t.Errorf("serve --interval %s was rejected as too fast: %v", raw, err)
		}
	}
}
