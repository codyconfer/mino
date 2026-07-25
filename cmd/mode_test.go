package cmd

import (
	"slices"
	"testing"

	"github.com/spf13/cobra"
)

func findCmd(root *cobra.Command, name string) *cobra.Command {
	for _, c := range root.Commands() {
		if c.Name() == name {
			return c
		}
	}
	return nil
}

func hasAlias(c *cobra.Command, alias string) bool {
	return slices.Contains(c.Aliases, alias)
}

func TestCommandTree(t *testing.T) {
	root := Root()

	for _, n := range []string{"deck", "serve", "daemon", "notes"} {
		if findCmd(root, n) == nil {
			t.Errorf("missing top-level command %q", n)
		}
	}
	if findCmd(root, "tui") != nil {
		t.Error("tui should be a deck alias, not its own top-level command")
	}
	if findCmd(root, "ntr") != nil {
		t.Error("ntr should be a notes alias, not its own top-level command")
	}

	deck := findCmd(root, "deck")
	if deck == nil || !hasAlias(deck, "tui") {
		t.Error("deck should carry the deprecated `tui` alias")
	}

	notes := findCmd(root, "notes")
	if notes == nil || !hasAlias(notes, "ntr") {
		t.Error("notes should carry the deprecated `ntr` alias")
	}
	for _, sub := range []string{"list", "add", "update", "rm", "tasks", "remind", "catch-up", "ui"} {
		if findCmd(notes, sub) == nil {
			t.Errorf("notes missing subcommand %q", sub)
		}
	}

	serve := findCmd(root, "serve")
	if serve.Flags().Lookup("tui") != nil || serve.Flags().Lookup("tray") != nil {
		t.Error("serve must not expose --tui or --tray")
	}

	daemon := findCmd(root, "daemon")
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

func TestGateMode(t *testing.T) {
	root := Root()
	want := map[string]string{
		"deck":   modeDeck,
		"serve":  modeServe,
		"daemon": modeDaemon,
		"fly":    modeCLI,
	}
	for name, exp := range want {
		c := findCmd(root, name)
		if c == nil {
			t.Fatalf("no command %q", name)
		}
		if got := gateMode(c); got != exp {
			t.Errorf("gateMode(%s) = %q, want %q", name, got, exp)
		}
	}

	install := findCmd(findCmd(root, "daemon"), "install")
	if got := gateMode(install); got != modeDaemon {
		t.Errorf("gateMode(daemon install) = %q, want %q", got, modeDaemon)
	}
}
