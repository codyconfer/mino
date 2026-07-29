package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"maps"
	"slices"
	"strings"

	"github.com/codyconfer/munin/internal/config"
)

const namespacePrefix = "signal:"

func Namespace(signal string) string { return namespacePrefix + signal }

func SignalOf(namespace string) (string, bool) {
	name, ok := strings.CutPrefix(namespace, namespacePrefix)
	return name, ok && name != ""
}

// Fingerprint hashes the config sections that change what a signal fetches. Editing
// github.max or gmail.query busts every cached result; editing output or cache.ttl does not.
func Fingerprint(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	c := *cfg
	c.Home, c.Output, c.Role, c.Timeout = "", "", "", ""
	c.Keybinds = nil
	c.Audit, c.Backup, c.Daemon, c.Cache = config.AuditConfig{}, config.BackupConfig{}, config.DaemonConfig{}, config.CacheConfig{}
	raw, err := json.Marshal(c)
	if err != nil {
		return ""
	}
	return digest(raw)
}

func entryKey(signal, role, fingerprint string, params map[string]string) string {
	var b strings.Builder
	b.WriteString(signal)
	b.WriteByte(0)
	b.WriteString(role)
	b.WriteByte(0)
	for _, k := range slices.Sorted(maps.Keys(params)) {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(params[k])
		b.WriteByte(0)
	}
	b.WriteString(fingerprint)
	return digest([]byte(b.String()))
}

func digest(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])[:16]
}
