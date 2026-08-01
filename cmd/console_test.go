package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestConsoleTargetUsesOnlyNamedWorkloads(t *testing.T) {
	for _, name := range []string{"deck", "fly", "query", "serve"} {
		cmd := &cobra.Command{Use: name}
		if got := consoleTarget(cmd, []string{"morning"}); got != "morning" {
			t.Errorf("%s target = %q", name, got)
		}
	}
	for _, name := range []string{"show", "login", "notes"} {
		cmd := &cobra.Command{Use: name}
		if got := consoleTarget(cmd, []string{"sensitive"}); got != "" {
			t.Errorf("%s exposed target %q", name, got)
		}
	}
}
