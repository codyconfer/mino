package google

import (
	"context"
	"io"

	"github.com/codyconfer/mino/external/plugins/internal/googleauth"
	"github.com/codyconfer/mino/plugin"
)

const PluginID = "external.google"

func Register() {
	plugin.RegisterLoginProvider(plugin.LoginProvider{
		PluginID: PluginID,
		Key:      "google",
		Label:    "Google",
		Signals:  []string{"calendar", "gmail", "docs", "drive", "tasks"},
		Fields: []plugin.LoginField{
			{Key: "plugins.google.oauth_client_id", Label: "OAuth client id", Value: settingValue("oauth_client_id")},
			{Key: "plugins.google.oauth_client_secret", Label: "OAuth client secret", Secret: true, Value: settingValue("oauth_client_secret")},
		},
		Authed: func(h plugin.Host) bool {
			return googleauth.Authed(googleauth.FromHost(h).Store)
		},
		Login: login,
	})
}

func settingValue(key string) func(plugin.Host) string {
	return func(h plugin.Host) string {
		if h == nil {
			return ""
		}
		return plugin.Setting(h.Settings("google"), key, "")
	}
}

func login(ctx context.Context, h plugin.Host, creds map[string]string, w io.Writer) error {
	ga := googleauth.FromHost(h)
	if v := creds["plugins.google.oauth_client_id"]; v != "" {
		ga.ClientID = v
	}
	if v := creds["plugins.google.oauth_client_secret"]; v != "" {
		ga.ClientSecret = v
	}
	return googleauth.Login(ctx, ga, w)
}
