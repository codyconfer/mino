package plugin

import "sync"

var (
	pathMu         sync.Mutex
	knownDataPaths []func(home string) string
)

func RegisterDataPath(fn func(home string) string) {
	if fn == nil {
		return
	}
	pathMu.Lock()
	knownDataPaths = append(knownDataPaths, fn)
	pathMu.Unlock()
}

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
