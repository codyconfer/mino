package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	sauth "github.com/codyconfer/sisyphus/auth"

	"github.com/codyconfer/munin/internal/errs"
)

func runTool(ctx context.Context, bins []string, name string, kind errs.Kind, notInstalledMsg, notInstalledHint, runHint string, args ...string) ([]byte, error) {
	out, err := sauth.RunTool(ctx, bins, name, args...)
	if err == nil {
		return out, nil
	}
	if errors.Is(err, sauth.ErrNotInstalled) {
		return nil, errs.New(kind, notInstalledMsg).WithHint("%s", notInstalledHint)
	}
	e := errs.Wrap(kind, err, name)
	if runHint != "" {
		e = e.WithHint("%s", runHint)
	}
	return nil, e
}

func GH(ctx context.Context, args ...string) ([]byte, error) {
	return runTool(ctx, []string{"gh"}, "gh", errs.KindAuth,
		"the GitHub CLI `gh` is not installed or not on PATH",
		"install gh and run `gh auth login`, or run `munin login github`",
		"run `gh auth login` or `munin login github` to (re)authenticate",
		args...)
}

func GHAPIGet(ctx context.Context, store TokenStore, apiURL, path string) ([]byte, error) {
	if GHAvailable() {
		return GH(ctx, "api", path)
	}
	tok, _ := GitHubToken(store)
	if tok == "" {
		return nil, errs.New(errs.KindAuth, "no GitHub authentication available").
			WithHint("install the gh CLI and run `gh auth login`, set GITHUB_TOKEN, or run `munin login github`")
	}
	base := strings.TrimRight(apiURL, "/")
	if base == "" {
		base = "https://api.github.com"
	}
	u := base + "/" + strings.TrimLeft(path, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, errs.Wrap(errs.KindSignal, err, "github: building request")
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := HTTPClient().Do(req)
	if err != nil {
		return nil, errs.Wrap(errs.KindSignal, err, "github: request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, classifyGitHubStatus(resp, errorExcerpt(resp.Body))
	}
	return readBounded(resp, "github", maxAPIResponseBytes)
}
