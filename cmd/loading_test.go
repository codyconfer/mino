package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestWantsLaunchLoading(t *testing.T) {
	root := &cobra.Command{Use: "munin"}
	deck := &cobra.Command{Use: "deck"}
	tui := &cobra.Command{Use: "tui"}
	daemon := &cobra.Command{Use: "daemon"}
	status := &cobra.Command{Use: "status"}
	fly := &cobra.Command{Use: "fly"}
	root.AddCommand(deck, tui, daemon, fly)
	daemon.AddCommand(status)

	cases := []struct {
		cmd  *cobra.Command
		want bool
	}{
		{deck, true},
		{tui, true},
		{daemon, true},
		{status, false},
		{fly, false},
		{root, false},
	}
	for _, tc := range cases {
		if got := wantsLaunchLoading(tc.cmd); got != tc.want {
			t.Errorf("wantsLaunchLoading(%q) = %v, want %v", tc.cmd.Name(), got, tc.want)
		}
	}
}
