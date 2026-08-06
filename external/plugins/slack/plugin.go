package slack

import (
	"strconv"

	"github.com/codyconfer/viewkit/glyph"
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
	if _, ok := plugin.Lookup(PluginID); ok {
		return
	}
	plugin.RegisterSignal(plugin.Descriptor{
		ID:                 PluginID,
		Kind:               plugin.KindSignal,
		Signal:             SignalName,
		Capabilities:       []plugin.Capability{plugin.CapQuery, plugin.CapStream, plugin.CapCacheable, plugin.CapDetail},
		Credentials:        []string{"slack", "slack-app", "slack-bot"},
		SettingsNamespaces: []string{SignalName},
	}, plugin.Builders{
		Query:  BuildQuery,
		Stream: BuildStream,
	})
	plugin.RegisterQueryParams(SignalName,
		plugin.ParamSpec{Key: "channel", Desc: "channel(s) to read", Example: "eng-standup,#alerts", Delim: ","},
		plugin.ParamSpec{Key: "mentions", Desc: "messages that mention you (needs search:read)", Example: "true", Values: []string{"true", "false"}},
		plugin.ParamSpec{Key: "dms", Desc: "recent direct-message conversations to include", Example: "5", Values: []string{"3", "5", "10"}},
		plugin.ParamSpec{Key: "search", Desc: "Slack search expression (needs search:read)", Example: "in:#eng deploy"},
		plugin.ParamSpec{Key: "mention_query", Desc: "extra search terms for mentions", Example: "in:#eng"},
		plugin.ParamSpec{Key: "limit", Desc: "maximum messages per surface", Example: "50", Values: []string{"10", "20", "50", "100"}},
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
	plugin.RegisterStatusContribution(PluginID, func(_, _ string) glyph.StatusContribution {
		return StatusContribution()
	})
	plugin.RegisterSeeds(PluginID, seedFiles())
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
	token, err := slackauth.UserToken(cfg.Store, cfg.TokenEnv)
	if err != nil {
		return nil, errx.Wrap(err, "slack authentication").WithHint("set %s", cfg.TokenEnv)
	}
	p := bc.Params()
	s := plugin.SettingsOf(bc, SignalName)

	return NewSpec(Spec{
		Token:        token,
		Channels:     ChannelList(params.Str(p, "channel", plugin.Setting(s, "channel", ""))),
		Mentions:     boolParam(p, "mentions", plugin.SettingBool(s, "mentions", false)),
		MentionQuery: params.Str(p, "mention_query", plugin.Setting(s, "mention_query", "")),
		DMs:          params.Int(p, "dms", plugin.SettingInt(s, "dms", 0)),
		Search:       params.Str(p, "search", ""),
		Limit:        params.Int(p, "limit", cfg.Limit),
		ResolveNames: plugin.SettingBool(s, "resolve_names", true),
		Workspace:    plugin.Setting(s, "workspace", ""),
		RetryMax:     plugin.SettingInt(s, "retry_max", retryMaxDefault),
	}), nil
}

func boolParam(p map[string]string, key string, def bool) bool {
	switch params.Str(p, key, "") {
	case "":
		return def
	case "false", "0", "no":
		return false
	default:
		return true
	}
}

func BuildStream(bc plugin.BuildContext) (plugin.Stream, error) {
	cfg, err := slackauth.FromBuildContext(bc)
	if err != nil {
		return nil, err
	}
	appToken, err := slackauth.AppToken(cfg.Store, cfg.AppTokenEnv)
	if err != nil || appToken == "" {
		return nil, errx.New("slack: no app-level token available for streaming").
			WithHint("export an app-level token ($%s=xapp-…) with connections:write", cfg.AppTokenEnv)
	}
	botToken, err := slackauth.BotToken(cfg.Store, cfg.BotTokenEnv)
	if err != nil || botToken == "" {
		return nil, errx.New("slack: no bot token available for streaming").
			WithHint("export a bot token ($%s=xoxb-…)", cfg.BotTokenEnv)
	}
	s := plugin.SettingsOf(bc, SignalName)
	return NewActive(botToken, appToken, ActiveOptions{
		Channels:  plugin.SettingList(s, "stream_channels"),
		Workspace: plugin.Setting(s, "workspace", ""),
	}), nil
}

func newSlackCmd() *cobra.Command {
	return cmd.QueryCmd(SignalName, "Slack channel activity, mentions, DMs and search", func(c *cobra.Command, p *map[string]string) {
		var (
			channel  string
			search   string
			mentions bool
			dms      int
		)
		c.Flags().StringVar(&channel, "channel", "", "channel(s) to read, comma separated (name or ID)")
		c.Flags().StringVar(&search, "search", "", "Slack search expression")
		c.Flags().BoolVar(&mentions, "mentions", false, "include messages that mention you")
		c.Flags().IntVar(&dms, "dms", 0, "include this many recent DM conversations")
		c.PreRun = func(*cobra.Command, []string) {
			if channel != "" {
				(*p)["channel"] = channel
			}
			if search != "" {
				(*p)["search"] = search
			}
			if mentions {
				(*p)["mentions"] = "true"
			}
			if dms > 0 {
				(*p)["dms"] = strconv.Itoa(dms)
			}
		}
	})
}
