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

	for _, n := range []string{"deck", "notes"} {
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
}

func TestServeIsCoreAndDaemonIsOptional(t *testing.T) {
	root := Root()
	if findCmd(root, "serve") == nil {
		t.Error("serve is core and must always be registered")
	}
	if findCmd(root, "daemon") != nil {
		t.Error("daemon must come from the optional daemon package, not cmd")
	}
}

func TestGateMode(t *testing.T) {
	root := Root()
	want := map[string]string{
		"deck":  modeDeck,
		"fly":   modeCLI,
		"serve": modeServe,
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
}
