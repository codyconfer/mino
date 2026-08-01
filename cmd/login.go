package cmd

import (
	"github.com/spf13/cobra"

	"github.com/codyconfer/mino/internal/app/loginflow"
	"github.com/codyconfer/mino/internal/auth"
	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/errs"
)

func newLoginCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "login <provider|signal>",
		Short: "Authenticate a signal provider via OAuth (guided)",
		Long: "Sign in to a signal provider. Accepts a provider (`github`, `google`,\n" +
			"`slack`) or any signal name — Google signals (`calendar`, `gmail`, `docs`,\n" +
			"`drive`, `tasks`) all resolve to the shared Google login. When run\n" +
			"interactively, mino prompts for any missing OAuth client credentials and\n" +
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
			if err := refuseUnreadableStore(p); err != nil {
				return err
			}
			return loginflow.RunCLI(cmd.Context(), shared, Scope(), p, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
}

func refuseUnreadableStore(p loginflow.Provider) error {
	if shared == nil || shared.Tokens == nil {
		return nil
	}
	if _, state, _ := auth.ReadCredential(shared.Tokens, p.Key); state != auth.CredUnreadable {
		return nil
	}
	path := config.DataPath(shared.Cfg.Home, config.TokensDB)
	return errs.Newf(errs.KindAuth, "the stored %s credential cannot be decrypted, so a new login would not be readable either", p.Label).
		WithHint("the credential store was written with a different encryption key: delete %s (this drops every cached token), or restore the keyring entry mino used before, then run `mino login %s` again", path, p.Key)
}
