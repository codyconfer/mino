package build

import (
	"strconv"
	"strings"
	"time"

	"github.com/codyconfer/munin/internal/auth"
	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/log"
	"github.com/codyconfer/munin/internal/plugin"
	"github.com/codyconfer/munin/internal/signals"
	"github.com/codyconfer/munin/internal/signals/active"
	"github.com/codyconfer/munin/internal/signals/cache"
	"github.com/codyconfer/munin/internal/signals/demo"
	"github.com/codyconfer/munin/internal/signals/gcal"
	"github.com/codyconfer/munin/internal/signals/gdocs"
	"github.com/codyconfer/munin/internal/signals/gdrive"
	gh "github.com/codyconfer/munin/internal/signals/github"
	gmailsrc "github.com/codyconfer/munin/internal/signals/gmail"
	"github.com/codyconfer/munin/internal/signals/gtasks"
	slacksrc "github.com/codyconfer/munin/internal/signals/slack"
	"github.com/codyconfer/munin/internal/token"
	pub "github.com/codyconfer/munin/plugin"

	_ "github.com/codyconfer/munin/internal/plugin/ntr"
)

func init() {
	plugin.RegisterBuiltins()
	registerStockBuilders()
}

var ErrNoActive = errs.New(errs.KindUsage, "signal has no active (streaming) implementation")

var ErrNoScheduled = errs.New(errs.KindUsage, "signal has no scheduled implementation")

func GoogleAuth(cfg *config.Config, tokens *token.Store) auth.GoogleAuth {
	return auth.GoogleAuth{
		Store:        tokens,
		ClientID:     cfg.Google.OAuthClientID,
		ClientSecret: cfg.Google.OAuthClientSecret,
	}
}

func ResolveWriteTarget(what, setting, configured, requested string) (string, error) {
	if configured == "" {
		return "", errs.Newf(errs.KindConfig, "no writable %s configured", what).WithHint("set `%s` in config", setting)
	}
	if requested == "" || strings.EqualFold(requested, configured) {
		return configured, nil
	}
	return "", errs.Newf(errs.KindUsage, "%s %q is read-only; only %q is writable (%s)", what, requested, configured, setting)
}

func Signal(name string, params map[string]string, cfg *config.Config, tokens *token.Store, results *cache.Store) (signals.Signal, error) {
	if !plugin.HasBuilder(name) {
		return nil, errs.Newf(errs.KindConfig, "unknown signal %q", name)
	}
	if !plugin.SignalEnabled(name) {
		return nil, errs.Newf(errs.KindConfig, "signal %q is disabled", name).
			WithHint("enable with `munin plugins enable` for the backing plugin")
	}
	q, err := plugin.BuildQuery(name, hostBuildCtx{signal: name, params: params, cfg: cfg, tokens: tokens, cache: results})
	if err != nil {
		return nil, err
	}
	if isNilRef(q) {
		return nil, errs.Newf(errs.KindInternal, "builder for signal %q returned no query", name)
	}
	return results.Wrap(q, name, cfg.Role, params), nil
}

func ActiveSignal(name string, params map[string]string, cfg *config.Config, tokens *token.Store, state *active.State) (signals.ActiveSignal, error) {
	if !plugin.HasBuilder(name) && !plugin.HasStreamBuilder(name) {
		return nil, errs.Newf(errs.KindConfig, "unknown signal %q", name)
	}
	if !plugin.SignalEnabled(name) {
		return nil, errs.Newf(errs.KindConfig, "signal %q is disabled", name).
			WithHint("enable with `munin plugins enable` for the backing plugin")
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
	src, err := plugin.BuildStream(name, hostBuildCtx{signal: name, params: params, cfg: cfg, tokens: tokens, state: state})
	if err != nil {
		return nil, err
	}
	if isNilRef(src) {
		return nil, errs.Newf(errs.KindInternal, "builder for signal %q returned no stream", name)
	}
	return src, nil
}

func ScheduledJob(name string, params map[string]string, cfg *config.Config, tokens *token.Store, state *active.State) (plugin.Scheduled, error) {
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
			WithHint("enable with `munin plugins enable` for the backing plugin")
	}
	job, err := pub.BuildScheduled(name, hostBuildCtx{signal: name, params: params, cfg: cfg, tokens: tokens, state: state})
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
	register := func(signal string, query plugin.QueryFunc, stream plugin.StreamFunc) {
		if _, ok := plugin.LookupBuilders(signal); ok {
			return
		}
		plugin.RegisterBuilders(signal, plugin.Builders{Query: query, Stream: stream})
	}

	register("demo",
		func(bc plugin.BuildContext) (plugin.Query, error) {
			return demo.Signal{}, nil
		},
		func(bc plugin.BuildContext) (plugin.Stream, error) {
			return demo.Signal{}, nil
		},
	)
	register("github",
		func(bc plugin.BuildContext) (plugin.Query, error) {
			h, ok := asHost(bc)
			if !ok {
				return nil, errs.New(errs.KindInternal, "github builder requires host build context")
			}
			return buildGithub(h.params, h.cfg, h.tokens, h.cache)
		},
		func(bc plugin.BuildContext) (plugin.Stream, error) {
			h, ok := asHost(bc)
			if !ok {
				return nil, errs.New(errs.KindInternal, "github stream builder requires host build context")
			}
			return buildActiveGithub(h.params, h.cfg, h.tokens, h.state)
		},
	)
	register("calendar",
		func(bc plugin.BuildContext) (plugin.Query, error) {
			h, ok := asHost(bc)
			if !ok {
				return nil, errs.New(errs.KindInternal, "calendar builder requires host build context")
			}
			return buildCalendar(h.params, h.cfg, h.tokens)
		},
		func(bc plugin.BuildContext) (plugin.Stream, error) {
			h, ok := asHost(bc)
			if !ok {
				return nil, errs.New(errs.KindInternal, "calendar stream builder requires host build context")
			}
			return buildActiveCalendar(h.params, h.cfg, h.tokens, h.state)
		},
	)
	register("gmail",
		func(bc plugin.BuildContext) (plugin.Query, error) {
			h, ok := asHost(bc)
			if !ok {
				return nil, errs.New(errs.KindInternal, "gmail builder requires host build context")
			}
			return buildGmail(h.params, h.cfg, h.tokens)
		},
		nil,
	)
	register("docs",
		func(bc plugin.BuildContext) (plugin.Query, error) {
			h, ok := asHost(bc)
			if !ok {
				return nil, errs.New(errs.KindInternal, "docs builder requires host build context")
			}
			return buildDocs(h.params, h.cfg, h.tokens)
		},
		nil,
	)
	register("drive",
		func(bc plugin.BuildContext) (plugin.Query, error) {
			h, ok := asHost(bc)
			if !ok {
				return nil, errs.New(errs.KindInternal, "drive builder requires host build context")
			}
			return buildDrive(h.params, h.cfg, h.tokens)
		},
		nil,
	)
	register("tasks",
		func(bc plugin.BuildContext) (plugin.Query, error) {
			h, ok := asHost(bc)
			if !ok {
				return nil, errs.New(errs.KindInternal, "tasks builder requires host build context")
			}
			return buildTasks(h.params, h.cfg, h.tokens)
		},
		func(bc plugin.BuildContext) (plugin.Stream, error) {
			h, ok := asHost(bc)
			if !ok {
				return nil, errs.New(errs.KindInternal, "tasks stream builder requires host build context")
			}
			return buildActiveTasks(h.params, h.cfg, h.tokens, h.state)
		},
	)
	register("slack",
		func(bc plugin.BuildContext) (plugin.Query, error) {
			h, ok := asHost(bc)
			if !ok {
				return nil, errs.New(errs.KindInternal, "slack builder requires host build context")
			}
			return buildSlack(h.params, h.cfg, h.tokens)
		},
		func(bc plugin.BuildContext) (plugin.Stream, error) {
			h, ok := asHost(bc)
			if !ok {
				return nil, errs.New(errs.KindInternal, "slack stream builder requires host build context")
			}
			return buildActiveSlack(h.params, h.cfg, h.tokens, h.state)
		},
	)
}

func buildGithub(params map[string]string, cfg *config.Config, tokens *token.Store, results *cache.Store) (signals.Signal, error) {
	backend, err := githubBackend(cfg, tokens)
	if err != nil {
		return nil, err
	}
	var opts []gh.Option
	if results != nil {
		opts = append(opts, gh.WithDetailCache(results,
			gh.CachePolicy{Read: results.Reads(), Write: results.Writes(), TTL: results.DetailTTL()}))
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

func githubBackend(cfg *config.Config, tokens *token.Store) (gh.Backend, error) {
	if auth.GHAvailable() {
		return gh.CLIBackend{}, nil
	}
	if tok, origin := auth.GitHubToken(tokens); tok != "" {
		base, err := gh.NormalizeAPIURL(cfg.GitHub.APIURL)
		if err != nil {
			return nil, err
		}
		log.Debugf("github: gh CLI not found; using %s via the REST API", origin)
		return gh.APIBackend{Token: tok, BaseURL: base}, nil
	}
	return nil, errs.New(errs.KindAuth, "no GitHub authentication available").WithHint("install the gh CLI and run `gh auth login`, set GITHUB_TOKEN, or run `munin login github`")
}

func buildActiveGithub(params map[string]string, cfg *config.Config, tokens *token.Store, state *active.State) (signals.ActiveSignal, error) {
	tok, _ := auth.GitHubToken(tokens)
	if tok == "" {
		return nil, ErrNoActive
	}
	base, err := gh.NormalizeAPIURL(cfg.GitHub.APIURL)
	if err != nil {
		return nil, err
	}
	interval, err := paramPollInterval(params, "github", 60*time.Second)
	if err != nil {
		return nil, err
	}
	return gh.NewActive(tok, base, interval, state), nil
}

func buildCalendar(params map[string]string, cfg *config.Config, tokens *token.Store) (signals.Signal, error) {
	calID := paramStr(params, "calendar_id", cfg.Cal.CalendarID)
	window := paramDuration(params, "window", parseWindow(cfg.Cal.Window, 24*time.Hour))
	max := paramInt(params, "max", cfg.Cal.Max)
	return gcal.New(calID, window, max, GoogleAuth(cfg, tokens)), nil
}

func buildActiveCalendar(params map[string]string, cfg *config.Config, tokens *token.Store, state *active.State) (signals.ActiveSignal, error) {
	calID := paramStr(params, "calendar_id", cfg.Cal.CalendarID)
	interval, err := paramPollInterval(params, "calendar", 60*time.Second)
	if err != nil {
		return nil, err
	}
	return gcal.NewActive(calID, GoogleAuth(cfg, tokens), interval, state), nil
}

func buildGmail(params map[string]string, cfg *config.Config, tokens *token.Store) (signals.Signal, error) {
	query := paramStr(params, "query", cfg.Gmail.Query)
	max := paramInt(params, "max", cfg.Gmail.Max)
	return gmailsrc.New(query, max, GoogleAuth(cfg, tokens)), nil
}

func buildDocs(params map[string]string, cfg *config.Config, tokens *token.Store) (signals.Signal, error) {
	recent := paramInt(params, "recent", cfg.Docs.Recent)
	return gdocs.New(recent, GoogleAuth(cfg, tokens)), nil
}

func buildDrive(_ map[string]string, cfg *config.Config, tokens *token.Store) (signals.Signal, error) {
	return gdrive.New(GoogleAuth(cfg, tokens), cfg.Drive.Folders, cfg.Drive.Recent), nil
}

func buildTasks(_ map[string]string, cfg *config.Config, tokens *token.Store) (signals.Signal, error) {
	return gtasks.New(GoogleAuth(cfg, tokens), cfg.Tasks.Lists, cfg.Tasks.ShowCompleted, cfg.Tasks.Max), nil
}

func buildActiveTasks(params map[string]string, cfg *config.Config, tokens *token.Store, state *active.State) (signals.ActiveSignal, error) {
	interval, err := paramPollInterval(params, "tasks", 60*time.Second)
	if err != nil {
		return nil, err
	}
	return gtasks.NewActive(GoogleAuth(cfg, tokens), cfg.Tasks.Lists, cfg.Tasks.ShowCompleted, interval, state), nil
}

func buildSlack(params map[string]string, cfg *config.Config, tokens *token.Store) (signals.Signal, error) {
	channel := params["channel"]
	if channel == "" {
		return nil, errs.New(errs.KindUsage, "slack: a channel is required").WithHint("use --channel or a query param")
	}
	token, err := auth.SlackToken(tokens, cfg.Slack.TokenEnv)
	if err != nil {
		return nil, errs.Wrapf(errs.KindAuth, err, "slack authentication").WithHint("set %s", cfg.Slack.TokenEnv)
	}
	limit := paramInt(params, "limit", cfg.Slack.Limit)
	return slacksrc.New(token, channel, limit), nil
}

func buildActiveSlack(_ map[string]string, cfg *config.Config, tokens *token.Store, _ *active.State) (signals.ActiveSignal, error) {
	appToken, err := auth.SlackAppToken(tokens, cfg.Slack.AppTokenEnv)
	if err != nil {
		return nil, ErrNoActive
	}
	botToken, err := auth.SlackBotToken(tokens, cfg.Slack.BotTokenEnv)
	if err != nil {
		return nil, ErrNoActive
	}
	if appToken == "" || botToken == "" {
		return nil, ErrNoActive
	}
	return slacksrc.NewActive(botToken, appToken), nil
}

func parseWindow(s string, def time.Duration) time.Duration {
	if s == "" {
		return def
	}
	if d, err := time.ParseDuration(s); err == nil {
		return d
	}
	return def
}

func paramStr(params map[string]string, key, def string) string {
	if v := params[key]; v != "" {
		return v
	}
	return def
}

func paramInt(params map[string]string, key string, def int) int {
	if v := params[key]; v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func paramDuration(params map[string]string, key string, def time.Duration) time.Duration {
	if v := params[key]; v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func paramPollInterval(params map[string]string, signal string, def time.Duration) (time.Duration, error) {
	v := params["interval"]
	if v == "" {
		return def, nil
	}
	return signals.ParsePollInterval(signal+": query param interval", v)
}
