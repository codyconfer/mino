package app

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/codyconfer/sisyphus"
	sconfig "github.com/codyconfer/sisyphus/config"
	"github.com/codyconfer/sisyphus/lifecycle"

	"github.com/codyconfer/munin/internal/audit"
	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/render"
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

func installSpec(home string, force bool) lifecycle.InstallSpec {
	stock := []lifecycle.FileSeed{
		{RelPath: "config.yaml", Content: []byte(defaultConfigYAML)},
		{RelPath: path.Join(config.DirQueries, "my-open-prs.yaml"), Content: []byte(sampleQueryYAML)},
		{RelPath: path.Join(config.DirFilters, "no-bots.yaml"), Content: []byte(sampleFilterYAML)},
		{RelPath: path.Join(config.DirFlights, "default.yaml"), Content: []byte(sampleFlightYAML)},
	}
	return lifecycle.InstallSpec{
		Home:  home,
		Force: force,
		Dirs: []string{
			config.DirQueries, config.DirFilters, config.DirFlights, config.DirRoles, config.DirLogs,
		},
		Files: mergeFileSeeds(stock, walkDefaults(getDefaultsFS())),
		After: seedStores,
	}
}

func seedRelPath(rel string) string {
	return path.Clean(strings.ReplaceAll(rel, `\`, "/"))
}

func mergeFileSeeds(base, overlay []lifecycle.FileSeed) []lifecycle.FileSeed {
	if len(overlay) == 0 {
		return normalizeFileSeeds(base)
	}
	idx := map[string]int{}
	out := make([]lifecycle.FileSeed, 0, len(base)+len(overlay))
	for _, f := range base {
		f.RelPath = seedRelPath(f.RelPath)
		idx[f.RelPath] = len(out)
		out = append(out, f)
	}
	for _, f := range overlay {
		f.RelPath = seedRelPath(f.RelPath)
		if i, ok := idx[f.RelPath]; ok {
			out[i] = f
			continue
		}
		idx[f.RelPath] = len(out)
		out = append(out, f)
	}
	return out
}

func normalizeFileSeeds(in []lifecycle.FileSeed) []lifecycle.FileSeed {
	out := make([]lifecycle.FileSeed, len(in))
	for i, f := range in {
		f.RelPath = seedRelPath(f.RelPath)
		out[i] = f
	}
	return out
}

func walkDefaults(fsys fs.FS) []lifecycle.FileSeed {
	if fsys == nil {
		return nil
	}
	var out []lifecycle.FileSeed
	_ = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		base := path.Base(p)
		if strings.HasPrefix(base, ".") {
			return nil
		}
		b, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		out = append(out, lifecycle.FileSeed{RelPath: path.Clean(p), Content: b})
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].RelPath < out[j].RelPath })
	return out
}

func seedStores(home string, created *[]string) error {
	if mgr, err := sisyphus.Open(context.Background(), home, sisyphus.Options{Mode: sisyphus.ModeBoth}); err == nil {
		if raw, format, err := sconfig.ReadFile(home); err == nil && len(raw) > 0 {
			_ = mgr.DB().Import(context.Background(), "config", raw, format)
		}
		for _, name := range []string{config.DirQueries, config.DirFilters, config.DirFlights, config.DirRoles} {
			if blob, has, err := sconfig.SerializeDir(filepath.Join(home, name)); err == nil && has {
				_ = mgr.DB().Import(context.Background(), name, blob, "collection")
			}
		}
		_ = mgr.Close()
		*created = append(*created, filepath.Join(home, "config.duckdb"))
	}
	if a, err := audit.Open(context.Background(), filepath.Join(home, "audit.duckdb")); err == nil {
		_ = a.Close()
		*created = append(*created, filepath.Join(home, "audit.duckdb"))
	}
	if tk, err := token.Open(context.Background(), filepath.Join(home, "tokens.duckdb")); err == nil {
		_ = tk.Close()
		*created = append(*created, filepath.Join(home, "tokens.duckdb"))
	}
	return nil
}

func Install(home string, force bool) ([]string, error) {
	if !force && ConfigExists(home) {
		return nil, errs.Newf(errs.KindConfig, "%s already has a config file", home).
			WithHint("use --force to overwrite, or `munin nuke` to reinstall")
	}
	created, err := lifecycle.Install(installSpec(home, force))
	if err != nil {
		return nil, errs.Wrap(errs.KindConfig, err, "install")
	}
	return created, nil
}

func Clean(w io.Writer, home string) error {
	entries := []string{
		"config.yaml", "config.yml", "config.json",
		config.DirQueries, config.DirFilters, config.DirFlights, config.DirRoles, config.DirLogs,
	}
	dest, moved, err := lifecycle.Clean(home, entries)
	if err != nil {
		return err
	}
	if len(moved) == 0 {
		fmt.Fprintln(w, "nothing to clean (no config/query/filter files present)")
		return nil
	}
	fmt.Fprintln(w, render.Success(fmt.Sprintf("archived %s to %s", strings.Join(moved, ", "), dest)))
	return nil
}

func Nuke(w io.Writer, home string) error {
	created, err := lifecycle.Nuke(home, installSpec(home, true))
	if err != nil {
		return errs.Wrapf(errs.KindInternal, err, "nuke %s", home)
	}
	fmt.Fprintln(w, render.Success(fmt.Sprintf("nuked and reinstalled %s (%d files)", home, len(created))))
	return nil
}
