package cmd

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/codyconfer/mino/internal/app/verify"
)

func newVerifyCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "verify [" + strings.Join(verify.Targets(), "|") + "]",
		Short:       "Validate config, roles, flights, queries, formatters, and onboarding with detailed diagnostics",
		Long:        "Checks referential integrity (flights → queries, roles → flights, queries →\nfilters/signals, flights/queries → formatters, roles → formatters) and enum\nvalues. Formatter templates are parsed and dry-run against an empty result set,\nand a role that reaches a flight or query whose formatter it does not list is\nflagged. Plugin descriptors are cross-checked against the host registry. On a\nproblem it prints the offending config snippet with secrets masked.",
		ValidArgs:   verify.Targets(),
		Args:        cobra.MatchAll(cobra.MaximumNArgs(1), cobra.OnlyValidArgs),
		Annotations: map[string]string{annoSkipOnboarding: "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			target := verify.TargetAll
			if len(args) == 1 {
				target = args[0]
			}
			return verify.Run(cmd.Context(), cmd.OutOrStdout(), Scope(), shared.Cfg, shared.Directives, shared.Tokens, target)
		},
	}
}
