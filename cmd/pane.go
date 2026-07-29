package cmd

import (
	"os"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/codyconfer/munin/internal/app/pane"
	"github.com/codyconfer/munin/internal/app/views"
	"github.com/codyconfer/munin/internal/deck"
	"github.com/codyconfer/munin/internal/errs"
)

func newPaneCmd() *cobra.Command {
	c := &cobra.Command{
		Use:    "pane",
		Short:  "Auxiliary tmux pane clients used by `munin deck --tmux`",
		Hidden: true,
		Annotations: map[string]string{
			annoGateMode: modeServe,
			annoThin:     "true",
		},
	}
	c.AddCommand(newPaneInboxCmd(), newPaneViewCmd())
	return c
}

func newPaneInboxCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "inbox [flight]",
		Short: "Attach to the owning munin's live event stream",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requirePaneTTY(); err != nil {
				return err
			}
			flight := "attached"
			if len(args) == 1 {
				flight = args[0]
			}
			events, ok := serveServer().Dial(cmd.Context())
			if !ok {
				return errs.Newf(errs.KindUsage, "no munin serve provider at %s", serveServer().SocketPath()).
					WithHint("this pane is opened by `munin deck --tmux`; start the deck first")
			}
			return deck.Run(views.WithOwnerWatch(views.NewServeView(flight, events), pane.OwnerWatch()))
		},
	}
}

func newPaneViewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "view <snapshot>",
		Short: "Render a snapshot written by the owning munin deck",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if err := requirePaneTTY(); err != nil {
				return err
			}
			return deck.Run(views.WithOwnerWatch(views.NewSnapshotView(args[0]), pane.OwnerWatch()))
		},
	}
}

func requirePaneTTY() error {
	if !term.IsTerminal(os.Stdout.Fd()) {
		return errs.New(errs.KindUsage, "munin pane requires an interactive terminal")
	}
	return nil
}
