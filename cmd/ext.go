package cmd

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/codyconfer/munin/internal/app"
	"github.com/codyconfer/munin/internal/pluginhost"
	"github.com/codyconfer/munin/internal/signals/build"
	"github.com/codyconfer/munin/plugin"
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

func Host() plugin.Host { return pluginhost.New(shared.Cfg, shared.Tokens) }

func SignalCmd(name, short string) *cobra.Command { return sourceCmd(name, short) }

func QueryCmd(name, short string, bind func(*cobra.Command, *map[string]string)) *cobra.Command {
	parent := &cobra.Command{Use: name, Short: short}
	params := map[string]string{}
	var ff filterFlags
	query := &cobra.Command{
		Use:   "query",
		Short: "Fetch " + name + " now, with optional filters",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return runSignal(c, name, params, &ff)
		},
	}
	if bind != nil {
		bind(query, &params)
	}
	ff.bind(query)
	parent.AddCommand(query)
	return parent
}

func EmitSections(c *cobra.Command, root string, sections []plugin.Section) error {
	return emit(c.OutOrStdout(), root, sections)
}

func RecordAction(label string, started, finished time.Time, sections []plugin.Section) {
	shared.Audit.RecordAction(label, shared.Role(), started, finished, sections)
}

func ResolveWriteTarget(what, setting, configured, requested string) (string, error) {
	return build.ResolveWriteTarget(what, setting, configured, requested)
}

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
