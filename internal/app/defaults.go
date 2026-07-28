package app

import (
	"io/fs"
	"sync"
)

var (
	defaultsMu sync.RWMutex
	defaultsFS fs.FS
)

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
