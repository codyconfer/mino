package config

import (
	"context"
	"path/filepath"

	"github.com/codyconfer/sisyphus"
	sconfig "github.com/codyconfer/sisyphus/config"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/log"
)

const (
	DirData  = ".data"
	DirPanes = "panes"
	ConfigDB = "config.duckdb"
	AuditDB  = "audit.duckdb"
	TokensDB = "tokens.duckdb"
	ServeDB  = "serve.duckdb"
	CacheDB  = "cache.duckdb"
	StateDB  = "state.duckdb"
)

const (
	ServeSocket  = "serve.sock"
	SocketPrefix = "mino"
)

// Defaults for the HTTP trigger API serve can expose.
const (
	HTTPTokenFile            = "http.token"
	HTTPLoopback             = "127.0.0.1"
	DefaultHTTPHost          = HTTPLoopback
	DefaultHTTPPort          = 7717
	DefaultHTTPMaxConcurrent = 4
)

func ServeSocketPath(home string) string { return filepath.Join(home, ServeSocket) }

// HTTPTokenPath is where a generated API bearer token is persisted.
func HTTPTokenPath(home string) string { return filepath.Join(DataDir(home), HTTPTokenFile) }

func DataDir(home string) string { return filepath.Join(home, DirData) }

func PanesDir(home string) string { return filepath.Join(DataDir(home), DirPanes) }

func DataPath(home, name string) string {
	dir := DataDir(home)
	if err := sconfig.EnsureDir(dir); err != nil {
		log.Warnf("data dir unavailable: %v", err)
	}
	return filepath.Join(dir, name)
}

func OpenStore(ctx context.Context, home string) (*sisyphus.ConfigStore, error) {
	if err := sconfig.EnsureDir(DataDir(home)); err != nil {
		return nil, errs.Wrap(errs.KindStore, err, "open store")
	}
	mgr, err := sisyphus.Open(ctx, home, sisyphus.Options{
		Backend:      sisyphus.BackendBoth,
		ConfigDBName: filepath.Join(DirData, ConfigDB),
	})
	if err != nil {
		return nil, err
	}
	dropLegacyRows(ctx, mgr)
	return mgr, nil
}

func dropLegacyRows(ctx context.Context, mgr *sisyphus.ConfigStore) {
	for _, name := range LegacyDirectiveRows() {
		if _, ok, err := mgr.Current(ctx, name); err != nil || !ok {
			continue
		}
		if err := mgr.Forget(ctx, name); err != nil {
			log.Debugf("dropping legacy %s row: %v", name, err)
		}
	}
}
