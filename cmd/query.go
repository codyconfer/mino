package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/filter"
)

func buildQuery(name string) (query, error) {
	q, ok := shared.Directives.Queries[name]
	if !ok {
		return query{}, errs.Newf(errs.KindUsage, "no saved query named %q", name).WithHint("run `mino query list` to see saved queries")
	}
	if !q.Runnable() {
		return query{}, errs.Newf(errs.KindUsage, "%q defines no signal, so there is nothing to run", name).
			WithHint("it is a filter-only document; reference it from a query's `filters:` list")
	}
	return buildQueryFrom(name, q)
}

func buildQueryFrom(label string, q config.Query) (query, error) {
	resolved, err := shared.Directives.Resolve(q)
	if err != nil {
		return query{}, err
	}
	params, err := filter.ExpandParams(q.Params, resolved)
	if err != nil {
		return query{}, errs.Wrapf(errs.KindConfig, err, "query %q", label)
	}
	src, err := buildSignal(q.Signal, params)
	if err != nil {
		return query{}, errs.Wrapf(errs.KindSignal, err, "query %q", label)
	}
	compiled, err := filter.CompileAll(resolved)
	if err != nil {
		return query{}, err
	}
	title := q.Display()
	if title == "" {
		title = label
	}
	return query{Label: label, Title: title, Src: src, Filters: compiled}, nil
}

func newQueryCmd() *cobra.Command {
	var ff formatterFlags
	c := &cobra.Command{
		Use:   "query [name]",
		Short: "Run a saved query by name (or list/show to inspect)",
		Long: "Run a saved query from ~/.mino/queries by name: `mino query <name>`.\n" +
			"With no name it lists the saved queries. Use `mino query show <name>` to\n" +
			"print a query's definition.\n\n" +
			"For an ad-hoc, one-off query against a single signal, use the signal's own\n" +
			"query subcommand instead, e.g. `mino github query` or `mino slack query`.",
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
			o, err := ff.resolve(cmd, shared.Directives.Queries[name].Formatter)
			if err != nil {
				return err
			}
			o.kind = "query"
			j, err := buildQuery(name)
			if err != nil {
				return err
			}
			return runQueriesWith(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), name, []query{j}, 0, o)
		},
	}
	ff.bind(c)
	c.AddCommand(newQueryListCmd(), newQueryShowCmd(), newQueryBuildCmd())
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
				return errs.Newf(errs.KindUsage, "no saved query named %q", args[0]).WithHint("run `mino query list` to see saved queries")
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
		fmt.Fprintln(cmd.OutOrStdout(), "no saved queries visible (check --role, or add YAML files under ~/.mino/queries)")
		return nil
	}
	sc := Scope()
	marker := sc.Theme.Accent.Render(sc.Glyphs.Bullet())
	for _, n := range names {
		q := shared.Directives.Queries[n]
		line := fmt.Sprintf("%s %-24s %s", marker, n, querySummary(sc.Theme, q))
		if q.Title != "" {
			line += "  " + sc.Theme.Dim.Render(q.Title)
		}
		fmt.Fprintln(cmd.OutOrStdout(), line)
	}
	return nil
}

func querySummary(th theme.Theme, q config.Query) string {
	if !q.Runnable() {
		return th.Dim.Render("filter-only")
	}
	if q.HasRules() {
		return fmt.Sprintf("signal=%s +rules", q.Signal)
	}
	return "signal=" + q.Signal
}
