package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/filter"
	"github.com/codyconfer/munin/internal/render/glyph"
)

func buildQuery(name string) (query, error) {
	q, ok := shared.Directives.Queries[name]
	if !ok {
		return query{}, errs.Newf(errs.KindUsage, "no saved query named %q", name).WithHint("run `munin query list` to see saved queries")
	}
	src, err := buildSignal(q.Signal, q.Params)
	if err != nil {
		return query{}, errs.Wrapf(errs.KindSignal, err, "query %q", name)
	}
	resolved, err := shared.Directives.Resolve(q)
	if err != nil {
		return query{}, err
	}
	compiled, err := filter.CompileAll(resolved)
	if err != nil {
		return query{}, err
	}
	return query{Label: name, Src: src, Filters: compiled}, nil
}

func newQueryCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "query [name]",
		Short: "Run a saved query by name (or list/show to inspect)",
		Long: "Run a saved query from ~/.munin/queries by name: `munin query <name>`.\n" +
			"With no name it lists the saved queries. Use `munin query show <name>` to\n" +
			"print a query's definition.\n\n" +
			"For an ad-hoc, one-off query against a single signal, use the signal's own\n" +
			"query subcommand instead, e.g. `munin github query` or `munin slack query`.",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeQueryNames,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return listQueries(cmd)
			}
			name := args[0]
			if _, ok := shared.Directives.Queries[name]; ok && !access().QueryVisible(name) {
				return notInRoleError("query", name)
			}
			j, err := buildQuery(name)
			if err != nil {
				return err
			}
			return runQueries(cmd.Context(), cmd.OutOrStdout(), []query{j}, 0)
		},
	}
	c.AddCommand(newQueryListCmd(), newQueryShowCmd())
	return c
}

func newQueryListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List saved query names",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return listQueries(cmd)
		},
	}
}

func newQueryShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "show <name>",
		Short:             "Show a saved query's definition",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeQueryNames,
		RunE: func(cmd *cobra.Command, args []string) error {
			q, ok := shared.Directives.Queries[args[0]]
			if !ok {
				return errs.Newf(errs.KindUsage, "no saved query named %q", args[0]).WithHint("run `munin query list` to see saved queries")
			}
			out, err := yaml.Marshal(q)
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), string(out))
			return nil
		},
	}
}

func listQueries(cmd *cobra.Command) error {
	names := visibleQueryNames()
	if len(names) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "no saved queries visible (check --role, or add YAML files under ~/.munin/queries)")
		return nil
	}
	marker := theme.Cur().Accent.Render(glyph.Bullet())
	for _, n := range names {
		q := shared.Directives.Queries[n]
		fmt.Fprintf(cmd.OutOrStdout(), "%s %-24s signal=%s\n", marker, n, q.Signal)
	}
	return nil
}

func completeQueryNames(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return visibleQueryNames(), cobra.ShellCompDirectiveNoFileComp
}
