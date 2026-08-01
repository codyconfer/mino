package plugin

import (
	"strings"

	"github.com/codyconfer/sisyphus/stream"
)

// ScopeKV confines a raw stream.KV handle to one plugin's own namespaces via
// stream.ScopedKV. Callers that hand a KV to plugin code must scope it first,
// or the plugin could read or wipe another signal's persisted cursors.
func ScopeKV(raw stream.KV, ownerID string) stream.KV {
	return stream.ScopedKV(raw, KVNamespacePrefix(ownerID))
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
