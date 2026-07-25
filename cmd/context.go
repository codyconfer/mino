package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/codyconfer/munin/internal/plugin"
	"github.com/codyconfer/munin/internal/signals/build"
)

func newContextCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context",
		Short: "Show or switch per-tool plugin contexts",
		Long:  "Contexts are per-tool active selections. Role activation can apply role.contexts bindings.",
	}
	cmd.AddCommand(newContextListCmd(), newContextSwitchCmd(), newContextApplyRoleCmd())
	return cmd
}

func newContextListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List known tool contexts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_ = build.KnownSignals()
			out := cmd.OutOrStdout()
			ctxs := plugin.ListContexts(cmd.Context())
			if len(ctxs) == 0 {
				fmt.Fprintln(out, "no context providers registered")
				return nil
			}
			for _, c := range ctxs {
				name := c.Name
				if name == "" {
					name = "(unset)"
				}
				fmt.Fprintf(out, "%s\t%s\n", c.Tool, name)
			}
			return nil
		},
	}
}

func newContextSwitchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "switch <tool> <name>",
		Short: "Switch a tool context",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = build.KnownSignals()
			if err := plugin.SwitchContext(cmd.Context(), args[0], args[1]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "switched %s → %s\n", args[0], args[1])
			return nil
		},
	}
}

func newContextApplyRoleCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "apply-role",
		Short: "Apply the active role's contexts bindings",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_ = build.KnownSignals()
			role := access().Role
			if role == "" {
				fmt.Fprintln(cmd.OutOrStdout(), "no active role")
				return nil
			}
			rd, ok := shared.Directives.Roles[role]
			if !ok || len(rd.Contexts) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "role has no contexts bindings")
				return nil
			}
			if err := plugin.ApplyRoleContexts(cmd.Context(), rd.Contexts); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "applied %d context binding(s) for role %s\n", len(rd.Contexts), role)
			return nil
		},
	}
}
