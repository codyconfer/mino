package docs

import (
	"github.com/spf13/cobra"

	"github.com/codyconfer/mino/cmd"
	"github.com/codyconfer/mino/external/plugins/internal/googleauth"
	"github.com/codyconfer/mino/external/plugins/internal/params"
	"github.com/codyconfer/mino/plugin"
)

const (
	PluginID   = "external.docs"
	SignalName = "docs"
)

func Register() {
	if _, ok := plugin.Lookup(PluginID); ok {
		return
	}
	plugin.RegisterSignal(plugin.Descriptor{
		ID:                 PluginID,
		Kind:               plugin.KindSignal,
		Signal:             SignalName,
		Capabilities:       []plugin.Capability{plugin.CapQuery, plugin.CapCacheable},
		Credentials:        []string{"google"},
		SettingsNamespaces: []string{SignalName, "google"},
	}, plugin.Builders{
		Query: BuildQuery,
	})
	plugin.RegisterQueryParams(SignalName,
		plugin.ParamSpec{Key: "recent", Desc: "how many recent documents to list", Example: "10", Values: []string{"10", "20", "50", "100"}},
	)
	cmd.RegisterCommand(func() *cobra.Command {
		return cmd.SignalCmd(SignalName, "Recently modified Google Docs")
	})
}

func BuildQuery(bc plugin.BuildContext) (plugin.Query, error) {
	ga, err := googleauth.FromBuildContext(bc)
	if err != nil {
		return nil, err
	}
	s := plugin.SettingsOf(bc, SignalName)
	recent := params.Int(bc.Params(), "recent", plugin.SettingInt(s, "recent", 10))
	return New(recent, ga), nil
}
