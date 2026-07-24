package cmd

import (
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/codyconfer/munin/internal/signals"
	"github.com/codyconfer/munin/internal/signals/build"
	"github.com/codyconfer/munin/internal/signals/gtasks"
)

func newTasksCmd() *cobra.Command {
	parent := &cobra.Command{
		Use:   "tasks",
		Short: "Google Tasks (read any list; create tasks only in the configured list)",
	}

	var ff filterFlags
	query := &cobra.Command{
		Use:   "query",
		Short: "List tasks from the configured lists",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSignal(cmd, "tasks", nil, &ff)
		},
	}
	ff.bind(query)

	var notes, due, list string
	add := &cobra.Command{
		Use:   "add <title>",
		Short: "Create a task in the writable list (tasks.list)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return addTask(cmd, strings.Join(args, " "), notes, due, list)
		},
	}
	add.Flags().StringVar(&notes, "notes", "", "task notes/body")
	add.Flags().StringVar(&due, "due", "", "due date (YYYY-MM-DD or RFC3339)")
	add.Flags().StringVar(&list, "list", "", "target list; must be the configured writable list")

	parent.AddCommand(query, add)
	return parent
}

func addTask(cmd *cobra.Command, title, notes, due, list string) error {
	target, err := build.ResolveWriteTarget("task list", "tasks.list", shared.Cfg.Tasks.List, list)
	if err != nil {
		return err
	}
	started := time.Now()
	item, err := gtasks.CreateTask(cmd.Context(), googleAuth(), target, title, notes, due)
	if err != nil {
		return err
	}
	sections := []signals.Section{{Signal: "tasks", Title: "Created task in " + target, Items: []signals.Item{item}}}
	shared.Audit.RecordAction("tasks add", shared.Cfg.Role, started, time.Now(), sections)
	return emit(cmd.OutOrStdout(), sections)
}
