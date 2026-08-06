package build

import (
	"strings"
	"time"

	"github.com/codyconfer/mino/internal/auth"
	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/log"
	"github.com/codyconfer/mino/internal/signals"
	"github.com/codyconfer/mino/internal/signals/active"
	"github.com/codyconfer/mino/internal/signals/cache"
	gl "github.com/codyconfer/mino/internal/signals/gitlab"
	"github.com/codyconfer/mino/internal/token"
)

const gitlabAuthHint = "configure gitlab.service_token for a service identity, install the glab CLI " +
	"and run `glab auth login`, set GITLAB_TOKEN, or run `mino login gitlab`"

func buildGitlab(params map[string]string, cfg *config.Config, tokens *token.Store, results *cache.Store) (signals.Signal, error) {
	sel, err := gitlabAuth(cfg, tokens)
	if err != nil {
		return nil, err
	}
	backend, rate, err := gitlabBackendFor(sel)
	if err != nil {
		return nil, err
	}

	opts := []gl.Option{gl.WithRateHint(rate)}
	if viewer := strings.TrimSpace(cfg.GitLab.Viewer); viewer != "" {
		opts = append(opts, gl.WithViewer(viewer))
	} else {
		warnViewerlessGitlabQueries(sel, params, cfg)
	}
	if results != nil {
		opts = append(opts, gl.WithDetailCache(results,
			gl.CachePolicy{Read: results.Reads(), Write: results.Writes(), TTL: results.DetailTTL()}))
	}
	if title := params["title"]; title != "" {
		opts = append(opts, gl.WithTitle(title))
	}
	return gl.New(effectiveGitlabSelectors(params, cfg), backend, cfg.GitLab.Max, opts...)
}

func buildActiveGitlab(params map[string]string, cfg *config.Config, tokens *token.Store, state *active.State) (signals.ActiveSignal, error) {
	sel, err := gitlabAuth(cfg, tokens)
	if err != nil {
		return nil, err
	}
	// No mechanism refusal here: GitLab has no /notifications asymmetry, so a service
	// token streams as well as a user one.
	if !sel.Authenticated() {
		return nil, errs.New(errs.KindAuth, "gitlab: realtime needs GitLab authentication").
			WithHint("%s", gitlabAuthHint)
	}
	backend, rate, err := gitlabBackendFor(sel)
	if err != nil {
		return nil, err
	}
	interval, err := paramPollInterval(params, "gitlab", 60*time.Second)
	if err != nil {
		return nil, err
	}

	opts := []gl.Option{gl.WithRateHint(rate)}
	if viewer := strings.TrimSpace(cfg.GitLab.Viewer); viewer != "" {
		opts = append(opts, gl.WithViewer(viewer))
	} else {
		warnViewerlessGitlabQueries(sel, params, cfg)
	}
	sig, err := gl.New(effectiveGitlabSelectors(params, cfg), backend, cfg.GitLab.Max, opts...)
	if err != nil {
		return nil, err
	}
	return gl.NewActive(sig, interval, state), nil
}

func gitlabAuth(cfg *config.Config, tokens *token.Store) (auth.GitLabSelection, error) {
	base, err := gl.NormalizeAPIURL(cfg.GitLab.APIURL)
	if err != nil {
		return auth.GitLabSelection{}, err
	}
	sel, err := auth.SelectGitLab(cfg.GitLab.AuthSpec(base, tokens))
	if err != nil {
		return auth.GitLabSelection{}, err
	}
	log.Debugf("%s", sel.Trace())
	return sel, nil
}

func gitlabBackendFor(sel auth.GitLabSelection) (gl.Backend, *gl.RateHint, error) {
	if sel.UsesGLabCLI() {
		return gl.CLIBackend{Hostname: auth.GLabHostname(sel.APIURL)}, nil, nil
	}
	if !sel.Authenticated() {
		return nil, nil, errs.New(errs.KindAuth, "no GitLab authentication available").
			WithHint("%s", gitlabAuthHint)
	}
	log.Debugf("gitlab: using %s via the REST API", sel.Origin)
	rate := gl.NewRateHint()
	base := sel.APIURL
	if base == "" {
		base = auth.GitLabAPIBase("")
	}
	return gl.APIBackend{Auth: sel, BaseURL: base, Rate: rate}, rate, nil
}

func effectiveGitlabSelectors(params map[string]string, cfg *config.Config) []string {
	if q := params["query"]; q != "" {
		return []string{q}
	}
	if len(cfg.GitLab.Queries) > 0 {
		return cfg.GitLab.Queries
	}
	return gl.DefaultQueries()
}

// warnViewerlessGitlabQueries warns rather than failing: a service token resolves @me
// through /user to the bot's own username, which is a wrong-but-plausible result.
func warnViewerlessGitlabQueries(sel auth.GitLabSelection, params map[string]string, cfg *config.Config) {
	if !sel.ServiceIdentity() {
		return
	}
	for _, q := range effectiveGitlabSelectors(params, cfg) {
		if strings.Contains(q, gl.ViewerAlias) {
			log.Warnf("gitlab: selector %q uses %s, which resolves to %s as a service identity; "+
				"set gitlab.viewer to the username mino should stand in for, or use scope:assigned",
				q, gl.ViewerAlias, sel.Origin)
		}
	}
}
