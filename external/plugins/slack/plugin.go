package slack

import (
	"github.com/spf13/cobra"

	"github.com/codyconfer/mino/cmd"
	"github.com/codyconfer/mino/external/plugins/internal/errx"
	"github.com/codyconfer/mino/external/plugins/internal/params"
	"github.com/codyconfer/mino/external/plugins/internal/slackauth"
	"github.com/codyconfer/mino/plugin"
)

const (
	PluginID   = "external.slack"
	SignalName = "slack"
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
		plugin.ParamSpec{Key: "channel", Desc: "channel to read (required)", Example: "eng-standup"},
		plugin.ParamSpec{Key: "limit", Desc: "maximum messages to return", Example: "50", Values: []string{"10", "20", "50", "100"}},
	)
	plugin.RegisterLoginProvider(plugin.LoginProvider{
		PluginID: PluginID,
		Key:      "slack",
		Label:    "Slack",
		Fields: []plugin.LoginField{
			{Key: "plugins.slack.oauth_client_id", Label: "OAuth client id", Value: settingValue("oauth_client_id")},
			{Key: "plugins.slack.oauth_client_secret", Label: "OAuth client secret", Secret: true, Value: settingValue("oauth_client_secret")},
		},
		Authed: func(h plugin.Host) bool {
			cfg := slackauth.FromHost(h)
			return slackauth.Authed(cfg.Store, cfg.TokenEnv)
		},
		Login: login,
	})
	cmd.RegisterCommand(newSlackCmd)
}

func settingValue(key string) func(plugin.Host) string {
	return func(h plugin.Host) string {
		if h == nil {
			return ""
		}
		return plugin.Setting(h.Settings(SignalName), key, "")
	}
}

func BuildQuery(bc plugin.BuildContext) (plugin.Query, error) {
	cfg, err := slackauth.FromBuildContext(bc)
	if err != nil {
		return nil, err
	}
	p := bc.Params()
	channel := p["channel"]
	if channel == "" {
		return nil, errx.New("slack: a channel is required").WithHint("use --channel or a query param")
	}
	token, err := slackauth.UserToken(cfg.Store, cfg.TokenEnv)
	if err != nil {
		return nil, errx.Wrap(err, "slack authentication").WithHint("set %s", cfg.TokenEnv)
	}
	return New(token, channel, params.Int(p, "limit", cfg.Limit)), nil
}

func BuildStream(bc plugin.BuildContext) (plugin.Stream, error) {
	cfg, err := slackauth.FromBuildContext(bc)
	if err != nil {
		return nil, err
	}
	appToken, err := slackauth.AppToken(cfg.Store, cfg.AppTokenEnv)
	if err != nil || appToken == "" {
		return nil, errx.New("slack: no app-level token available for streaming")
	}
	botToken, err := slackauth.BotToken(cfg.Store, cfg.BotTokenEnv)
	if err != nil || botToken == "" {
		return nil, errx.New("slack: no bot token available for streaming")
	}
	return NewActive(botToken, appToken), nil
}

func newSlackCmd() *cobra.Command {
	return cmd.QueryCmd(SignalName, "Slack channel activity", func(c *cobra.Command, params *map[string]string) {
		var channel string
		c.Flags().StringVar(&channel, "channel", "", "channel to read (name or ID)")
		c.PreRun = func(*cobra.Command, []string) {
			if channel != "" {
				(*params)["channel"] = channel
			}
		}
	})
}
