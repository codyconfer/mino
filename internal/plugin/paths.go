package plugin

import "sync"

var (
	pathMu sync.Mutex
	// knownDataPaths are home-relative (or absolute) plugin DB paths that should
	// join encrypted backups even before the store is opened (ADR-11).
	knownDataPaths []func(home string) string
)

// RegisterDataPath records a function that returns a plugin DB path under home.
// Backup runners should union these with sisyphus/store.BackupPaths().
func RegisterDataPath(fn func(home string) string) {
	if fn == nil {
		return
	}
	pathMu.Lock()
	knownDataPaths = append(knownDataPaths, fn)
	pathMu.Unlock()
}

// DataPaths returns known plugin data paths for home (deduped, non-empty).
func DataPaths(home string) []string {
	pathMu.Lock()
	fns := append([]func(string) string{}, knownDataPaths...)
	pathMu.Unlock()
	seen := map[string]bool{}
	var out []string
	for _, fn := range fns {
		p := fn(home)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}
