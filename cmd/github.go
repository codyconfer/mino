package cmd

import (
	"github.com/spf13/cobra"

	"github.com/codyconfer/munin/internal/auth"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/signals"
	gh "github.com/codyconfer/munin/internal/signals/github"
)

func newGithubCmd() *cobra.Command {
	return sourceCmd("github", "GitHub activity (PRs, review requests)")
}

func buildGithub(params map[string]string) (signals.Signal, error) {
	queries := shared.cfg.GitHub.Queries
	if q := params["query"]; q != "" {
		queries = []string{q}
	}
	backend, err := githubBackend()
	if err != nil {
		return nil, err
	}
	return gh.New(queries, backend, shared.cfg.GitHub.Max), nil
}

func githubBackend() (gh.Backend, error) {
	if auth.GHAvailable() {
		return gh.CLIBackend{}, nil
	}
	if tok, origin := auth.GitHubToken(shared.tokens); tok != "" {
		verbosef("github: gh CLI not found; using %s via the REST API", origin)
		return gh.APIBackend{Token: tok, BaseURL: shared.cfg.GitHub.APIURL}, nil
	}
	return nil, errs.New(errs.KindAuth, "no GitHub authentication available").WithHint("install the gh CLI and run `gh auth login`, set GITHUB_TOKEN, or run `munin login github`")
}
