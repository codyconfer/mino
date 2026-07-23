package icons

import (
	"path/filepath"

	sconfig "github.com/codyconfer/sisyphus/config"
	"github.com/codyconfer/sisyphus/daemon"
)

var iconExtMIME = map[string]string{
	".png": "image/png",
	".ico": "image/x-icon",
	".svg": "image/svg+xml",
}

func LoadStateIcons(home, theme string) int {
	Register(theme)
	dir := filepath.Join(home, "icons")
	loaded := 0
	for _, s := range daemon.States() {
		for _, ext := range []string{".png", ".ico", ".svg"} {
			raw, ok, err := sconfig.ReadRaw(filepath.Join(dir, s.String()+ext))
			if err != nil || !ok {
				continue
			}
			daemon.SetStateIcon(s, daemon.Asset{Name: "state:" + s.String(), MIME: iconExtMIME[ext], Bytes: raw})
			loaded++
			break
		}
	}
	return loaded
}
