package app

import (
	"os"

	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/keymap"
	"github.com/codyconfer/mino/internal/log"
	"github.com/codyconfer/mino/internal/render/glyph"
)

func Bootstrap() {
	glyph.Resolve()
	keymap.Register()
	applyLogLevel()
	applyLogColor()
}

func applyLogLevel() {
	lvl := os.Getenv("MINO_LOG_LEVEL")
	if lvl == "" {
		lvl = config.LoadGlobalSettings().LogLevel
	}
	if l, ok := log.ParseLevel(lvl); ok {
		log.SetLevel(l)
	}
}

func applyLogColor() {
	c := os.Getenv("MINO_LOG_COLOR")
	if c == "" {
		c = config.LoadGlobalSettings().LogColor
	}
	if m, ok := log.ParseColorMode(c); ok {
		log.SetColorMode(m)
	}
}
