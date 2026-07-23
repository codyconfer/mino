package cmd

import (
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/codyconfer/munin/internal/audit"
	"github.com/codyconfer/munin/internal/errs"
)

func newHistoryCmd() *cobra.Command {
	var limit int
	c := &cobra.Command{
		Use:   "history [show <id>]",
		Short: "Recall past flights and query results from the audit trail",
		Long: "The audit trail records every flight and query run (with timestamps and\n" +
			"item counts) in DuckDB. `munin history` lists recent runs; `munin history\n" +
			"show <id>` recalls a run's stored results. This is a record of what you ran\n" +
			"over time, not a cache.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return listHistory(cmd.OutOrStdout(), limit)
		},
	}
	c.Flags().IntVar(&limit, "limit", 20, "max runs to list")

	c.AddCommand(&cobra.Command{
		Use:   "show <id>",
		Short: "Recall a past run's stored results",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return errs.Newf(errs.KindUsage, "invalid run id %q", args[0])
			}
			return showHistory(cmd.OutOrStdout(), id)
		},
	})
	return c
}

func listHistory(w io.Writer, limit int) error {
	runs, err := shared.audit.RecentRuns(limit)
	if err != nil {
		return err
	}
	if len(runs) == 0 {
		fmt.Fprintln(w, "no recorded runs (audit trail is empty or disabled)")
		return nil
	}
	for _, r := range runs {
		fmt.Fprintf(w, "%-6d %s  %-6s %-18s role=%-8s %s\n",
			r.ID, r.Started.Format("2006-01-02 15:04"), r.Kind, r.Name, dash(r.Role), status(r))
	}
	return nil
}

func showHistory(w io.Writer, id int64) error {
	run, ok, err := shared.audit.Run(id)
	if err != nil {
		return err
	}
	if !ok {
		return errs.Newf(errs.KindUsage, "no run with id %d", id)
	}
	printRun(w, run)

	if run.Kind == "flight" {
		children, err := shared.audit.ChildRuns(id)
		if err != nil {
			return err
		}
		for _, ch := range children {
			fmt.Fprintln(w)
			printRun(w, ch)
			printItems(w, ch.ID)
		}
		return nil
	}
	printItems(w, id)
	return nil
}

func printRun(w io.Writer, r audit.FlightRow) {
	dur := ""
	if !r.Finished.IsZero() {
		dur = " in " + r.Finished.Sub(r.Started).Round(time.Millisecond).String()
	}
	fmt.Fprintf(w, "#%d  %s  %s %q  (role=%s)  %s%s\n",
		r.ID, r.Started.Format("2006-01-02 15:04:05"), r.Kind, r.Name, dash(r.Role), status(r), dur)
}

func printItems(w io.Writer, runID int64) {
	items, err := shared.audit.Items(runID)
	if err != nil || len(items) == 0 {
		return
	}
	for _, it := range items {
		when := ""
		if !it.Ts.IsZero() {
			when = "  " + it.Ts.Format("2006-01-02 15:04")
		}
		fmt.Fprintf(w, "    • %s  %s%s\n", it.Title, dash(it.Subtitle), when)
		if it.URL != "" {
			fmt.Fprintf(w, "      %s\n", it.URL)
		}
	}
}

func status(r audit.FlightRow) string {
	if r.Error != "" {
		return "error: " + r.Error
	}
	return fmt.Sprintf("%d items", r.ItemCount)
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
