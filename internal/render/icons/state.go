package icons

import (
	"path/filepath"

	sconfig "github.com/codyconfer/sisyphus/config"
	"github.com/codyconfer/sisyphus/daemon"
)

var iconExtMIME = map[string]string{
	".png": "image/png",
	".ico": "image/x-icon",
}

func LoadStateIcons(home, theme string) int {
	Register(theme)
	dir := filepath.Join(home, "icons")
	loaded := 0
	for _, s := range daemon.States() {
		for _, ext := range overrideExts() {
			raw, ok, err := sconfig.ReadRaw(filepath.Join(dir, s.String()+ext))
			if err != nil || !ok {
				continue
			}
			mime, raw := prepareAsset(iconExtMIME[ext], raw)
			daemon.SetStateIcon(s, daemon.Asset{Name: "state:" + s.String(), MIME: mime, Bytes: raw})
			loaded++
			break
		}
	}
	return loaded
}
