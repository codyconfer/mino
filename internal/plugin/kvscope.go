package plugin

import (
	"context"
	"strings"
	"time"

	"github.com/codyconfer/sisyphus/daemon"
	"github.com/codyconfer/sisyphus/kv"
)

// ScopeKV confines a raw daemon.KV handle to one plugin's own namespaces.
// daemon.KV is Get/Put/Delete/List across *every* namespace, so a plugin handed
// the raw handle can read or wipe any other signal's persisted cursors. Callers
// that hand a KV to plugin code must scope it first.
func ScopeKV(raw daemon.KV, ownerID string) daemon.KV {
	if raw == nil {
		return nil
	}
	return scopedKV{kv: raw, prefix: KVNamespacePrefix(ownerID)}
}

// KVNamespacePrefix is the namespace prefix owned by ownerID. "/" is stripped
// from the id so no owner's prefix can contain the separator, and therefore no
// owner can address another owner's namespaces.
func KVNamespacePrefix(ownerID string) string {
	if strings.TrimSpace(ownerID) == "" {
		ownerID = "unattributed"
	}
	return strings.ReplaceAll(ownerID, "/", "_") + "/"
}

type scopedKV struct {
	kv     daemon.KV
	prefix string
}

func (s scopedKV) scope(namespace string) string { return s.prefix + namespace }

func (s scopedKV) Get(ctx context.Context, namespace, key string) (kv.Entry, bool, error) {
	return s.kv.Get(ctx, s.scope(namespace), key)
}

func (s scopedKV) Put(ctx context.Context, namespace, key, value string, expiry time.Time) error {
	return s.kv.Put(ctx, s.scope(namespace), key, value, expiry)
}

func (s scopedKV) Delete(ctx context.Context, namespace, key string) error {
	return s.kv.Delete(ctx, s.scope(namespace), key)
}

func (s scopedKV) List(ctx context.Context, namespace string) (map[string]kv.Entry, error) {
	return s.kv.List(ctx, s.scope(namespace))
}
