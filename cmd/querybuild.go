package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/filter"
	"github.com/codyconfer/munin/internal/signals/build"
)

type queryBuildFlags struct {
	signal  string
	params  []string
	filters []string
	field   string
	include string
	exclude string
	title   string
	save    string
	dryRun  bool
}

func newQueryBuildCmd() *cobra.Command {
	var f queryBuildFlags
	c := &cobra.Command{
		Use:   "build --signal <name> [--param k=v]...",
		Short: "Compose and run an ad-hoc query, optionally saving it",
		Long: "Builds a query from flags, runs it, and prints the results. Nothing is\n" +
			"written unless you pass --save, which stores the query under\n" +
			"~/.munin/queries so it can be run by name and used in flights.\n\n" +
			"Use --dry-run to print the query definition without running it.\n\n" +
			"For the same thing interactively, open the deck and pick `Build a query`.\n\n" +
			knownParamHelp(),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q, err := f.query()
			if err != nil {
				return err
			}
			if f.save != "" {
				if err := checkSaveName(q.Name); err != nil {
					return err
				}
			}
			if f.dryRun {
				return printQueryDoc(cmd, q)
			}
			label := q.Name
			if label == "" {
				label = "ad-hoc"
			}
			j, err := buildQueryFrom(label, q)
			if err != nil {
				return err
			}
			if err := runQueries(cmd.Context(), cmd.OutOrStdout(), label, []query{j}, 0); err != nil {
				return err
			}
			if f.save == "" {
				return nil
			}
			return saveBuiltQuery(cmd, q)
		},
	}
	c.Flags().StringVar(&f.signal, "signal", "", "signal to query (required)")
	c.Flags().StringArrayVar(&f.params, "param", nil, "query param as key=value (repeatable)")
	c.Flags().StringArrayVar(&f.filters, "filter", nil, "saved filter to apply (repeatable)")
	c.Flags().StringVar(&f.field, "field", "", "field for the inline rule (blank = whole item)")
	c.Flags().StringVar(&f.include, "include", "", "inline rule include regex")
	c.Flags().StringVar(&f.exclude, "exclude", "", "inline rule exclude regex")
	c.Flags().StringVar(&f.title, "title", "", "display title for the results panel")
	c.Flags().StringVar(&f.save, "save", "", "save the query under this name")
	c.Flags().BoolVar(&f.dryRun, "dry-run", false, "print the query definition instead of running it")
	_ = c.MarkFlagRequired("signal")
	if err := c.RegisterFlagCompletionFunc("signal", completeSignalNames); err != nil {
		panic(err)
	}
	if err := c.RegisterFlagCompletionFunc("filter", completeFilterNames); err != nil {
		panic(err)
	}
	return c
}

func (f queryBuildFlags) query() (config.Query, error) {
	if !build.KnownSignals()[f.signal] {
		return config.Query{}, errs.Newf(errs.KindUsage, "unknown signal %q", f.signal).
			WithHint("known signals: %s", strings.Join(build.QueryableSignals(), ", "))
	}
	params, err := parseParamFlags(f.params)
	if err != nil {
		return config.Query{}, err
	}
	q := config.Query{Name: f.save, Title: f.title, Signal: f.signal}
	if len(params) > 0 {
		q.Params = params
	}
	for _, ref := range f.filters {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		if _, ok := shared.Directives.LookupFilter(ref); !ok {
			return config.Query{}, errs.Newf(errs.KindUsage, "unknown filter %q", ref).
				WithHint("run `munin filter list` to see the saved filters")
		}
		q.Filters = append(q.Filters, config.QueryFilter{Ref: ref})
	}
	r := filter.Rule{Field: f.field, Include: f.include, Exclude: f.exclude}
	if r != (filter.Rule{}) {
		if _, err := filter.Compile(filter.Filter{Name: "ad-hoc", Rules: []filter.Rule{r}}); err != nil {
			return config.Query{}, err
		}
		q.Rules = []filter.Rule{r}
	}
	return q, nil
}

func parseParamFlags(pairs []string) (map[string]string, error) {
	out := map[string]string{}
	for _, pair := range pairs {
		k, val, found := strings.Cut(pair, "=")
		k = strings.TrimSpace(k)
		if !found || k == "" {
			return nil, errs.Newf(errs.KindUsage, "--param %q is not key=value", pair)
		}
		out[k] = val
	}
	return out, nil
}

func printQueryDoc(cmd *cobra.Command, q config.Query) error {
	out, err := yaml.Marshal(q)
	if err != nil {
		return err
	}
	fmt.Fprint(cmd.OutOrStdout(), string(out))
	return nil
}

func checkSaveName(name string) error {
	if _, exists := shared.Directives.Queries[name]; exists {
		return errs.Newf(errs.KindUsage, "a query named %q already exists", name).
			WithHint("pick another --save name, or edit ~/.munin/queries/%s.yaml", name)
	}
	return nil
}

func saveBuiltQuery(cmd *cobra.Command, q config.Query) error {
	path, stored, err := config.SaveCollectionItem(shared.Mgr, shared.Cfg.Home, config.DirQueries, q.Name, q)
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "\nsaved %s\n", path)
	if !stored {
		fmt.Fprintln(out, "the config store is unavailable; run `munin import queries` to store it.")
		return nil
	}
	fmt.Fprintln(out, "imported the queries collection into DuckDB.")
	return nil
}

func knownParamHelp() string {
	var b strings.Builder
	b.WriteString("Known params by signal:\n")
	for _, sig := range build.ParamSignals() {
		keys := make([]string, 0, 4)
		for _, p := range build.QueryParams(sig) {
			keys = append(keys, p.Key)
		}
		fmt.Fprintf(&b, "  %-10s %s\n", sig, strings.Join(keys, ", "))
	}
	b.WriteString("\nSignals not listed take no params, or accept params defined by their plugin.")
	return b.String()
}

func completeSignalNames(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	return build.QueryableSignals(), cobra.ShellCompDirectiveNoFileComp
}

func completeFilterNames(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	names := shared.Directives.FilterNames()
	sort.Strings(names)
	return names, cobra.ShellCompDirectiveNoFileComp
}
