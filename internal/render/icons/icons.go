package icons

import (
	"embed"

	"github.com/codyconfer/sisyphus/daemon"
)

//go:embed data
var files embed.FS

func Register(theme string) {
	if theme != "light" {
		theme = "dark"
	}
	for _, s := range daemon.States() {
		b, err := files.ReadFile("data/" + theme + "/" + s.String() + ".png")
		if err != nil || len(b) == 0 {
			continue
		}
		mime, b := prepareAsset("image/png", b)
		daemon.SetStateIcon(s, daemon.Asset{Name: "state:" + s.String(), MIME: mime, Bytes: b})
	}
}
