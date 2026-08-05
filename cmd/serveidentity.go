package cmd

import (
	"regexp"
	"strings"
	"time"

	"github.com/codyconfer/mino/internal/app/serve"
	"github.com/codyconfer/mino/internal/auth"
	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/errs"
)

var githubLoginPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9]|-(?:[A-Za-z0-9]))*$`)

func resolveServeHTTPIdentity() (serve.HTTPIdentityOptions, error) {
	idcfg := shared.Cfg.Daemon.HTTP.Identity
	if !idcfg.Active() {
		return serve.HTTPIdentityOptions{}, nil
	}
	provider := idcfg.ProviderName()
	if provider != config.DefaultHTTPIdentityProvider {
		return serve.HTTPIdentityOptions{}, errs.Newf(errs.KindConfig,
			"daemon.http.identity.provider %q is not a login provider this build serves", provider).
			WithHint("the stock binary serves %s", config.DefaultHTTPIdentityProvider)
	}
	logins, err := checkIdentityLogins(idcfg.LoginNames())
	if err != nil {
		return serve.HTTPIdentityOptions{}, err
	}
	clientID := strings.TrimSpace(idcfg.ClientID)
	if clientID == "" {
		clientID = strings.TrimSpace(shared.Cfg.GitHub.OAuthClientID)
	}
	if clientID == "" {
		return serve.HTTPIdentityOptions{}, errs.New(errs.KindConfig,
			"daemon.http.identity is enabled but neither daemon.http.identity.client_id nor "+
				"github.oauth_client_id is set").
			WithHint("use an OAuth App client id with device flow enabled")
	}
	ttl, err := checkIdentitySessionTTL(idcfg.SessionTTL)
	if err != nil {
		return serve.HTTPIdentityOptions{}, err
	}
	deviceURL, tokenURL, err := auth.GitHubOAuthEndpoints(shared.Cfg.GitHub.APIURL)
	if err != nil {
		return serve.HTTPIdentityOptions{}, err
	}
	return serve.HTTPIdentityOptions{
		Enabled:       true,
		Provider:      provider,
		ClientID:      clientID,
		Scopes:        strings.TrimSpace(idcfg.Scopes),
		APIURL:        shared.Cfg.GitHub.APIURL,
		DeviceURL:     deviceURL,
		TokenURL:      tokenURL,
		AllowedLogins: logins,
		SessionTTL:    ttl,
	}, nil
}

func checkIdentityLogins(logins []string) ([]string, error) {
	if len(logins) == 0 {
		return nil, errs.New(errs.KindConfig,
			"daemon.http.identity is enabled but daemon.http.identity.allowed_logins is empty").
			WithHint("list the logins that may sign in; an empty list would let any account on the forge " +
				"trigger flights, run queries and execute plugin actions here")
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(logins))
	for _, l := range logins {
		if !githubLoginPattern.MatchString(l) || len(l) > 39 {
			return nil, errs.Newf(errs.KindConfig,
				"daemon.http.identity.allowed_logins entry %q is not a login", l).
				WithHint("use the bare account name, no @ and no spaces; an entry that cannot match " +
					"would leave the allow-list silently denying everyone")
		}
		folded := strings.ToLower(l)
		if seen[folded] {
			return nil, errs.Newf(errs.KindConfig,
				"daemon.http.identity.allowed_logins lists %q twice", l).
				WithHint("logins are case-insensitive, so %q and %q are the same account", l, folded)
		}
		seen[folded] = true
		out = append(out, l)
	}
	return out, nil
}

func checkIdentitySessionTTL(raw string) (time.Duration, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		v = config.DefaultHTTPSessionTTL
	}
	ttl, err := time.ParseDuration(v)
	if err != nil {
		return 0, errs.Newf(errs.KindConfig, "daemon.http.identity.session_ttl %q is not a duration", raw).
			WithHint("use a Go duration such as 12h")
	}
	if ttl < config.MinHTTPSessionTTL {
		return 0, errs.Newf(errs.KindConfig, "daemon.http.identity.session_ttl %s is below %s",
			ttl, config.MinHTTPSessionTTL)
	}
	if ttl > config.MaxHTTPSessionTTL {
		return 0, errs.Newf(errs.KindConfig, "daemon.http.identity.session_ttl %s is above %s",
			ttl, config.MaxHTTPSessionTTL).
			WithHint("a session that outlives the machine is daemon.http.token with extra steps")
	}
	return ttl, nil
}
