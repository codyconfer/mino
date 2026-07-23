package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/codyconfer/munin/internal/auth"
	"github.com/codyconfer/munin/internal/errs"
)

const defaultGitHubOAuthScope = "repo read:org"

func newLoginCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "login <service>",
		Short: "Authenticate a signal via OAuth device flow (fallback when its CLI is absent)",
		Long: "Obtain and cache an OAuth token for a signal without its CLI. Supports\n" +
			"`github` (device flow), `google`, and `slack` (browser loopback). Each needs\n" +
			"its OAuth client credentials in config. Tokens are cached under <home>/tokens\n" +
			"and used by the signal's direct API client.",
		Args:        cobra.ExactArgs(1),
		ValidArgs:   []string{"github", "google", "slack"},
		Annotations: map[string]string{annoSkipOnboarding: "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "github":
				return loginGitHub(cmd)
			case "google":
				return loginGoogle(cmd)
			case "slack":
				return loginSlack(cmd)
			default:
				return errs.Newf(errs.KindUsage, "unsupported login service %q", args[0]).
					WithHint("supported services: github, google, slack")
			}
		},
	}
	return c
}

func loginGoogle(cmd *cobra.Command) error {
	if err := auth.GoogleLogin(cmd.Context(), googleAuth(), cmd.ErrOrStderr()); err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), "Google authorized — token cached; used when gcloud ADC is unavailable.")
	return nil
}

func loginSlack(cmd *cobra.Command) error {
	sa := auth.SlackAuth{
		Store:        shared.tokens,
		ClientID:     shared.cfg.Slack.OAuthClientID,
		ClientSecret: shared.cfg.Slack.OAuthClientSecret,
		UserScopes:   shared.cfg.Slack.UserScopes,
	}
	if err := auth.SlackLogin(cmd.Context(), sa, cmd.ErrOrStderr()); err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), "Slack authorized — user token cached.")
	return nil
}

func loginGitHub(cmd *cobra.Command) error {
	clientID := shared.cfg.GitHub.OAuthClientID
	if clientID == "" {
		return errs.New(errs.KindConfig, "github.oauth_client_id is not set").
			WithHint("set `github.oauth_client_id` in config.yaml (a GitHub OAuth App client id) to use device-flow login")
	}
	scope := shared.cfg.GitHub.OAuthScopes
	if scope == "" {
		scope = defaultGitHubOAuthScope
	}
	token, err := auth.GitHubDeviceFlow(cmd.Context(), clientID, scope, cmd.ErrOrStderr())
	if err != nil {
		return err
	}
	if err := auth.CacheGitHubToken(shared.tokens, token, scope); err != nil {
		return errs.Wrap(errs.KindAuth, err, "caching token")
	}
	fmt.Fprintln(cmd.OutOrStdout(), "GitHub authorized — token cached; munin will use the REST API when gh is unavailable.")
	return nil
}
