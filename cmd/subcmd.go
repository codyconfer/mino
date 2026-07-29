package cmd

import (
	"github.com/spf13/cobra"

	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/filter"
	"github.com/codyconfer/munin/internal/plugin"
)

type filterFlags struct {
	names    []string
	includes []string
	excludes []string
}

func (ff *filterFlags) bind(cmd *cobra.Command) {
	f := cmd.Flags()
	f.StringSliceVar(&ff.names, "filter", nil, "apply a saved filter set by name (repeatable)")
	f.StringArrayVar(&ff.includes, "include", nil, "keep only items whose body matches this regex (repeatable)")
	f.StringArrayVar(&ff.excludes, "exclude", nil, "drop items whose body matches this regex (repeatable)")
}

func (ff *filterFlags) compile() ([]filter.Compiled, error) {
	var sets []filter.Filter
	for _, n := range ff.names {
		f, ok := shared.Directives.LookupFilter(n)
		if !ok {
			return nil, errs.Newf(errs.KindUsage, "unknown filter %q", n).
				WithHint("see `munin filter list`")
		}
		sets = append(sets, f)
	}
	inline := filter.Filter{Name: "flags"}
	for _, p := range ff.includes {
		inline.Rules = append(inline.Rules, filter.Rule{Include: p})
	}
	for _, p := range ff.excludes {
		inline.Rules = append(inline.Rules, filter.Rule{Exclude: p})
	}
	if len(inline.Rules) > 0 {
		sets = append(sets, inline)
	}
	return filter.CompileAll(sets)
}

func sourceCmd(name, short string) *cobra.Command {
	parent := &cobra.Command{Use: name, Short: short}

	var ff filterFlags
	query := &cobra.Command{
		Use:   "query",
		Short: "Fetch " + name + " now, with optional filters",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSignal(cmd, name, nil, &ff)
		},
	}
	ff.bind(query)
	parent.AddCommand(query)
	if plugin.HasCapability(name, plugin.CapDetail) {
		parent.AddCommand(sourceShowCmd(name))
	}
	return parent
}

func sourceShowCmd(name string) *cobra.Command {
	return &cobra.Command{
		Use:               "show <url>",
		Short:             "Show details for one " + name + " item",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runShow(cmd, args[0], name)
		},
	}
}

func runSignal(cmd *cobra.Command, name string, params map[string]string, ff *filterFlags) error {
	src, err := buildSignal(name, params)
	if err != nil {
		return err
	}
	compiled, err := ff.compile()
	if err != nil {
		return err
	}
	return runQueries(cmd.Context(), cmd.OutOrStdout(), name, []query{{Label: name, Src: src, Filters: compiled}}, 0)
}
