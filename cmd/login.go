package cmd

import (
	"slices"

	"github.com/spf13/cobra"

	"github.com/codyconfer/mino/internal/app/loginflow"
	"github.com/codyconfer/mino/internal/auth"
	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/gitauth"
)

func newLoginCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "login <provider|signal>",
		Short: "Authenticate a signal provider via OAuth (guided)",
		Long: "Sign in to a signal provider. Accepts a provider (`github`, `gitlab`,\n" +
			"`gitea`, `forgejo`, `google`, `slack`) or any signal name — Google signals\n" +
			"(`calendar`, `gmail`, `docs`, `drive`, `tasks`) all resolve to the shared\n" +
			"Google login. When run interactively, mino prompts for whatever the provider\n" +
			"needs: OAuth client credentials are saved to config, while a pasted Gitea\n" +
			"access token is only ever sealed in the credential store. Tokens are cached\n" +
			"(encrypted) under <home> and used by the signal's direct API client.",
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
			if err := refuseWhenServiceAuthWins(p); err != nil {
				return err
			}
			return loginflow.RunCLI(cmd.Context(), shared, Scope(), p, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
}

func refuseWhenServiceAuthWins(p loginflow.Provider) error {
	if shared == nil || !gitauth.Known(p.Key) {
		return nil
	}
	prov, id, err := shared.GitAuth()
	if err != nil || prov == nil || id == nil || !id.ServiceIdentity() {
		return nil
	}
	if prov.Name() != p.Key && !slices.Contains(p.Signals, prov.Name()) {
		return nil
	}
	return errs.Newf(errs.KindUsage, "git service auth is configured (%s), so a personal login would never be used", id.Origin()).
		WithHint("%s", serviceAuthHint(prov.Name()))
}

func serviceAuthHint(provider string) string {
	switch provider {
	case "gitea", "forgejo":
		return "unset gitea.service_token (and MINO_GITEA_SERVICE_TOKEN) to log in as yourself, or run " +
			"`mino verify auth` to check the service credential"
	case "gitlab":
		return "unset gitlab.service_token (and MINO_GITLAB_SERVICE_TOKEN) to log in as yourself, or run " +
			"`mino verify auth` to check the service credential"
	case "github":
		return "unset github.app / github.service_token (and the matching MINO_GITHUB_* vars) to log in as " +
			"yourself, or run `mino verify auth` to check the service credential"
	}
	return "unset the service credential for " + provider + " to log in as yourself, or run `mino verify auth` to check it"
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
