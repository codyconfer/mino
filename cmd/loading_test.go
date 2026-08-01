package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestWantsLaunchLoading(t *testing.T) {
	root := &cobra.Command{Use: "mino"}
	marked := &cobra.Command{Use: "deck", Annotations: map[string]string{AnnoLaunchLoading: "true"}}
	sub := &cobra.Command{Use: "status"}
	fly := &cobra.Command{Use: "fly"}
	root.AddCommand(marked, fly)
	marked.AddCommand(sub)

	cases := []struct {
		cmd  *cobra.Command
		want bool
	}{
		{marked, true},
		{sub, false},
		{fly, false},
		{root, false},
		{nil, false},
	}
	for _, tc := range cases {
		if got := wantsLaunchLoading(tc.cmd); got != tc.want {
			t.Errorf("wantsLaunchLoading(%v) = %v, want %v", tc.cmd, got, tc.want)
		}
	}
}

func TestDeckShowsLaunchLoading(t *testing.T) {
	deck := findCmd(Root(), "deck")
	if deck == nil {
		t.Fatal("no deck command")
	}
	if !wantsLaunchLoading(deck) {
		t.Error("deck should show the launch spinner")
	}
}
