package cmd

import (
	"os"
	"sync"

	"github.com/spf13/cobra"

	"github.com/codyconfer/munin/internal/render"
)

var (
	launchLoadMu sync.Mutex
	launchLoad   *render.Loading
)

func wantsLaunchLoading(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	switch cmd.Name() {
	case "deck", "tui":
		return true
	case "daemon":
		p := cmd.Parent()
		return p != nil && p.Parent() == nil
	}
	return false
}

func startLaunchLoading() {
	launchLoadMu.Lock()
	defer launchLoadMu.Unlock()
	if launchLoad != nil {
		return
	}
	launchLoad = render.StartLoading(render.LoadingOptions{
		Writer:      os.Stderr,
		Message:     "starting…",
		DoneMessage: "ready",
	})
}

func stopLaunchLoading() {
	launchLoadMu.Lock()
	defer launchLoadMu.Unlock()
	if launchLoad == nil {
		return
	}
	launchLoad.Done()
	launchLoad = nil
}
