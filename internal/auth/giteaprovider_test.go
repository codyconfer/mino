package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/gitauth"
)

type giteaServer struct {
	*httptest.Server

	mu    sync.Mutex
	paths []string
	auth  []string
}

func newGiteaServer(t *testing.T, routes map[string]func(w http.ResponseWriter)) *giteaServer {
	t.Helper()
	g := &giteaServer{}
	g.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		g.paths = append(g.paths, r.URL.Path)
		g.auth = append(g.auth, r.Header.Get("Authorization"))
		g.mu.Unlock()
		if h, ok := routes[r.URL.Path]; ok {
			h(w)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	}))
	t.Cleanup(g.Close)
	return g
}

func (g *giteaServer) requested() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.paths...)
}

func (g *giteaServer) authHeaders() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.auth...)
}

func json200(body string) func(w http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}
}

func status(code int, body string) func(w http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	}
}

func giteaEnv(t *testing.T, settings map[string]string) gitauth.Env {
	t.Helper()
	clearAmbientGitea(t)
	t.Setenv("GITEA_TOKEN", "pat-tok")
	return gitauth.Env{
		Store:   memStore{},
		Setting: func(key string) string { return settings[key] },
	}
}

func newTestGiteaProvider(t *testing.T, name string, settings map[string]string) (gitauth.Provider, gitauth.Identity) {
	t.Helper()
	p, err := gitauth.New(name, giteaEnv(t, settings))
	if err != nil {
		t.Fatalf("gitauth.New(%q): %v", name, err)
	}
	id, err := p.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	return p, id
}

func TestGiteaProviderRegistersBothNames(t *testing.T) {
	cases := []struct {
		name      string
		wantLabel string
	}{
		{"gitea", "Gitea"},
		{"forgejo", "Forgejo"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !gitauth.Known(c.name) {
				t.Fatalf("%q is not a registered git provider", c.name)
			}
			p, _ := newTestGiteaProvider(t, c.name, map[string]string{"url": "https://git.example.com"})
			if p.Name() != c.name {
				t.Errorf("Name() = %q, want %q", p.Name(), c.name)
			}
			if p.Label() != c.wantLabel {
				t.Errorf("Label() = %q, want %q", p.Label(), c.wantLabel)
			}
			if p.Host() != "git.example.com" {
				t.Errorf("Host() = %q, want git.example.com", p.Host())
			}
			if got := gitauth.SignalOf(p); got != "gitea" {
				t.Errorf("SignalOf() = %q, want gitea; forgejo has no signal of its own", got)
			}
		})
	}
}

func TestGiteaProviderRequiresAnInstanceURL(t *testing.T) {
	for _, name := range []string{"gitea", "forgejo"} {
		t.Run(name, func(t *testing.T) {
			_, err := gitauth.New(name, giteaEnv(t, nil))
			if err == nil {
				t.Fatal("a provider was built with no instance URL; there is no default host to fall back to")
			}
			if errs.KindOf(err) != errs.KindConfig {
				t.Errorf("kind = %v, want %v", errs.KindOf(err), errs.KindConfig)
			}
			if !strings.Contains(err.Error(), "gitea.url") {
				t.Errorf("message = %q, want it to name gitea.url even under the %s alias, since both read one section", err, name)
			}
		})
	}
}

func TestGiteaProviderRejectsAnInsecureURL(t *testing.T) {
	_, err := gitauth.New("gitea", giteaEnv(t, map[string]string{"url": "http://git.example.com"}))
	if err == nil {
		t.Fatal("a remote http instance URL was accepted")
	}
	if errs.KindOf(err) != errs.KindConfig {
		t.Errorf("kind = %v, want %v", errs.KindOf(err), errs.KindConfig)
	}
}

func TestGiteaAPIGetUsesTheTokenAuthorizationScheme(t *testing.T) {
	srv := newGiteaServer(t, map[string]func(http.ResponseWriter){
		"/api/v1/user": json200(`{"login":"alice"}`),
	})
	p, id := newTestGiteaProvider(t, "gitea", map[string]string{"url": srv.URL})

	acct, err := p.Account(context.Background(), id)
	if err != nil {
		t.Fatalf("Account: %v", err)
	}
	if acct.Login != "alice" {
		t.Errorf("Login = %q, want alice", acct.Login)
	}
	if got := srv.requested(); len(got) != 1 || got[0] != "/api/v1/user" {
		t.Errorf("requested %v, want [/api/v1/user]", got)
	}
	got := srv.authHeaders()[0]
	if got != "token pat-tok" {
		t.Errorf("Authorization = %q, want %q; Gitea has accepted the token scheme on every version and Bearer only on recent ones", got, "token pat-tok")
	}
}

func TestGiteaRateLimitReportsThatItIsUnreported(t *testing.T) {
	p, id := newTestGiteaProvider(t, "gitea", map[string]string{"url": "https://git.example.com"})

	rl, err := p.RateLimit(context.Background(), id)
	if !errors.Is(err, gitauth.ErrRateLimitUnreported) {
		t.Fatalf("err = %v, want ErrRateLimitUnreported; a nil error with a zero limit renders as 0/0, which the status strip paints as throttled", err)
	}
	if rl != (gitauth.RateLimit{}) {
		t.Errorf("RateLimit = %+v, want the zero value", rl)
	}
}

func TestGiteaSigningKeyUsesOneKeyListForBothKinds(t *testing.T) {
	const gpgKeys = `[{"key_id":"ABCDEF0123456789","emails":[{"email":"alice@example.com","verified":true}],"subkeys":[]}]`
	const sshKeys = `[{"key":"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIfake alice@host","key_type":"user"},
		{"key":"ssh-rsa AAAAB3NzaC1yc2Eother deploy@ci","key_type":"deploy"}]`

	srv := newGiteaServer(t, map[string]func(http.ResponseWriter){
		"/api/v1/user/gpg_keys": json200(gpgKeys),
		"/api/v1/user/keys":     json200(sshKeys),
	})
	p, id := newTestGiteaProvider(t, "gitea", map[string]string{"url": srv.URL})
	ctx := context.Background()

	gpg := p.SigningKeyRegistered(ctx, id, gitauth.SigningGPG, "0xABCDEF0123456789")
	if gpg.Err != nil || !gpg.Registered {
		t.Fatalf("gpg check = %+v, want registered", gpg)
	}
	if len(gpg.Identities) != 1 || gpg.Identities[0] != "alice@example.com" {
		t.Errorf("gpg identities = %v, want the verified address", gpg.Identities)
	}

	ssh := p.SigningKeyRegistered(ctx, id, gitauth.SigningSSH, "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIfake other-comment")
	if ssh.Err != nil || !ssh.Registered {
		t.Fatalf("ssh check = %+v, want registered; gitea verifies signatures against ordinary user keys", ssh)
	}

	want := []string{"/api/v1/user/gpg_keys", "/api/v1/user/keys"}
	got := srv.requested()
	for i, path := range want {
		if i >= len(got) || got[i] != path {
			t.Fatalf("requested %v, want %v; there is no user/ssh_signing_keys on gitea", got, want)
		}
	}
}

func TestGiteaEmailVerifiedIgnoresUnverifiedAddresses(t *testing.T) {
	srv := newGiteaServer(t, map[string]func(http.ResponseWriter){
		"/api/v1/user/emails": json200(`[{"email":"alice@example.com","verified":false,"primary":true},
			{"email":"a@example.com","verified":true,"primary":false}]`),
	})
	p, id := newTestGiteaProvider(t, "gitea", map[string]string{"url": srv.URL})
	ctx := context.Background()

	if ok, err := p.EmailVerified(ctx, id, "alice@example.com"); err != nil || ok {
		t.Errorf("EmailVerified(unverified) = %v/%v, want false", ok, err)
	}
	if ok, err := p.EmailVerified(ctx, id, "A@Example.com"); err != nil || !ok {
		t.Errorf("EmailVerified(verified) = %v/%v, want true (case-insensitive)", ok, err)
	}
}

func TestGiteaUploadKeyFixPointsAtTheInstanceSettings(t *testing.T) {
	p, _ := newTestGiteaProvider(t, "forgejo", map[string]string{"url": "https://git.example.com"})

	for _, kind := range []gitauth.SigningKeyKind{gitauth.SigningGPG, gitauth.SigningSSH} {
		fix := strings.Join(p.UploadKeyFix(kind, "ABCDEF"), " ")
		if !strings.Contains(fix, "https://git.example.com/user/settings/keys") {
			t.Errorf("%s fix = %q, want the instance key page", kind, fix)
		}
	}
	if fix := strings.Join(p.UploadKeyFix(gitauth.SigningSSH, ""), " "); !strings.Contains(fix, "Forgejo") {
		t.Errorf("ssh fix = %q, want it to name Forgejo under the alias", fix)
	}
}

func TestGiteaStatusNamesTheOriginAndTheLoginCommand(t *testing.T) {
	p, id := newTestGiteaProvider(t, "forgejo", map[string]string{"url": "https://git.example.com"})

	st := p.Status(context.Background(), id)
	if !st.OK || !strings.Contains(st.Detail, "$GITEA_TOKEN") {
		t.Errorf("Status = %+v, want ok naming the ambient origin", st)
	}

	clearAmbientGitea(t)
	p2, err := gitauth.New("forgejo", gitauth.Env{
		Store:   memStore{},
		Setting: func(key string) string { return map[string]string{"url": "https://git.example.com"}[key] },
	})
	if err != nil {
		t.Fatal(err)
	}
	id2, err := p2.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	st = p2.Status(context.Background(), id2)
	if st.OK {
		t.Fatal("Status reported ok with no credential")
	}
	if fix := strings.Join(st.Fix, " "); !strings.Contains(fix, "mino login forgejo") {
		t.Errorf("Fix = %v, want it to name the provider the user configured", st.Fix)
	}
}

func TestGiteaScopeFixNamesReadUserAndIsSilentForNetworkErrors(t *testing.T) {
	srv := newGiteaServer(t, map[string]func(http.ResponseWriter){
		"/api/v1/user/gpg_keys": status(http.StatusForbidden, `{"message":"token does not have at least one of required scope(s): [read:user]"}`),
	})
	p, id := newTestGiteaProvider(t, "gitea", map[string]string{"url": srv.URL})

	check := p.SigningKeyRegistered(context.Background(), id, gitauth.SigningGPG, "ABCDEF")
	if check.Err == nil {
		t.Fatal("a 403 was treated as success")
	}
	fix := strings.Join(check.Fix, " ")
	if !strings.Contains(fix, "read:user") || !strings.Contains(fix, "/user/settings/applications") {
		t.Errorf("Fix = %v, want the token page and the scope", check.Fix)
	}

	dead, deadID := newTestGiteaProvider(t, "gitea", map[string]string{"url": "https://127.0.0.1:1"})
	if check := dead.SigningKeyRegistered(context.Background(), deadID, gitauth.SigningGPG, "ABCDEF"); len(check.Fix) != 0 {
		t.Errorf("Fix = %v for a transport failure, want none: rescoping a token does not fix an unreachable host", check.Fix)
	}
}

func TestGiteaNotFoundHintNamesBothScopeAndVersion(t *testing.T) {
	srv := newGiteaServer(t, nil)
	p, id := newTestGiteaProvider(t, "gitea", map[string]string{"url": srv.URL})

	check := p.SigningKeyRegistered(context.Background(), id, gitauth.SigningSSH, "ssh-ed25519 AAAA x")
	if check.Err == nil {
		t.Fatal("a 404 was treated as success")
	}
	hint := errs.Hint(check.Err)
	if !strings.Contains(hint, "read:user") || !strings.Contains(hint, "api/swagger") {
		t.Errorf("hint = %q, want both causes named: gitea answers 404 for a missing scope and for a missing endpoint", hint)
	}
}

func TestGiteaFindingsUseTheProviderNameAndNameGiteaKeys(t *testing.T) {
	p, id := newTestGiteaProvider(t, "forgejo", map[string]string{
		"url":           "https://git.example.com",
		"service_token": "svc-tok",
	})

	var names []string
	for _, f := range p.Findings(context.Background(), id) {
		names = append(names, f.Name)
	}
	for _, want := range []string{"forgejo.auth.selected", "forgejo.url", "forgejo.service_token"} {
		if !contains(names, want) {
			t.Errorf("findings %v are missing %q", names, want)
		}
	}

	bare, bareID := newTestGiteaProvider(t, "gitea", map[string]string{"api_url": "https://git.example.com/api/v1"})
	for _, f := range bare.Findings(context.Background(), bareID) {
		if f.Name == "gitea.url" && f.Msg != "https://git.example.com/api/v1" {
			t.Errorf("gitea.url finding = %q, want the resolved API base", f.Msg)
		}
	}
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
