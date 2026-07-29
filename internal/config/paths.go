package config

import (
	"context"
	"os"
	"path/filepath"

	"github.com/codyconfer/sisyphus"

	"github.com/codyconfer/munin/internal/errs"
)

const (
	DirData  = ".data"
	DirPanes = "panes"
	ConfigDB = "config.duckdb"
	AuditDB  = "audit.duckdb"
	TokensDB = "tokens.duckdb"
	ServeDB  = "serve.duckdb"
	CacheDB  = "cache.duckdb"
)

const (
	ServeSocket  = "serve.sock"
	SocketPrefix = "munin"
)

func ServeSocketPath(home string) string { return filepath.Join(home, ServeSocket) }

func DataDir(home string) string { return filepath.Join(home, DirData) }

func PanesDir(home string) string { return filepath.Join(DataDir(home), DirPanes) }

func DataPath(home, name string) string {
	dir := DataDir(home)
	_ = os.MkdirAll(dir, 0o700)
	return filepath.Join(dir, name)
}

func OpenStore(ctx context.Context, home string) (*sisyphus.Manager, error) {
	if err := os.MkdirAll(DataDir(home), 0o700); err != nil {
		return nil, errs.Wrapf(errs.KindStore, err, "create %s", DataDir(home))
	}
	return sisyphus.Open(ctx, home, sisyphus.Options{
		Mode:         sisyphus.ModeBoth,
		ConfigDBName: filepath.Join(DirData, ConfigDB),
	})
}
