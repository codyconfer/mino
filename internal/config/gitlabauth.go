package config

import "github.com/codyconfer/mino/internal/auth"

func (g GitLabConfig) AuthSpec(base string, store auth.TokenStore) auth.GitLabSpec {
	return auth.GitLabSpec{
		APIURL:        base,
		ServiceToken:  g.ServiceToken,
		OAuthClientID: g.OAuthClientID,
		Store:         store,
	}
}
