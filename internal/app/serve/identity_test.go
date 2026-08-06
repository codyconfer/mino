package serve

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/codyconfer/sisyphus/kv"

	"github.com/codyconfer/mino/internal/app/serve/httpapi"
)

func testIdentityOptions() HTTPIdentityOptions {
	return HTTPIdentityOptions{
		Enabled:       true,
		Provider:      "github",
		ClientID:      "cid",
		AllowedLogins: []string{"codyconfer"},
		SessionTTL:    time.Hour,
	}
}

func openTestKV(t *testing.T) (*kv.Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "serve.duckdb")
	store, err := kv.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("kv.Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, path
}

func TestNewSessionStoreIsNilForANilKV(t *testing.T) {
	if got := newSessionStore(nil); got != nil {
		t.Error("newSessionStore returned a non-nil interface wrapping a nil store, which would make " +
			"identity login look wired and then fail every request")
	}
}

func TestSessionStoreRoundTripsAndPersists(t *testing.T) {
	store, path := openTestKV(t)
	s := newSessionStore(store)
	rec := httpapi.Session{
		ID: "abc123", Provider: "github", Login: "codyconfer", UserID: 42,
		Binding: "bind", CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := s.Put(context.Background(), "hash-one", rec); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := kv.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("reopening: %v", err)
	}
	defer reopened.Close()
	got, err := newSessionStore(reopened).List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	back, ok := got["hash-one"]
	if !ok {
		t.Fatal("the session did not survive a reopen; a device flow needs a human, so dropping " +
			"sessions on restart is hostile")
	}
	if back.Login != rec.Login || back.UserID != rec.UserID || back.Binding != rec.Binding {
		t.Errorf("got %+v, want %+v", back, rec)
	}
}

func TestSessionStoreDeleteRemovesTheRecord(t *testing.T) {
	store, _ := openTestKV(t)
	s := newSessionStore(store)
	rec := httpapi.Session{ID: "x", ExpiresAt: time.Now().Add(time.Hour)}
	if err := s.Put(context.Background(), "h", rec); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := s.Delete(context.Background(), "h"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, err := s.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("sessions = %d after Delete, want 0", len(got))
	}
}

func TestIdentityBindingChangesWithTheAllowList(t *testing.T) {
	base := testIdentityOptions()
	first := base.binding("/home/mino")

	changed := base
	changed.AllowedLogins = []string{"codyconfer", "someone-else"}
	if changed.binding("/home/mino") == first {
		t.Error("the binding did not change when the allow-list did, so outstanding sessions would " +
			"survive a removal the operator expects to be immediate")
	}

	rotated := base
	rotated.ClientID = "other"
	if rotated.binding("/home/mino") == first {
		t.Error("the binding did not change when the client id did")
	}

	if base.binding("/other/home") == first {
		t.Error("the binding did not change with the home dir")
	}

	reordered := base
	reordered.AllowedLogins = []string{"CODYCONFER"}
	if reordered.binding("/home/mino") != first {
		t.Error("the binding changed for a case difference in the allow-list, which would log " +
			"everyone out on a cosmetic config edit")
	}
}

func TestIdentityProvidersAreClosedInPackage(t *testing.T) {
	if got := apiIdentityProviders(HTTPIdentityOptions{}); got != nil {
		t.Error("providers were wired with identity login off")
	}
	got := apiIdentityProviders(testIdentityOptions())
	if _, ok := got["github"]; !ok || len(got) != 1 {
		t.Errorf("providers = %v, want exactly github", got)
	}
	unknown := testIdentityOptions()
	unknown.Provider = "bitbucket"
	if got := apiIdentityProviders(unknown); got != nil {
		t.Error("an unknown provider was wired; the inbound set is closed in-package on purpose")
	}
	forge := testIdentityOptions()
	forge.Provider = "gitlab"
	if got := apiIdentityProviders(forge); got != nil {
		t.Error("gitlab was wired for inbound identity just because it is a registered git provider; " +
			"the two registries are deliberately separate")
	}
}

func TestGitHubIdentityPollResolvesTheLoginAndKeepsNoToken(t *testing.T) {
	var sawToken string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawToken = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"login":"codyconfer","id":42,"type":"User"}`))
	}))
	defer api.Close()
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"gho_caller_token"}`))
	}))
	defer tokenSrv.Close()

	g := githubIdentity{clientID: "cid", apiURL: api.URL, tokenURL: tokenSrv.URL}
	res, err := g.Poll(context.Background(), "dc")
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if res.Login != "codyconfer" || res.UserID != 42 || res.Kind != "User" {
		t.Errorf("got %+v, want codyconfer/42/User", res)
	}
	if !strings.Contains(sawToken, "gho_caller_token") {
		t.Errorf("the identity call used %q, want the device-flow token; anything else would "+
			"authenticate the machine", sawToken)
	}
	if g.clientID == "gho_caller_token" {
		t.Error("the provider retained the forge token")
	}
}

func TestGitHubIdentityPollPassesThroughProtocolStates(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		check      func(httpapi.DeviceResult) bool
	}{
		{"pending", `{"error":"authorization_pending"}`, func(r httpapi.DeviceResult) bool { return r.Pending }},
		{"slow down", `{"error":"slow_down"}`, func(r httpapi.DeviceResult) bool { return r.Pending && r.SlowDown }},
		{"denied", `{"error":"access_denied"}`, func(r httpapi.DeviceResult) bool { return r.Denied }},
		{"expired", `{"error":"expired_token"}`, func(r httpapi.DeviceResult) bool { return r.Expired }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			g := githubIdentity{clientID: "cid", tokenURL: srv.URL}
			res, err := g.Poll(context.Background(), "dc")
			if err != nil {
				t.Fatalf("Poll: %v", err)
			}
			if !tc.check(res) {
				t.Errorf("got %+v for %s", res, tc.name)
			}
			if res.Login != "" {
				t.Errorf("a non-terminal poll carried a login: %+v", res)
			}
		})
	}
}

func TestGitHubIdentityStartReportsTheUserCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"device_code":"dc","user_code":"UC-9","verification_uri":"https://x","expires_in":900,"interval":5}`))
	}))
	defer srv.Close()
	g := githubIdentity{clientID: "cid", deviceURL: srv.URL}
	st, err := g.Start(context.Background())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if st.UserCode != "UC-9" || st.DeviceCode != "dc" {
		t.Errorf("got %+v, want the codes from the response", st)
	}
	if st.Interval != 5*time.Second {
		t.Errorf("interval = %s, want 5s", st.Interval)
	}
}
