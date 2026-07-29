package app

import (
	"context"
	"io"
	"os"
	"sync"
	"time"

	"github.com/codyconfer/sisyphus"

	"github.com/codyconfer/munin/internal/audit"
	"github.com/codyconfer/munin/internal/auth"
	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/log"
	"github.com/codyconfer/munin/internal/plugin"
	"github.com/codyconfer/munin/internal/role"
	"github.com/codyconfer/munin/internal/signals/cache"
	"github.com/codyconfer/munin/internal/token"
)

type App struct {
	Cfg        *config.Config
	Directives *config.Directives
	Audit      *audit.Store
	Tokens     *token.Store
	Cache      *cache.Store
	Mgr        *sisyphus.Manager

	ghAuth       ghAuthCache
	roleDebounce roleDebounce
}

type ghAuthCache struct {
	mu      sync.Mutex
	checked bool
	ok      bool
}

func (a *App) GitHubAuthed() bool {
	a.ghAuth.mu.Lock()
	defer a.ghAuth.mu.Unlock()
	if !a.ghAuth.checked {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		a.ghAuth.ok, _ = auth.GitHubAuthStatus(ctx, a.Tokens)
		a.ghAuth.checked = true
	}
	return a.ghAuth.ok
}

func (a *App) ResetGitHubAuth() {
	a.ghAuth.mu.Lock()
	a.ghAuth.checked = false
	a.ghAuth.mu.Unlock()
}

type Options struct {
	Home        string
	ConfigFile  string
	Output      string
	Role        string
	Timeout     string
	CacheTTL    string
	NoCache     bool
	Refresh     bool
	Reconcile   config.ReconcilePolicy
	Verbose     bool
	Interactive bool
	In          io.Reader
	Out         io.Writer
}

func Load(opts Options) (*App, error) {
	log.SetVerbose(opts.Verbose)
	if home, err := config.Home(opts.Home); err == nil {
		_ = os.Chmod(home, 0o700)
	}
	cfg, directives, mgr, err := config.LoadConfigAndDirectives(opts.Home, opts.ConfigFile, opts.Reconcile, opts.Interactive, opts.In, opts.Out)
	if err != nil {
		return nil, err
	}
	if opts.Output != "" {
		cfg.Output = opts.Output
	}
	if opts.Role != "" {
		cfg.Role = opts.Role
	}
	if opts.Timeout != "" {
		cfg.Timeout = opts.Timeout
	}
	if opts.CacheTTL != "" {
		if _, err := time.ParseDuration(opts.CacheTTL); err != nil {
			return nil, errs.Newf(errs.KindUsage, "--cache-ttl %q is not a valid duration", opts.CacheTTL).
				WithHint("use a Go duration like 60s, 5m, or 0 to disable")
		}
		// An explicit flag beats the config file outright, per-signal entries included —
		// otherwise `--cache-ttl 0` would still cache anything listed under cache.signals.
		cfg.Cache.TTL, cfg.Cache.Signals = opts.CacheTTL, nil
		cfg.Cache.DetailTTL = opts.CacheTTL
	}
	a := &App{Cfg: cfg, Directives: directives, Mgr: mgr}
	a.openCache(cacheMode(opts))
	a.openTokens()
	a.openAudit()
	a.syncRoleLifecycle()
	return a, nil
}

func (a *App) ActivateRole(name string) error {
	if a == nil || a.Cfg == nil {
		return errs.New(errs.KindInternal, "app not loaded")
	}
	if name == a.Cfg.Role {
		return nil
	}
	a.invalidateRoleDebounce()
	a.Cfg.Role = name
	a.syncRoleLifecycle()
	return nil
}

func (a *App) syncRoleLifecycle() {
	if a == nil || a.Cfg == nil {
		return
	}
	home := a.Cfg.Home
	prev := role.LoadActive(home)
	next := a.Cfg.Role
	if prev != next {
		a.runRoleExit(prev)
		a.runRoleEnter(next)
		if err := role.SaveActive(home, next); err != nil {
			log.Warnf("role state: %v", err)
		}
	} else if next != "" {
		a.refreshRoleStatus(next)
	} else {
		role.ClearStatusChips()
	}
	a.applyRoleContexts()
}

func (a *App) runRoleExit(name string) {
	role.ClearStatusChips()
	if name == "" || a.Directives == nil {
		return
	}
	rd, ok := a.Directives.Roles[name]
	if !ok {
		log.Warnf("role exit: %q not defined; skipping hooks", name)
		return
	}
	if err := role.RunExit(rd); err != nil {
		log.Warnf("role %q exit hooks: %v", name, err)
	}
}

func (a *App) runRoleEnter(name string) {
	if name == "" || a.Directives == nil {
		role.ClearStatusChips()
		return
	}
	rd, ok := a.Directives.Roles[name]
	if !ok {
		log.Warnf("role enter: %q not defined; skipping hooks", name)
		role.ClearStatusChips()
		return
	}
	if err := role.RunEnter(rd); err != nil {
		log.Warnf("role %q enter hooks: %v", name, err)
	}
	a.applyRoleStatus(rd)
}

func (a *App) refreshRoleStatus(name string) {
	if name == "" || a.Directives == nil {
		role.ClearStatusChips()
		return
	}
	rd, ok := a.Directives.Roles[name]
	if !ok {
		role.ClearStatusChips()
		return
	}
	a.applyRoleStatus(rd)
}

func (a *App) applyRoleStatus(rd config.RoleDef) {
	chips, warnings := role.CollectStatus(rd)
	for _, w := range warnings {
		log.Warnf("role %q status: %v", rd.Name, w)
	}
	role.SetStatusChips(chips)
}

func (a *App) applyRoleContexts() {
	if a == nil || a.Cfg.Role == "" || a.Directives == nil {
		return
	}
	rd, ok := a.Directives.Roles[a.Cfg.Role]
	if !ok || len(rd.Contexts) == 0 {
		return
	}
	if err := plugin.ApplyRoleContexts(context.Background(), rd.Contexts); err != nil {
		log.Warnf("role contexts: %v", err)
	}
}

func (a *App) ReloadDirectives() error {
	return a.RefreshDirectives(config.ReconcileApply)
}

func (a *App) RefreshDirectives(policy config.ReconcilePolicy) error {
	if a == nil || a.Cfg == nil {
		return errs.New(errs.KindInternal, "app not loaded")
	}
	d, err := config.ReloadDirectives(a.Mgr, a.Cfg.Home, policy)
	if err != nil {
		return err
	}
	a.Directives = d
	a.applyRoleContexts()
	return nil
}

const DefaultSignalTimeout = 30 * time.Second

func (a *App) SourceTimeout() time.Duration {
	if a != nil && a.Cfg != nil && a.Cfg.Timeout != "" {
		if d, err := time.ParseDuration(a.Cfg.Timeout); err == nil && d > 0 {
			return d
		}
	}
	return DefaultSignalTimeout
}

func cacheMode(opts Options) cache.Mode {
	switch {
	case opts.NoCache:
		return cache.ModeOff
	case opts.Refresh:
		return cache.ModeRefresh
	default:
		return cache.ModeUse
	}
}

func (a *App) openCache(mode cache.Mode) {
	cc := a.Cfg.Cache
	cc.DetailTTL = config.ResolveDetailTTL(cc.DetailTTL, config.LoadGlobalSettings().DetailCacheTTL)
	a.Cache = cache.New(a.Cfg.Home, cc, cache.Fingerprint(a.Cfg), mode)
}

func (a *App) openTokens() {
	ts, err := token.Open(context.Background(), config.DataPath(a.Cfg.Home, config.TokensDB))
	if err != nil {
		log.Debugf("token store unavailable: %v", err)
		return
	}
	a.Tokens = ts
}

func (a *App) openAudit() {
	if !a.Cfg.Audit.Enabled {
		return
	}
	path := a.Cfg.Audit.Path
	if path == "" {
		path = config.DataPath(a.Cfg.Home, config.AuditDB)
	}
	st, err := audit.Open(context.Background(), path)
	if err != nil {
		log.Debugf("audit disabled: %v", err)
		return
	}
	a.Audit = st
}

func (a *App) Shutdown() {
	if a == nil {
		return
	}
	a.FlushRoleLifecycle()
	if a.Audit != nil {
		_ = a.Audit.Close()
	}
	if a.Tokens != nil {
		_ = a.Tokens.Close()
	}
	_ = a.Cache.Close()
	if a.Mgr != nil {
		_ = a.Mgr.Close()
	}
}

func (a *App) CloseDBs() {
	if a.Audit != nil {
		_ = a.Audit.Close()
		a.Audit = nil
	}
	if a.Tokens != nil {
		_ = a.Tokens.Close()
		a.Tokens = nil
	}
	_ = a.Cache.Close()
	if a.Mgr != nil {
		_ = a.Mgr.Close()
		a.Mgr = nil
	}
}

func (a *App) Access() config.Access {
	return config.NewAccess(a.Cfg.Role, a.Directives.Roles)
}

func (a *App) VisibleQueries() []string {
	ac := a.Access()
	var out []string
	for _, n := range a.Directives.QueryNames() {
		if ac.QueryVisible(n) {
			out = append(out, n)
		}
	}
	return out
}

func (a *App) VisibleFilters() []string {
	ac := a.Access()
	var out []string
	for _, n := range a.Directives.FilterNames() {
		if ac.QueryVisible(n) {
			out = append(out, n)
		}
	}
	return out
}

func (a *App) VisibleFlights() []string {
	ac := a.Access()
	var out []string
	for _, n := range a.Directives.FlightNames() {
		if ac.FlightVisible(n) {
			out = append(out, n)
		}
	}
	return out
}

func (a *App) NotInRoleError(kind, name string) error {
	return errs.Newf(errs.KindUsage, "%s %q is not available in role %q", kind, name, a.Cfg.Role)
}
