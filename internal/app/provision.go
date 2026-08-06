package app

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	sconfig "github.com/codyconfer/sisyphus/config"
	"github.com/codyconfer/sisyphus/lifecycle"

	"github.com/codyconfer/viewkit/ui"

	"github.com/codyconfer/mino/internal/audit"
	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/render"
	"github.com/codyconfer/mino/internal/state"
	"github.com/codyconfer/mino/internal/token"
)

const (
	defaultConfigYAML = `# mino configuration — see the README for all options.
# This is the only file that must be named config.yaml and sit at the root.
# Directive files carry a ` + "`type:`" + ` and may live anywhere below here.
output: terminal

audit:
  enabled: true

# External TUIs, opened from the deck by hotkey. A tool whose binary is not on
# PATH leaves its binding inert. {{context.<tool>}} expands to the context mino
# has selected for that tool; write it inside a single --flag={{…}} token so the
# whole flag drops when nothing is selected.
# tools:
#   k9s:
#     argv: [k9s, "--context={{context.kubectl}}"]
# keybinds:
#   alt+k: tool:k9s
`
	sampleQueryYAML = `name: my-open-prs
type: query
signal: github
params:
  query: "is:open is:pr author:@me"
`
	sisyphusOpenPRsYAML = `name: sisyphus-open-prs
type: query
signal: github
params:
  title: "sisyphus · open PRs"
  query: "repo:codyconfer/sisyphus is:open is:pr"
`
	sisyphusCIYAML = `name: sisyphus-ci
type: query
signal: github
params:
  title: "sisyphus · latest CI"
  actions: codyconfer/sisyphus
`
	viewkitOpenPRsYAML = `name: viewkit-open-prs
type: query
signal: github
params:
  title: "viewkit · open PRs"
  query: "repo:codyconfer/viewkit is:open is:pr"
`
	viewkitCIYAML = `name: viewkit-ci
type: query
signal: github
params:
  title: "viewkit · latest CI"
  actions: codyconfer/viewkit
`
	minoOpenPRsYAML = `name: mino-open-prs
type: query
signal: github
params:
  title: "mino · open PRs"
  query: "repo:codyconfer/mino is:open is:pr"
`
	minoCIYAML = `name: mino-ci
type: query
signal: github
params:
  title: "mino · latest CI"
  actions: codyconfer/mino
`
	sampleFilterYAML = `name: no-bots
type: filter
rules:
  - field: meta.author
    exclude: "(?i)bot$"
`
	sampleFlightYAML = `name: default
type: flight
queries: [sisyphus-open-prs, sisyphus-ci, viewkit-open-prs, viewkit-ci, mino-open-prs, mino-ci]
`
	sampleDefaultRoleYAML = `name: default
type: role
home: default
flights: [default]
queries: [my-open-prs]
`
	sampleFormatterYAML = `name: standup
type: formatter
title: Daily Standup
template: |
  ## Standup {{ now | date "2006-01-02" }}
  {{ range .Queries }}
  ### {{ .Title | default .Query }} ({{ .Count }})
  {{ range .Items }}- [{{ .Title }}]({{ .URL }}){{ with .Meta.author }} — @{{ . }}{{ end }}
  {{ end }}{{ else }}
  Nothing on the board today.
  {{ end }}
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

// NeedsInstall reports whether the home has no file-backed or stored config.
// It is deliberately conservative: an unreadable store is treated as
// potentially populated so startup can handle and report it normally.
func NeedsInstall(home string) (bool, error) {
	if ConfigExists(home) {
		return false, nil
	}
	directives, err := config.DirectiveFiles(home)
	if err != nil {
		return false, err
	}
	if len(directives) > 0 {
		return false, nil
	}

	dbPath := filepath.Join(home, config.DirData, config.ConfigDB)
	if _, err := os.Stat(dbPath); err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, errs.Wrap(errs.KindConfig, err, "inspect config store")
	}

	mgr, err := config.OpenStore(context.Background(), home)
	if err != nil {
		return false, nil
	}
	defer mgr.Close()
	for _, name := range []string{config.ConfigDirective, config.DirectivesDirective} {
		cur, ok, err := mgr.Current(context.Background(), name)
		if err != nil {
			return false, errs.Wrap(errs.KindStore, err, "inspect config store")
		}
		if ok && strings.TrimSpace(cur.Content) != "" {
			return false, nil
		}
	}
	return true, nil
}

func installSpec(home string, force bool) lifecycle.InstallSpec {
	stock := []lifecycle.FileSeed{
		{RelPath: "config.yaml", Content: []byte(defaultConfigYAML)},
		{RelPath: path.Join(config.DirQueries, "my-open-prs.yaml"), Content: []byte(sampleQueryYAML)},
		{RelPath: path.Join(config.DirQueries, "sisyphus-open-prs.yaml"), Content: []byte(sisyphusOpenPRsYAML)},
		{RelPath: path.Join(config.DirQueries, "sisyphus-ci.yaml"), Content: []byte(sisyphusCIYAML)},
		{RelPath: path.Join(config.DirQueries, "viewkit-open-prs.yaml"), Content: []byte(viewkitOpenPRsYAML)},
		{RelPath: path.Join(config.DirQueries, "viewkit-ci.yaml"), Content: []byte(viewkitCIYAML)},
		{RelPath: path.Join(config.DirQueries, "mino-open-prs.yaml"), Content: []byte(minoOpenPRsYAML)},
		{RelPath: path.Join(config.DirQueries, "mino-ci.yaml"), Content: []byte(minoCIYAML)},
		{RelPath: path.Join(config.DirQueries, "no-bots.yaml"), Content: []byte(sampleFilterYAML)},
		{RelPath: path.Join(config.DirFlights, "default.yaml"), Content: []byte(sampleFlightYAML)},
		{RelPath: path.Join(config.DirFormatters, "standup.yaml"), Content: []byte(sampleFormatterYAML)},
		{RelPath: "default.yaml", Content: []byte(sampleDefaultRoleYAML)},
	}
	return lifecycle.InstallSpec{
		Home:  home,
		Force: force,
		Dirs: []string{
			config.DirQueries, config.DirFlights, config.DirFormatters, config.DirDuckDB, config.DirLogs, config.DirData,
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
	if mgr, err := config.OpenStore(context.Background(), home); err == nil {
		if raw, format, err := sconfig.ReadFile(home); err == nil && len(raw) > 0 {
			_ = mgr.Import(context.Background(), "config", raw, format)
		}
		if blob, has, err := config.SerializeDirectives(home); err == nil && has {
			_ = mgr.Import(context.Background(), config.DirectivesDirective, blob, "collection")
		}
		_ = mgr.Close()
		*created = append(*created, config.DataPath(home, config.ConfigDB))
	}
	if a, err := audit.Open(context.Background(), config.DataPath(home, config.AuditDB)); err == nil {
		_ = a.Close()
		*created = append(*created, config.DataPath(home, config.AuditDB))
	}
	if tk, err := token.Open(context.Background(), config.DataPath(home, config.TokensDB)); err == nil {
		_ = tk.Close()
		*created = append(*created, config.DataPath(home, config.TokensDB))
	}
	if st, err := state.Open(context.Background(), config.DataPath(home, config.StateDB)); err == nil {
		_ = st.Close()
		*created = append(*created, config.DataPath(home, config.StateDB))
	}
	return nil
}

func Install(home string, force bool) ([]string, error) {
	if !force && ConfigExists(home) {
		return nil, errs.Newf(errs.KindConfig, "%s already has a config file", home).
			WithHint("use --force to overwrite, or `mino nuke` then `mino install`")
	}
	created, err := lifecycle.Install(installSpec(home, force))
	if err != nil {
		return nil, errs.Wrap(errs.KindConfig, err, "install")
	}
	return created, nil
}

func Clean(w io.Writer, scope *ui.Scope, home string) error {
	entries := []string{
		"config.yaml", "config.yml", "config.json",
		config.DirQueries, config.DirFlights, config.DirFormatters, config.DirDuckDB, config.DirLogs, config.DirReports,
	}
	rels, err := config.DirectiveFiles(home)
	if err != nil {
		return err
	}
	entries = append(entries, rels...)
	dest, moved, err := lifecycle.Clean(home, entries)
	if err != nil {
		return err
	}
	if len(moved) == 0 {
		fmt.Fprintln(w, "nothing to clean (no config/query/filter files present)")
		return nil
	}
	fmt.Fprintln(w, render.Success(scope, fmt.Sprintf("archived %s to %s", strings.Join(moved, ", "), dest)))
	return nil
}

func Nuke(w io.Writer, scope *ui.Scope, home string) error {
	if home == "" {
		return errs.New(errs.KindConfig, "empty home directory")
	}
	if err := sconfig.RemoveAll(home); err != nil {
		return errs.Wrapf(errs.KindInternal, err, "removing %s", home)
	}
	if err := clearGlobalHomeIfMatches(home); err != nil {
		return err
	}
	fmt.Fprintln(w, render.Success(scope, fmt.Sprintf("nuked %s", home)))
	fmt.Fprintln(w, "run `mino install` to recreate defaults in ~/.mino (or pass --home/--dir)")
	return nil
}

func clearGlobalHomeIfMatches(home string) error {
	gs := config.LoadGlobalSettings()
	if gs.Home == "" {
		return nil
	}
	configured, err := config.Home(gs.Home)
	if err != nil {
		return nil
	}
	if filepath.Clean(configured) != filepath.Clean(home) {
		return nil
	}
	gs.Home = ""
	if err := config.SaveGlobalSettings(gs); err != nil {
		return errs.Wrap(errs.KindConfig, err, "clearing settings home after nuke")
	}
	return nil
}
