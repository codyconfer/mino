package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/codyconfer/munin/internal/errs"
)

func newFilterCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "filter",
		Short: "Inspect saved regex filter sets",
	}
	c.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "List saved filter names",
			RunE: func(cmd *cobra.Command, _ []string) error {
				names := visibleFilterNames()
				if len(names) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "no saved filters visible (check --role, or add YAML files under ~/.munin/filters)")
					return nil
				}
				for _, n := range names {
					f := shared.Directives.Filters[n]
					fmt.Fprintf(cmd.OutOrStdout(), "%-24s %d rule(s)\n", n, len(f.Rules))
				}
				return nil
			},
		},
		&cobra.Command{
			Use:   "show <name>",
			Short: "Show a saved filter's rules",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				f, ok := shared.Directives.Filters[args[0]]
				if !ok {
					return errs.Newf(errs.KindUsage, "no saved filter named %q", args[0]).WithHint("run `munin filter list` to see saved filters")
				}
				out, err := yaml.Marshal(f)
				if err != nil {
					return err
				}
				fmt.Fprint(cmd.OutOrStdout(), string(out))
				return nil
			},
		},
	)
	return c
}
