package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/codyconfer/mino/internal/render/glyph"
)

var Version = "dev"

const annoSkipAppLoad = "mino_skip_app_load"

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the mino version",
		Args:  cobra.NoArgs,
		Annotations: map[string]string{
			annoSkipOnboarding: "true",
			annoSkipAppLoad:    "true",
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), versionLine())
			return nil
		},
	}
}

func versionLine() string {
	return fmt.Sprintf("%s MINO %s", glyph.Brand(), Version)
}

func skipsAppLoad(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		if c.Annotations[annoSkipAppLoad] == "true" {
			return true
		}
	}
	return false
}
