package httpapi

import (
	"net/http"
	"testing"
	"time"

	"github.com/codyconfer/mino/internal/errs"
)

func TestWhoAmIDistinguishesTheTwoCredentials(t *testing.T) {
	p := newFakeProvider(authorized())
	srv := identityAPI(t, identityDeps(p, newFakeSessions()))
	tok := signIn(t, srv)

	res := do(t, srv, "GET", "/api/v1/auth/session", tok, nil)
	if res.status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", res.status, res.body)
	}
	var out sessionInfo
	res.decode(t, &out)
	if out.Kind != "session" || out.Login != testLogin {
		t.Errorf("kind = %q login = %q, want session/%s", out.Kind, out.Login, testLogin)
	}
	if res.contains(tok) {
		t.Errorf("whoami echoed the session token back:\n%s", res.body)
	}
	if res.contains(hashToken(tok)) {
		t.Errorf("whoami echoed the session hash, which is the store's lookup key:\n%s", res.body)
	}

	var static sessionInfo
	do(t, srv, "GET", "/api/v1/auth/session", testToken, nil).decode(t, &static)
	if static.Kind != "token" {
		t.Errorf("kind = %q for the static token, want token", static.Kind)
	}
	if static.Login != "" {
		t.Errorf("login = %q for the static token, want empty; it is not a person", static.Login)
	}
}

func TestDeleteSessionRevokesImmediately(t *testing.T) {
	p := newFakeProvider(authorized())
	store := newFakeSessions()
	srv := identityAPI(t, identityDeps(p, store))
	tok := signIn(t, srv)

	if res := do(t, srv, "DELETE", "/api/v1/auth/session", tok, nil); res.status != http.StatusNoContent {
		t.Fatalf("status = %d, want 204: %s", res.status, res.body)
	}
	if res := do(t, srv, "GET", "/api/v1/status", tok, nil); res.status != http.StatusUnauthorized {
		t.Errorf("status = %d after revoking, want 401", res.status)
	}
	if store.count() != 0 {
		t.Errorf("stored sessions = %d after revoking, want 0", store.count())
	}
}

func TestDeleteSessionWithTheStaticTokenIsAConflict(t *testing.T) {
	p := newFakeProvider(authorized())
	srv := identityAPI(t, identityDeps(p, newFakeSessions()))
	res := do(t, srv, "DELETE", "/api/v1/auth/session", testToken, nil)
	if res.status != http.StatusConflict {
		t.Errorf("status = %d, want 409; the caller is authenticated and understood, there is simply "+
			"no session to delete", res.status)
	}
	if res := do(t, srv, "GET", "/api/v1/status", testToken, nil); res.status != http.StatusOK {
		t.Error("the static token stopped working after a DELETE it was never allowed to make")
	}
}

func TestExpiredSessionIsIndistinguishableFromNoCredential(t *testing.T) {
	p := newFakeProvider(authorized())
	store := newFakeSessions()
	d := identityDeps(p, store)
	srv := newTestServer(t, New(Config{
		Token: testToken, TokenSource: "test", MaxConcurrent: 2,
		AllowedLogins: []string{testLogin}, SessionTTL: time.Millisecond,
	}, d))
	tok := signIn(t, srv)
	time.Sleep(5 * time.Millisecond)

	expired := do(t, srv, "GET", "/api/v1/status", tok, nil)
	if expired.status != http.StatusUnauthorized {
		t.Fatalf("status = %d for an expired session, want 401: %s", expired.status, expired.body)
	}
	none := do(t, srv, "GET", "/api/v1/status", "", nil)
	wrong := do(t, srv, "GET", "/api/v1/status", "mino_s_totally-made-up-token-value", nil)
	if string(expired.body) != string(none.body) || string(expired.body) != string(wrong.body) {
		t.Errorf("the three 401 bodies differ, which tells a caller which credential mino holds:\n"+
			"expired: %s\nnone:    %s\nwrong:   %s", expired.body, none.body, wrong.body)
	}
	if store.count() != 0 {
		t.Errorf("stored sessions = %d after an expired lookup, want 0; the record must be dropped",
			store.count())
	}
}

func TestChangingTheAllowListInvalidatesEverySession(t *testing.T) {
	p := newFakeProvider(authorized())
	store := newFakeSessions()
	d := identityDeps(p, store)
	binding := "binding-one"
	d.AuthBinding = func() string { return binding }
	srv := identityAPI(t, d)
	tok := signIn(t, srv)

	if res := do(t, srv, "GET", "/api/v1/status", tok, nil); res.status != http.StatusOK {
		t.Fatalf("status = %d before the config change, want 200", res.status)
	}
	binding = "binding-two"
	if res := do(t, srv, "GET", "/api/v1/status", tok, nil); res.status != http.StatusUnauthorized {
		t.Errorf("status = %d after the allow-list changed, want 401; an operator who removes a login "+
			"expects that person out now, not when the session expires", res.status)
	}
	if store.count() != 0 {
		t.Errorf("stored sessions = %d, want 0; the stale record must be dropped, not just ignored",
			store.count())
	}
}

func TestSessionsSurviveANewAPIOverTheSameStore(t *testing.T) {
	p := newFakeProvider(authorized())
	store := newFakeSessions()
	cfg := Config{
		Token: testToken, TokenSource: "test", MaxConcurrent: 2,
		AllowedLogins: []string{testLogin}, SessionTTL: time.Hour,
	}
	srv := newTestServer(t, New(cfg, identityDeps(p, store)))
	tok := signIn(t, srv)

	restarted := newTestServer(t, New(cfg, identityDeps(newFakeProvider(), store)))
	if res := do(t, restarted, "GET", "/api/v1/status", tok, nil); res.status != http.StatusOK {
		t.Errorf("status = %d against a fresh API over the same store, want 200; a device flow needs "+
			"a human, so dropping sessions on restart is hostile: %s", res.status, res.body)
	}
}

func TestARestartDropsSessionsMintedUnderAnotherBinding(t *testing.T) {
	p := newFakeProvider(authorized())
	store := newFakeSessions()
	cfg := Config{
		Token: testToken, TokenSource: "test", MaxConcurrent: 2,
		AllowedLogins: []string{testLogin}, SessionTTL: time.Hour,
	}
	srv := newTestServer(t, New(cfg, identityDeps(p, store)))
	tok := signIn(t, srv)

	d := identityDeps(newFakeProvider(), store)
	d.AuthBinding = func() string { return "binding-two" }
	restarted := newTestServer(t, New(cfg, d))
	if res := do(t, restarted, "GET", "/api/v1/status", tok, nil); res.status != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401; a session minted under a different allow-list must not "+
			"survive the restart that changed it", res.status)
	}
	if store.count() != 0 {
		t.Errorf("stored sessions = %d after the reload, want 0", store.count())
	}
}

func TestAnUnreadableSessionStoreLeavesTheStaticTokenWorking(t *testing.T) {
	store := newFakeSessions()
	store.listErr = errs.New(errs.KindStore, "the store is unavailable")
	srv := identityAPI(t, identityDeps(newFakeProvider(), store))
	if res := do(t, srv, "GET", "/api/v1/status", testToken, nil); res.status != http.StatusOK {
		t.Errorf("status = %d, want 200; an unreadable session store must not take the API down",
			res.status)
	}
	if res := do(t, srv, "GET", "/api/v1/status", "mino_s_nope", nil); res.status != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401; session lookups must fail closed", res.status)
	}
}

func TestSessionLookupDoesNotReadTheStore(t *testing.T) {
	p := newFakeProvider(authorized())
	store := newFakeSessions()
	srv := identityAPI(t, identityDeps(p, store))
	tok := signIn(t, srv)

	before := store.puts
	for i := 0; i < 20; i++ {
		do(t, srv, "GET", "/api/v1/status", tok, nil)
		do(t, srv, "GET", "/api/v1/status", "mino_s_bogus", nil)
	}
	if store.puts != before {
		t.Errorf("the store was written %d times while serving requests, want 0; a store round trip "+
			"per request lets an anonymous caller generate database work with bogus tokens",
			store.puts-before)
	}
}

func TestConfigEndpointLeaksNoCredential(t *testing.T) {
	p := newFakeProvider(authorized())
	srv := identityAPI(t, identityDeps(p, newFakeSessions()))
	tok := signIn(t, srv)
	res := do(t, srv, "GET", "/api/v1/config", tok, nil)
	if res.status != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.status)
	}
	for name, secret := range map[string]string{
		"session token": tok,
		"session hash":  hashToken(tok),
		"device code":   testDeviceCode,
		"static token":  testToken,
	} {
		if res.contains(secret) {
			t.Errorf("the config body carried the %s", name)
		}
	}
}

func TestPendingAuthsIsReported(t *testing.T) {
	p := newFakeProvider(DeviceResult{Pending: true})
	api := New(Config{
		Token: testToken, TokenSource: "test", MaxConcurrent: 2,
		AllowedLogins: []string{testLogin}, SessionTTL: time.Hour,
	}, identityDeps(p, newFakeSessions()))
	srv := newTestServer(t, api)
	freezeAuthClock(t)
	startDevice(t, srv)
	if got := api.PendingAuths(); got != 1 {
		t.Errorf("PendingAuths = %d, want 1", got)
	}
}
