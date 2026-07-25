package cmd

import (
	"github.com/spf13/cobra"

	"github.com/codyconfer/munin/internal/app/loginflow"
)

func newLoginCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "login <provider|signal>",
		Short: "Authenticate a signal provider via OAuth (guided)",
		Long: "Sign in to a signal provider. Accepts a provider (`github`, `google`,\n" +
			"`slack`) or any signal name — Google signals (`calendar`, `gmail`, `docs`,\n" +
			"`drive`, `tasks`) all resolve to the shared Google login. When run\n" +
			"interactively, munin prompts for any missing OAuth client credentials and\n" +
			"saves them to config before starting the browser/device flow. Tokens are\n" +
			"cached (encrypted) under <home> and used by the signal's direct API client.",
		Args:        cobra.ExactArgs(1),
		ValidArgs:   loginflow.Names(),
		Annotations: map[string]string{annoSkipOnboarding: "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := loginflow.ResolveOrErr(args[0])
			if err != nil {
				return err
			}
			return loginflow.RunCLI(cmd.Context(), shared, p, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
}
