package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/log"
)

var testAppKey = sync.OnceValue(func() *rsa.PrivateKey {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	return k
})

func pkcs1PEM(t *testing.T) []byte {
	t.Helper()
	return pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(testAppKey()),
	})
}

func writeAppKey(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.pem")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

type appServer struct {
	srv       *httptest.Server
	mints     atomic.Int64
	lists     atomic.Int64
	expiresIn time.Duration
	installs  []map[string]any
	mintCode  int
	seenAuth  atomic.Value
}

func newAppServer(t *testing.T) *appServer {
	t.Helper()
	a := &appServer{expiresIn: time.Hour}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /app/installations", func(w http.ResponseWriter, r *http.Request) {
		a.lists.Add(1)
		a.seenAuth.Store(r.Header.Get("Authorization"))
		list := a.installs
		if list == nil {
			list = []map[string]any{{"id": 78901234, "account": map[string]any{"login": "acme"}}}
		}
		_ = json.NewEncoder(w).Encode(list)
	})
	mux.HandleFunc("POST /app/installations/{id}/access_tokens", func(w http.ResponseWriter, r *http.Request) {
		a.mints.Add(1)
		a.seenAuth.Store(r.Header.Get("Authorization"))
		if a.mintCode != 0 {
			w.WriteHeader(a.mintCode)
			_, _ = w.Write([]byte(`{"message":"nope"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"token":       fmt.Sprintf("ghs_minted_%d", a.mints.Load()),
			"expires_at":  time.Now().Add(a.expiresIn).UTC().Format(time.RFC3339),
			"permissions": map[string]string{"issues": "read", "pull_requests": "read"},
		})
	})
	a.srv = httptest.NewTLSServer(mux)
	t.Cleanup(a.srv.Close)
	return a
}

func (a *appServer) source(t *testing.T, app GitHubAppSpec) *appSource {
	t.Helper()
	app.PrivateKeyPath = writeAppKey(t, pkcs1PEM(t))
	if app.ID == "" {
		app.ID = "123456"
	}
	src, _, err := newAppSource(GitHubSpec{APIURL: a.srv.URL, App: app, Store: memStore{}})
	if err != nil {
		t.Fatalf("newAppSource: %v", err)
	}
	p := src.(*appSource)
	p.http = a.srv.Client()
	return p
}

func TestAppSourceMintsAnInstallationTokenWithTheJWT(t *testing.T) {
	clearAmbientGitHub(t)
	srv := newAppServer(t)
	p := srv.source(t, GitHubAppSpec{InstallationID: "78901234"})

	tok, err := p.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if !strings.HasPrefix(tok, "ghs_minted_") {
		t.Errorf("token = %q, want the minted installation token", tok)
	}
	if got := srv.seenAuth.Load().(string); !strings.HasPrefix(got, "Bearer eyJ") {
		t.Errorf("Authorization = %q; the mint must present the signed app JWT, not a token", got)
	}
	if srv.lists.Load() != 0 {
		t.Errorf("listed installations %d times despite a configured installation_id", srv.lists.Load())
	}
}

func TestAppTokenIsReusedUntilTheRefreshMargin(t *testing.T) {
	clearAmbientGitHub(t)
	srv := newAppServer(t)
	p := srv.source(t, GitHubAppSpec{InstallationID: "78901234"})

	for range 5 {
		if _, err := p.Token(context.Background()); err != nil {
			t.Fatalf("Token: %v", err)
		}
	}
	if srv.mints.Load() != 1 {
		t.Errorf("minted %d times for 5 calls on a 1h token; re-minting per call would burn the "+
			"installation-token rate limit and add latency to every fetch", srv.mints.Load())
	}
}

func TestAppTokenRefreshesInsideTheMargin(t *testing.T) {
	clearAmbientGitHub(t)
	srv := newAppServer(t)
	srv.expiresIn = appRefreshMargin / 2
	p := srv.source(t, GitHubAppSpec{InstallationID: "78901234"})

	first, err := p.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	second, err := p.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if srv.mints.Load() != 2 {
		t.Errorf("minted %d times; a token already inside the %s refresh margin must be replaced",
			srv.mints.Load(), appRefreshMargin)
	}
	if first == second {
		t.Error("the refreshed token is identical to the expiring one")
	}
}

func TestConcurrentTokenCallsMintExactlyOnce(t *testing.T) {
	clearAmbientGitHub(t)
	srv := newAppServer(t)
	p := srv.source(t, GitHubAppSpec{InstallationID: "78901234"})

	var wg sync.WaitGroup
	toks := make([]string, 6)
	errsOut := make([]error, 6)
	for i := range toks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			toks[i], errsOut[i] = p.Token(context.Background())
		}()
	}
	wg.Wait()

	for i, err := range errsOut {
		if err != nil {
			t.Fatalf("goroutine %d: %v", i, err)
		}
	}
	if srv.mints.Load() != 1 {
		t.Errorf("minted %d times for 6 concurrent callers; one flight fans out to several parallel "+
			"fetches, so without single-flight every expiry would trigger a burst of exchanges",
			srv.mints.Load())
	}
	for i, tok := range toks {
		if tok != toks[0] {
			t.Errorf("goroutine %d got %q, want the shared %q", i, tok, toks[0])
		}
	}
}

func TestARefreshFailureKeepsAStillValidToken(t *testing.T) {
	clearAmbientGitHub(t)
	srv := newAppServer(t)
	srv.expiresIn = appRefreshMargin / 2
	p := srv.source(t, GitHubAppSpec{InstallationID: "78901234"})

	first, err := p.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	srv.mintCode = http.StatusInternalServerError
	if _, err := p.Token(context.Background()); err == nil {
		t.Fatal("a failing mint returned no error")
	}
	p.mu.Lock()
	cached := p.token
	p.mu.Unlock()
	if cached != first {
		t.Errorf("cached token = %q after a failed refresh, want the still-valid %q; discarding a token "+
			"that is good for another few minutes turns a transient blip into an outage", cached, first)
	}
}

func TestAppMintFailureNeverFallsBackToAHumanToken(t *testing.T) {
	clearAmbientGitHub(t)
	withoutGHOnPath(t)
	srv := newAppServer(t)
	srv.mintCode = http.StatusUnauthorized
	t.Setenv("GITHUB_TOKEN", "a-human-token")
	p := srv.source(t, GitHubAppSpec{InstallationID: "78901234"})

	tok, err := p.Token(context.Background())
	if err == nil {
		t.Fatal("a rejected app jwt produced a token")
	}
	if tok != "" {
		t.Errorf("token = %q on failure; want empty", tok)
	}
	if strings.Contains(err.Error(), "a-human-token") {
		t.Error("the error leaked the ambient token")
	}
	if errs.KindOf(err) != errs.KindAuth {
		t.Errorf("kind = %v, want KindAuth for a rejected jwt", errs.KindOf(err))
	}
	if h := errs.Hint(err); !strings.Contains(h, "clock") {
		t.Errorf("hint = %q; a 401 on the mint is usually clock skew or an id/key mismatch, and the hint "+
			"should say so rather than sending the user to re-login", h)
	}
}

func TestAppSourceDiscoversASoleInstallation(t *testing.T) {
	clearAmbientGitHub(t)
	srv := newAppServer(t)
	p := srv.source(t, GitHubAppSpec{})

	if _, err := p.Token(context.Background()); err != nil {
		t.Fatalf("Token: %v", err)
	}
	if srv.lists.Load() != 1 {
		t.Errorf("listed installations %d times, want 1", srv.lists.Load())
	}
	if _, err := p.Token(context.Background()); err != nil {
		t.Fatalf("Token: %v", err)
	}
	if srv.lists.Load() != 1 {
		t.Errorf("re-listed installations on a later call; discovery must happen at most once per process")
	}
}

func TestAmbiguousInstallationsErrorAndNameEveryCandidate(t *testing.T) {
	clearAmbientGitHub(t)
	srv := newAppServer(t)
	srv.installs = []map[string]any{
		{"id": 111, "account": map[string]any{"login": "acme"}},
		{"id": 222, "account": map[string]any{"login": "globex"}},
	}
	p := srv.source(t, GitHubAppSpec{})

	_, err := p.Token(context.Background())
	if err == nil {
		t.Fatal("two installations were resolved without asking; guessing an identity is never acceptable")
	}
	for _, want := range []string{"111", "222", "acme", "globex", "installation_id"} {
		if !strings.Contains(err.Error()+errs.Hint(err), want) {
			t.Errorf("error/hint does not mention %q: %v (%s)", want, err, errs.Hint(err))
		}
	}
}

func TestNoInstallationsIsAConfigError(t *testing.T) {
	clearAmbientGitHub(t)
	srv := newAppServer(t)
	srv.installs = []map[string]any{}
	p := srv.source(t, GitHubAppSpec{})

	_, err := p.Token(context.Background())
	if err == nil || errs.KindOf(err) != errs.KindConfig {
		t.Fatalf("err = %v (kind %v), want a KindConfig error naming the missing installation",
			err, errs.KindOf(err))
	}
}

func TestAppRefusesANonHTTPSAPIURL(t *testing.T) {
	clearAmbientGitHub(t)
	_, _, err := newAppSource(GitHubSpec{
		APIURL: "http://ghe.example.com/api/v3",
		App:    GitHubAppSpec{ID: "123456", PrivateKeyPath: writeAppKey(t, pkcs1PEM(t))},
		Store:  memStore{},
	})
	if err == nil {
		t.Fatal("accepted a cleartext api_url; the app flow sends an RSA-signed JWT and receives a " +
			"long-lived installation token, neither of which may cross the network in clear")
	}
	if errs.KindOf(err) != errs.KindConfig {
		t.Errorf("kind = %v, want KindConfig", errs.KindOf(err))
	}
}

func TestParseAppKeyNamesTheLikelyWrongFile(t *testing.T) {
	openssh := pem.EncodeToMemory(&pem.Block{Type: "OPENSSH PRIVATE KEY", Bytes: []byte("ssh-key-bytes")})
	encrypted := pem.EncodeToMemory(&pem.Block{
		Type:    "RSA PRIVATE KEY",
		Headers: map[string]string{"Proc-Type": "4,ENCRYPTED"},
		Bytes:   []byte("encrypted"),
	})
	small, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	smallPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(small),
	})

	for _, tc := range []struct {
		name, want string
		data       []byte
	}{
		{"not pem", "not PEM", []byte("hello")},
		{"ssh key", "SSH private key", openssh},
		{"encrypted", "passphrase-protected", encrypted},
		{"too small", "1024 bits", smallPEM},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseAppKey(tc.data, "the test key")
			if err == nil {
				t.Fatalf("parseAppKey accepted %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q; want it to say %q rather than a bare ASN.1 failure", err, tc.want)
			}
		})
	}
}

func TestParseAppKeyAcceptsPKCS1AndPKCS8(t *testing.T) {
	pkcs8Bytes, err := x509.MarshalPKCS8PrivateKey(testAppKey())
	if err != nil {
		t.Fatal(err)
	}
	pkcs8 := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8Bytes})

	for name, data := range map[string][]byte{"pkcs1": pkcs1PEM(t), "pkcs8": pkcs8} {
		if _, err := parseAppKey(data, "the test key"); err != nil {
			t.Errorf("%s: %v; GitHub ships PKCS#1 but users routinely re-encode with openssl pkcs8", name, err)
		}
	}
}

func TestInlineAppKeyEnvVarIsAcceptedRawAndBase64(t *testing.T) {
	raw := pkcs1PEM(t)
	for name, value := range map[string]string{
		"raw":    string(raw),
		"base64": base64Std(raw),
	} {
		t.Run(name, func(t *testing.T) {
			clearAmbientGitHub(t)
			t.Setenv(envAppKey, value)
			data, source, err := appKeyPEM(GitHubSpec{Store: memStore{}})
			if err != nil {
				t.Fatalf("appKeyPEM: %v", err)
			}
			if _, err := parseAppKey(data, source); err != nil {
				t.Errorf("parseAppKey: %v; container platforms often only inject env, and multi-line "+
					"values get mangled, so both spellings have to work", err)
			}
			if !strings.Contains(source, envAppKey) {
				t.Errorf("source = %q, want it to name %s", source, envAppKey)
			}
		})
	}
}

func TestAppKeyMaterialNeverReachesErrorsOrLogs(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	log.SetVerbose(true)
	t.Cleanup(func() { log.SetOutput(os.Stderr); log.SetVerbose(false) })

	clearAmbientGitHub(t)
	srv := newAppServer(t)
	p := srv.source(t, GitHubAppSpec{InstallationID: "78901234"})
	tok, err := p.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}

	srv.mintCode = http.StatusUnauthorized
	p.Invalidate()
	_, mintErr := p.Token(context.Background())
	if mintErr == nil {
		t.Fatal("expected the second mint to fail")
	}

	keyBody := string(pkcs1PEM(t))
	logged := buf.String()
	for _, forbidden := range []string{"BEGIN RSA PRIVATE KEY", tok} {
		if strings.Contains(logged, forbidden) {
			t.Errorf("the log contains %q; logs land in the log dir and in tmux scrollback", forbidden)
		}
		if strings.Contains(mintErr.Error()+errs.Hint(mintErr), forbidden) {
			t.Errorf("the error contains %q", forbidden)
		}
	}
	if strings.Contains(logged, keyBody[40:80]) {
		t.Error("a slice of the private key body reached the log")
	}
}

func TestInvalidateForcesAFreshMint(t *testing.T) {
	clearAmbientGitHub(t)
	srv := newAppServer(t)
	p := srv.source(t, GitHubAppSpec{InstallationID: "78901234"})

	if _, err := p.Token(context.Background()); err != nil {
		t.Fatalf("Token: %v", err)
	}
	p.Invalidate()
	if _, err := p.Token(context.Background()); err != nil {
		t.Fatalf("Token after Invalidate: %v", err)
	}
	if srv.mints.Load() != 2 {
		t.Errorf("minted %d times; Invalidate is what lets an operator who just fixed a permission pick "+
			"up a new token without restarting", srv.mints.Load())
	}
}

func TestAppJWTCarriesTheIssuerAndABackdatedIat(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	tok, err := appJWT(testAppKey(), "123456", now)
	if err != nil {
		t.Fatalf("appJWT: %v", err)
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("jwt has %d segments, want 3", len(parts))
	}
	if parts[0] != appJWTHeader {
		t.Errorf("header = %q, want the constant RS256 header", parts[0])
	}
	var claims struct {
		Iat int64 `json:"iat"`
		Exp int64 `json:"exp"`
		Iss int64 `json:"iss"`
	}
	raw, err := base64Raw(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &claims); err != nil {
		t.Fatal(err)
	}
	if claims.Iss != 123456 {
		t.Errorf("iss = %v, want the numeric app id; GitHub documents iss as the App ID and rejects a "+
			"jwt it cannot decode, so sending it as a JSON string risks a 401 that no fake test server "+
			"would reproduce", claims.Iss)
	}
	if bytes.Contains(raw, []byte(`"iss":"`)) {
		t.Errorf("iss is encoded as a JSON string: %s", raw)
	}
	if claims.Iat != now.Add(-appJWTBackdate).Unix() {
		t.Errorf("iat = %d; it must be backdated so a slightly fast clock does not get the jwt rejected",
			claims.Iat)
	}
	if d := time.Unix(claims.Exp, 0).Sub(now); d > 10*time.Minute {
		t.Errorf("exp is %s out; GitHub rejects app jwts longer than 10 minutes", d)
	}
}

func base64Std(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

func base64Raw(s string) ([]byte, error) { return base64.RawURLEncoding.DecodeString(s) }
