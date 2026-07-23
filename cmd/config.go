package cmd

import (
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/sisyphus/redact"
)

func newConfigCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "config",
		Short: "Show the active config (from the DuckDB config store) and its history",
		Long: "munin's config is stored in DuckDB: on startup the config file is hashed\n" +
			"and, when changed, imported as the new current (the prior version archived).\n" +
			"`munin config` prints the active config; `config history` lists prior versions.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			if shared.mgr == nil {
				return errs.New(errs.KindStore, "config DB unavailable")
			}
			cur, ok, err := shared.mgr.DB().Current("config")
			if err != nil {
				return err
			}
			if !ok {
				fmt.Fprintln(out, "no config stored yet (no config file found and DB is empty)")
				return nil
			}
			fmt.Fprintf(out, "# active config (%s, applied %s, version %s)\n\n",
				cur.Format, cur.At.Format("2006-01-02 15:04:05"), shortHash(cur.Hash))
			fmt.Fprintln(out, redact.Config([]byte(cur.Content), cur.Format))
			return nil
		},
	}

	c.AddCommand(&cobra.Command{
		Use:   "history",
		Short: "List archived config versions (newest first)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return configHistory(cmd.OutOrStdout())
		},
	})
	return c
}

func configHistory(w io.Writer) error {
	if shared.mgr == nil {
		return errs.New(errs.KindStore, "config DB unavailable")
	}
	versions, err := shared.mgr.DB().History("config", 50)
	if err != nil {
		return err
	}
	if len(versions) == 0 {
		fmt.Fprintln(w, "no archived config versions (config has not changed since first import)")
		return nil
	}
	for _, v := range versions {
		fmt.Fprintf(w, "%s  %-4s  %s\n", v.At.Format(time.RFC3339), v.Format, shortHash(v.Hash))
	}
	return nil
}

func shortHash(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return h
}
