package cmd

import (
	"os"
	"sync"

	"github.com/spf13/cobra"

	"github.com/codyconfer/mino/internal/render"
)

var (
	launchLoadMu sync.Mutex
	launchLoad   *render.Loading
)

func wantsLaunchLoading(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	return cmd.Annotations[AnnoLaunchLoading] == "true"
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
		UI:          Scope(),
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
