package plugin

import (
	"path/filepath"

	sysdaemon "github.com/codyconfer/sisyphus/daemon"

	"github.com/codyconfer/munin/internal/config"
	pub "github.com/codyconfer/munin/plugin"
)

const serveSocketName = "serve.sock"
const servePipePrefix = "munin"

func init() {
	pub.SetServiceAttachedFunc(ServiceAttached)
}

// Option configures a contribution Descriptor at registration time.
type Option = pub.Option

// WithServiceOnly marks a contribution as service-only (re-export).
func WithServiceOnly() Option { return pub.WithServiceOnly() }

// ServiceAttachedAt reports whether a live serve/daemon socket exists at home.
// Under nodaemon builds, daemon.Attached is always false.
func ServiceAttachedAt(home string) bool {
	if home == "" {
		return false
	}
	return sysdaemon.Attached(servePipePrefix, filepath.Join(home, serveSocketName))
}

// ServiceAttached reports whether a live munin serve/daemon socket exists
// for the active home (MUNIN_HOME / global home / default ~/.munin).
func ServiceAttached() bool {
	home, err := config.Home("")
	if err != nil || home == "" {
		return false
	}
	return ServiceAttachedAt(home)
}

// UIVisible reports whether d should appear in interactive UI lists.
func UIVisible(d Descriptor) bool { return pub.UIVisible(d) }

// ViewUIVisible reports whether a KindView contribution should appear in UI.
func ViewUIVisible(viewID string) bool { return pub.ViewUIVisible(viewID) }

// ActionUIVisible reports whether a KindAction contribution should appear in UI.
func ActionUIVisible(signal, name string) bool {
	return pub.ActionUIVisible(signal, name)
}
