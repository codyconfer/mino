package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/codyconfer/munin/internal/plugin"
	"github.com/codyconfer/munin/internal/signals/build"
)

func newActionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "action",
		Short: "List or run CapAction bindings",
		Long:  "Actions are discoverable write/side-effect capabilities advertised by plugins.",
	}
	cmd.AddCommand(newActionListCmd(), newActionRunCmd())
	return cmd
}

func newActionListCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "list [signal]",
		Short:             "List registered actions (optionally for one signal)",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeCacheSignals,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = build.KnownSignals()
			out := cmd.OutOrStdout()
			if len(args) == 1 {
				sig := args[0]
				acts := build.Actions(sig)
				if len(acts) == 0 {
					fmt.Fprintf(out, "no actions for signal %q\n", sig)
					return nil
				}
				for _, a := range acts {
					fmt.Fprintf(out, "%s\t%s\n", a.Signal, a.Name)
				}
				return nil
			}
			seen := map[string]bool{}
			for _, d := range plugin.All() {
				if d.Kind != plugin.KindSignal || d.Signal == "" || seen[d.Signal] {
					continue
				}
				seen[d.Signal] = true
				if !plugin.HasCapability(d.Signal, plugin.CapAction) {
					continue
				}
				acts := build.Actions(d.Signal)
				names := make([]string, 0, len(acts))
				for _, a := range acts {
					names = append(names, a.Name)
				}
				fmt.Fprintf(out, "%-12s caps=action actions=[%s]\n", d.Signal, strings.Join(names, ","))
			}
			return nil
		},
	}
}

func newActionRunCmd() *cobra.Command {
	var params []string
	cmd := &cobra.Command{
		Use:               "run <signal> <name>",
		Short:             "Run a registered action",
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: completeActionRun,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = build.KnownSignals()
			p := map[string]string{"home": shared.Cfg.Home, "role": shared.Cfg.Role}
			for _, kv := range params {
				k, v, ok := strings.Cut(kv, "=")
				if !ok || k == "" {
					return fmt.Errorf("param must be key=value: %q", kv)
				}
				p[k] = v
			}
			if err := build.Action(cmd.Context(), args[0], args[1], p); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "ok %s/%s\n", args[0], args[1])
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&params, "param", nil, "action param key=value (repeatable)")
	return cmd
}
