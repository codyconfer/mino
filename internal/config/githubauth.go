package config

import "github.com/codyconfer/mino/internal/auth"

func (g GitHubConfig) AuthSpec(base string, store auth.TokenStore) auth.GitHubSpec {
	return auth.GitHubSpec{
		APIURL:       base,
		ServiceToken: g.ServiceToken,
		Store:        store,
		App: auth.GitHubAppSpec{
			ID:             g.App.ID,
			InstallationID: g.App.InstallationID,
			PrivateKeyPath: g.App.PrivateKeyPath,
		},
	}
}
