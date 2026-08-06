package auth

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/gitauth"
)

func init() {
	gitauth.Register("github", newGitHubProvider)
}

type githubProvider struct {
	spec GitHubSpec
}

func newGitHubProvider(env gitauth.Env) (gitauth.Provider, error) {
	base, err := NormalizeGitHubAPIURL(env.Get("api_url"))
	if err != nil {
		return nil, err
	}
	return &githubProvider{spec: GitHubSpec{
		APIURL:       base,
		ServiceToken: env.Get("service_token"),
		Store:        env.Store,
		App: GitHubAppSpec{
			ID:             env.Get("app.id"),
			InstallationID: env.Get("app.installation_id"),
			PrivateKeyPath: env.Get("app.private_key_path"),
		},
	}}, nil
}

func (p *githubProvider) Name() string  { return "github" }
func (p *githubProvider) Label() string { return "GitHub" }

func (p *githubProvider) Host() string {
	if h := GHHostname(p.spec.APIURL); h != "" {
		return h
	}
	return "github.com"
}

func (p *githubProvider) Resolve() (gitauth.Identity, error) {
	sel, err := SelectGitHub(p.spec)
	if err != nil {
		return nil, err
	}
	return githubIdentity{sel: sel}, nil
}

func (p *githubProvider) Status(ctx context.Context, id gitauth.Identity) gitauth.AuthStatus {
	sel, ok := selectionOf(id)
	if !ok {
		return gitauth.AuthStatus{Detail: "no GitHub authentication resolved", Fix: p.loginFix()}
	}
	if sel.UsesGHCLI() {
		args := append([]string{"auth", "status"}, GHHostFlag(sel.APIURL)...)
		if _, err := GH(ctx, args...); err == nil {
			return gitauth.AuthStatus{OK: true, Detail: "gh CLI is logged in"}
		}
	}
	if !sel.Authenticated() {
		return gitauth.AuthStatus{Detail: "no working GitHub authentication found", Fix: p.loginFix()}
	}
	if _, err := sel.Token(ctx); err != nil {
		return gitauth.AuthStatus{Detail: "using " + sel.Origin + ": " + err.Error(), Fix: p.loginFix()}
	}
	return gitauth.AuthStatus{OK: true, Detail: "using " + sel.Origin}
}

func (p *githubProvider) loginFix() []string {
	return []string{"gh auth login", "mino login github"}
}

func (p *githubProvider) Account(ctx context.Context, id gitauth.Identity) (gitauth.Account, error) {
	sel, ok := selectionOf(id)
	if !ok {
		return gitauth.Account{}, errs.New(errs.KindAuth, "no GitHub authentication resolved")
	}
	// An App installation token has no user, so GET /user is not merely failing, it is
	// inapplicable. Naming the installation beats a doomed request.
	if sel.Mech == GitHubAppAuth {
		return gitauth.Account{Login: sel.Origin}, nil
	}
	raw, err := GHAPIGet(ctx, sel, "user")
	if err != nil {
		return gitauth.Account{}, err
	}
	var u struct {
		Login string `json:"login"`
	}
	_ = json.Unmarshal(raw, &u)
	return gitauth.Account{Login: u.Login}, nil
}

func (p *githubProvider) RateLimit(ctx context.Context, id gitauth.Identity) (gitauth.RateLimit, error) {
	sel, ok := selectionOf(id)
	if !ok {
		return gitauth.RateLimit{}, errs.New(errs.KindAuth, "no GitHub authentication resolved")
	}
	raw, err := GHAPIGet(ctx, sel, "rate_limit")
	if err != nil {
		return gitauth.RateLimit{}, err
	}
	var r struct {
		Resources struct {
			Core struct {
				Limit     int `json:"limit"`
				Remaining int `json:"remaining"`
			} `json:"core"`
		} `json:"resources"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return gitauth.RateLimit{}, errs.Wrap(errs.KindSignal, err, "github: decoding rate limit")
	}
	return gitauth.RateLimit{Limit: r.Resources.Core.Limit, Remaining: r.Resources.Core.Remaining}, nil
}

func (p *githubProvider) SigningKeyRegistered(ctx context.Context, id gitauth.Identity, kind gitauth.SigningKeyKind, key string) gitauth.KeyCheck {
	sel, ok := selectionOf(id)
	if !ok {
		return gitauth.KeyCheck{Err: errs.New(errs.KindAuth, "no GitHub authentication resolved")}
	}
	path, scope := "user/gpg_keys", "admin:gpg_key"
	if kind == gitauth.SigningSSH {
		path, scope = "user/ssh_signing_keys", "admin:ssh_signing_key"
	}
	raw, err := GHAPIGet(ctx, sel, path)
	if err != nil {
		return gitauth.KeyCheck{Err: err, Fix: p.scopeFix(err, scope)}
	}
	if kind == gitauth.SigningSSH {
		return gitauth.KeyCheck{Registered: sshKeyRegistered(raw, key)}
	}
	ok2, ids := gpgKeyLookup(raw, key)
	return gitauth.KeyCheck{Registered: ok2, Identities: ids}
}

func (p *githubProvider) EmailVerified(ctx context.Context, id gitauth.Identity, email string) (bool, error) {
	sel, ok := selectionOf(id)
	if !ok {
		return false, errs.New(errs.KindAuth, "no GitHub authentication resolved")
	}
	raw, err := GHAPIGet(ctx, sel, "user/emails")
	if err != nil {
		return false, err
	}
	return emailVerified("github", raw, email)
}

func (p *githubProvider) UploadKeyFix(kind gitauth.SigningKeyKind, key string) []string {
	if kind == gitauth.SigningSSH {
		return []string{
			"gh ssh-key add <path-to-.pub> --type signing",
			"or paste the key at https://" + p.Host() + "/settings/ssh/new (Key type: Signing Key)",
		}
	}
	return []string{
		"gpg --armor --export " + key + " | gh gpg-key add -",
		"or: gpg --armor --export " + key + "   then paste at https://" + p.Host() + "/settings/gpg/new",
	}
}

func (p *githubProvider) scopeFix(err error, scope string) []string {
	if err == nil {
		return nil
	}
	s := strings.ToLower(err.Error())
	if strings.Contains(s, strings.ToLower(scope)) ||
		strings.Contains(s, "404") ||
		strings.Contains(s, "not found") {
		return []string{"gh auth refresh -h " + p.Host() + " -s " + scope}
	}
	return nil
}

func (p *githubProvider) Findings(ctx context.Context, id gitauth.Identity) []gitauth.Finding {
	sel, _ := selectionOf(id)
	var out []gitauth.Finding

	out = append(out, gitauth.Finding{
		Name: "github.auth.selected",
		OK:   sel.Authenticated(),
		Warn: !sel.Authenticated(),
		Msg:  githubSelectedMsg(sel),
	})

	if p.spec.App.Requested() {
		out = append(out, p.appFindings(ctx, sel)...)
	}
	if p.spec.ServiceToken != "" {
		out = append(out, gitauth.Finding{Name: "github.service_token", OK: true, Msg: "set"})
	}
	return out
}

func githubSelectedMsg(sel GitHubSelection) string {
	if !sel.Authenticated() {
		return "no GitHub authentication resolved"
	}
	kind := "user identity"
	if sel.ServiceIdentity() {
		kind = "service identity"
	}
	return sel.Origin + " (" + kind + ")"
}

func (p *githubProvider) appFindings(ctx context.Context, sel GitHubSelection) []gitauth.Finding {
	app := p.spec.App
	out := []gitauth.Finding{githubIDFinding("github.app.id", app.ID, true)}

	if app.InstallationID == "" {
		out = append(out, gitauth.Finding{
			Name: "github.app.installation_id", OK: true,
			Msg: "unset; mino will discover it if the app has exactly one installation",
		})
	} else {
		out = append(out, githubIDFinding("github.app.installation_id", app.InstallationID, true))
	}
	out = append(out, githubAppKeyFinding(p.spec))

	if sel.Mech == GitHubAppAuth {
		f := gitauth.Finding{Name: "github.app.installation"}
		if _, err := sel.Token(ctx); err != nil {
			f.Warn, f.Msg = true, err.Error()
		} else {
			f.OK, f.Msg = true, "minted an installation token"
		}
		out = append(out, f)
		out = append(out, gitauth.Finding{
			Name: "github.app.realtime", Warn: true,
			Msg: "a GitHub App installation token cannot read /notifications, so it drives no realtime " +
				"source; set github.service_token to a machine-user PAT for the notification stream",
		})
	}
	return out
}

func githubIDFinding(name, id string, required bool) gitauth.Finding {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return gitauth.Finding{Name: name, Warn: required, OK: !required, Msg: "unset"}
	}
	if _, err := strconv.Atoi(trimmed); err != nil {
		return gitauth.Finding{Name: name, Warn: true, Msg: strconv.Quote(id) + " is not numeric"}
	}
	return gitauth.Finding{Name: name, OK: true, Msg: trimmed}
}

type githubIdentity struct {
	sel GitHubSelection
}

func (i githubIdentity) Token(ctx context.Context) (string, error) { return i.sel.Token(ctx) }
func (i githubIdentity) Origin() string                            { return i.sel.Origin }
func (i githubIdentity) Authenticated() bool                       { return i.sel.Authenticated() }
func (i githubIdentity) ServiceIdentity() bool                     { return i.sel.ServiceIdentity() }
func (i githubIdentity) Trace() string                             { return i.sel.Trace() }
func (i githubIdentity) Invalidate()                               { i.sel.Invalidate() }

func selectionOf(id gitauth.Identity) (GitHubSelection, bool) {
	gi, ok := id.(githubIdentity)
	if !ok {
		return GitHubSelection{}, false
	}
	return gi.sel, true
}

func githubAppKeyFinding(spec GitHubSpec) gitauth.Finding {
	f := gitauth.Finding{Name: "github.app.private_key"}
	if os.Getenv(envAppKey) != "" {
		f.OK, f.Msg = true, "supplied inline via $"+envAppKey
		return f
	}
	path := strings.TrimSpace(spec.App.PrivateKeyPath)
	if path == "" {
		f.Warn, f.Msg = true, "no key configured; set github.app.private_key_path"
		return f
	}
	info, err := os.Stat(path)
	if err != nil {
		f.Warn, f.Msg = true, path+": "+err.Error()
		return f
	}
	if info.IsDir() {
		f.Warn, f.Msg = true, path+" is a directory"
		return f
	}
	mode := info.Mode().Perm()
	f.OK, f.Msg = true, path+" (mode "+strconv.FormatUint(uint64(mode), 8)+")"
	if mode&0o077 != 0 {
		f.OK, f.Warn = false, true
		f.Msg += " is readable by other users"
	}
	return f
}
