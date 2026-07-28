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

type Option = pub.Option

func WithServiceOnly() Option { return pub.WithServiceOnly() }

func ServiceAttachedAt(home string) bool {
	if home == "" {
		return false
	}
	return sysdaemon.Attached(servePipePrefix, filepath.Join(home, serveSocketName))
}

func ServiceAttached() bool {
	home, err := config.Home("")
	if err != nil || home == "" {
		return false
	}
	return ServiceAttachedAt(home)
}

func UIVisible(d Descriptor) bool { return pub.UIVisible(d) }

func ViewUIVisible(viewID string) bool { return pub.ViewUIVisible(viewID) }

func ActionUIVisible(signal, name string) bool {
	return pub.ActionUIVisible(signal, name)
}
