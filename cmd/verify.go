package cmd

import (
	"github.com/spf13/cobra"

	"github.com/codyconfer/munin/internal/verify"
)

func newVerifyCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "verify [config|roles|flights|queries|onboarding|all]",
		Short:       "Validate config, roles, flights, queries, and onboarding with detailed diagnostics",
		Long:        "Checks referential integrity (flights → queries, roles → flights, queries →\nfilters/signals) and enum values. On a problem it prints the offending config\nsnippet with secrets masked.",
		ValidArgs:   []string{"config", "roles", "flights", "queries", "all", "onboarding"},
		Args:        cobra.MaximumNArgs(1),
		Annotations: map[string]string{annoSkipOnboarding: "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			target := "all"
			if len(args) == 1 {
				target = args[0]
			}
			return verify.Run(cmd.Context(), cmd.OutOrStdout(), shared.cfg, shared.directives, shared.tokens, target)
		},
	}
}
