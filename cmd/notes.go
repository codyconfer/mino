package cmd

import (
	"github.com/spf13/cobra"

	"github.com/codyconfer/mino/internal/plugin/ntr"
)

func newNotesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "notes",
		Aliases: []string{"ntr"},
		Short:   "Notes, tasks, and reminders (reminders UI is service-only)",
	}
	cmd.AddCommand(
		newNotesListCmd(),
		newNotesAddCmd(),
		newNotesUpdateCmd(),
		newNotesRMCmd(),
		newNotesTasksCmd(),
		newNotesRemindCmd(),
		newNotesCatchUpCmd(),
		newNotesUICmd(),
	)
	return cmd
}

func notesHomeRole() (home, role string) {
	home = shared.Cfg.Home
	role = shared.Cfg.Role
	if role == "" {
		role = "default"
	}
	return home, role
}

func newNotesUICmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ui",
		Short: "Open the Notes TUI (viewkit/deck)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			home, role := notesHomeRole()
			return ntr.RunTUI(home, role)
		},
	}
}

func newNotesListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List notes",
		RunE: func(cmd *cobra.Command, _ []string) error {
			home, role := notesHomeRole()
			return ntr.CLINotesList(cmd.Context(), cmd.OutOrStdout(), Scope(), home, role)
		},
	}
}

func newNotesAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <title> [body]",
		Short: "Add a note",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := ""
			if len(args) > 1 {
				body = args[1]
			}
			home, role := notesHomeRole()
			return ntr.CLINotesAdd(cmd.Context(), cmd.OutOrStdout(), Scope(), home, role, args[0], body)
		},
	}
}

func newNotesUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "update <id> <title> [body]",
		Short: "Update a note",
		Args:  cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := ""
			if len(args) > 2 {
				body = args[2]
			}
			home, role := notesHomeRole()
			return ntr.CLINotesUpdate(cmd.Context(), cmd.OutOrStdout(), Scope(), home, role, args[0], args[1], body)
		},
	}
}

func newNotesRMCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <id>",
		Short: "Delete a note",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			home, role := notesHomeRole()
			return ntr.CLINotesRM(cmd.Context(), cmd.OutOrStdout(), Scope(), home, role, args[0])
		},
	}
}

func newNotesTasksCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "tasks", Short: "Manage local tasks"}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "List open tasks",
			RunE: func(cmd *cobra.Command, _ []string) error {
				home, role := notesHomeRole()
				return ntr.CLITasksList(cmd.Context(), cmd.OutOrStdout(), Scope(), home, role)
			},
		},
		&cobra.Command{
			Use:   "add <title>",
			Short: "Add a task",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				home, role := notesHomeRole()
				return ntr.CLITasksAdd(cmd.Context(), cmd.OutOrStdout(), Scope(), home, role, args[0])
			},
		},
		&cobra.Command{
			Use:   "done <id>",
			Short: "Mark a task done",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				home, role := notesHomeRole()
				return ntr.CLITasksDone(cmd.Context(), cmd.OutOrStdout(), Scope(), home, role, args[0])
			},
		},
		&cobra.Command{
			Use:   "undo <id>",
			Short: "Reopen a done task",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				home, role := notesHomeRole()
				return ntr.CLITasksUndo(cmd.Context(), cmd.OutOrStdout(), Scope(), home, role, args[0])
			},
		},
		&cobra.Command{
			Use:   "rm <id>",
			Short: "Delete a task",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				home, role := notesHomeRole()
				return ntr.CLITasksRM(cmd.Context(), cmd.OutOrStdout(), Scope(), home, role, args[0])
			},
		},
	)
	return cmd
}

func newNotesRemindCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "remind", Short: "Manage reminders"}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "List open reminders",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				home, role := notesHomeRole()
				return ntr.CLIRemindList(cmd.Context(), cmd.OutOrStdout(), Scope(), home, role)
			},
		},
		&cobra.Command{
			Use:   "add <title> <duration>",
			Short: "Create a reminder due after duration (e.g. 10m, 2h)",
			Args:  cobra.ExactArgs(2),
			RunE: func(cmd *cobra.Command, args []string) error {
				home, role := notesHomeRole()
				return ntr.CLIRemindAdd(cmd.Context(), cmd.OutOrStdout(), Scope(), home, role, args[0], args[1])
			},
		},
		&cobra.Command{
			Use:   "done <id>",
			Short: "Mark a reminder done (removes from open list)",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				home, role := notesHomeRole()
				return ntr.CLIRemindDone(cmd.Context(), cmd.OutOrStdout(), Scope(), home, role, args[0])
			},
		},
	)
	return cmd
}

func newNotesCatchUpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "catch-up",
		Short: "Fire due reminders (CLI watermark catch-up)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			home, role := notesHomeRole()
			return ntr.CLICatchUp(cmd.Context(), cmd.OutOrStdout(), Scope(), home, role)
		},
	}
}
