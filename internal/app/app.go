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

	mu            sync.RWMutex
	roleTransient bool

	thin         bool
	ghAuth       ghAuthCache
	roleDebounce roleDebounce
}

func (a *App) Role() string {
	if a == nil {
		return ""
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.Cfg == nil {
		return ""
	}
	return a.Cfg.Role
}

func (a *App) Dirs() *config.Directives {
	if a == nil {
		return &config.Directives{}
	}
	a.mu.RLock()
	d := a.Directives
	a.mu.RUnlock()
	if d == nil {
		return &config.Directives{}
	}
	return d
}

func (a *App) RoleDef(name string) (config.RoleDef, bool) {
	if name == "" {
		return config.RoleDef{}, false
	}
	rd, ok := a.Dirs().Roles[name]
	return rd, ok
}

func (a *App) setRole(name string) {
	if a == nil {
		return
	}
	a.mu.Lock()
	if a.Cfg != nil {
		a.Cfg.Role = name
	}
	a.mu.Unlock()
}

func (a *App) setDirectives(d *config.Directives) {
	if a == nil {
		return
	}
	a.mu.Lock()
	a.Directives = d
	a.mu.Unlock()
}

func (a *App) transientRole() bool {
	if a == nil {
		return false
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.roleTransient
}

func (a *App) clearTransientRole() {
	if a == nil {
		return
	}
	a.mu.Lock()
	a.roleTransient = false
	a.mu.Unlock()
}

func (a *App) home() string {
	if a == nil || a.Cfg == nil {
		return ""
	}
	return a.Cfg.Home
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
	Thin        bool
	Completion  bool
	In          io.Reader
	Out         io.Writer
}

func Load(opts Options) (*App, error) {
	log.SetVerbose(opts.Verbose)
	if opts.Completion {
		opts.Reconcile = config.ReconcileIgnore
		opts.Interactive = false
	}
	if home, err := config.Home(opts.Home); err == nil {
		_ = os.Chmod(home, 0o700)
	}
	cfg, directives, mgr, err := loadConfig(opts)
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
		d, err := time.ParseDuration(opts.Timeout)
		if err != nil {
			return nil, errs.Newf(errs.KindUsage, "--timeout %q is not a valid duration", opts.Timeout).
				WithHint("use a Go duration like 30s or 2m")
		}
		if d <= 0 {
			return nil, errs.Newf(errs.KindUsage, "--timeout %q must be greater than zero", opts.Timeout).
				WithHint("use a positive Go duration like 30s or 2m")
		}
		cfg.Timeout = opts.Timeout
	}
	if opts.CacheTTL != "" {
		if _, err := time.ParseDuration(opts.CacheTTL); err != nil {
			return nil, errs.Newf(errs.KindUsage, "--cache-ttl %q is not a valid duration", opts.CacheTTL).
				WithHint("use a Go duration like 60s, 5m, or 0 to disable")
		}
		cfg.Cache.TTL, cfg.Cache.Signals = opts.CacheTTL, nil
		cfg.Cache.DetailTTL = opts.CacheTTL
	}
	a := &App{Cfg: cfg, Directives: directives, Mgr: mgr, thin: opts.Thin, roleTransient: sessionScopedRole(opts)}
	if opts.Thin || opts.Completion {
		return a, nil
	}
	a.openCache(cacheMode(opts))
	a.openTokens()
	a.openAudit()
	a.syncRoleLifecycle()
	return a, nil
}

func loadConfig(opts Options) (*config.Config, *config.Directives, *sisyphus.Manager, error) {
	if opts.Thin {
		cfg, directives, err := config.LoadConfigAndDirectivesFromFiles(opts.Home, opts.ConfigFile)
		return cfg, directives, nil, err
	}
	return config.LoadConfigAndDirectives(opts.Home, opts.ConfigFile, opts.Reconcile, opts.Interactive, opts.In, opts.Out)
}

func (a *App) Thin() bool { return a != nil && a.thin }

const envSessionRole = "MUNIN_ROLE"

func sessionScopedRole(opts Options) bool {
	return opts.Role != "" || os.Getenv(envSessionRole) != ""
}

func (a *App) ActivateRole(name string) error {
	if a == nil || a.Cfg == nil {
		return errs.New(errs.KindInternal, "app not loaded")
	}
	if name == a.Role() && !a.transientRole() {
		return nil
	}
	if _, ok := a.RoleDef(name); !ok && name != "" {
		return errs.Newf(errs.KindUsage, "unknown role %q", name).
			WithHint("run `munin role` to list defined roles")
	}
	if err := a.persistRole(name); err != nil {
		return err
	}
	a.invalidateRoleDebounce()
	a.setRole(name)
	a.settleRoleChange()
	return nil
}

func (a *App) persistRole(name string) error {
	if _, err := config.SetValues(a.home(), map[string]any{"role": name}); err != nil {
		return err
	}
	a.clearTransientRole()
	return nil
}

func (a *App) syncRoleLifecycle() {
	if a == nil || a.Cfg == nil || a.thin {
		return
	}
	if a.transientRole() {
		a.refreshRoleStatus(a.Role())
		a.applyRoleContexts()
		return
	}
	a.settleRoleChange()
}

func (a *App) settleRoleChange() {
	p := a.planRoleLifecycle()
	a.runRolePlan(p)
	a.commitRolePlan(p)
}

func (a *App) refreshRoleStatus(name string) {
	rd, ok := a.RoleDef(name)
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
	rd, ok := a.RoleDef(a.Role())
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
	d, err := config.ReloadDirectives(a.Mgr, a.home(), policy)
	if err != nil {
		return err
	}
	a.setDirectives(d)
	a.applyRoleContexts()
	return nil
}

func (a *App) HasStore() bool {
	return a != nil && a.Mgr != nil
}

func (a *App) StoreRevision() (string, bool) {
	if a == nil || a.Mgr == nil {
		return "", false
	}
	return a.Mgr.DB().Revision()
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

func (a *App) Access() config.Access { return a.accessIn(a.Dirs()) }

func (a *App) accessIn(d *config.Directives) config.Access {
	return config.NewAccess(a.Role(), d.Roles)
}

func visibleNames(names []string, allowed func(string) bool) []string {
	var out []string
	for _, n := range names {
		if allowed(n) {
			out = append(out, n)
		}
	}
	return out
}

func (a *App) VisibleQueries() []string {
	d := a.Dirs()
	return visibleNames(d.QueryNames(), a.accessIn(d).QueryVisible)
}

func (a *App) VisibleFilters() []string {
	d := a.Dirs()
	return visibleNames(d.FilterNames(), a.accessIn(d).QueryVisible)
}

func (a *App) VisibleFlights() []string {
	d := a.Dirs()
	return visibleNames(d.FlightNames(), a.accessIn(d).FlightVisible)
}

func (a *App) VisibleFormatters() []string {
	d := a.Dirs()
	return visibleNames(d.FormatterNames(), a.accessIn(d).FormatterVisible)
}

func (a *App) NotInRoleError(kind, name string) error {
	return errs.Newf(errs.KindUsage, "%s %q is not available in role %q", kind, name, a.Role())
}
