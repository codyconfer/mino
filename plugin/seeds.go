package plugin

import (
	"path"
	"path/filepath"
	"sort"
	"sync"
)

// FileSeed is an example directive (or other home-relative file) provisioned by
// `munin plugins install`. Content is compile-time data — not a dynamically
// loaded binary.
type FileSeed struct {
	RelPath string
	Content []byte
}

var (
	seedMu      sync.RWMutex
	seedCatalog = map[string][]FileSeed{}
)

// RegisterSeeds associates example directive seeds with a compile-time plugin id.
// Call from app.Options.RegisterPlugins (overlays) or package init (stock).
// Passing a nil/empty files slice clears any prior seeds for that id.
func RegisterSeeds(pluginID string, files []FileSeed) {
	if pluginID == "" {
		return
	}
	seedMu.Lock()
	defer seedMu.Unlock()
	if len(files) == 0 {
		delete(seedCatalog, pluginID)
		return
	}
	dst := make([]FileSeed, len(files))
	for i, f := range files {
		dst[i] = FileSeed{
			RelPath: path.Clean(filepath.ToSlash(f.RelPath)),
			Content: append([]byte(nil), f.Content...),
		}
	}
	seedCatalog[pluginID] = dst
}

// SeedsFor returns a copy of example seeds registered for pluginID (may be empty).
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

// SeedPluginIDs returns plugin ids that have registered example seeds.
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
