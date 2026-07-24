package app

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/codyconfer/sisyphus"

	"github.com/codyconfer/munin/internal/audit"
	"github.com/codyconfer/munin/internal/auth"
	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/log"
	"github.com/codyconfer/munin/internal/token"
)

type App struct {
	Cfg        *config.Config
	Directives *config.Directives
	Audit      *audit.Store
	Tokens     *token.Store
	Mgr        *sisyphus.Manager

	ghAuth ghAuthCache
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
	cfg, directives, mgr, err := config.LoadConfigAndDirectives(opts.Home, opts.ConfigFile, opts.Interactive, opts.In, opts.Out)
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
	a := &App{Cfg: cfg, Directives: directives, Mgr: mgr}
	a.openTokens()
	a.openAudit()
	return a, nil
}

func (a *App) openTokens() {
	ts, err := token.Open(filepath.Join(a.Cfg.Home, "tokens.duckdb"))
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
		path = filepath.Join(a.Cfg.Home, "audit.duckdb")
	}
	st, err := audit.Open(path)
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
	if a.Audit != nil {
		_ = a.Audit.Close()
	}
	if a.Tokens != nil {
		_ = a.Tokens.Close()
	}
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
		if ac.FilterVisible(n) {
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
