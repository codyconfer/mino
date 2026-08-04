package config

import "strings"

func (c *Config) GitProvider() string {
	if c == nil {
		return ""
	}
	return strings.TrimSpace(c.Git.Provider)
}

func (c *Config) GitSettings(provider string) func(string) string {
	if c == nil {
		return func(string) string { return "" }
	}
	if provider == "github" {
		return c.githubSetting
	}
	section := c.PluginSettings(provider)
	return func(key string) string {
		v, ok := section[settingKey(key)]
		if !ok {
			return ""
		}
		s, _ := v.(string)
		return s
	}
}

func (c *Config) githubSetting(key string) string {
	switch key {
	case "api_url":
		return c.GitHub.APIURL
	case "service_token":
		return c.GitHub.ServiceToken
	case "viewer":
		return c.GitHub.Viewer
	case "app.id":
		return c.GitHub.App.ID
	case "app.installation_id":
		return c.GitHub.App.InstallationID
	case "app.private_key_path":
		return c.GitHub.App.PrivateKeyPath
	case "oauth_client_id":
		return c.GitHub.OAuthClientID
	case "oauth_scopes":
		return c.GitHub.OAuthScopes
	}
	return ""
}
