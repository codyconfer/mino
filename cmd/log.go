package cmd

import (
	"os"
	"path/filepath"

	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/log"
)

func routeLogs(mode string, fullScreen bool) {
	if shared == nil || shared.Cfg == nil {
		return
	}
	if mode == "daemon" {
		return
	}
	dir := config.LogDir(shared.Cfg.Home)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		log.Debugf("log dir unavailable: %v", err)
		return
	}
	if _, err := log.SetFileSink(filepath.Join(dir, "mino.log")); err != nil {
		log.Debugf("log file unavailable: %v", err)
		return
	}
	if mode == "command" || mode == "deck" || fullScreen {
		log.ClearConsole()
	}
}
