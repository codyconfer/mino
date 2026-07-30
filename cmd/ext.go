package cmd

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/codyconfer/munin/internal/app"
)

const AnnoLaunchLoading = "munin_launch_loading"

const (
	AnnoGateMode = annoGateMode
	ModeDaemon   = modeDaemon
)

type CommandFunc func() *cobra.Command

var registrations []CommandFunc

func RegisterCommand(fn CommandFunc) {
	registrations = append(registrations, fn)
}

func registered() []*cobra.Command {
	out := make([]*cobra.Command, 0, len(registrations))
	for _, fn := range registrations {
		out = append(out, fn())
	}
	return out
}

func App() *app.App { return shared }

func ResolveFlight(args []string) (string, error) { return resolveServeFlight(args) }

func DefaultFlight() string { return defaultFlightName() }

func CompleteFlights(c *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return completeFlightNames(c, args, toComplete)
}

func ServeInterval() time.Duration { return configServeInterval() }

func CheckServeInterval(fromFlag bool, d time.Duration) error { return checkServeInterval(fromFlag, d) }

func ServeTheme() string { return configServeTheme() }

func StartLoading() { startLaunchLoading() }

func StopLoading() { stopLaunchLoading() }
