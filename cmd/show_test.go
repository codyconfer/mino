package cmd

import (
	"strings"
	"testing"

	"github.com/codyconfer/munin/internal/plugin"
)

func TestShowCommandIsRegistered(t *testing.T) {
	root := Root()
	show := findCmd(root, "show")
	if show == nil {
		t.Fatal("missing top-level `show` command")
	}
	if gateMode(show) != modeCLI {
		t.Errorf("show gate mode = %q, want %q", gateMode(show), modeCLI)
	}
	if show.Flags().Lookup("signal") == nil {
		t.Error("show should expose a --signal flag")
	}
}

func TestShowRequiresExactlyOneArg(t *testing.T) {
	show := findCmd(Root(), "show")
	if show == nil {
		t.Fatal("missing show command")
	}
	for _, args := range [][]string{{}, {"a", "b"}} {
		if err := show.Args(show, args); err == nil {
			t.Errorf("Args(%v) = nil, want an error", args)
		}
	}
	if err := show.Args(show, []string{"https://github.com/acme/tools/pull/1"}); err != nil {
		t.Errorf("Args with one URL = %v", err)
	}
}

func TestSignalsWithDetailGetAShowSubcommand(t *testing.T) {
	root := Root()
	github := findCmd(root, "github")
	if github == nil {
		t.Fatal("missing github command")
	}
	if findCmd(github, "show") == nil {
		t.Error("github advertises CapDetail so it should expose `github show`")
	}

	if plugin.HasCapability("gmail", plugin.CapDetail) {
		t.Skip("gmail now supports details; pick another detail-less signal")
	}
	gmail := findCmd(root, "gmail")
	if gmail == nil {
		t.Fatal("missing gmail command")
	}
	if findCmd(gmail, "show") != nil {
		t.Error("gmail does not advertise CapDetail so it should not expose `show`")
	}
}

func TestShowRejectsABlankURL(t *testing.T) {
	err := runShow(newShowCmd(), "   ", "github")
	if err == nil {
		t.Fatal("want an error for a blank URL")
	}
	if !strings.Contains(err.Error(), "URL") {
		t.Errorf("err = %v, want it to mention the missing URL", err)
	}
}
