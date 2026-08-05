package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/codyconfer/mino/internal/errs"
)

const (
	testLogin      = "codyconfer"
	testDeviceCode = "device-code-that-must-not-escape"
)

type fakeSessions struct {
	mu      sync.Mutex
	recs    map[string]Session
	putErr  error
	listErr error
	puts    int
}

func newFakeSessions() *fakeSessions {
	return &fakeSessions{recs: map[string]Session{}}
}

func (f *fakeSessions) Put(_ context.Context, hash string, s Session) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.puts++
	if f.putErr != nil {
		return f.putErr
	}
	f.recs[hash] = s
	return nil
}

func (f *fakeSessions) Delete(_ context.Context, hash string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.recs, hash)
	return nil
}

func (f *fakeSessions) List(_ context.Context) (map[string]Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make(map[string]Session, len(f.recs))
	for k, v := range f.recs {
		out[k] = v
	}
	return out, nil
}

func (f *fakeSessions) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.recs)
}

func (f *fakeSessions) raw() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, _ := json.Marshal(f.recs)
	return string(b)
}

type fakeProvider struct {
	mu       sync.Mutex
	startErr error
	start    DeviceAuth
	results  []DeviceResult
	pollErr  error
	starts   int
	polls    int
}

func newFakeProvider(results ...DeviceResult) *fakeProvider {
	return &fakeProvider{
		start: DeviceAuth{
			DeviceCode:      testDeviceCode,
			UserCode:        "WXYZ-1234",
			VerificationURI: "https://github.com/login/device",
			Interval:        5 * time.Second,
			ExpiresIn:       15 * time.Minute,
		},
		results: results,
	}
}

func (f *fakeProvider) Start(context.Context) (DeviceAuth, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.starts++
	if f.startErr != nil {
		return DeviceAuth{}, f.startErr
	}
	return f.start, nil
}

func (f *fakeProvider) Poll(_ context.Context, code string) (DeviceResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.polls++
	if code != f.start.DeviceCode {
		return DeviceResult{}, fmt.Errorf("polled with %q, want the device code", code)
	}
	if f.pollErr != nil {
		return DeviceResult{}, f.pollErr
	}
	if len(f.results) == 0 {
		return DeviceResult{Pending: true}, nil
	}
	res := f.results[0]
	if len(f.results) > 1 {
		f.results = f.results[1:]
	}
	return res, nil
}

func (f *fakeProvider) counts() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.starts, f.polls
}

func authorized() DeviceResult {
	return DeviceResult{Login: testLogin, UserID: 4242, Kind: "User"}
}

func identityDeps(p IdentityProvider, store SessionStore) Deps {
	d := fakeDeps()
	d.Identity = map[string]IdentityProvider{"github": p}
	d.Sessions = store
	d.AuthBinding = func() string { return "binding-one" }
	return d
}

func identityAPI(t *testing.T, d Deps, logins ...string) *httptest.Server {
	t.Helper()
	if len(logins) == 0 {
		logins = []string{testLogin}
	}
	return newTestServer(t, New(Config{
		Token: testToken, TokenSource: "test", MaxConcurrent: 2,
		AllowedLogins: logins, SessionTTL: time.Hour,
	}, d))
}

func freezeAuthClock(t *testing.T) func(time.Duration) {
	t.Helper()
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	orig := authClock
	authClock = func() time.Time { return now }
	t.Cleanup(func() { authClock = orig })
	return func(d time.Duration) { now = now.Add(d) }
}

func startDevice(t *testing.T, srv *httptest.Server) reply {
	t.Helper()
	return do(t, srv, "POST", "/api/v1/auth/device/github", "", strings.NewReader("{}"))
}

func pollDevice(t *testing.T, srv *httptest.Server, authID string) reply {
	t.Helper()
	body := strings.NewReader(fmt.Sprintf(`{"auth_id":%q}`, authID))
	return do(t, srv, "POST", "/api/v1/auth/device/github/token", "", body)
}

func authIDOf(t *testing.T, r reply) string {
	t.Helper()
	var out struct {
		AuthID string `json:"auth_id"`
	}
	r.decode(t, &out)
	if out.AuthID == "" {
		t.Fatalf("the start response carried no auth_id: %s", r.body)
	}
	return out.AuthID
}

func signIn(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	adv := freezeAuthClock(t)
	id := authIDOf(t, startDevice(t, srv))
	adv(10 * time.Second)
	res := pollDevice(t, srv, id)
	if res.status != http.StatusOK {
		t.Fatalf("poll status = %d, want 200: %s", res.status, res.body)
	}
	var out struct {
		SessionToken string `json:"session_token"`
	}
	res.decode(t, &out)
	if out.SessionToken == "" {
		t.Fatalf("the mint response carried no session_token: %s", res.body)
	}
	return out.SessionToken
}

func TestIdentityRoutesDoNotExistWhenIdentityIsOff(t *testing.T) {
	srv := testAPI(t, fakeDeps())
	for _, path := range []string{"/api/v1/auth/device/github", "/api/v1/auth/device/github/token"} {
		res := do(t, srv, "POST", path, "", strings.NewReader("{}"))
		if res.status != http.StatusNotFound {
			t.Errorf("POST %s status = %d, want 404; a sign-in route that exists but refuses is "+
				"attack surface for no benefit", path, res.status)
		}
	}
	res := do(t, srv, "GET", "/api/v1/auth/session", testToken, nil)
	if res.status != http.StatusNotFound {
		t.Errorf("GET session status = %d, want 404 while identity login is off", res.status)
	}
}

func TestIdentityNeedsBothAProviderAndAStore(t *testing.T) {
	d := fakeDeps()
	d.Identity = map[string]IdentityProvider{"github": newFakeProvider()}
	srv := newTestServer(t, New(Config{Token: testToken, TokenSource: "test", MaxConcurrent: 2}, d))
	res := startDevice(t, srv)
	if res.status != http.StatusNotFound {
		t.Errorf("status = %d, want 404; a provider with no store would mint sessions that stop "+
			"working at the next restart", res.status)
	}
}

func TestSignInMintsAWorkingSession(t *testing.T) {
	p := newFakeProvider(DeviceResult{Pending: true}, authorized())
	store := newFakeSessions()
	srv := identityAPI(t, identityDeps(p, store))
	adv := freezeAuthClock(t)

	start := startDevice(t, srv)
	if start.status != http.StatusCreated {
		t.Fatalf("start status = %d, want 201: %s", start.status, start.body)
	}
	if start.contains(testDeviceCode) {
		t.Fatalf("the start response leaked the device code, which a caller could replay "+
			"against the provider:\n%s", start.body)
	}
	id := authIDOf(t, start)

	adv(10 * time.Second)
	pending := pollDevice(t, srv, id)
	if pending.status != http.StatusAccepted {
		t.Fatalf("pending poll status = %d, want 202: %s", pending.status, pending.body)
	}
	if pending.header.Get("Retry-After") == "" {
		t.Error("no Retry-After on a pending poll, so a client cannot pace itself")
	}

	adv(10 * time.Second)
	minted := pollDevice(t, srv, id)
	if minted.status != http.StatusOK {
		t.Fatalf("mint status = %d, want 200: %s", minted.status, minted.body)
	}
	var out struct {
		SessionToken string `json:"session_token"`
		Login        string `json:"login"`
	}
	minted.decode(t, &out)
	if out.Login != testLogin {
		t.Errorf("login = %q, want %q", out.Login, testLogin)
	}
	if store.count() != 1 {
		t.Errorf("stored sessions = %d, want 1", store.count())
	}

	res := do(t, srv, "GET", "/api/v1/status", out.SessionToken, nil)
	if res.status != http.StatusOK {
		t.Fatalf("status with a session = %d, want 200: %s", res.status, res.body)
	}
	res = do(t, srv, "GET", "/api/v1/status", testToken, nil)
	if res.status != http.StatusOK {
		t.Errorf("status with the static token = %d, want 200; identity login is additive and must "+
			"not lock out the token the tray and compose file use", res.status)
	}
}

func TestSessionTokenIsNeverStoredInTheClear(t *testing.T) {
	p := newFakeProvider(authorized())
	store := newFakeSessions()
	srv := identityAPI(t, identityDeps(p, store))
	tok := signIn(t, srv)
	if strings.Contains(store.raw(), tok) {
		t.Fatalf("the store holds the session token, so a stolen store file is a stolen credential:\n%s",
			store.raw())
	}
	if _, ok := store.recs[hashToken(tok)]; !ok {
		t.Error("the store is not keyed by the token hash, so a lookup cannot find the session")
	}
}

func TestUnknownLoginIsForbiddenNotBadGateway(t *testing.T) {
	p := newFakeProvider(DeviceResult{Login: "stranger", UserID: 99, Kind: "User"})
	store := newFakeSessions()
	srv := identityAPI(t, identityDeps(p, store))
	adv := freezeAuthClock(t)
	id := authIDOf(t, startDevice(t, srv))
	adv(10 * time.Second)
	res := pollDevice(t, srv, id)
	if res.status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; KindAuth maps to 502 by default, which would send the "+
			"operator to debug GitHub for a decision mino made: %s", res.status, res.body)
	}
	if res.contains("session_token") {
		t.Errorf("a refused sign-in returned a token:\n%s", res.body)
	}
	if store.count() != 0 {
		t.Errorf("stored sessions = %d after a refused sign-in, want 0", store.count())
	}
	if res.contains(testLogin) {
		t.Errorf("the refusal listed the allow-list back to the caller:\n%s", res.body)
	}
}

func TestAllowListIsCaseInsensitive(t *testing.T) {
	p := newFakeProvider(DeviceResult{Login: "CodyConfer", UserID: 7, Kind: "User"})
	srv := identityAPI(t, identityDeps(p, newFakeSessions()), "codyCONFER")
	adv := freezeAuthClock(t)
	id := authIDOf(t, startDevice(t, srv))
	adv(10 * time.Second)
	if res := pollDevice(t, srv, id); res.status != http.StatusOK {
		t.Fatalf("status = %d, want 200; forge logins are case-insensitive: %s", res.status, res.body)
	}
}

func TestNonUserAccountsAreRefused(t *testing.T) {
	p := newFakeProvider(DeviceResult{Login: testLogin, UserID: 1, Kind: "Bot"})
	srv := identityAPI(t, identityDeps(p, newFakeSessions()))
	adv := freezeAuthClock(t)
	id := authIDOf(t, startDevice(t, srv))
	adv(10 * time.Second)
	res := pollDevice(t, srv, id)
	if res.status != http.StatusForbidden {
		t.Errorf("status = %d for a Bot account, want 403; a bot is not an interactive session",
			res.status)
	}
}

func TestEmptyAllowListPermitsNobody(t *testing.T) {
	p := newFakeProvider(authorized())
	srv := newTestServer(t, New(Config{
		Token: testToken, TokenSource: "test", MaxConcurrent: 2, SessionTTL: time.Hour,
	}, identityDeps(p, newFakeSessions())))
	adv := freezeAuthClock(t)
	id := authIDOf(t, startDevice(t, srv))
	adv(10 * time.Second)
	if res := pollDevice(t, srv, id); res.status != http.StatusForbidden {
		t.Errorf("status = %d with an empty allow-list, want 403; read as \"allow all\" this would put "+
			"flight triggers behind nothing but a forge account", res.status)
	}
}

func TestDeadAuthorizationsAreIndistinguishable(t *testing.T) {
	bodies := map[string]string{}
	statuses := map[string]int{}

	run := func(name string, res DeviceResult) {
		p := newFakeProvider(res)
		srv := identityAPI(t, identityDeps(p, newFakeSessions()))
		adv := freezeAuthClock(t)
		id := authIDOf(t, startDevice(t, srv))
		adv(10 * time.Second)
		r := pollDevice(t, srv, id)
		statuses[name], bodies[name] = r.status, string(r.body)
	}
	run("denied", DeviceResult{Denied: true})
	run("expired", DeviceResult{Expired: true})

	srv := identityAPI(t, identityDeps(newFakeProvider(), newFakeSessions()))
	r := pollDevice(t, srv, "never-existed")
	statuses["unknown"], bodies["unknown"] = r.status, string(r.body)

	for name, st := range statuses {
		if st != http.StatusGone {
			t.Errorf("%s status = %d, want 410", name, st)
		}
	}
	first := bodies["denied"]
	for name, b := range bodies {
		if b != first {
			t.Errorf("the %s body differs from the denied body, which tells anyone holding a stale "+
				"auth_id that it was once real:\n%s\n%s", name, first, b)
		}
	}
}

func TestUnknownAuthIDDoesNotReachTheProvider(t *testing.T) {
	p := newFakeProvider()
	srv := identityAPI(t, identityDeps(p, newFakeSessions()))
	pollDevice(t, srv, "never-existed")
	if _, polls := p.counts(); polls != 0 {
		t.Errorf("the provider was polled %d times for an unknown auth_id, want 0; that is free "+
			"outbound work for an anonymous caller", polls)
	}
}

func TestAuthIDIsSingleUse(t *testing.T) {
	p := newFakeProvider(authorized())
	store := newFakeSessions()
	srv := identityAPI(t, identityDeps(p, store))
	adv := freezeAuthClock(t)
	id := authIDOf(t, startDevice(t, srv))
	adv(10 * time.Second)
	if res := pollDevice(t, srv, id); res.status != http.StatusOK {
		t.Fatalf("first poll status = %d, want 200: %s", res.status, res.body)
	}
	adv(10 * time.Second)
	if res := pollDevice(t, srv, id); res.status != http.StatusGone {
		t.Errorf("second poll status = %d, want 410; a replayed auth_id must not mint a second "+
			"session", res.status)
	}
	if store.count() != 1 {
		t.Errorf("stored sessions = %d, want 1", store.count())
	}
}

func TestPollingFasterThanTheIntervalIsRefusedWithoutTouchingTheProvider(t *testing.T) {
	p := newFakeProvider(DeviceResult{Pending: true})
	srv := identityAPI(t, identityDeps(p, newFakeSessions()))
	adv := freezeAuthClock(t)
	id := authIDOf(t, startDevice(t, srv))

	adv(time.Second)
	res := pollDevice(t, srv, id)
	if res.status != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429; the interval is the provider's, and burning it gets every "+
			"caller of this client id throttled: %s", res.status, res.body)
	}
	if res.header.Get("Retry-After") == "" {
		t.Error("no Retry-After on a too-soon poll")
	}
	if _, polls := p.counts(); polls != 0 {
		t.Errorf("the provider was polled %d times, want 0", polls)
	}
}

func TestSlowDownWidensTheInterval(t *testing.T) {
	p := newFakeProvider(DeviceResult{Pending: true, SlowDown: true})
	srv := identityAPI(t, identityDeps(p, newFakeSessions()))
	adv := freezeAuthClock(t)
	id := authIDOf(t, startDevice(t, srv))
	adv(10 * time.Second)
	res := pollDevice(t, srv, id)
	if res.status != http.StatusAccepted {
		t.Fatalf("status = %d, want 202: %s", res.status, res.body)
	}
	var out struct {
		Interval int `json:"interval"`
	}
	res.decode(t, &out)
	if out.Interval <= 5 {
		t.Errorf("interval = %d after slow_down, want more than the original 5", out.Interval)
	}
}

func TestPendingAuthorizationsAreCappedAndNotEvicted(t *testing.T) {
	p := newFakeProvider(DeviceResult{Pending: true})
	srv := identityAPI(t, identityDeps(p, newFakeSessions()))
	adv := freezeAuthClock(t)

	first := authIDOf(t, startDevice(t, srv))
	limited := false
	for i := 0; i < maxPendingAuths+2; i++ {
		if res := startDevice(t, srv); res.status == http.StatusTooManyRequests {
			limited = true
			if res.header.Get("Retry-After") == "" {
				t.Error("no Retry-After when refusing a start")
			}
			break
		}
	}
	if !limited {
		t.Fatalf("starts were never refused; an anonymous caller can allocate unbounded server "+
			"state and outbound calls (cap is %d per IP)", maxPendingPerIP)
	}
	adv(10 * time.Second)
	if res := pollDevice(t, srv, first); res.status != http.StatusAccepted {
		t.Errorf("the first authorization returned %d after the cap was hit, want 202; evicting it "+
			"would let one caller cancel someone else's sign-in", res.status)
	}
}

func TestStartRateIsLimitedPerRemoteAddr(t *testing.T) {
	p := newFakeProvider(DeviceResult{Pending: true})
	srv := identityAPI(t, identityDeps(p, newFakeSessions()))
	freezeAuthClock(t)
	for i := 0; i < maxStartsPerIP; i++ {
		startDevice(t, srv)
	}
	if res := startDevice(t, srv); res.status != http.StatusTooManyRequests {
		t.Errorf("start %d status = %d, want 429", maxStartsPerIP+1, res.status)
	}
}

func TestForwardedForCannotBuyMoreStarts(t *testing.T) {
	p := newFakeProvider(DeviceResult{Pending: true})
	srv := identityAPI(t, identityDeps(p, newFakeSessions()))
	freezeAuthClock(t)
	last := 0
	for i := 0; i < maxStartsPerIP+4; i++ {
		req, _ := http.NewRequestWithContext(context.Background(), "POST",
			srv.URL+"/api/v1/auth/device/github", strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("10.0.0.%d", i))
		res, err := srv.Client().Do(req)
		if err != nil {
			t.Fatalf("POST: %v", err)
		}
		res.Body.Close()
		last = res.StatusCode
	}
	if last != http.StatusTooManyRequests {
		t.Errorf("status = %d after spoofing X-Forwarded-For, want 429; gin's ClientIP trusts that "+
			"header by default, so a limit keyed on it is one header away from useless", last)
	}
}

func TestSignInRoutesRequireJSONContentType(t *testing.T) {
	srv := identityAPI(t, identityDeps(newFakeProvider(), newFakeSessions()))
	req, _ := http.NewRequestWithContext(context.Background(), "POST",
		srv.URL+"/api/v1/auth/device/github", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "text/plain")
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnsupportedMediaType {
		t.Errorf("status = %d, want 415; a request a browser can send without a preflight is one a "+
			"hostile page can fire from a victim's machine", res.StatusCode)
	}
}

func TestNoCORSHeadersAreEverEmitted(t *testing.T) {
	srv := identityAPI(t, identityDeps(newFakeProvider(), newFakeSessions()))
	for _, tc := range []struct{ method, path, token string }{
		{"GET", "/healthz", ""},
		{"GET", "/api/v1/status", testToken},
		{"POST", "/api/v1/auth/device/github", ""},
	} {
		req, _ := http.NewRequestWithContext(context.Background(), tc.method, srv.URL+tc.path,
			strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "https://evil.example")
		if tc.token != "" {
			req.Header.Set("Authorization", "Bearer "+tc.token)
		}
		res, err := srv.Client().Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", tc.method, tc.path, err)
		}
		res.Body.Close()
		for h := range res.Header {
			if strings.HasPrefix(http.CanonicalHeaderKey(h), "Access-Control-") {
				t.Errorf("%s %s emitted %s; one CORS header would make these responses readable by "+
					"any page the user visits", tc.method, tc.path, h)
			}
		}
	}
}

func TestSignInRoutesKeepTheLoopbackHostGuard(t *testing.T) {
	srv := identityAPI(t, identityDeps(newFakeProvider(), newFakeSessions()))
	req, _ := http.NewRequestWithContext(context.Background(), "POST",
		srv.URL+"/api/v1/auth/device/github", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Host = "evil.example"
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d for a non-loopback Host, want 401; the sign-in routes never reach the "+
			"bearer check the rebinding guard used to live in", res.StatusCode)
	}
}

func TestUnknownProviderIs404(t *testing.T) {
	srv := identityAPI(t, identityDeps(newFakeProvider(), newFakeSessions()))
	res := do(t, srv, "POST", "/api/v1/auth/device/gitlab", "", strings.NewReader("{}"))
	if res.status != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.status)
	}
}

func TestMissingAuthIDIs400(t *testing.T) {
	srv := identityAPI(t, identityDeps(newFakeProvider(), newFakeSessions()))
	for _, body := range []string{`{}`, `{"auth_id":""}`} {
		res := do(t, srv, "POST", "/api/v1/auth/device/github/token", "", strings.NewReader(body))
		if res.status != http.StatusBadRequest {
			t.Errorf("status = %d for %s, want 400", res.status, body)
		}
	}
	res := do(t, srv, "POST", "/api/v1/auth/device/github/token", "",
		strings.NewReader(`{"auth_id":"x","nope":1}`))
	if res.status != http.StatusBadRequest {
		t.Errorf("status = %d for an unknown field, want 400", res.status)
	}
}

func TestProviderFailureMapsUpstream(t *testing.T) {
	for _, tc := range []struct {
		kind errs.Kind
		want int
	}{
		{errs.KindSignal, http.StatusBadGateway},
		{errs.KindAuth, http.StatusBadGateway},
		{errs.KindConfig, http.StatusBadRequest},
		{errs.KindStore, http.StatusServiceUnavailable},
	} {
		p := newFakeProvider()
		p.startErr = errs.New(tc.kind, "upstream said no")
		srv := identityAPI(t, identityDeps(p, newFakeSessions()))
		freezeAuthClock(t)
		if res := startDevice(t, srv); res.status != tc.want {
			t.Errorf("kind %s mapped to %d, want %d; statusFor must stay untouched",
				tc.kind, res.status, tc.want)
		}
	}
}

func TestSessionStoreFailureFailsTheMint(t *testing.T) {
	p := newFakeProvider(authorized())
	store := newFakeSessions()
	store.putErr = errs.New(errs.KindStore, "the store is unavailable")
	srv := identityAPI(t, identityDeps(p, store))
	adv := freezeAuthClock(t)
	id := authIDOf(t, startDevice(t, srv))
	adv(10 * time.Second)
	res := pollDevice(t, srv, id)
	if res.status != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: %s", res.status, res.body)
	}
	if res.contains("session_token") {
		t.Errorf("a session the daemon could not persist was handed to the caller, so nobody can "+
			"revoke it after a restart:\n%s", res.body)
	}
}

func TestAuthEventsAreAudited(t *testing.T) {
	var mu sync.Mutex
	events := map[string]map[string]string{}
	p := newFakeProvider(authorized())
	d := identityDeps(p, newFakeSessions())
	d.AuditAuth = func(event string, attrs map[string]string) {
		mu.Lock()
		defer mu.Unlock()
		events[event] = attrs
	}
	srv := identityAPI(t, d)
	tok := signIn(t, srv)
	do(t, srv, "DELETE", "/api/v1/auth/session", tok, nil)

	mu.Lock()
	defer mu.Unlock()
	for _, want := range []string{"auth.device.start", "auth.session.mint", "auth.session.revoke"} {
		if _, ok := events[want]; !ok {
			t.Errorf("no %s audit row; an operator cannot see who signed in", want)
		}
	}
	for event, attrs := range events {
		for k, v := range attrs {
			if strings.Contains(v, tok) || strings.Contains(v, testDeviceCode) ||
				strings.Contains(v, hashToken(tok)) {
				t.Errorf("%s carried a secret in %s; mino audit prints these rows to a terminal "+
					"and into the log dir", event, k)
			}
		}
	}
}

func TestDeniedSignInIsAudited(t *testing.T) {
	var got bool
	p := newFakeProvider(DeviceResult{Login: "stranger", UserID: 5, Kind: "User"})
	d := identityDeps(p, newFakeSessions())
	d.AuditAuth = func(event string, _ map[string]string) {
		if event == "auth.denied.allowlist" {
			got = true
		}
	}
	srv := identityAPI(t, d)
	adv := freezeAuthClock(t)
	id := authIDOf(t, startDevice(t, srv))
	adv(10 * time.Second)
	pollDevice(t, srv, id)
	if !got {
		t.Error("an allow-list refusal was not audited; that is the row an operator most needs")
	}
}

func TestConcurrentPollsMintExactlyOneSession(t *testing.T) {
	p := newFakeProvider(authorized())
	store := newFakeSessions()
	srv := identityAPI(t, identityDeps(p, store))
	adv := freezeAuthClock(t)
	id := authIDOf(t, startDevice(t, srv))
	adv(10 * time.Second)

	var wg sync.WaitGroup
	codes := make([]int, 4)
	for i := range codes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes[i] = pollDevice(t, srv, id).status
		}()
	}
	wg.Wait()
	ok := 0
	for _, c := range codes {
		if c == http.StatusOK {
			ok++
		}
	}
	if ok != 1 {
		t.Errorf("%d of %d concurrent polls minted a session, want exactly 1; two sessions from one "+
			"authorization means revoking one leaves the other alive", ok, len(codes))
	}
	if store.count() != 1 {
		t.Errorf("stored sessions = %d, want 1", store.count())
	}
}

func TestUnauthenticatedSignInRoutesNeedNoToken(t *testing.T) {
	p := newFakeProvider(DeviceResult{Pending: true})
	srv := identityAPI(t, identityDeps(p, newFakeSessions()))
	freezeAuthClock(t)
	if res := startDevice(t, srv); res.status != http.StatusCreated {
		t.Fatalf("start status = %d without a token, want 201: %s", res.status, res.body)
	}
	if res := do(t, srv, "GET", "/api/v1/status", "", nil); res.status != http.StatusUnauthorized {
		t.Errorf("status without a token = %d, want 401; the sign-in exemption must not widen", res.status)
	}
}
