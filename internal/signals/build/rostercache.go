package build

import (
	"context"
	"time"

	"github.com/codyconfer/sisyphus/kv"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/log"
)

type rosterCache struct {
	home string
}

func (c rosterCache) open(ctx context.Context) (*kv.Store, bool) {
	if c.home == "" {
		return nil, false
	}
	store, err := kv.Open(ctx, config.DataPath(c.home, config.ServeDB))
	if err != nil {
		log.Debugf("github: team cache unavailable: %v", err)
		return nil, false
	}
	return store, true
}

func (c rosterCache) Get(ctx context.Context, namespace, key string) (string, bool) {
	store, ok := c.open(ctx)
	if !ok {
		return "", false
	}
	defer store.Close()
	entry, found, err := store.Get(ctx, namespace, key)
	if err != nil {
		log.Debugf("github: team cache read failed: %v", err)
		return "", false
	}
	return entry.Value, found
}

func (c rosterCache) Put(ctx context.Context, namespace, key, value string, expiry time.Time) {
	store, ok := c.open(ctx)
	if !ok {
		return
	}
	defer store.Close()
	if err := store.Put(ctx, namespace, key, value, expiry); err != nil {
		log.Debugf("github: team cache write failed: %v", err)
	}
}
