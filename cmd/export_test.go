package cmd

import "testing"

func TestValidateDirectiveArg(t *testing.T) {
	for _, ok := range []string{"config", "queries", "filters", "flights", "roles", "all"} {
		if err := validateDirectiveArg(ok); err != nil {
			t.Errorf("validateDirectiveArg(%q) unexpected error: %v", ok, err)
		}
	}
	if err := validateDirectiveArg("bogus"); err == nil {
		t.Error("expected error for unknown directive")
	}
}
