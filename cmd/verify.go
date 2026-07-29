package cmd

import (
	"github.com/spf13/cobra"

	"github.com/codyconfer/munin/internal/app/verify"
)

func newVerifyCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "verify [config|roles|flights|queries|formatters|onboarding|all]",
		Short:       "Validate config, roles, flights, queries, formatters, and onboarding with detailed diagnostics",
		Long:        "Checks referential integrity (flights → queries, roles → flights, queries →\nfilters/signals, flights/queries → formatters, roles → formatters) and enum\nvalues. Formatter templates are parsed and dry-run against an empty result set,\nand a role that reaches a flight or query whose formatter it does not list is\nflagged. On a problem it prints the offending config snippet with secrets\nmasked.",
		ValidArgs:   []string{"config", "roles", "flights", "queries", "formatters", "all", "onboarding"},
		Args:        cobra.MaximumNArgs(1),
		Annotations: map[string]string{annoSkipOnboarding: "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			target := "all"
			if len(args) == 1 {
				target = args[0]
			}
			return verify.Run(cmd.Context(), cmd.OutOrStdout(), shared.Cfg, shared.Directives, shared.Tokens, target)
		},
	}
}
