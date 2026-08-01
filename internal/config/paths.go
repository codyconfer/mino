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
)

const (
	ServeSocket  = "serve.sock"
	SocketPrefix = "mino"
)

func ServeSocketPath(home string) string { return filepath.Join(home, ServeSocket) }

func DataDir(home string) string { return filepath.Join(home, DirData) }

func PanesDir(home string) string { return filepath.Join(DataDir(home), DirPanes) }

func DataPath(home, name string) string {
	dir := DataDir(home)
	if err := sconfig.EnsureDir(dir); err != nil {
		log.Warnf("data dir unavailable: %v", err)
	}
	return filepath.Join(dir, name)
}

func OpenStore(ctx context.Context, home string) (*sisyphus.Manager, error) {
	if err := sconfig.EnsureDir(DataDir(home)); err != nil {
		return nil, errs.Wrap(errs.KindStore, err, "open store")
	}
	mgr, err := sisyphus.Open(ctx, home, sisyphus.Options{
		Mode:         sisyphus.ModeBoth,
		ConfigDBName: filepath.Join(DirData, ConfigDB),
	})
	if err != nil {
		return nil, err
	}
	dropLegacyRows(ctx, mgr)
	return mgr, nil
}

func dropLegacyRows(ctx context.Context, mgr *sisyphus.Manager) {
	db := mgr.DB()
	if db == nil {
		return
	}
	for _, name := range LegacyDirectiveRows() {
		if _, ok, err := db.Current(ctx, name); err != nil || !ok {
			continue
		}
		if err := db.Forget(ctx, name); err != nil {
			log.Debugf("dropping legacy %s row: %v", name, err)
		}
	}
}
