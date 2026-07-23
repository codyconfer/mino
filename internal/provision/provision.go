package provision

import (
	"path/filepath"

	"github.com/codyconfer/sisyphus"
	sconfig "github.com/codyconfer/sisyphus/config"

	"github.com/codyconfer/munin/internal/audit"
	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/token"
)

const (
	defaultConfigYAML = `# munin configuration — see the README for all options.
# Flights and roles are their own directive collections (flights/, roles/).
output: terminal

audit:
  enabled: true
`
	sampleQueryYAML = `name: my-open-prs
signal: github
params:
  query: "is:open is:pr author:@me"
`
	sampleFilterYAML = `name: no-bots
rules:
  - field: meta.author
    exclude: "(?i)bot$"
`
	sampleFlightYAML = `name: default
queries: [my-open-prs]
`
)

func ConfigExists(home string) bool {
	for _, n := range []string{"config.yaml", "config.yml", "config.json"} {
		if sconfig.IsFile(filepath.Join(home, n)) {
			return true
		}
	}
	return false
}

func Install(home string, force bool) ([]string, error) {
	if ConfigExists(home) && !force {
		return nil, errs.Newf(errs.KindConfig, "%s already has a config file", home).
			WithHint("use --force to overwrite, or `munin nuke` to reinstall")
	}

	for _, d := range []string{
		home,
		filepath.Join(home, config.DirQueries),
		filepath.Join(home, config.DirFilters),
		filepath.Join(home, config.DirFlights),
		filepath.Join(home, config.DirRoles),
	} {
		if err := sconfig.EnsureDir(d); err != nil {
			return nil, err
		}
	}

	var created []string
	files := []struct{ path, content string }{
		{filepath.Join(home, "config.yaml"), defaultConfigYAML},
		{filepath.Join(home, config.DirQueries, "my-open-prs.yaml"), sampleQueryYAML},
		{filepath.Join(home, config.DirFilters, "no-bots.yaml"), sampleFilterYAML},
		{filepath.Join(home, config.DirFlights, "default.yaml"), sampleFlightYAML},
	}
	for _, f := range files {
		if !force && sconfig.IsFile(f.path) {
			continue
		}
		if _, err := sconfig.WriteItem(filepath.Dir(f.path), filepath.Base(f.path), []byte(f.content)); err != nil {
			return nil, err
		}
		created = append(created, f.path)
	}

	if mgr, err := sisyphus.Open(home, sisyphus.Options{Mode: sisyphus.ModeBoth}); err == nil {
		if raw, format, err := sconfig.ReadFile(home); err == nil && len(raw) > 0 {
			_ = mgr.DB().Import("config", raw, format)
		}
		for _, name := range []string{config.DirQueries, config.DirFilters, config.DirFlights, config.DirRoles} {
			if blob, has, err := sconfig.SerializeDir(filepath.Join(home, name)); err == nil && has {
				_ = mgr.DB().Import(name, blob, "collection")
			}
		}
		_ = mgr.Close()
		created = append(created, filepath.Join(home, "config.duckdb"))
	}
	if a, err := audit.Open(filepath.Join(home, "audit.duckdb")); err == nil {
		_ = a.Close()
		created = append(created, filepath.Join(home, "audit.duckdb"))
	}
	if tk, err := token.Open(filepath.Join(home, "tokens.duckdb")); err == nil {
		_ = tk.Close()
		created = append(created, filepath.Join(home, "tokens.duckdb"))
	}
	return created, nil
}
