//go:build !nodaemon

package daemon

import (
	"testing"

	"github.com/spf13/cobra"

	"github.com/codyconfer/mino/cmd"
	"github.com/codyconfer/mino/internal/app/views"
	"github.com/codyconfer/mino/internal/config"
)

func findCmd(root *cobra.Command, name string) *cobra.Command {
	for _, c := range root.Commands() {
		if c.Name() == name {
			return c
		}
	}
	return nil
}

func gateMode(c *cobra.Command) string {
	for p := c; p != nil; p = p.Parent() {
		if m, ok := p.Annotations[cmd.AnnoGateMode]; ok && m != "" {
			return m
		}
	}
	return ""
}

func TestDaemonCommandTree(t *testing.T) {
	root := cmd.Root()

	serve := findCmd(root, "serve")
	if serve == nil {
		t.Fatal("missing top-level command \"serve\"")
	}
	if serve.Flags().Lookup("tui") != nil || serve.Flags().Lookup("tray") != nil {
		t.Error("serve must not expose --tui or --tray")
	}

	daemon := findCmd(root, "daemon")
	if daemon == nil {
		t.Fatal("missing top-level command \"daemon\"")
	}
	for _, s := range []string{"install", "uninstall", "start", "stop", "restart", "status", "attach"} {
		if findCmd(daemon, s) == nil {
			t.Errorf("daemon missing subcommand %q", s)
		}
	}
	run := findCmd(daemon, "run")
	if run == nil {
		t.Error("daemon missing hidden run entrypoint for the OS service")
	} else if !run.Hidden {
		t.Error("daemon run should be hidden (OS service entrypoint)")
	}
}

func TestDaemonGateMode(t *testing.T) {
	daemon := findCmd(cmd.Root(), "daemon")
	if daemon == nil {
		t.Fatal("no command \"daemon\"")
	}
	if got := gateMode(daemon); got != cmd.ModeDaemon {
		t.Errorf("gateMode(daemon) = %q, want %q", got, cmd.ModeDaemon)
	}
	if got := gateMode(findCmd(daemon, "install")); got != cmd.ModeDaemon {
		t.Errorf("gateMode(daemon install) = %q, want %q", got, cmd.ModeDaemon)
	}
}

func TestDaemonShowsLaunchLoading(t *testing.T) {
	daemon := findCmd(cmd.Root(), "daemon")
	if daemon.Annotations[cmd.AnnoLaunchLoading] != "true" {
		t.Error("top-level daemon should show the launch spinner")
	}
	for _, sub := range []string{"run", "install", "status"} {
		if findCmd(daemon, sub).Annotations[cmd.AnnoLaunchLoading] == "true" {
			t.Errorf("daemon %s must not show the launch spinner", sub)
		}
	}
}

func TestRegistersSettingsSection(t *testing.T) {
	var haveEntry, haveField, haveValues bool
	for _, s := range views.SettingsSections() {
		for _, e := range s.StatusBar {
			if e.ID == "daemon" {
				haveEntry = true
			}
		}
		if s.Values != nil {
			haveValues = true
		}
		if s.Fields == nil {
			continue
		}
		for _, f := range s.Fields(&config.Config{}) {
			if f.Key == "daemon.tray" {
				haveField = true
			}
		}
	}
	if !haveEntry {
		t.Error("daemon did not register the daemon status-bar entry")
	}
	if !haveField {
		t.Error("daemon did not register the daemon.tray settings field")
	}
	if !haveValues {
		t.Error("daemon did not register a settings Values applier")
	}
}
