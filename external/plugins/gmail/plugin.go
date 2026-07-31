package gmail

import (
	"github.com/spf13/cobra"

	"github.com/codyconfer/munin/cmd"
	"github.com/codyconfer/munin/external/plugins/internal/googleauth"
	"github.com/codyconfer/munin/external/plugins/internal/params"
	"github.com/codyconfer/munin/plugin"
)

const (
	PluginID   = "external.gmail"
	SignalName = "gmail"
)

var searchTerms = []string{
	"is:unread", "is:starred", "is:important", "in:inbox",
	"has:attachment", "newer_than:2d", "newer_than:7d", "category:primary",
}

func Register() {
	plugin.RegisterSignal(plugin.Descriptor{
		ID:           PluginID,
		Kind:         plugin.KindSignal,
		Signal:       SignalName,
		Capabilities: []plugin.Capability{plugin.CapQuery, plugin.CapCacheable},
	}, plugin.Builders{
		Query: BuildQuery,
	})
	plugin.RegisterQueryParams(SignalName,
		plugin.ParamSpec{Key: "query", Desc: "Gmail search expression", Example: "is:unread in:inbox newer_than:2d", Values: searchTerms, Delim: " "},
		plugin.ParamSpec{Key: "max", Desc: "maximum messages to return", Example: "10", Values: []string{"10", "20", "50", "100"}},
	)
	cmd.RegisterCommand(func() *cobra.Command {
		return cmd.SignalCmd(SignalName, "Matching Gmail messages")
	})
}

func BuildQuery(bc plugin.BuildContext) (plugin.Query, error) {
	ga, err := googleauth.FromBuildContext(bc)
	if err != nil {
		return nil, err
	}
	p := bc.Params()
	s := plugin.SettingsOf(bc, SignalName)
	query := params.Str(p, "query", plugin.Setting(s, "query", "is:unread in:inbox"))
	max := params.Int(p, "max", plugin.SettingInt(s, "max", 15))
	return New(query, max, ga), nil
}
