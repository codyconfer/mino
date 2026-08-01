package plugin

import (
	"strings"

	"github.com/codyconfer/sisyphus/daemon"
)

// ScopeKV confines a raw daemon.KV handle to one plugin's own namespaces via
// daemon.ScopedKV. Callers that hand a KV to plugin code must scope it first,
// or the plugin could read or wipe another signal's persisted cursors.
func ScopeKV(raw daemon.KV, ownerID string) daemon.KV {
	return daemon.ScopedKV(raw, KVNamespacePrefix(ownerID))
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
