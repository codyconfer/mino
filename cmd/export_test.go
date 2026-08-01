package cmd

import (
	"strings"
	"testing"

	"github.com/codyconfer/mino/internal/config"
)

func TestValidateDirectiveArg(t *testing.T) {
	for _, ok := range []string{"config", "directives", "all"} {
		if err := validateDirectiveArg(ok); err != nil {
			t.Errorf("validateDirectiveArg(%q) unexpected error: %v", ok, err)
		}
	}
	for _, deprecated := range []string{"queries", "flights", "roles"} {
		if err := validateDirectiveArg(deprecated); err != nil {
			t.Errorf("validateDirectiveArg(%q) should still be accepted: %v", deprecated, err)
		}
		got, err := config.ResolveDirectiveArg(deprecated)
		if err != nil {
			t.Errorf("ResolveDirectiveArg(%q): %v", deprecated, err)
			continue
		}
		if got != config.DirectivesDirective {
			t.Errorf("ResolveDirectiveArg(%q) = %q, want %q", deprecated, got, config.DirectivesDirective)
		}
	}
	for _, bad := range []string{"bogus", "filters", ""} {
		if err := validateDirectiveArg(bad); err == nil {
			t.Errorf("expected error for unknown directive %q", bad)
		}
	}
}

func TestExportAndImportHelpNameTheDirectives(t *testing.T) {
	for _, name := range []string{"export", "apply"} {
		c := findCmd(Root(), name)
		if c == nil {
			t.Fatalf("missing %s command", name)
		}
		for _, want := range []string{"config, directives, all", "deprecated aliases"} {
			if !strings.Contains(c.Long, want) {
				t.Errorf("%s help does not mention %q:\n%s", name, want, c.Long)
			}
		}
	}
}
