package calendar

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/codyconfer/mino/cmd"
	"github.com/codyconfer/mino/external/plugins/internal/googleauth"
	"github.com/codyconfer/mino/external/plugins/internal/params"
	"github.com/codyconfer/mino/external/plugins/internal/stream"
	"github.com/codyconfer/mino/plugin"
)

const (
	PluginID   = "external.calendar"
	SignalName = "calendar"
)

func Register() {
	plugin.RegisterSignal(plugin.Descriptor{
		ID:           PluginID,
		Kind:         plugin.KindSignal,
		Signal:       SignalName,
		Capabilities: []plugin.Capability{plugin.CapQuery, plugin.CapStream, plugin.CapCacheable},
	}, plugin.Builders{
		Query:  BuildQuery,
		Stream: BuildStream,
	})
	plugin.RegisterQueryParams(SignalName,
		plugin.ParamSpec{Key: "calendar_id", Desc: "calendar to read", Example: "primary", Values: []string{"primary"}},
		plugin.ParamSpec{Key: "window", Desc: "how far ahead to look", Example: "12h", Values: []string{"4h", "8h", "12h", "24h", "72h", "168h"}},
		plugin.ParamSpec{Key: "max", Desc: "maximum events to return", Example: "20", Values: []string{"10", "20", "50", "100"}},
	)
	cmd.RegisterCommand(func() *cobra.Command {
		c := cmd.SignalCmd(SignalName, "Upcoming Google Calendar events")
		c.Aliases = []string{"cal"}
		return c
	})
}

func BuildQuery(bc plugin.BuildContext) (plugin.Query, error) {
	ga, err := googleauth.FromBuildContext(bc)
	if err != nil {
		return nil, err
	}
	p := bc.Params()
	s := plugin.SettingsOf(bc, SignalName)
	calID := params.Str(p, "calendar_id", plugin.Setting(s, "calendar_id", "primary"))
	window := params.Duration(p, "window", params.Window(plugin.Setting(s, "window", ""), defaultWindow))
	max := params.Int(p, "max", plugin.SettingInt(s, "max", 50))
	return New(calID, window, max, ga), nil
}

func BuildStream(bc plugin.BuildContext) (plugin.Stream, error) {
	ga, err := googleauth.FromBuildContext(bc)
	if err != nil {
		return nil, err
	}
	p := bc.Params()
	s := plugin.SettingsOf(bc, SignalName)
	calID := params.Str(p, "calendar_id", plugin.Setting(s, "calendar_id", "primary"))
	interval, err := params.PollInterval(p, SignalName, 60*time.Second)
	if err != nil {
		return nil, err
	}
	return NewActive(calID, ga, interval, stream.StateOf(bc)), nil
}
