package plugin

import (
	"bytes"
	"os"
	"path"
	"path/filepath"
	"strings"

	pub "github.com/codyconfer/munin/plugin"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/errs"
)

type FileSeed = pub.FileSeed

func RegisterSeeds(pluginID string, files []FileSeed) { pub.RegisterSeeds(pluginID, files) }

func SeedsFor(pluginID string) []FileSeed { return pub.SeedsFor(pluginID) }

func SeedPluginIDs() []string { return pub.SeedPluginIDs() }

type InstallOptions struct {
	Force bool
}

type InstallResult struct {
	PluginID  string
	Installed bool
	Enabled   bool
	Written   []string
	Skipped   []string
}

func Install(home, id string, opts InstallOptions) (InstallResult, error) {
	res := InstallResult{PluginID: id}
	if _, ok := Lookup(id); !ok {
		return res, errs.Newf(errs.KindConfig, "unknown plugin %q", id).
			WithHint("plugins are linked at compile time; use `munin plugins list` for ids in this binary")
	}
	if err := setInstalledEnabled(id, true, true); err != nil {
		return res, err
	}
	res.Installed = true
	res.Enabled = true

	written, skipped, err := writeSeeds(home, SeedsFor(id), opts)
	if err != nil {
		return res, err
	}
	res.Written = written
	res.Skipped = skipped
	return res, nil
}

func seedTarget(home, rel string) (string, error) {
	trimmed := strings.TrimSpace(rel)
	if trimmed == "" {
		return "", errs.Newf(errs.KindConfig, "seed path is empty")
	}
	clean := filepath.FromSlash(strings.ReplaceAll(trimmed, `\`, "/"))
	if filepath.IsAbs(clean) || filepath.VolumeName(clean) != "" {
		return "", errs.Newf(errs.KindConfig, "seed path %q must be relative to the munin home", rel)
	}
	abs := filepath.Join(home, clean)
	inside, err := filepath.Rel(home, abs)
	if err != nil || inside == "." || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
		return "", errs.Newf(errs.KindConfig, "seed path %q escapes the munin home", rel)
	}
	if err := checkResolvedWithinHome(home, abs, rel); err != nil {
		return "", err
	}
	return abs, nil
}

// checkResolvedWithinHome repeats the containment check on resolved paths. The
// string comparison above cannot see a symlink at, say, $HOME/queries pointing
// outside the munin home, so on its own it lets both writes and removals land
// outside while reporting success.
func checkResolvedWithinHome(home, abs, rel string) error {
	realHome, err := resolveExisting(home)
	if err != nil {
		return errs.Wrapf(errs.KindConfig, err, "resolve munin home")
	}
	realParent, err := resolveExisting(filepath.Dir(abs))
	if err != nil {
		return errs.Wrapf(errs.KindConfig, err, "resolve parent of seed %q", rel)
	}
	within, err := filepath.Rel(realHome, realParent)
	if err != nil || within == ".." || filepath.IsAbs(within) ||
		strings.HasPrefix(within, ".."+string(filepath.Separator)) {
		return errs.Newf(errs.KindConfig, "seed path %q resolves outside the munin home", rel).
			WithHint("a symlink inside the munin home points elsewhere; remove or repoint it")
	}
	if fi, err := os.Lstat(abs); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return errs.Newf(errs.KindConfig, "seed path %q is a symlink; refusing to follow it", rel)
	}
	return nil
}

// resolveExisting resolves symlinks on the deepest existing ancestor of path and
// re-attaches the components that do not exist yet.
func resolveExisting(path string) (string, error) {
	path = filepath.Clean(path)
	var tail []string
	for {
		resolved, err := filepath.EvalSymlinks(path)
		if err == nil {
			for i := len(tail) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, tail[i])
			}
			return resolved, nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(path)
		if parent == path {
			return "", err
		}
		tail = append(tail, filepath.Base(path))
		path = parent
	}
}

func writeSeeds(home string, seeds []FileSeed, opts InstallOptions) (written, skipped []string, err error) {
	for _, seed := range seeds {
		rel := seed.RelPath
		abs, err := seedTarget(home, rel)
		if err != nil {
			return written, skipped, err
		}
		if !opts.Force {
			if _, err := os.Stat(abs); err == nil {
				skipped = append(skipped, rel)
				continue
			} else if !os.IsNotExist(err) {
				return written, skipped, errs.Wrapf(errs.KindConfig, err, "stat %s", rel)
			}
		}
		if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
			return written, skipped, errs.Wrapf(errs.KindConfig, err, "create dir for %s", rel)
		}
		if err := os.WriteFile(abs, seed.Content, 0o600); err != nil {
			return written, skipped, errs.Wrapf(errs.KindConfig, err, "write %s", rel)
		}
		written = append(written, rel)
	}
	return written, skipped, nil
}

type UninstallOptions struct {
	KeepSeeds bool
	Force     bool
}

type UninstallResult struct {
	PluginID    string
	Uninstalled bool
	Disabled    bool
	Removed     []string
	Kept        []string
}

func Uninstall(home, id string, opts UninstallOptions) (UninstallResult, error) {
	res := UninstallResult{PluginID: id}
	if _, ok := Lookup(id); !ok {
		return res, errs.Newf(errs.KindConfig, "unknown plugin %q", id).
			WithHint("plugins are linked at compile time; use `munin plugins list` for ids in this binary")
	}
	if IsInternal(id) {
		return res, errs.Newf(errs.KindConfig, "cannot uninstall built-in plugin %q", id).
			WithHint("internal plugins are always present; use disable to deactivate, or reinstall to refresh seeds")
	}
	if err := setInstalledEnabled(id, false, false); err != nil {
		return res, err
	}
	res.Uninstalled = true
	res.Disabled = true

	if opts.KeepSeeds {
		for _, seed := range SeedsFor(id) {
			res.Kept = append(res.Kept, seed.RelPath)
		}
		return res, nil
	}

	removed, kept, err := removeSeeds(home, SeedsFor(id), opts.Force)
	res.Removed = append(res.Removed, removed...)
	res.Kept = append(res.Kept, kept...)
	if err != nil {
		return res, err
	}
	return res, nil
}

func removeSeeds(home string, seeds []FileSeed, force bool) (removed, kept []string, err error) {
	for _, seed := range seeds {
		rel := seed.RelPath
		abs, err := seedTarget(home, rel)
		if err != nil {
			return removed, kept, err
		}
		existing, err := os.ReadFile(abs)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return removed, kept, errs.Wrapf(errs.KindConfig, err, "read %s", rel)
		}
		if !force && !bytes.Equal(existing, seed.Content) {
			kept = append(kept, rel)
			continue
		}
		if err := os.Remove(abs); err != nil {
			return removed, kept, errs.Wrapf(errs.KindConfig, err, "remove %s", rel)
		}
		removed = append(removed, rel)
	}
	return removed, kept, nil
}

func init() {
	registerStockSeeds()
}

func seedRel(dir, name string) string {
	return path.Join(dir, name)
}

func registerStockSeeds() {
	RegisterSeeds("munin.ntr", []FileSeed{
		{RelPath: seedRel(config.DirQueries, "ntr-list.yaml"), Content: []byte(seedNTRListYAML)},
		{RelPath: seedRel(config.DirFlights, "ntr.yaml"), Content: []byte(seedNTRFlightYAML)},
	})
}

const (
	seedNTRListYAML = `name: ntr-list
type: query
signal: ntr
params: {}
`
	seedNTRFlightYAML = `# Scheduled reminders fire when this flight is served (` + "`munin serve ntr`" + `).
# NTR ReminderJob shares the daemon/serve notify sink (tray on daemon; desktop/terminal on serve).
name: ntr
type: flight
queries: [ntr-list]
`
)
