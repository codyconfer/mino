package plugin

import (
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type FileSeed struct {
	RelPath string
	Content []byte
}

var (
	seedMu      sync.RWMutex
	seedCatalog = map[string][]FileSeed{}
)

func RegisterSeeds(pluginID string, files []FileSeed) {
	if pluginID == "" {
		return
	}
	noteRegistrationCheckpoint(pluginID)
	if len(files) == 0 {
		seedMu.Lock()
		delete(seedCatalog, pluginID)
		seedMu.Unlock()
		return
	}
	dst := make([]FileSeed, 0, len(files))
	var rejected []string
	for _, f := range files {
		rel, ok := cleanSeedRel(f.RelPath)
		if !ok {
			rejected = append(rejected, f.RelPath)
			continue
		}
		dst = append(dst, FileSeed{
			RelPath: rel,
			Content: append([]byte(nil), f.Content...),
		})
	}

	seedMu.Lock()
	if len(dst) == 0 {
		delete(seedCatalog, pluginID)
	} else {
		seedCatalog[pluginID] = dst
	}
	seedMu.Unlock()

	for _, rel := range rejected {
		noteDiagnosticf(pluginID, "", "",
			"seed path %q is not a safe path relative to the munin home; seed skipped", rel)
	}
	if len(dst) == 0 {
		noteDiagnosticf(pluginID, "", "",
			"every seed was rejected, so this plugin registered no seeds; `munin plugins install %s` will write nothing", pluginID)
	}
}

func cleanSeedRel(rel string) (string, bool) {
	rel = strings.TrimSpace(rel)
	if rel == "" || filepath.IsAbs(rel) || filepath.VolumeName(rel) != "" {
		return "", false
	}
	slash := strings.ReplaceAll(rel, `\`, "/")
	if strings.HasPrefix(slash, "/") {
		return "", false
	}
	clean := path.Clean(slash)
	switch {
	case clean == "." || clean == "..":
		return "", false
	case strings.HasPrefix(clean, "../"), strings.HasPrefix(clean, "/"):
		return "", false
	}
	return clean, true
}

func SeedsFor(pluginID string) []FileSeed {
	seedMu.RLock()
	defer seedMu.RUnlock()
	src := seedCatalog[pluginID]
	if len(src) == 0 {
		return nil
	}
	out := make([]FileSeed, len(src))
	for i, f := range src {
		out[i] = FileSeed{
			RelPath: f.RelPath,
			Content: append([]byte(nil), f.Content...),
		}
	}
	return out
}

func SeedPluginIDs() []string {
	seedMu.RLock()
	defer seedMu.RUnlock()
	out := make([]string, 0, len(seedCatalog))
	for id, files := range seedCatalog {
		if len(files) > 0 {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}
