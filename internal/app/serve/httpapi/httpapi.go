// Package httpapi serves the loopback HTTP trigger API that mino serve can
// expose: flights, saved queries and plugin actions run over HTTP instead of the
// CLI, and the serve event stream is mirrored over SSE.
//
// It must not import internal/app/serve — serve imports this package — so the
// event encoder and every run path arrive through Deps.
package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/log"
	"github.com/codyconfer/mino/internal/signals"
)

const (
	// maxBodyBytes bounds a trigger request body; they only carry params.
	maxBodyBytes = 64 << 10
	// eventBuffer matches serve's socket subscriber buffer.
	eventBuffer = 256
	// maxSSEConns is deliberately below ipc's 64: stream.Subject fans out from a
	// single goroutine, so every subscriber costs a send on every event.
	maxSSEConns = 16
	// sseWriteGrace mirrors ipc's broadcastWriteTimeout.
	sseWriteGrace = 10 * time.Second
)

// sseHeartbeat is a var so tests can shrink it.
var sseHeartbeat = 20 * time.Second

// SourceInfo describes one realtime source serve has open.
type SourceInfo struct {
	Signal       string `json:"signal"`
	LatencyFloor string `json:"latency_floor"`
}

// ActionInfo describes one registered plugin action.
type ActionInfo struct {
	Signal      string `json:"signal"`
	Name        string `json:"name"`
	ServiceOnly bool   `json:"service_only"`
}

// Status is the /api/v1/status payload.
type Status struct {
	Flight       string       `json:"flight"`
	Role         string       `json:"role"`
	Home         string       `json:"home"`
	Interval     string       `json:"interval"`
	Socket       string       `json:"socket"`
	Sources      []SourceInfo `json:"sources"`
	SSEClients   int          `json:"sse_clients"`
	RunsInFlight int          `json:"runs_in_flight"`
	UptimeSec    int64        `json:"uptime_s"`
}

// Deps are the behaviours the API needs from serve. Injecting them keeps this
// package free of the serve import and trivially fakeable in tests.
type Deps struct {
	// RunFlight and RunQuery return sections plus a total-failure error.
	RunFlight func(ctx context.Context, name string) ([]signals.Section, error)
	RunQuery  func(ctx context.Context, name string) ([]signals.Section, error)
	RunAction func(ctx context.Context, signal, name string, params map[string]string) error

	// EmitJSON writes sections in the exact shape `-o json` produces.
	EmitJSON func(w io.Writer, root string, sections []signals.Section) error
	// Tally reports how many sections failed out of how many.
	Tally func(sections []signals.Section) (failed, total int)

	// FlightExists / QueryExists separate 404 from 403 and 400.
	FlightExists func(name string) bool
	QueryExists  func(name string) bool
	// FlightVisible / QueryVisible apply the serve role's scope.
	FlightVisible func(name string) bool
	QueryVisible  func(name string) bool

	Flights func(all bool) any
	Queries func(all bool) any
	Actions func(signal string) []ActionInfo
	Config  func() any
	Status  func() Status

	// ActionExists and SignalEnabled separate 404 from 409 before dispatch,
	// because build.Action reports a missing action as errs.KindSignal, which
	// would otherwise surface as a misleading 502.
	ActionExists  func(signal, name string) bool
	SignalEnabled func(signal string) bool

	// Subscribe returns an event channel and its unsubscribe func.
	Subscribe func(buffer int) (<-chan signals.Event, func())
	// Encode renders one event as single-line JSON for an SSE data frame.
	Encode func(signals.Event) ([]byte, error)

	// Timeout bounds a single action run.
	Timeout func() time.Duration

	Identity    map[string]IdentityProvider
	Sessions    SessionStore
	AuthBinding func() string
	AuditAuth   func(event string, attrs map[string]string)
}

type DeviceAuth struct {
	DeviceCode              string
	UserCode                string
	VerificationURI         string
	VerificationURIComplete string
	Interval                time.Duration
	ExpiresIn               time.Duration
}

type DeviceResult struct {
	Pending  bool
	SlowDown bool
	Denied   bool
	Expired  bool
	Login    string
	UserID   int64
	Kind     string
}

type IdentityProvider interface {
	Start(ctx context.Context) (DeviceAuth, error)
	Poll(ctx context.Context, deviceCode string) (DeviceResult, error)
}

// Config configures the API handler.
type Config struct {
	Token       string
	TokenSource string
	BindHost    string
	// MaxConcurrent bounds simultaneous flight/query/action runs.
	MaxConcurrent int
	AllowedLogins []string
	SessionTTL    time.Duration
}

// API serves the trigger endpoints.
type API struct {
	deps        Deps
	token       string
	tokenSource string
	hostGuard   bool

	identity      bool
	allowed       map[string]bool
	providerNames []string
	authHint      string
	sessions      *sessions

	pendingMu sync.Mutex
	pending   map[string]*pendingAuth
	rate      map[string]*rateEntry

	runs     chan struct{}
	sseSlot  chan struct{}
	authSlot chan struct{}
	started  time.Time
}

// New returns an API ready to serve.
func New(cfg Config, d Deps) *API {
	limit := cfg.MaxConcurrent
	if limit <= 0 {
		limit = 1
	}
	ttl := cfg.SessionTTL
	if ttl <= 0 {
		ttl = 12 * time.Hour
	}
	a := &API{
		deps:        d,
		token:       cfg.Token,
		tokenSource: cfg.TokenSource,
		hostGuard:   LoopbackBind(cfg.BindHost),
		allowed:     map[string]bool{},
		pending:     map[string]*pendingAuth{},
		rate:        map[string]*rateEntry{},
		runs:        make(chan struct{}, limit),
		sseSlot:     make(chan struct{}, maxSSEConns),
		authSlot:    make(chan struct{}, maxAuthOutbound),
		started:     time.Now(),
	}
	a.identity = len(d.Identity) > 0 && d.Sessions != nil
	for _, l := range cfg.AllowedLogins {
		if v := strings.ToLower(strings.TrimSpace(l)); v != "" {
			a.allowed[v] = true
		}
	}
	for name := range d.Identity {
		a.providerNames = append(a.providerNames, name)
	}
	sort.Strings(a.providerNames)
	a.sessions = newSessions(d.Sessions, ttl)
	a.authHint = authHint(cfg.Token != "", a.identity, cfg.TokenSource, a.providerNames, a.hostGuard)
	if a.identity {
		a.sessions.load(context.Background(), a.binding())
	}
	return a
}

func authHint(hasToken, identity bool, tokenSource string, providers []string, loopback bool) string {
	tokenPart := "pass the API bearer token"
	if hasToken && loopback && tokenSource != "" {
		tokenPart = "pass the token from " + tokenSource
	}
	if !identity {
		return tokenPart
	}
	signIn := "POST /api/v1/auth/device/" + providers[0] + " to sign in"
	if !hasToken {
		return signIn
	}
	return tokenPart + ", or " + signIn
}

// Handler returns the routed handler.
func (a *API) Handler() http.Handler {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	r.HandleMethodNotAllowed = true
	r.RedirectTrailingSlash = false
	r.RedirectFixedPath = false
	r.UseRawPath = true
	r.UnescapePathValues = true
	_ = r.SetTrustedProxies(nil)

	r.Use(a.recover500())
	r.NoRoute(a.notFound)
	r.NoMethod(a.notAllowed)

	r.GET("/healthz", a.healthz)

	if a.identity {
		login := r.Group("/api/v1/auth", a.hostGuardOnly())
		login.POST("/device/:provider", a.authDevice)
		login.POST("/device/:provider/token", a.authToken)
	}

	v1 := r.Group("/api/v1", a.authRequired())
	v1.GET("/status", a.status)
	v1.GET("/list", a.list)
	v1.GET("/config", a.config)
	v1.GET("/actions", a.listActions)
	v1.GET("/actions/:signal", a.listActions)
	v1.GET("/events", a.events)
	v1.POST("/flights/:name", a.runFlight)
	v1.POST("/queries/:name", a.runQuery)
	v1.POST("/actions/:signal/:name", a.runAction)
	if a.identity {
		v1.GET("/auth/session", a.authSession)
		v1.DELETE("/auth/session", a.authLogout)
	}
	return r
}

func (a *API) notFound(c *gin.Context) {
	abortErrStatus(c, http.StatusNotFound, errs.KindUsage, "no such endpoint",
		"the trigger routes live under /api/v1; GET /healthz needs no token")
}

func (a *API) notAllowed(c *gin.Context) {
	abortErrStatus(c, http.StatusMethodNotAllowed, errs.KindUsage,
		"that method is not allowed here", "the run endpoints are POST, the read endpoints GET")
}

func (a *API) recover500() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			v := recover()
			if v == nil {
				return
			}
			if err, ok := v.(error); ok && errors.Is(err, http.ErrAbortHandler) {
				panic(v)
			}
			log.Warnf("serve: http api: panic on %s: %v\n%s", c.FullPath(), v, debug.Stack())
			if c.Writer.Written() {
				c.Abort()
				return
			}
			abortErrStatus(c, http.StatusInternalServerError, errs.KindInternal,
				"the request failed", "the serve log has the detail")
		}()
		c.Next()
	}
}

// Uptime reports how long the API has been up.
func (a *API) Uptime() time.Duration { return time.Since(a.started) }

// SSEClients reports how many SSE connections are open.
func (a *API) SSEClients() int { return len(a.sseSlot) }

// RunsInFlight reports how many runs are executing.
func (a *API) RunsInFlight() int { return len(a.runs) }
