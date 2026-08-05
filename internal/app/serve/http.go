package serve

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/codyconfer/sisyphus/kv"
	"github.com/codyconfer/sisyphus/stream"

	"github.com/codyconfer/mino/internal/app/flight"
	"github.com/codyconfer/mino/internal/app/run"
	"github.com/codyconfer/mino/internal/app/serve/httpapi"
	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/log"
	"github.com/codyconfer/mino/internal/plugin"
	"github.com/codyconfer/mino/internal/signals"
	"github.com/codyconfer/mino/internal/signals/build"
)

// apiShutdownGrace is a var so tests can shrink it.
var apiShutdownGrace = 5 * time.Second

const apiReadHeaderTimeout = 10 * time.Second

// apiListener binds the API address, returning nil when the API is off.
//
// Unlike socket(), a taken port is a hard error: the API is explicitly opted
// into, the port may belong to an unrelated program, and there is no
// attach-to-the-existing-one story. Silently not listening would leave the
// caller staring at "connection refused" with nothing to explain it.
func (s *Server) apiListener(opt RunOptions) (net.Listener, error) {
	if !opt.HTTP {
		return nil, nil
	}
	addr := net.JoinHostPort(apiHost(opt), strconv.Itoa(opt.HTTPPort))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, errs.Wrapf(errs.KindConfig, err, "binding the http api on %s", addr).
			WithHint("something else is using port %d, or %s is not an address this machine can bind; "+
				"pass --http-port / --http-host, or set daemon.http.port / daemon.http.host",
				opt.HTTPPort, apiHost(opt))
	}
	return ln, nil
}

func apiHost(opt RunOptions) string {
	if h := strings.TrimSpace(opt.HTTPHost); h != "" {
		return h
	}
	return config.HTTPLoopback
}

// httpAPI serves the trigger API over ln; the returned func shuts it down.
func (s *Server) httpAPI(ctx context.Context, ln net.Listener, subj *stream.Subject[signals.Event], kvStore *kv.Store, opt RunOptions, srcs []httpapi.SourceInfo) func() {
	if ln == nil {
		return func() {}
	}
	api := httpapi.New(httpapi.Config{
		Token:         opt.HTTPToken,
		TokenSource:   opt.HTTPTokenSource,
		BindHost:      apiHost(opt),
		MaxConcurrent: s.apiMaxConcurrent(),
		AllowedLogins: opt.HTTPIdentity.AllowedLogins,
		SessionTTL:    opt.HTTPIdentity.SessionTTL,
	}, s.apiDeps(subj, kvStore, opt, srcs))

	srv := &http.Server{
		Handler:           api.Handler(),
		ReadHeaderTimeout: apiReadHeaderTimeout,
		IdleTimeout:       2 * time.Minute,
		// WriteTimeout stays zero on purpose: it would guillotine SSE streams
		// mid-flight. Per-write deadlines bound a stalled peer instead.
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Warnf("serve: http api: %v", err)
		}
	}()
	// Straight to stderr, like the "watching <signal>" lines: the default log
	// level is warn, so log.Infof here would hide the one place the user learns
	// the port and where to read the token from. Never print the token itself —
	// it would land in the log dir and in tmux scrollback.
	fmt.Fprintf(os.Stderr, "http api  http://%s (token: %s)\n", ln.Addr(), opt.HTTPTokenSource)
	if opt.HTTPIdentity.Enabled {
		if kvStore == nil {
			fmt.Fprintf(os.Stderr, "http api  WARNING %s sign-in is configured but the session store at %s "+
				"could not be opened, so every sign-in will fail\n",
				opt.HTTPIdentity.Provider, config.DataPath(s.Cfg.Home, config.ServeDB))
		} else {
			fmt.Fprintf(os.Stderr, "http api  %s sign-in: %d allowed login(s), sessions expire after %s\n",
				opt.HTTPIdentity.Provider, len(opt.HTTPIdentity.AllowedLogins), opt.HTTPIdentity.SessionTTL)
		}
	}
	if !httpapi.LoopbackBind(apiHost(opt)) {
		guard := "the bearer token is the only guard"
		if opt.HTTPIdentity.Enabled {
			guard = "the bearer token and an allow-listed " + opt.HTTPIdentity.Provider +
				" sign-in are the only guards, and both cross the network in cleartext"
		}
		fmt.Fprintf(os.Stderr, "http api  WARNING bound to %s: flight, query and action triggers are "+
			"reachable beyond this machine and %s\n", apiHost(opt), guard)
	}

	return func() {
		// context.Background, not ctx: ctx is already cancelled by the time this
		// runs, which would give Shutdown zero grace.
		sctx, cancel := context.WithTimeout(context.Background(), apiShutdownGrace)
		defer cancel()
		if err := srv.Shutdown(sctx); err != nil {
			log.Debugf("serve: http api shutdown timed out; closing: %v", err)
			_ = srv.Close()
		}
	}
}

func (s *Server) apiMaxConcurrent() int {
	if n := s.Cfg.Daemon.HTTP.MaxConcurrent; n > 0 {
		return n
	}
	return config.DefaultHTTPMaxConcurrent
}

// apiDeps wires the API to this server. Handlers must use these accessors rather
// than reading s.Directives or s.Cfg.Role directly: Dirs() and Role() take the
// App read lock, so a concurrent RefreshDirectives is safe only through them.
func (s *Server) apiDeps(subj *stream.Subject[signals.Event], kvStore *kv.Store, opt RunOptions, srcs []httpapi.SourceInfo) httpapi.Deps {
	return httpapi.Deps{
		RunFlight: func(ctx context.Context, name string) ([]signals.Section, error) {
			return run.Flight(ctx, s.App, name)
		},
		RunQuery: func(ctx context.Context, name string) ([]signals.Section, error) {
			return run.Query(ctx, s.App, name)
		},
		RunAction: func(ctx context.Context, signal, name string, params map[string]string) error {
			return run.Action(ctx, s.App, signal, name, params)
		},
		EmitJSON: func(w io.Writer, root string, sections []signals.Section) error {
			// "json" not s.Cfg.Output: the API is JSON regardless of how serve
			// was told to render to its terminal.
			return flight.Emit(w, "json", root, sections)
		},
		Tally: func(sections []signals.Section) (int, int) {
			o := flight.Tally(sections)
			return o.Failed, o.Sections
		},
		FlightExists: func(name string) bool {
			_, ok := s.Dirs().Flights[name]
			return ok
		},
		QueryExists: func(name string) bool {
			_, ok := s.Dirs().Queries[name]
			return ok
		},
		FlightVisible: func(name string) bool { return s.Access().FlightVisible(name) },
		QueryVisible:  func(name string) bool { return s.Access().QueryVisible(name) },
		Flights:       func(all bool) any { return s.apiFlights(all) },
		Queries:       func(all bool) any { return s.apiQueries(all) },
		Actions:       apiActions,
		Config:        s.apiConfig,
		Status: func() httpapi.Status {
			return httpapi.Status{
				Flight:   opt.Flight,
				Role:     s.Role(),
				Home:     s.Cfg.Home,
				Interval: opt.Interval.String(),
				Socket:   s.SocketPath(),
				Sources:  srcs,
			}
		},
		ActionExists: func(signal, name string) bool {
			_ = build.KnownSignals()
			_, ok := plugin.LookupAction(signal, name)
			return ok
		},
		SignalEnabled: plugin.SignalEnabled,
		Subscribe: func(buffer int) (<-chan signals.Event, func()) {
			ch := subj.Subscribe(buffer)
			return ch, func() { subj.Unsubscribe(ch) }
		},
		Encode:  Encode,
		Timeout: s.SourceTimeout,

		Identity:    apiIdentityProviders(opt.HTTPIdentity),
		Sessions:    newSessionStore(kvStore),
		AuthBinding: func() string { return opt.HTTPIdentity.binding(s.Cfg.Home) },
		AuditAuth:   s.apiAuditAuth,
	}
}

func (s *Server) apiAuditAuth(event string, attrs map[string]string) {
	s.Audit.RecordAuth(event, s.Role(), attrs)
}

// apiConfig returns the active config with every secret redacted.
func (s *Server) apiConfig() any {
	c := *s.Cfg
	if c.Daemon.HTTP.Token != "" {
		c.Daemon.HTTP.Token = "<set>"
	}
	if c.GitHub.ServiceToken != "" {
		c.GitHub.ServiceToken = "<set>"
	}
	return c
}

type apiFlight struct {
	Name      string   `json:"name"`
	Queries   []string `json:"queries"`
	Formatter string   `json:"formatter,omitempty"`
}

func (s *Server) apiFlights(all bool) []apiFlight {
	d := s.Dirs()
	names := s.VisibleFlights()
	if all {
		names = d.FlightNames()
	}
	out := make([]apiFlight, 0, len(names))
	for _, n := range names {
		f := d.Flights[n]
		out = append(out, apiFlight{Name: n, Queries: f.Queries, Formatter: f.Formatter})
	}
	return out
}

type apiQuery struct {
	Name     string `json:"name"`
	Title    string `json:"title,omitempty"`
	Signal   string `json:"signal,omitempty"`
	Runnable bool   `json:"runnable"`
}

func (s *Server) apiQueries(all bool) []apiQuery {
	d := s.Dirs()
	names := s.VisibleQueries()
	if all {
		names = d.QueryNames()
	}
	out := make([]apiQuery, 0, len(names))
	for _, n := range names {
		q := d.Queries[n]
		out = append(out, apiQuery{Name: n, Title: q.Display(), Signal: q.Signal, Runnable: q.Runnable()})
	}
	return out
}

// apiActions lists registered actions, for one signal or all of them.
func apiActions(signal string) []httpapi.ActionInfo {
	// Forces builder registration; without it the list comes back empty.
	_ = build.KnownSignals()
	var out []httpapi.ActionInfo
	add := func(sig string) {
		for _, a := range build.Actions(sig) {
			out = append(out, httpapi.ActionInfo{
				Signal:      a.Signal,
				Name:        a.Name,
				ServiceOnly: !plugin.ActionUIVisible(a.Signal, a.Name),
			})
		}
	}
	if signal != "" {
		add(signal)
		return out
	}
	seen := map[string]bool{}
	for _, d := range plugin.All() {
		if d.Kind != plugin.KindSignal || d.Signal == "" || seen[d.Signal] {
			continue
		}
		seen[d.Signal] = true
		if !plugin.HasCapability(d.Signal, plugin.CapAction) {
			continue
		}
		add(d.Signal)
	}
	return out
}
