package verify

import (
	"context"
	"strconv"
	"strings"

	"github.com/codyconfer/mino/internal/app/onboard"
	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/gitauth"
)

func Auth(ctx context.Context, cfg *config.Config, p gitauth.Provider, id gitauth.Identity) []Finding {
	if p == nil {
		return []Finding{{
			Name: "git.provider", Warn: true,
			Msg: "no git provider resolved; known providers: " + strings.Join(gitauth.Names(), ", "),
		}}
	}

	out := []Finding{{
		Name: "git.provider", OK: true,
		Msg: p.Name() + " (" + p.Label() + " at " + p.Host() + ")",
	}}

	if id != nil {
		out = append(out, Finding{
			Name:    "git.auth.trace",
			OK:      true,
			Msg:     "resolution order",
			Snippet: id.Trace(),
		})
	}

	if id != nil && id.ServiceIdentity() && !onboard.ServiceAuthAllowed() {
		out = append(out, Finding{
			Name: "git.auth.service_build",
			Warn: true,
			Msg: "this binary was not built with SERVICE_AUTH=1, so the signing checks still apply " +
				"even though a service identity is configured",
		})
	}

	for _, f := range p.Findings(ctx, id) {
		out = append(out, Finding{Name: f.Name, Msg: f.Msg, OK: f.OK, Warn: f.Warn})
	}

	out = append(out, viewerFinding(cfg, id))
	return out
}

func viewerFinding(cfg *config.Config, id gitauth.Identity) Finding {
	f := Finding{Name: "github.viewer"}
	if cfg == nil {
		f.OK = true
		return f
	}
	if v := strings.TrimSpace(cfg.GitHub.Viewer); v != "" {
		f.OK, f.Msg = true, v+" replaces @me in queries"
		return f
	}
	if id == nil || !id.ServiceIdentity() {
		f.OK, f.Msg = true, "unset; @me resolves to the authenticated user"
		return f
	}
	for _, q := range effectiveQueries(cfg.GitHub) {
		if strings.Contains(q, "@me") {
			f.Warn = true
			f.Msg = "unset, but " + strconv.Quote(q) + " uses @me, which matches nothing as a service " +
				"identity; queries return empty with no error"
			return f
		}
	}
	f.OK, f.Msg = true, "unset; no configured query uses @me"
	return f
}

func effectiveQueries(gh config.GitHubConfig) []string {
	if len(gh.Queries) > 0 {
		return gh.Queries
	}
	return []string{"is:open is:pr author:@me", "is:open is:pr review-requested:@me"}
}
