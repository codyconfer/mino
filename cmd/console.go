package cmd

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/codyconfer/mino/internal/console"
	"github.com/codyconfer/mino/internal/tmux"
)

func setConsoleMetadata(cmd *cobra.Command, args []string) {
	parts := []string{strings.ReplaceAll(strings.TrimPrefix(cmd.CommandPath(), "mino "), " ", " · ")}
	if consoleTarget(cmd, args) != "" {
		parts = append(parts, consoleTarget(cmd, args))
	}
	if role := shared.Role(); role != "" {
		parts = append(parts, "role: "+role)
	}
	title := console.Title(parts...)
	_ = console.Set(console.Writer(), title)
	if tmux.Inside() && tmux.SelfPane() != "" {
		_ = tmux.SetTitle(tmux.SelfPane(), title)
	}
}

func consoleTarget(cmd *cobra.Command, args []string) string {
	if len(args) == 0 {
		return ""
	}
	switch cmd.Name() {
	case "deck", "fly", "query", "serve":
		return args[0]
	default:
		return ""
	}
}
