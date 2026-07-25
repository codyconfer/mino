package plugin

import (
	"bytes"
	"os"
	"path"
	"path/filepath"

	pub "github.com/codyconfer/munin/plugin"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/errs"
)

// FileSeed is an example directive seed (re-export of public SDK).
type FileSeed = pub.FileSeed

// RegisterSeeds associates example directive seeds with a compile-time plugin id.
// Prefer github.com/codyconfer/munin/plugin.RegisterSeeds from overlays.
func RegisterSeeds(pluginID string, files []FileSeed) { pub.RegisterSeeds(pluginID, files) }

// SeedsFor returns example seeds for pluginID.
func SeedsFor(pluginID string) []FileSeed { return pub.SeedsFor(pluginID) }

// SeedPluginIDs returns plugin ids that have registered example seeds.
func SeedPluginIDs() []string { return pub.SeedPluginIDs() }

// InstallOptions controls plugins install behavior.
type InstallOptions struct {
	// Force overwrites existing seed files even when contents differ.
	Force bool
}

// InstallResult reports what plugins install did.
type InstallResult struct {
	PluginID  string
	Installed bool
	Enabled   bool
	Written   []string // home-relative paths created or overwritten
	Skipped   []string // home-relative paths left untouched
}

// Install adds a compile-time registered plugin to the managed/installed set,
// enables it, and provisions its example directive seeds into home. It does
// not load shared libraries or mutate the Go plugin registry at runtime.
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

func writeSeeds(home string, seeds []FileSeed, opts InstallOptions) (written, skipped []string, err error) {
	for _, seed := range seeds {
		rel := seed.RelPath
		abs := filepath.Join(home, filepath.FromSlash(rel))
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

// UninstallOptions controls plugins uninstall behavior.
type UninstallOptions struct {
	// KeepSeeds disables the plugin without removing provisioned example files.
	KeepSeeds bool
	// Force removes seed files even when the on-disk content no longer matches
	// the catalog (user edits). Without Force, modified files are preserved.
	Force bool
}

// UninstallResult reports what plugins uninstall did.
type UninstallResult struct {
	PluginID    string
	Uninstalled bool
	Disabled    bool
	Removed     []string
	Kept        []string // skipped (modified without --force, or --keep-seeds)
}

// Uninstall removes a compile-time registered plugin from the managed/installed
// set, disables it, and by default removes example directive seeds that still
// match the catalog. Disable alone (SetEnabled false) keeps the plugin listed;
// uninstall is what drops it from the Plugins TUI until installed again.
// It never unloads compile-linked plugin code.
func Uninstall(home, id string, opts UninstallOptions) (UninstallResult, error) {
	res := UninstallResult{PluginID: id}
	if _, ok := Lookup(id); !ok {
		return res, errs.Newf(errs.KindConfig, "unknown plugin %q", id).
			WithHint("plugins are linked at compile time; use `munin plugins list` for ids in this binary")
	}
	// Clear installed + disable in one write (disable ≠ uninstall).
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

	for _, seed := range SeedsFor(id) {
		rel := seed.RelPath
		abs := filepath.Join(home, filepath.FromSlash(rel))
		existing, err := os.ReadFile(abs)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return res, errs.Wrapf(errs.KindConfig, err, "read %s", rel)
		}
		if !opts.Force && !bytes.Equal(existing, seed.Content) {
			res.Kept = append(res.Kept, rel)
			continue
		}
		if err := os.Remove(abs); err != nil {
			return res, errs.Wrapf(errs.KindConfig, err, "remove %s", rel)
		}
		res.Removed = append(res.Removed, rel)
	}
	return res, nil
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
signal: ntr
params: {}
`
	seedNTRFlightYAML = `# Scheduled reminders fire when this flight is served (` + "`munin serve ntr`" + `).
# NTR ReminderJob shares the daemon/serve notify sink (tray on daemon; desktop/terminal on serve).
name: ntr
queries: [ntr-list]
`
)
