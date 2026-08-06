package auth

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/codyconfer/mino/internal/errs"
)

const (
	giteaCredKey        = "gitea"
	giteaServiceCredKey = "gitea-service"
	giteaAPIPath        = "/api/v1"
)

const (
	originGiteaEnvToken   = "$GITEA_TOKEN"
	originForgejoEnvToken = "$FORGEJO_TOKEN"
	originGiteaStored     = "cached personal access token"
)

const GiteaDefaultForge = "gitea"

type GiteaMechanism int

const (
	GiteaNone GiteaMechanism = iota
	GiteaServiceToken
	GiteaEnvToken
	GiteaStoredToken
)

func (m GiteaMechanism) String() string {
	switch m {
	case GiteaServiceToken:
		return "service token"
	case GiteaEnvToken:
		return "env token"
	case GiteaStoredToken:
		return "stored token"
	}
	return "none"
}

type GiteaSpec struct {
	Forge        string
	URL          string
	APIURL       string
	ServiceToken string
	Store        TokenStore
}

func (s GiteaSpec) forge() string {
	if f := strings.TrimSpace(s.Forge); f != "" {
		return f
	}
	return GiteaDefaultForge
}

func (s GiteaSpec) APIBase() string {
	if api := strings.TrimRight(strings.TrimSpace(s.APIURL), "/"); api != "" {
		return api
	}
	if root := strings.TrimRight(strings.TrimSpace(s.URL), "/"); root != "" {
		return root + giteaAPIPath
	}
	return ""
}

func (s GiteaSpec) WebBase() string {
	if root := strings.TrimRight(strings.TrimSpace(s.URL), "/"); root != "" {
		return root
	}
	api := strings.TrimRight(strings.TrimSpace(s.APIURL), "/")
	return strings.TrimSuffix(api, giteaAPIPath)
}

type GiteaSelection struct {
	Mech   GiteaMechanism
	Origin string
	APIURL string
	WebURL string

	forge string
	trace string
	src   TokenSource
}

func (s GiteaSelection) Token(ctx context.Context) (string, error) {
	if s.src == nil {
		return "", errs.Newf(errs.KindAuth, "no %s authentication available", s.Forge()).
			WithHint("run `mino login %s` to store a personal access token, set $GITEA_TOKEN, "+
				"or configure gitea.service_token for a service identity", s.Forge())
	}
	return s.src.Token(ctx)
}

func (s GiteaSelection) Forge() string {
	if s.forge == "" {
		return GiteaDefaultForge
	}
	return s.forge
}

func (s GiteaSelection) ServiceIdentity() bool { return s.Mech == GiteaServiceToken }

func (s GiteaSelection) Authenticated() bool { return s.Mech != GiteaNone }

func (s GiteaSelection) Trace() string { return s.trace }

func (s GiteaSelection) Invalidate() {
	if inv, ok := s.src.(interface{ Invalidate() }); ok {
		inv.Invalidate()
	}
}

func StaticGiteaToken(tok string) TokenSource { return staticSource{token: tok} }

func StaticGiteaSelection(spec GiteaSpec, tok, origin string) GiteaSelection {
	return GiteaSelection{
		Mech:   GiteaEnvToken,
		Origin: origin,
		APIURL: spec.APIBase(),
		WebURL: spec.WebBase(),
		forge:  spec.forge(),
		src:    staticSource{token: tok},
	}
}

func GiteaToken(store TokenStore) (token, origin string) {
	if t := os.Getenv("GITEA_TOKEN"); t != "" {
		return t, originGiteaEnvToken
	}
	if t := os.Getenv("FORGEJO_TOKEN"); t != "" {
		return t, originForgejoEnvToken
	}
	if c, ok := getCred(store, giteaCredKey); ok {
		return c.AccessToken, originGiteaStored
	}
	return "", ""
}

func GiteaTokenOrigin(store TokenStore) string {
	_, origin := GiteaToken(store)
	return origin
}

func CacheGiteaToken(store TokenStore, token string) error {
	if store == nil {
		return errs.New(errs.KindStore, "no credential store is available")
	}
	return store.Put(context.Background(), giteaCredKey, Credential{AccessToken: token, Scope: giteaTokenScope})
}

func SelectGitea(spec GiteaSpec) (GiteaSelection, error) {
	tiers := make([]string, 0, 4)
	note := func(format string, args ...any) { tiers = append(tiers, fmt.Sprintf(format, args...)) }

	sel := GiteaSelection{APIURL: spec.APIBase(), WebURL: spec.WebBase(), forge: spec.forge()}
	finish := func(mech GiteaMechanism, origin string, src TokenSource) (GiteaSelection, error) {
		note("-> selected %s (%s)", mech, origin)
		sel.Mech, sel.Origin, sel.src = mech, origin, src
		sel.trace = spec.forge() + ": auth tiers: " + strings.Join(tiers, " ")
		return sel, nil
	}

	if tok := strings.TrimSpace(spec.ServiceToken); tok != "" {
		note("service_token=set")
		return finish(GiteaServiceToken, "gitea.service_token", staticSource{token: tok})
	}
	note("service_token=unset")

	if c, ok := getCred(spec.Store, giteaServiceCredKey); ok && c.AccessToken != "" {
		note("store:%s=hit", giteaServiceCredKey)
		return finish(GiteaServiceToken, "sealed store "+strconv.Quote(giteaServiceCredKey),
			staticSource{token: c.AccessToken})
	}
	note("store:%s=miss", giteaServiceCredKey)

	if tok, origin := GiteaToken(spec.Store); tok != "" {
		note("ambient=%s", origin)
		mech := GiteaEnvToken
		if origin == originGiteaStored {
			mech = GiteaStoredToken
		}
		return finish(mech, origin, staticSource{token: tok})
	}
	note("ambient=none")

	sel.trace = spec.forge() + ": auth tiers: " + strings.Join(tiers, " ") + " -> none"
	return sel, nil
}
