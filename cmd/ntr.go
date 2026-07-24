package cmd

import (
	"github.com/spf13/cobra"

	"github.com/codyconfer/munin/internal/plugin/ntr"
)

func newNTRCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ntr",
		Short: "Notes, tasks, and reminders (NTR plugin)",
	}
	cmd.AddCommand(
		newNTRNotesCmd(),
		newNTRTasksCmd(),
		newNTRRemindCmd(),
		newNTRCatchUpCmd(),
		newNTRUICmd(),
	)
	return cmd
}

func ntrHomeRole() (home, role string) {
	home = shared.Cfg.Home
	role = shared.Cfg.Role
	if role == "" {
		role = "default"
	}
	return home, role
}

func newNTRUICmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ui",
		Short: "Open the NTR TUI (viewkit/deck)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			home, role := ntrHomeRole()
			return ntr.RunTUI(home, role)
		},
	}
}

func newNTRNotesCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "notes", Short: "Manage notes"}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "List notes",
			RunE: func(cmd *cobra.Command, _ []string) error {
				home, role := ntrHomeRole()
				return ntr.CLINotesList(cmd.Context(), cmd.OutOrStdout(), home, role)
			},
		},
		&cobra.Command{
			Use:   "add <title> [body]",
			Short: "Add a note",
			Args:  cobra.RangeArgs(1, 2),
			RunE: func(cmd *cobra.Command, args []string) error {
				body := ""
				if len(args) > 1 {
					body = args[1]
				}
				home, role := ntrHomeRole()
				return ntr.CLINotesAdd(cmd.Context(), cmd.OutOrStdout(), home, role, args[0], body)
			},
		},
		&cobra.Command{
			Use:   "rm <id>",
			Short: "Delete a note",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				home, role := ntrHomeRole()
				return ntr.CLINotesRM(cmd.Context(), cmd.OutOrStdout(), home, role, args[0])
			},
		},
	)
	return cmd
}

func newNTRTasksCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "tasks", Short: "Manage tasks"}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "List open tasks",
			RunE: func(cmd *cobra.Command, _ []string) error {
				home, role := ntrHomeRole()
				return ntr.CLITasksList(cmd.Context(), cmd.OutOrStdout(), home, role)
			},
		},
		&cobra.Command{
			Use:   "add <title>",
			Short: "Add a task",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				home, role := ntrHomeRole()
				return ntr.CLITasksAdd(cmd.Context(), cmd.OutOrStdout(), home, role, args[0])
			},
		},
		&cobra.Command{
			Use:   "done <id>",
			Short: "Mark a task done",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				home, role := ntrHomeRole()
				return ntr.CLITasksDone(cmd.Context(), cmd.OutOrStdout(), home, role, args[0])
			},
		},
	)
	return cmd
}

func newNTRRemindCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "remind", Short: "Manage reminders"}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "List open reminders",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				home, role := ntrHomeRole()
				return ntr.CLIRemindList(cmd.Context(), cmd.OutOrStdout(), home, role)
			},
		},
		&cobra.Command{
			Use:   "add <title> <duration>",
			Short: "Create a reminder due after duration (e.g. 10m, 2h)",
			Args:  cobra.ExactArgs(2),
			RunE: func(cmd *cobra.Command, args []string) error {
				home, role := ntrHomeRole()
				return ntr.CLIRemindAdd(cmd.Context(), cmd.OutOrStdout(), home, role, args[0], args[1])
			},
		},
	)
	return cmd
}

func newNTRCatchUpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "catch-up",
		Short: "Fire due reminders (CLI watermark catch-up)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			home, role := ntrHomeRole()
			return ntr.CLICatchUp(cmd.Context(), cmd.OutOrStdout(), home, role)
		},
	}
}
