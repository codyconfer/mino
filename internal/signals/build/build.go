package build

import (
	"strings"
	"time"

	"github.com/codyconfer/mino/internal/auth"
	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/log"
	"github.com/codyconfer/mino/internal/plugin"
	"github.com/codyconfer/mino/internal/signals"
	"github.com/codyconfer/mino/internal/signals/active"
	"github.com/codyconfer/mino/internal/signals/cache"
	gh "github.com/codyconfer/mino/internal/signals/github"
	"github.com/codyconfer/mino/internal/token"
	pub "github.com/codyconfer/mino/plugin"

	_ "github.com/codyconfer/mino/internal/plugin/ntr"
)

func init() {
	plugin.RegisterBuiltins()
	registerStockBuilders()
	registerStockQueryParams()
}

var ErrNoActive = errs.New(errs.KindUsage, "signal has no active (streaming) implementation")

var ErrNoScheduled = errs.New(errs.KindUsage, "signal has no scheduled implementation")

func ResolveWriteTarget(what, setting, configured, requested string) (string, error) {
	if configured == "" {
		return "", errs.Newf(errs.KindConfig, "no writable %s configured", what).WithHint("set `%s` in config", setting)
	}
	if requested == "" || strings.EqualFold(requested, configured) {
		return configured, nil
	}
	return "", errs.Newf(errs.KindUsage, "%s %q is read-only; only %q is writable (%s)", what, requested, configured, setting)
}

func Signal(name string, params map[string]string, role string, cfg *config.Config, tokens *token.Store, results *cache.Store) (signals.Signal, error) {
	if !plugin.HasBuilder(name) {
		return nil, errs.Newf(errs.KindConfig, "unknown signal %q", name)
	}
	if !plugin.SignalEnabled(name) {
		return nil, errs.Newf(errs.KindConfig, "signal %q is disabled", name).
			WithHint("enable with `mino plugins enable` for the backing plugin")
	}
	q, err := plugin.BuildQuery(name, newHostBuildCtx(name, params, role, cfg, tokens, nil, results))
	if err != nil {
		return nil, err
	}
	if isNilRef(q) {
		return nil, errs.Newf(errs.KindInternal, "builder for signal %q returned no query", name)
	}
	return results.Wrap(q, name, role, params), nil
}

func ActiveSignal(name string, params map[string]string, role string, cfg *config.Config, tokens *token.Store, state *active.State) (signals.ActiveSignal, error) {
	if !plugin.HasBuilder(name) && !plugin.HasStreamBuilder(name) {
		return nil, errs.Newf(errs.KindConfig, "unknown signal %q", name)
	}
	if !plugin.SignalEnabled(name) {
		return nil, errs.Newf(errs.KindConfig, "signal %q is disabled", name).
			WithHint("enable with `mino plugins enable` for the backing plugin")
	}
	if !plugin.HasStreamBuilder(name) {
		if plugin.HasCapability(name, plugin.CapStream) {
			return nil, errs.Newf(errs.KindInternal, "signal %q advertises CapStream but has no active builder", name)
		}
		return nil, ErrNoActive
	}
	if !plugin.HasCapability(name, plugin.CapStream) {
		return nil, errs.Newf(errs.KindConfig, "signal %q does not advertise CapStream", name)
	}
	src, err := plugin.BuildStream(name, newHostBuildCtx(name, params, role, cfg, tokens, state, nil))
	if err != nil {
		return nil, err
	}
	if isNilRef(src) {
		return nil, errs.Newf(errs.KindInternal, "builder for signal %q returned no stream", name)
	}
	return src, nil
}

func ScheduledJob(name string, params map[string]string, role string, cfg *config.Config, tokens *token.Store, state *active.State) (plugin.Scheduled, error) {
	if !pub.HasScheduledBuilder(name) {
		if plugin.HasCapability(name, plugin.CapScheduled) {
			return nil, errs.Newf(errs.KindInternal, "signal %q advertises CapScheduled but has no scheduled builder", name)
		}
		return nil, ErrNoScheduled
	}
	if !plugin.HasCapability(name, plugin.CapScheduled) {
		return nil, errs.Newf(errs.KindConfig, "signal %q does not advertise CapScheduled", name)
	}
	if !plugin.SignalEnabled(name) {
		return nil, errs.Newf(errs.KindConfig, "signal %q is disabled", name).
			WithHint("enable with `mino plugins enable` for the backing plugin")
	}
	job, err := pub.BuildScheduled(name, newHostBuildCtx(name, params, role, cfg, tokens, state, nil))
	if err != nil {
		return nil, err
	}
	if isNilRef(job) {
		return nil, errs.Newf(errs.KindInternal, "builder for signal %q returned no scheduled job", name)
	}
	return job, nil
}

func ScheduledSignals() []string {
	var out []string
	for _, name := range pub.ScheduledSignals() {
		if plugin.HasCapability(name, plugin.CapScheduled) && plugin.SignalEnabled(name) {
			out = append(out, name)
		}
	}
	return out
}

func HasScheduledBuilder(signal string) bool {
	return pub.HasScheduledBuilder(signal)
}

func KnownSignals() map[string]bool {
	return plugin.KnownSignals()
}

func HasBuilder(signal string) bool {
	return plugin.HasBuilder(signal)
}

func HasActiveBuilder(signal string) bool {
	return plugin.HasStreamBuilder(signal)
}

func BuilderSignals() map[string]bool {
	return plugin.BuilderSignals()
}

func registerStockBuilders() {
	if _, ok := plugin.LookupBuilders("github"); ok {
		return
	}
	plugin.RegisterBuilders("github", plugin.Builders{
		Query: func(bc plugin.BuildContext) (plugin.Query, error) {
			h, ok := asHost(bc)
			if !ok {
				return nil, errs.New(errs.KindInternal, "github builder requires host build context")
			}
			return buildGithub(h.params, h.cfg, h.tokens, h.cache)
		},
		Stream: func(bc plugin.BuildContext) (plugin.Stream, error) {
			h, ok := asHost(bc)
			if !ok {
				return nil, errs.New(errs.KindInternal, "github stream builder requires host build context")
			}
			return buildActiveGithub(h.params, h.cfg, h.tokens, h.state)
		},
	})
}

func buildGithub(params map[string]string, cfg *config.Config, tokens *token.Store, results *cache.Store) (signals.Signal, error) {
	sel, err := githubAuth(cfg, tokens)
	if err != nil {
		return nil, err
	}
	backend, err := githubBackendFor(sel)
	if err != nil {
		return nil, err
	}
	var opts []gh.Option
	if viewer := strings.TrimSpace(cfg.GitHub.Viewer); viewer != "" {
		opts = append(opts, gh.WithViewer(viewer))
	} else {
		warnViewerlessQueries(sel, params, cfg)
	}
	if results != nil {
		opts = append(opts, gh.WithDetailCache(results,
			gh.CachePolicy{Read: results.Reads(), Write: results.Writes(), TTL: results.DetailTTL()}))
	}
	if title := params["title"]; title != "" {
		opts = append(opts, gh.WithTitle(title))
	}
	if ref := params["actions"]; ref != "" {
		repo, err := gh.ParseRepositoryRef(ref)
		if err != nil {
			return nil, err
		}
		actions, ok := backend.(gh.ActionsBackend)
		if !ok {
			return nil, errs.New(errs.KindInternal, "github backend does not support Actions")
		}
		return gh.NewActions(repo, actions, opts...), nil
	}
	if ref := params["project"]; ref != "" {
		owner, number, err := gh.ParseProjectRef(ref)
		if err != nil {
			return nil, err
		}
		spec := gh.ProjectSpec{
			Owner:  owner,
			Number: number,
			Filter: params["filter"],
			Title:  params["title"],
			Field:  params["field"],
			Team:   params["team"],
		}
		var roster gh.RosterCache
		if spec.Team != "" && results != nil {
			roster = results
		}
		return gh.NewProject(spec, backend, cfg.GitHub.Max, roster, opts...), nil
	}
	queries := cfg.GitHub.Queries
	if q := params["query"]; q != "" {
		queries = []string{q}
	}
	return gh.New(queries, backend, cfg.GitHub.Max, opts...), nil
}

func githubAuth(cfg *config.Config, tokens *token.Store) (auth.GitHubSelection, error) {
	base, err := gh.NormalizeAPIURL(cfg.GitHub.APIURL)
	if err != nil {
		return auth.GitHubSelection{}, err
	}
	sel, err := auth.SelectGitHub(cfg.GitHub.AuthSpec(base, tokens))
	if err != nil {
		return auth.GitHubSelection{}, err
	}
	log.Debugf("%s", sel.Trace())
	return sel, nil
}

func warnViewerlessQueries(sel auth.GitHubSelection, params map[string]string, cfg *config.Config) {
	if !sel.ServiceIdentity() {
		return
	}
	for _, q := range effectiveGithubQueries(params, cfg) {
		if strings.Contains(q, "@me") {
			log.Warnf("github: query %q uses @me, which resolves to nothing as %s; "+
				"set github.viewer to the login mino should stand in for, or rewrite the query "+
				"with author:<login> or org:<org>", q, sel.Origin)
		}
	}
	if f := params["filter"]; strings.Contains(f, "@me") {
		log.Warnf("github: project filter %q uses @me, which resolves to nothing as %s; "+
			"set github.viewer", f, sel.Origin)
	}
}

func effectiveGithubQueries(params map[string]string, cfg *config.Config) []string {
	if q := params["query"]; q != "" {
		return []string{q}
	}
	if len(cfg.GitHub.Queries) > 0 {
		return cfg.GitHub.Queries
	}
	return gh.DefaultQueries()
}

func githubBackend(cfg *config.Config, tokens *token.Store) (gh.Backend, error) {
	sel, err := githubAuth(cfg, tokens)
	if err != nil {
		return nil, err
	}
	return githubBackendFor(sel)
}

func githubBackendFor(sel auth.GitHubSelection) (gh.Backend, error) {
	if sel.UsesGHCLI() {
		return gh.CLIBackend{Hostname: auth.GHHostname(sel.APIURL)}, nil
	}
	if !sel.Authenticated() {
		return nil, errs.New(errs.KindAuth, "no GitHub authentication available").
			WithHint("configure github.app or github.service_token for a service identity, install the " +
				"gh CLI and run `gh auth login`, set GITHUB_TOKEN, or run `mino login github`")
	}
	log.Debugf("github: using %s via the REST API", sel.Origin)
	return gh.APIBackend{Auth: sel, BaseURL: sel.APIURL}, nil
}

func buildActiveGithub(params map[string]string, cfg *config.Config, tokens *token.Store, state *active.State) (signals.ActiveSignal, error) {
	sel, err := githubAuth(cfg, tokens)
	if err != nil {
		return nil, err
	}
	if !sel.Authenticated() {
		return nil, errs.New(errs.KindAuth, "github: realtime needs GitHub authentication").
			WithHint("configure github.service_token, set GITHUB_TOKEN, run `gh auth login`, or " +
				"run `mino login github`")
	}
	if sel.Mech == auth.GitHubAppAuth {
		return nil, errs.New(errs.KindConfig,
			"github: realtime notifications need a user token; a GitHub App installation token cannot read /notifications").
			WithHint("set github.service_token to a machine-user PAT alongside github.app, or drop github " +
				"from the flight")
	}
	interval, err := paramPollInterval(params, "github", 60*time.Second)
	if err != nil {
		return nil, err
	}
	return gh.NewActive(sel, sel.APIURL, interval, state), nil
}

func paramPollInterval(params map[string]string, signal string, def time.Duration) (time.Duration, error) {
	v := params["interval"]
	if v == "" {
		return def, nil
	}
	return signals.ParsePollInterval(signal+": query param interval", v)
}
