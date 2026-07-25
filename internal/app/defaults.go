package app

import (
	"io/fs"
	"sync"
)

var (
	defaultsMu sync.RWMutex
	defaultsFS fs.FS
)

// SetDefaultsFS records an optional overlay seed filesystem used by Install.
// Safe to call from the public app.Run hook before CLI.
func SetDefaultsFS(fsys fs.FS) {
	defaultsMu.Lock()
	defaultsFS = fsys
	defaultsMu.Unlock()
}

func getDefaultsFS() fs.FS {
	defaultsMu.RLock()
	defer defaultsMu.RUnlock()
	return defaultsFS
}
