package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/errs"
)

type LocalPlugin struct {
	ID          string
	Name        string
	Dir         string
	Description string
	Seeds       []FileSeed
	Registered  bool
	Installable bool
	Reason      string
}

type InstallCandidate struct {
	ID          string
	Label       string
	Desc        string
	Source      string
	LocalDir    string
	Seeds       []FileSeed
	Installable bool
	Reason      string
}

type localManifest struct {
	ID          string `yaml:"id" json:"id"`
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`
}

func PluginsDir(home string) string { return config.PluginsDir(home) }

func DiscoverLocal(home string) ([]LocalPlugin, error) {
	root := PluginsDir(home)
	ents, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, errs.Wrapf(errs.KindConfig, err, "read %s", config.DirPlugins)
	}
	out := make([]LocalPlugin, 0, len(ents))
	for _, ent := range ents {
		if !ent.IsDir() || strings.HasPrefix(ent.Name(), ".") {
			continue
		}
		out = append(out, inspectLocalPlugin(filepath.Join(root, ent.Name()), ent.Name()))
	}
	sortLocal(out)
	resolveLocalIDs(out)
	sortLocal(out)
	return out, nil
}

func sortLocal(out []LocalPlugin) {
	sort.Slice(out, func(i, j int) bool {
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		return out[i].Name < out[j].Name
	})
}

func resolveLocalIDs(out []LocalPlugin) {
	claimed := map[string]string{}
	for i := range out {
		lp := &out[i]
		if owner, dup := claimed[lp.ID]; dup {
			conflict := lp.ID
			if _, taken := claimed[lp.Name]; !taken {
				lp.ID = lp.Name
				_, lp.Registered = Lookup(lp.ID)
			}
			lp.Installable = false
			lp.Reason = fmt.Sprintf("id %q is already claimed by .plugins/%s", conflict, owner)
		}
		if _, ok := claimed[lp.ID]; !ok {
			claimed[lp.ID] = lp.Name
		}
	}
}

func inspectLocalPlugin(dir, name string) LocalPlugin {
	lp := LocalPlugin{Name: name, Dir: dir, ID: name}
	m, ok, err := readLocalManifest(dir)
	if err != nil {
		lp.Reason = err.Error()
		return lp
	}
	if ok {
		claimed := strings.TrimSpace(m.ID)
		if claimed == "" {
			claimed = strings.TrimSpace(m.Name)
		}
		switch {
		case claimed == "" || claimed == name:
		case !validLocalID(claimed):
			lp.Reason = fmt.Sprintf("manifest id %q is not a usable plugin id", claimed)
			return lp
		default:
			if _, registered := Lookup(claimed); registered {
				lp.Reason = fmt.Sprintf("manifest id %q belongs to a plugin built into this binary; rename .plugins/%s to %s to extend it", claimed, name, claimed)
				return lp
			}
			lp.ID = claimed
		}
		lp.Description = strings.TrimSpace(m.Description)
	}
	seeds, err := collectLocalSeeds(dir)
	if err != nil {
		lp.Reason = err.Error()
		return lp
	}
	lp.Seeds = seeds
	_, lp.Registered = Lookup(lp.ID)
	switch {
	case lp.Registered:
		lp.Installable = true
	case len(lp.Seeds) > 0:
		lp.Installable = true
	default:
		lp.Reason = "not linked into this binary and no directive seeds"
	}
	return lp
}

func validLocalID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	if strings.ContainsAny(id, " \t\r\n/\\") {
		return false
	}
	for _, r := range id {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func readLocalManifest(dir string) (localManifest, bool, error) {
	for _, name := range []string{"plugin.yaml", "plugin.yml", "plugin.json"} {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return localManifest{}, false, errs.Wrapf(errs.KindConfig, err, "read %s", name)
		}
		var m localManifest
		switch filepath.Ext(name) {
		case ".json":
			if err := json.Unmarshal(raw, &m); err != nil {
				return localManifest{}, false, errs.Wrapf(errs.KindConfig, err, "parse %s", name)
			}
		default:
			if err := yaml.Unmarshal(raw, &m); err != nil {
				return localManifest{}, false, errs.Wrapf(errs.KindConfig, err, "parse %s", name)
			}
		}
		return m, true, nil
	}
	return localManifest{}, false, nil
}

func collectLocalSeeds(dir string) ([]FileSeed, error) {
	var out []FileSeed
	for _, role := range []string{config.DirQueries, config.DirFlights} {
		roleDir := filepath.Join(dir, role)
		ents, err := os.ReadDir(roleDir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, errs.Wrapf(errs.KindConfig, err, "read %s", role)
		}
		for _, ent := range ents {
			if ent.IsDir() {
				continue
			}
			ext := strings.ToLower(filepath.Ext(ent.Name()))
			if ext != ".yaml" && ext != ".yml" && ext != ".json" {
				continue
			}
			rel := path.Join(role, ent.Name())
			raw, err := os.ReadFile(filepath.Join(roleDir, ent.Name()))
			if err != nil {
				return nil, errs.Wrapf(errs.KindConfig, err, "read %s", rel)
			}
			out = append(out, FileSeed{RelPath: rel, Content: raw})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RelPath < out[j].RelPath })
	return out, nil
}

func ListInstallCandidates(home string) ([]InstallCandidate, error) {
	ensureLoaded()
	local, err := DiscoverLocal(home)
	if err != nil {
		return nil, err
	}
	byID := map[string]LocalPlugin{}
	for _, lp := range local {
		if !lp.Installable {
			continue
		}
		if _, dup := byID[lp.ID]; dup {
			continue
		}
		byID[lp.ID] = lp
	}

	seen := map[string]bool{}
	var out []InstallCandidate
	for _, d := range Primaries() {
		if Installed(d.ID) {
			continue
		}
		c := InstallCandidate{
			ID:          d.ID,
			Label:       d.ID,
			Source:      "registry",
			Installable: true,
		}
		nSeeds := len(SeedsFor(d.ID))
		state := "enabled"
		if !Enabled(d.ID) {
			state = "disabled"
		}
		c.Desc = "available · " + state
		if nSeeds > 0 {
			c.Desc += " · catalog seeds"
		}
		if lp, ok := byID[d.ID]; ok {
			c.Source = "local+registry"
			c.LocalDir = lp.Dir
			c.Seeds = lp.Seeds
			c.Desc += " · .plugins/" + lp.Name
			if lp.Description != "" {
				c.Desc += " · " + lp.Description
			} else if n := len(lp.Seeds); n > 0 {
				c.Desc += " · " + seedCount(n)
			}
			seen[d.ID] = true
		}
		out = append(out, c)
	}
	for _, lp := range local {
		if seen[lp.ID] {
			continue
		}
		if lp.Installable && lp.Registered && Installed(lp.ID) {
			continue
		}
		c := InstallCandidate{
			ID:          lp.ID,
			Label:       lp.ID,
			LocalDir:    lp.Dir,
			Seeds:       lp.Seeds,
			Source:      "local",
			Installable: lp.Installable,
			Reason:      lp.Reason,
		}
		parts := []string{".plugins/" + lp.Name}
		if lp.Description != "" {
			parts = append(parts, lp.Description)
		}
		if n := len(lp.Seeds); n > 0 {
			parts = append(parts, seedCount(n))
		}
		if !lp.Installable {
			parts = append(parts, lp.Reason)
		} else if !lp.Registered {
			parts = append(parts, "seed pack")
		}
		c.Desc = strings.Join(parts, " · ")
		out = append(out, c)
	}
	return out, nil
}

func seedCount(n int) string {
	if n == 1 {
		return "1 seed"
	}
	return fmt.Sprintf("%d seeds", n)
}

func InstallCandidateEntry(home string, c InstallCandidate, opts InstallOptions) (InstallResult, error) {
	if !c.Installable {
		reason := c.Reason
		if reason == "" {
			reason = "not installable"
		}
		return InstallResult{PluginID: c.ID}, errs.Newf(errs.KindConfig, "cannot install %q: %s", c.ID, reason).
			WithHint("place queries/filters/flights seeds under .plugins/<name>/, or link the plugin at compile time")
	}

	res := InstallResult{PluginID: c.ID}
	if _, ok := Lookup(c.ID); ok {
		got, err := Install(home, c.ID, opts)
		if err != nil {
			return got, err
		}
		res = got
	}
	if len(c.Seeds) == 0 && c.LocalDir != "" {
		seeds, err := collectLocalSeeds(c.LocalDir)
		if err != nil {
			return res, err
		}
		c.Seeds = seeds
	}
	if len(c.Seeds) == 0 {
		if res.PluginID == "" {
			res.PluginID = c.ID
		}
		if _, ok := Lookup(c.ID); !ok {
			return res, errs.Newf(errs.KindConfig, "cannot install %q: no seeds and not registered", c.ID)
		}
		return res, nil
	}
	written, skipped, err := writeSeeds(home, c.Seeds, opts)
	if err != nil {
		return res, err
	}
	res.Written = appendUnique(res.Written, written...)
	res.Skipped = appendUnique(res.Skipped, skipped...)
	return res, nil
}

func appendUnique(dst []string, extra ...string) []string {
	seen := map[string]bool{}
	for _, s := range dst {
		seen[s] = true
	}
	for _, s := range extra {
		if seen[s] {
			continue
		}
		seen[s] = true
		dst = append(dst, s)
	}
	return dst
}
