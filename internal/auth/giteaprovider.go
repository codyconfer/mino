package auth

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/gitauth"
)

func init() {
	gitauth.Register("gitea", giteaFactory("gitea"))
	gitauth.Register("forgejo", giteaFactory("forgejo"))
}

func giteaFactory(name string) gitauth.Factory {
	return func(env gitauth.Env) (gitauth.Provider, error) { return newGiteaProvider(name, env) }
}

type giteaProvider struct {
	name string
	spec GiteaSpec
}

func newGiteaProvider(name string, env gitauth.Env) (gitauth.Provider, error) {
	root, err := NormalizeGiteaURL(env.Get("url"))
	if err != nil {
		return nil, err
	}
	api, err := NormalizeGiteaAPIURL(env.Get("api_url"))
	if err != nil {
		return nil, err
	}
	if root == "" && api == "" {
		return nil, errs.New(errs.KindConfig, "gitea.url is not set").WithHint("%s", giteaURLHint)
	}
	return &giteaProvider{name: name, spec: GiteaSpec{
		Forge:        name,
		URL:          root,
		APIURL:       api,
		ServiceToken: env.Get("service_token"),
		Store:        env.Store,
	}}, nil
}

func (p *giteaProvider) Name() string { return p.name }

func (p *giteaProvider) Label() string {
	if p.name == "forgejo" {
		return "Forgejo"
	}
	return "Gitea"
}

func (p *giteaProvider) Signal() string { return "gitea" }

func (p *giteaProvider) Host() string {
	if h := GiteaHostname(p.spec.WebBase()); h != "" {
		return h
	}
	return GiteaHostname(p.spec.APIBase())
}

func (p *giteaProvider) Resolve() (gitauth.Identity, error) {
	sel, err := SelectGitea(p.spec)
	if err != nil {
		return nil, err
	}
	return giteaIdentity{sel: sel}, nil
}

func (p *giteaProvider) Status(ctx context.Context, id gitauth.Identity) gitauth.AuthStatus {
	sel, ok := giteaSelectionOf(id)
	if !ok {
		return gitauth.AuthStatus{Detail: "no " + p.Label() + " authentication resolved", Fix: p.loginFix()}
	}
	if !sel.Authenticated() {
		return gitauth.AuthStatus{Detail: "no working " + p.Label() + " authentication found", Fix: p.loginFix()}
	}
	if _, err := sel.Token(ctx); err != nil {
		return gitauth.AuthStatus{Detail: "using " + sel.Origin + ": " + err.Error(), Fix: p.loginFix()}
	}
	return gitauth.AuthStatus{OK: true, Detail: "using " + sel.Origin}
}

func (p *giteaProvider) loginFix() []string {
	return []string{"mino login " + p.name, "or set $GITEA_TOKEN"}
}

func (p *giteaProvider) Account(ctx context.Context, id gitauth.Identity) (gitauth.Account, error) {
	sel, ok := giteaSelectionOf(id)
	if !ok {
		return gitauth.Account{}, p.unresolved()
	}
	raw, err := GiteaAPIGet(ctx, sel, "user")
	if err != nil {
		return gitauth.Account{}, err
	}
	var u struct {
		Login string `json:"login"`
	}
	_ = json.Unmarshal(raw, &u)
	return gitauth.Account{Login: u.Login}, nil
}

func (p *giteaProvider) RateLimit(context.Context, gitauth.Identity) (gitauth.RateLimit, error) {
	return gitauth.RateLimit{}, gitauth.ErrRateLimitUnreported
}

func (p *giteaProvider) SigningKeyRegistered(ctx context.Context, id gitauth.Identity, kind gitauth.SigningKeyKind, key string) gitauth.KeyCheck {
	sel, ok := giteaSelectionOf(id)
	if !ok {
		return gitauth.KeyCheck{Err: p.unresolved()}
	}
	path := "user/gpg_keys"
	if kind == gitauth.SigningSSH {
		path = "user/keys"
	}
	raw, err := GiteaAPIGet(ctx, sel, path)
	if err != nil {
		return gitauth.KeyCheck{Err: err, Fix: p.scopeFix(err)}
	}
	if kind == gitauth.SigningSSH {
		return gitauth.KeyCheck{Registered: sshKeyRegistered(raw, key)}
	}
	found, ids := gpgKeyLookup(raw, key)
	return gitauth.KeyCheck{Registered: found, Identities: ids}
}

func (p *giteaProvider) EmailVerified(ctx context.Context, id gitauth.Identity, email string) (bool, error) {
	sel, ok := giteaSelectionOf(id)
	if !ok {
		return false, p.unresolved()
	}
	raw, err := GiteaAPIGet(ctx, sel, "user/emails")
	if err != nil {
		return false, err
	}
	return emailVerified(p.name, raw, email)
}

func (p *giteaProvider) UploadKeyFix(kind gitauth.SigningKeyKind, key string) []string {
	page := p.webURL("/user/settings/keys")
	if kind == gitauth.SigningSSH {
		return []string{
			"paste the key at " + page + " under Manage SSH Keys",
			p.Label() + " uses one key for access and signing, so there is no signing key type to pick",
		}
	}
	return []string{
		"gpg --armor --export " + key,
		"then paste it at " + page + " under Manage GPG Keys",
	}
}

func (p *giteaProvider) Findings(_ context.Context, id gitauth.Identity) []gitauth.Finding {
	sel, _ := giteaSelectionOf(id)
	out := []gitauth.Finding{{
		Name: p.name + ".auth.selected",
		OK:   sel.Authenticated(),
		Warn: !sel.Authenticated(),
		Msg:  giteaSelectedMsg(p.Label(), sel),
	}}

	endpoint := gitauth.Finding{Name: p.name + ".url"}
	if base := p.spec.APIBase(); base != "" {
		endpoint.OK, endpoint.Msg = true, base
	} else {
		endpoint.Warn, endpoint.Msg = true, "unset; set gitea.url to your instance root"
	}
	out = append(out, endpoint)

	if p.spec.ServiceToken != "" {
		out = append(out, gitauth.Finding{Name: p.name + ".service_token", OK: true, Msg: "set"})
	}
	return out
}

func giteaSelectedMsg(label string, sel GiteaSelection) string {
	if !sel.Authenticated() {
		return "no " + label + " authentication resolved"
	}
	kind := "user identity"
	if sel.ServiceIdentity() {
		kind = "service identity"
	}
	return sel.Origin + " (" + kind + ")"
}

func (p *giteaProvider) unresolved() error {
	return errs.Newf(errs.KindAuth, "no %s authentication resolved", p.Label())
}

func (p *giteaProvider) webURL(path string) string {
	if base := p.spec.WebBase(); base != "" {
		return GiteaWebURL(base, path)
	}
	return path
}

func (p *giteaProvider) scopeFix(err error) []string {
	if err == nil {
		return nil
	}
	s := strings.ToLower(err.Error())
	if strings.Contains(s, "401") || strings.Contains(s, "403") ||
		strings.Contains(s, "404") || strings.Contains(s, "not found") {
		return []string{
			"regenerate the token at " + p.webURL("/user/settings/applications") + " with the read:user scope",
			"then run `mino login " + p.name + "`",
		}
	}
	return nil
}

type giteaIdentity struct {
	sel GiteaSelection
}

func (i giteaIdentity) Token(ctx context.Context) (string, error) { return i.sel.Token(ctx) }
func (i giteaIdentity) Origin() string                            { return i.sel.Origin }
func (i giteaIdentity) Authenticated() bool                       { return i.sel.Authenticated() }
func (i giteaIdentity) ServiceIdentity() bool                     { return i.sel.ServiceIdentity() }
func (i giteaIdentity) Trace() string                             { return i.sel.Trace() }
func (i giteaIdentity) Invalidate()                               { i.sel.Invalidate() }

func giteaSelectionOf(id gitauth.Identity) (GiteaSelection, bool) {
	gi, ok := id.(giteaIdentity)
	if !ok {
		return GiteaSelection{}, false
	}
	return gi.sel, true
}
