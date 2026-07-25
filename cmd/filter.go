package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/plugin"
	"github.com/codyconfer/munin/internal/signals/build"
)

func newFilterCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "filter",
		Short: "Inspect saved regex filter sets and plugin filter engines",
		Long: `List and show filter contributions.

Saved YAML filters live under ~/.munin/filters. Plugins may also register
KindFilter contributions:

  RegisterFilter       — YAML-shaped include/exclude regex rules
  RegisterFilterEngine — custom Go filter logic (same query ref syntax)

Plugin engines appear in list with kind=engine.`,
	}
	c.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "List saved filter names and plugin filter contributions",
			RunE: func(cmd *cobra.Command, _ []string) error {
				_ = build.KnownSignals()
				out := cmd.OutOrStdout()
				names := visibleFilterNames()
				if len(names) == 0 && len(plugin.FilterNames()) == 0 {
					fmt.Fprintln(out, "no saved filters visible (check --role, or add YAML files under ~/.munin/filters)")
					return nil
				}
				seen := map[string]bool{}
				for _, n := range names {
					f := shared.Directives.Filters[n]
					fmt.Fprintf(out, "%-24s kind=yaml   %d rule(s)\n", n, len(f.Rules))
					seen[n] = true
				}
				for _, n := range plugin.FilterNames() {
					if seen[n] {
						continue
					}
					kind := "rules"
					if plugin.HasFilterEngine(n) {
						kind = "engine"
					}
					f, _ := plugin.LookupFilter(n)
					fmt.Fprintf(out, "%-24s kind=%-6s %d rule(s)\n", n, kind, len(f.Rules))
				}
				return nil
			},
		},
		&cobra.Command{
			Use:   "show <name>",
			Short: "Show a saved filter's rules (or note a plugin engine)",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				_ = build.KnownSignals()
				name := args[0]
				if f, ok := shared.Directives.Filters[name]; ok {
					out, err := yaml.Marshal(f)
					if err != nil {
						return err
					}
					fmt.Fprint(cmd.OutOrStdout(), string(out))
					return nil
				}
				if plugin.HasFilterEngine(name) {
					fmt.Fprintf(cmd.OutOrStdout(), "name: %s\nkind: engine\n# Go FilterFunc — no YAML rules\n", name)
					return nil
				}
				if f, ok := plugin.LookupFilter(name); ok {
					out, err := yaml.Marshal(f)
					if err != nil {
						return err
					}
					fmt.Fprint(cmd.OutOrStdout(), string(out))
					return nil
				}
				return errs.Newf(errs.KindUsage, "no saved filter named %q", name).WithHint("run `munin filter list` to see saved filters")
			},
		},
	)
	return c
}
