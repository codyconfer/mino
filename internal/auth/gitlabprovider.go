package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/gitauth"
)

func init() {
	gitauth.Register("gitlab", newGitLabProvider)
}

const gitlabReadScope = "read_user"

type gitlabProvider struct {
	spec GitLabSpec

	mu        sync.Mutex
	confirmed []string
	loaded    bool
}

func newGitLabProvider(env gitauth.Env) (gitauth.Provider, error) {
	base, err := NormalizeGitLabAPIURL(env.Get("api_url"))
	if err != nil {
		return nil, err
	}
	return &gitlabProvider{spec: GitLabSpec{
		APIURL:        base,
		ServiceToken:  env.Get("service_token"),
		OAuthClientID: env.Get("oauth_client_id"),
		Store:         env.Store,
	}}, nil
}

func (p *gitlabProvider) Name() string  { return "gitlab" }
func (p *gitlabProvider) Label() string { return "GitLab" }

func (p *gitlabProvider) Host() string {
	if h := GLabHostname(p.spec.APIURL); h != "" {
		return h
	}
	return defaultGitLabHost
}

func (p *gitlabProvider) Resolve() (gitauth.Identity, error) {
	sel, err := SelectGitLab(p.spec)
	if err != nil {
		return nil, err
	}
	return gitlabIdentity{sel: sel}, nil
}

func (p *gitlabProvider) Status(ctx context.Context, id gitauth.Identity) gitauth.AuthStatus {
	sel, ok := gitlabSelectionOf(id)
	if !ok {
		return gitauth.AuthStatus{Detail: "no GitLab authentication resolved", Fix: p.loginFix()}
	}
	if sel.UsesGLabCLI() {
		args := append([]string{"auth", "status"}, GLabHostFlag(sel.APIURL)...)
		if _, err := GLab(ctx, args...); err == nil {
			return gitauth.AuthStatus{OK: true, Detail: "glab CLI is logged in"}
		}
	}
	if !sel.Authenticated() {
		return gitauth.AuthStatus{Detail: "no working GitLab authentication found", Fix: p.loginFix()}
	}
	if _, err := sel.Token(ctx); err != nil {
		return gitauth.AuthStatus{Detail: "using " + sel.Origin + ": " + err.Error(), Fix: p.loginFix()}
	}
	return gitauth.AuthStatus{OK: true, Detail: "using " + sel.Origin}
}

func (p *gitlabProvider) loginFix() []string {
	return []string{"glab auth login", "mino login gitlab"}
}

func (p *gitlabProvider) Account(ctx context.Context, id gitauth.Identity) (gitauth.Account, error) {
	sel, ok := gitlabSelectionOf(id)
	if !ok {
		return gitauth.Account{}, errs.New(errs.KindAuth, "no GitLab authentication resolved")
	}
	u, err := p.currentUser(ctx, sel)
	if err != nil {
		return gitauth.Account{}, err
	}
	return gitauth.Account{Login: u.Username}, nil
}

type gitlabUser struct {
	Username string `json:"username"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	State    string `json:"state"`
}

func (p *gitlabProvider) currentUser(ctx context.Context, sel GitLabSelection) (gitlabUser, error) {
	raw, err := GLAPIGet(ctx, sel, "user")
	if err != nil {
		return gitlabUser{}, err
	}
	var u gitlabUser
	if err := json.Unmarshal(raw, &u); err != nil {
		return gitlabUser{}, errs.Wrap(errs.KindSignal, err, "gitlab: decoding user")
	}
	return u, nil
}

func (p *gitlabProvider) RateLimit(ctx context.Context, id gitauth.Identity) (gitauth.RateLimit, error) {
	sel, ok := gitlabSelectionOf(id)
	if !ok {
		return gitauth.RateLimit{}, errs.New(errs.KindAuth, "no GitLab authentication resolved")
	}
	_, hdr, err := glAPIGet(ctx, sel, "user")
	if err != nil {
		return gitauth.RateLimit{}, err
	}
	limit, lok := headerInt(hdr, "RateLimit-Limit")
	remaining, rok := headerInt(hdr, "RateLimit-Remaining")
	if !lok || !rok {
		return gitauth.RateLimit{}, errs.New(errs.KindSignal, "gitlab does not report a rate limit for this host")
	}
	return gitauth.RateLimit{Limit: limit, Remaining: remaining}, nil
}

func (p *gitlabProvider) SigningKeyRegistered(ctx context.Context, id gitauth.Identity, kind gitauth.SigningKeyKind, key string) gitauth.KeyCheck {
	sel, ok := gitlabSelectionOf(id)
	if !ok {
		return gitauth.KeyCheck{Err: errs.New(errs.KindAuth, "no GitLab authentication resolved")}
	}
	if kind == gitauth.SigningSSH {
		raw, err := GLAPIGet(ctx, sel, "user/keys")
		if err != nil {
			return gitauth.KeyCheck{Err: err, Fix: p.scopeFix(err)}
		}
		return gitauth.KeyCheck{Registered: gitlabSSHKeyRegistered(raw, key)}
	}
	return p.gpgKeyCheck(ctx, sel, key)
}

type gitlabSSHKey struct {
	Key       string `json:"key"`
	UsageType string `json:"usage_type"`
}

func gitlabSSHKeyRegistered(raw []byte, pubKey string) bool {
	var keys []gitlabSSHKey
	if err := json.Unmarshal(raw, &keys); err != nil {
		return false
	}
	want := normalizeSSHPubKey(pubKey)
	if want == "" {
		return false
	}
	for _, k := range keys {
		if normalizeSSHPubKey(k.Key) != want {
			continue
		}
		switch k.UsageType {
		case "", "signing", "auth_and_signing":
			return true
		}
	}
	return false
}

func (p *gitlabProvider) gpgKeyCheck(ctx context.Context, sel GitLabSelection, key string) gitauth.KeyCheck {
	raw, err := GLAPIGet(ctx, sel, "user/gpg_keys")
	if err != nil {
		return gitauth.KeyCheck{Err: err, Fix: p.scopeFix(err)}
	}
	var keys []struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(raw, &keys); err != nil {
		return gitauth.KeyCheck{Err: errs.Wrap(errs.KindSignal, err, "gitlab: decoding gpg keys")}
	}

	var matched []armoredKey
	for _, entry := range keys {
		parsed, err := parseArmoredGPGKey(ctx, entry.Key)
		if err != nil {
			return gitauth.KeyCheck{
				Err: errs.Wrap(errs.KindConfig, err, "gitlab: reading a registered gpg key"),
				Fix: []string{"install GnuPG so mino can read the key GitLab returns"},
			}
		}
		if armoredKeyMatches(parsed, key) {
			matched = append(matched, parsed)
		}
	}
	if len(matched) == 0 {
		return gitauth.KeyCheck{}
	}

	confirmed, _ := p.confirmedEmails(ctx, sel)
	var ids []string
	for _, k := range matched {
		for _, email := range k.Emails {
			if containsFold(confirmed, email) && !containsFold(ids, email) {
				ids = append(ids, email)
			}
		}
	}
	return gitauth.KeyCheck{Registered: true, Identities: ids}
}

func (p *gitlabProvider) EmailVerified(ctx context.Context, id gitauth.Identity, email string) (bool, error) {
	sel, ok := gitlabSelectionOf(id)
	if !ok {
		return false, errs.New(errs.KindAuth, "no GitLab authentication resolved")
	}
	confirmed, err := p.confirmedEmails(ctx, sel)
	if err != nil {
		return false, err
	}
	return containsFold(confirmed, email), nil
}

func (p *gitlabProvider) confirmedEmails(ctx context.Context, sel GitLabSelection) ([]string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.loaded {
		return p.confirmed, nil
	}

	var out []string
	if u, err := p.currentUser(ctx, sel); err == nil && u.Email != "" {
		out = append(out, u.Email)
	}
	raw, err := GLAPIGet(ctx, sel, "user/emails")
	if err != nil {
		return nil, err
	}
	var list []struct {
		Email       string  `json:"email"`
		ConfirmedAt *string `json:"confirmed_at"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, errs.Wrap(errs.KindSignal, err, "gitlab: decoding emails")
	}
	for _, e := range list {
		if e.Email == "" || e.ConfirmedAt == nil || *e.ConfirmedAt == "" {
			continue
		}
		if !containsFold(out, e.Email) {
			out = append(out, e.Email)
		}
	}
	p.confirmed, p.loaded = out, true
	return out, nil
}

func containsFold(list []string, want string) bool {
	for _, s := range list {
		if strings.EqualFold(s, want) {
			return true
		}
	}
	return false
}

func (p *gitlabProvider) UploadKeyFix(kind gitauth.SigningKeyKind, key string) []string {
	if kind == gitauth.SigningSSH {
		return []string{
			"glab ssh-key add <path-to-.pub> --title mino",
			"then set Usage type to \"Signing\" at https://" + p.Host() + "/-/user_settings/ssh_keys",
		}
	}
	return []string{
		"gpg --armor --export " + key,
		"then paste the block at https://" + p.Host() + "/-/user_settings/gpg_keys",
	}
}

func (p *gitlabProvider) scopeFix(err error) []string {
	if err == nil {
		return nil
	}
	s := strings.ToLower(err.Error())
	if !strings.Contains(s, gitlabReadScope) &&
		!strings.Contains(s, "401") &&
		!strings.Contains(s, "403") &&
		!strings.Contains(s, "404") &&
		!strings.Contains(s, "not found") {
		return nil
	}
	return []string{
		"glab auth login --hostname " + p.Host() + " --scopes " + gitlabReadScope,
		"or set gitlab.service_token to a token with the " + gitlabReadScope + " scope",
	}
}

func (p *gitlabProvider) Findings(_ context.Context, id gitauth.Identity) []gitauth.Finding {
	sel, _ := gitlabSelectionOf(id)
	out := []gitauth.Finding{{
		Name: "gitlab.auth.selected",
		OK:   sel.Authenticated(),
		Warn: !sel.Authenticated(),
		Msg:  gitlabSelectedMsg(sel),
	}}

	out = append(out, gitauth.Finding{Name: "gitlab.api_url", OK: true, Msg: p.apiURLMsg()})

	if p.spec.ServiceToken != "" {
		out = append(out, gitauth.Finding{Name: "gitlab.service_token", OK: true, Msg: "set"})
	}
	if sel.Mech == GitLabCLI {
		out = append(out, gitauth.Finding{
			Name: "gitlab.cli", OK: true,
			Msg: "glab is authenticated against " + p.Host(),
		})
	}
	if f, ok := p.tokenExpiryFinding(sel); ok {
		out = append(out, f)
	}
	if p.spec.OAuthClientID == "" {
		out = append(out, gitauth.Finding{
			Name: "gitlab.oauth_client_id", OK: true,
			Msg: "unset; `mino login gitlab` needs it",
		})
	} else {
		out = append(out, gitauth.Finding{Name: "gitlab.oauth_client_id", OK: true, Msg: "set"})
	}

	return out
}

func (p *gitlabProvider) apiURLMsg() string {
	host := p.Host()
	if host == defaultGitLabHost {
		return host + " (gitlab.com)"
	}
	return host + " (self-managed)"
}

func (p *gitlabProvider) tokenExpiryFinding(sel GitLabSelection) (gitauth.Finding, bool) {
	if sel.Mech != GitLabStoredToken {
		return gitauth.Finding{}, false
	}
	c, ok := GitLabCredential(p.spec.Store)
	if !ok || c.Expiry.IsZero() {
		return gitauth.Finding{}, false
	}
	if c.RefreshToken != "" && p.spec.OAuthClientID != "" {
		return gitauth.Finding{
			Name: "gitlab.token.expiry", OK: true,
			Msg: "expires in " + time.Until(c.Expiry).Round(time.Minute).String() + ", refreshes automatically",
		}, true
	}
	reason := "no refresh token"
	if c.RefreshToken != "" {
		reason = "gitlab.oauth_client_id is unset, so it cannot be refreshed"
	}
	return gitauth.Finding{
		Name: "gitlab.token.expiry", Warn: true,
		Msg: "the cached token expires in " + time.Until(c.Expiry).Round(time.Minute).String() +
			" and carries " + reason,
	}, true
}

func gitlabSelectedMsg(sel GitLabSelection) string {
	if !sel.Authenticated() {
		return "no GitLab authentication resolved"
	}
	kind := "user identity"
	if sel.ServiceIdentity() {
		kind = "service identity"
	}
	return sel.Origin + " (" + kind + ")"
}

type gitlabIdentity struct {
	sel GitLabSelection
}

func (i gitlabIdentity) Token(ctx context.Context) (string, error) { return i.sel.Token(ctx) }
func (i gitlabIdentity) Origin() string                            { return i.sel.Origin }
func (i gitlabIdentity) Authenticated() bool                       { return i.sel.Authenticated() }
func (i gitlabIdentity) ServiceIdentity() bool                     { return i.sel.ServiceIdentity() }
func (i gitlabIdentity) Trace() string                             { return i.sel.Trace() }
func (i gitlabIdentity) Invalidate()                               { i.sel.Invalidate() }

func gitlabSelectionOf(id gitauth.Identity) (GitLabSelection, bool) {
	gi, ok := id.(gitlabIdentity)
	if !ok {
		return GitLabSelection{}, false
	}
	return gi.sel, true
}

func headerInt(hdr http.Header, key string) (int, bool) {
	v := strings.TrimSpace(hdr.Get(key))
	if v == "" {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, false
	}
	return n, true
}
