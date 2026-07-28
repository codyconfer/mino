package cmd

import "testing"

func TestValidateDirectiveArg(t *testing.T) {
	for _, ok := range []string{"config", "queries", "flights", "roles", "all"} {
		if err := validateDirectiveArg(ok); err != nil {
			t.Errorf("validateDirectiveArg(%q) unexpected error: %v", ok, err)
		}
	}
	for _, bad := range []string{"bogus", "filters"} {
		if err := validateDirectiveArg(bad); err == nil {
			t.Errorf("expected error for unknown directive %q", bad)
		}
	}
}
