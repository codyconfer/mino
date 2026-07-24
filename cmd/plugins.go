package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/codyconfer/munin/internal/plugin"
	"github.com/codyconfer/munin/internal/signals/build"
)

func newPluginsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugins",
		Short: "List and enable/disable compile-time plugins",
		Long:  "Plugins are registered at compile time; enable/disable controls runtime activation and verify (ADR-13).",
	}
	cmd.AddCommand(newPluginsListCmd(), newPluginsEnableCmd(), newPluginsDisableCmd())
	return cmd
}

func newPluginsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List registered plugins",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_ = build.KnownSignals() // ensure builtins init
			plugin.LoadEnabled()
			out := cmd.OutOrStdout()
			for _, row := range plugin.ListEnabled() {
				d, _ := plugin.Lookup(row.ID)
				state := "enabled"
				if !row.Enabled {
					state = "disabled"
				}
				caps := make([]string, len(d.Capabilities))
				for i, c := range d.Capabilities {
					caps[i] = string(c)
				}
				fmt.Fprintf(out, "%-18s %-8s kind=%s signal=%s caps=[%s]\n",
					row.ID, state, d.Kind, d.Signal, strings.Join(caps, ","))
			}
			return nil
		},
	}
}

func newPluginsEnableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enable <plugin-id>",
		Short: "Enable a registered plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = build.KnownSignals()
			if err := plugin.SetEnabled(args[0], true); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "enabled %s\n", args[0])
			return nil
		},
	}
}

func newPluginsDisableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disable <plugin-id>",
		Short: "Disable a registered plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = build.KnownSignals()
			if err := plugin.SetEnabled(args[0], false); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "disabled %s\n", args[0])
			return nil
		},
	}
}
