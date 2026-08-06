package config

import "github.com/codyconfer/mino/internal/auth"

func (g GiteaConfig) AuthSpec(forge string, store auth.TokenStore) auth.GiteaSpec {
	return auth.GiteaSpec{
		Forge:        forge,
		URL:          g.URL,
		APIURL:       g.APIURL,
		ServiceToken: g.ServiceToken,
		Store:        store,
	}
}
