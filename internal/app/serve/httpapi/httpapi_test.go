package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/codyconfer/mino/internal/app/flight"
	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/signals"
)

const testToken = "test-token-long-enough"

// fakeDeps is a fully wired Deps whose behaviour tests override field by field.
func fakeDeps() Deps {
	return Deps{
		RunFlight: func(context.Context, string) ([]signals.Section, error) { return nil, nil },
		RunQuery:  func(context.Context, string) ([]signals.Section, error) { return nil, nil },
		RunAction: func(context.Context, string, string, map[string]string) error { return nil },
		EmitJSON: func(w io.Writer, root string, s []signals.Section) error {
			return flight.Emit(w, "json", root, s)
		},
		Tally: func(s []signals.Section) (int, int) {
			o := flight.Tally(s)
			return o.Failed, o.Sections
		},
		FlightExists:  func(string) bool { return true },
		QueryExists:   func(string) bool { return true },
		FlightVisible: func(string) bool { return true },
		QueryVisible:  func(string) bool { return true },
		Flights:       func(bool) any { return []string{"default"} },
		Queries:       func(bool) any { return []string{"prs"} },
		Actions:       func(string) []ActionInfo { return []ActionInfo{{Signal: "ntr", Name: "note.add"}} },
		Config:        func() any { return map[string]string{"home": "/tmp"} },
		Status:        func() Status { return Status{Flight: "default", Role: "test"} },
		ActionExists:  func(string, string) bool { return true },
		SignalEnabled: func(string) bool { return true },
		Subscribe: func(buffer int) (<-chan signals.Event, func()) {
			ch := make(chan signals.Event, buffer)
			return ch, func() {}
		},
		Encode:  func(signals.Event) ([]byte, error) { return []byte(`{}`), nil },
		Timeout: func() time.Duration { return time.Second },
	}
}

func newTestServer(t *testing.T, api *API) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(api.Handler())
	t.Cleanup(srv.Close)
	return srv
}

func testAPI(t *testing.T, d Deps) *httptest.Server {
	t.Helper()
	return newTestServer(t, New(Config{Token: testToken, TokenSource: "test", MaxConcurrent: 2}, d))
}

// reply is a fully-read response, so tests never juggle a live body.
type reply struct {
	status int
	header http.Header
	body   []byte
}

func (r reply) contains(s string) bool { return strings.Contains(string(r.body), s) }

func (r reply) decode(t *testing.T, v any) {
	t.Helper()
	if err := json.Unmarshal(r.body, v); err != nil {
		t.Fatalf("decoding %s: %v", r.body, err)
	}
}

func do(t *testing.T, srv *httptest.Server, method, path, token string, body io.Reader) reply {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), method, srv.URL+path, body)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("reading %s %s: %v", method, path, err)
	}
	return reply{status: res.StatusCode, header: res.Header, body: raw}
}

func TestUnauthenticatedRequestsAreRejected(t *testing.T) {
	srv := testAPI(t, fakeDeps())
	for _, tc := range []struct{ name, header string }{
		{"no header", ""},
		{"wrong token", "Bearer nope"},
		{"basic scheme", "Basic " + testToken},
		{"bare token", testToken},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequestWithContext(context.Background(), "GET", srv.URL+"/api/v1/status", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			res, err := srv.Client().Do(req)
			if err != nil {
				t.Fatalf("GET: %v", err)
			}
			defer res.Body.Close()
			if res.StatusCode != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401; an unauthenticated caller could trigger runs", res.StatusCode)
			}
			b, _ := io.ReadAll(res.Body)
			if strings.Contains(string(b), testToken) {
				t.Errorf("the 401 body echoed the expected token, handing it to the caller:\n%s", b)
			}
			if res.Header.Get("WWW-Authenticate") == "" {
				t.Error("no WWW-Authenticate header, so a client cannot tell which scheme to use")
			}
		})
	}
}

func TestTokenPrefixIsRejected(t *testing.T) {
	srv := testAPI(t, fakeDeps())
	// ConstantTimeCompare must reject a prefix; a naive HasPrefix would not.
	res := do(t, srv, "GET", "/api/v1/status", testToken[:10], nil)
	if res.status != http.StatusUnauthorized {
		t.Errorf("status = %d for a token prefix, want 401", res.status)
	}
}

func TestCorrectTokenIsAccepted(t *testing.T) {
	srv := testAPI(t, fakeDeps())
	res := do(t, srv, "GET", "/api/v1/status", testToken, nil)
	if res.status != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.status)
	}
	var st Status
	res.decode(t, &st)
	if st.Flight != "default" {
		t.Errorf("flight = %q, want default", st.Flight)
	}
}

func TestAnEmptyTokenNeverAuthenticates(t *testing.T) {
	srv := newTestServer(t, New(Config{Token: "", TokenSource: "test", MaxConcurrent: 2}, fakeDeps()))
	for _, header := range []string{"Bearer ", "Bearer", "bearer  "} {
		req, _ := http.NewRequestWithContext(context.Background(), "GET", srv.URL+"/api/v1/status", nil)
		req.Header.Set("Authorization", header)
		res, err := srv.Client().Do(req)
		if err != nil {
			t.Fatalf("GET with %q: %v", header, err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusUnauthorized {
			t.Errorf("status = %d for %q against an empty token, want 401", res.StatusCode, header)
		}
	}
}

func TestHealthzNeedsNoToken(t *testing.T) {
	srv := testAPI(t, fakeDeps())
	res := do(t, srv, "GET", "/healthz", "", nil)
	if res.status != http.StatusOK {
		t.Fatalf("status = %d, want 200; healthz exists so a caller can confirm the listener "+
			"before wrestling with the bearer header", res.status)
	}
}

func TestHostHeaderMustBeLoopback(t *testing.T) {
	srv := testAPI(t, fakeDeps())
	req, _ := http.NewRequestWithContext(context.Background(), "GET", srv.URL+"/api/v1/status", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Host = "evil.example"
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d for a non-loopback Host, want 401; this is the DNS-rebinding guard", res.StatusCode)
	}
}

func TestHostHeaderIsNotCheckedWhenBoundOffBox(t *testing.T) {
	srv := newTestServer(t, New(Config{
		Token: testToken, TokenSource: "test", BindHost: "0.0.0.0", MaxConcurrent: 2,
	}, fakeDeps()))
	req, _ := http.NewRequestWithContext(context.Background(), "GET", srv.URL+"/api/v1/status", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Host = "mino-container"
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d for a non-loopback Host against an off-box bind, want 200; requiring a "+
			"loopback Host would make every caller outside the container fail", res.StatusCode)
	}
}

func TestLoopbackBind(t *testing.T) {
	for _, tc := range []struct {
		host string
		want bool
	}{
		{"", true},
		{"127.0.0.1", true},
		{"localhost", true},
		{"::1", true},
		{"[::1]", true},
		{"127.0.0.53", true},
		{"0.0.0.0", false},
		{"::", false},
		{"192.168.1.10", false},
		{"mino-container", false},
	} {
		if got := LoopbackBind(tc.host); got != tc.want {
			t.Errorf("LoopbackBind(%q) = %v, want %v", tc.host, got, tc.want)
		}
	}
}

func TestWrongMethodIs405(t *testing.T) {
	srv := testAPI(t, fakeDeps())
	res := do(t, srv, "GET", "/api/v1/flights/default", testToken, nil)
	if res.status != http.StatusMethodNotAllowed {
		t.Errorf("GET on a POST-only trigger = %d, want 405; gin leaves HandleMethodNotAllowed off, "+
			"and a 404 here would read as a missing route", res.status)
	}
	if allow := res.header.Get("Allow"); !strings.Contains(allow, "POST") {
		t.Errorf("Allow = %q, want POST listed so a client can correct itself", allow)
	}
}

func TestRoutesLiveUnderAPIV1AndTheOldPathsAreGone(t *testing.T) {
	srv := testAPI(t, fakeDeps())
	if res := do(t, srv, "GET", "/api/v1/status", testToken, nil); res.status != http.StatusOK {
		t.Errorf("GET /api/v1/status = %d, want 200", res.status)
	}
	for _, path := range []string{"/v1/status", "/v1/list", "/status"} {
		res := do(t, srv, "GET", path, testToken, nil)
		if res.status != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404; a pre-move path that still works leaves half the callers "+
				"on a prefix nobody maintains", path, res.status)
		}
	}
}

func TestEveryJSONBodyIsBareApplicationJSON(t *testing.T) {
	d := fakeDeps()
	d.RunFlight = func(context.Context, string) ([]signals.Section, error) { return okSections(), nil }
	srv := testAPI(t, d)

	for _, tc := range []struct {
		name, method, path, token string
	}{
		{"success", "POST", "/api/v1/flights/default", testToken},
		{"read", "GET", "/api/v1/status", testToken},
		{"unauthorized", "GET", "/api/v1/status", ""},
		{"not found", "GET", "/v1/status", testToken},
		{"wrong method", "GET", "/api/v1/flights/default", testToken},
	} {
		res := do(t, srv, tc.method, tc.path, tc.token, nil)
		if got := res.header.Get("Content-Type"); got != "application/json" {
			t.Errorf("%s: Content-Type = %q, want exactly application/json; gin's own JSON render "+
				"appends charset=utf-8, which breaks clients matching the header and the -o json "+
				"comparison", tc.name, got)
		}
	}
}

func TestJSONBodiesAreIndentedAndUnescaped(t *testing.T) {
	d := fakeDeps()
	d.Config = func() any { return map[string]string{"hint": "a && b <c>"} }
	srv := testAPI(t, d)

	res := do(t, srv, "GET", "/api/v1/config", testToken, nil)
	if !res.contains("a && b <c>") {
		t.Errorf("body escaped HTML:\n%s\nSetEscapeHTML(false) is what keeps a response identical to "+
			"-o json", res.body)
	}
	if !res.contains("\n  ") {
		t.Errorf("body is not two-space indented:\n%s", res.body)
	}
}

func TestErrorBodyCarriesKindAndHint(t *testing.T) {
	d := fakeDeps()
	d.FlightExists = func(string) bool { return false }
	srv := testAPI(t, d)

	res := do(t, srv, "POST", "/api/v1/flights/nope", testToken, nil)
	if res.status != http.StatusNotFound {
		t.Fatalf("status = %d for an unknown flight, want 404", res.status)
	}
	var body errBody
	res.decode(t, &body)
	if body.Error.Kind == "" || body.Error.Message == "" {
		t.Errorf("error envelope = %+v; kind and message are what let a client branch", body.Error)
	}
	if body.Error.Hint == "" {
		t.Error("no hint; the CLI always offers one and the API should not be worse")
	}
}

func TestErrorBodyStripsTerminalEscapes(t *testing.T) {
	// An action error relays whatever a plugin or remote API said, so it is the
	// realistic path for hostile text to reach a response body.
	d := fakeDeps()
	d.RunAction = func(context.Context, string, string, map[string]string) error {
		return errs.New(errs.KindSignal, "boom \x1b[31mred\x07 \u202ereversed").
			WithHint("also hostile \x1b[0m")
	}
	srv := testAPI(t, d)

	res := do(t, srv, "POST", "/api/v1/actions/ntr/note.add", testToken, nil)
	if res.status != http.StatusBadGateway {
		t.Fatalf("status = %d for a failing action, want 502", res.status)
	}
	for _, bad := range []string{"\x1b", "\x07", "\u202e"} {
		if res.contains(bad) {
			t.Errorf("error body carries %q; signal errors relay remote text and must be sanitized:\n%q",
				bad, res.body)
		}
	}
}

func TestUnknownBodyFieldIs400(t *testing.T) {
	srv := testAPI(t, fakeDeps())
	res := do(t, srv, "POST", "/api/v1/flights/default", testToken, strings.NewReader(`{"nope":1}`))
	if res.status != http.StatusBadRequest {
		t.Errorf("status = %d for an unknown body field, want 400; silently ignoring it would let a "+
			"newer client think an option applied", res.status)
	}
}

func TestNonJSONBodyIs400(t *testing.T) {
	srv := testAPI(t, fakeDeps())
	req, _ := http.NewRequestWithContext(context.Background(), "POST", srv.URL+"/api/v1/flights/default",
		strings.NewReader("k=v"))
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d for a form body, want 400", res.StatusCode)
	}
}

func TestKindToStatus(t *testing.T) {
	for _, tc := range []struct {
		kind errs.Kind
		want int
	}{
		{errs.KindUsage, http.StatusBadRequest},
		{errs.KindConfig, http.StatusBadRequest},
		{errs.KindAuth, http.StatusBadGateway},
		{errs.KindSignal, http.StatusBadGateway},
		{errs.KindStore, http.StatusServiceUnavailable},
		{errs.KindOnboarding, http.StatusPreconditionFailed},
		{errs.KindBackup, http.StatusInternalServerError},
		{errs.KindInternal, http.StatusInternalServerError},
	} {
		if got := statusFor(errs.New(tc.kind, "x")); got != tc.want {
			t.Errorf("statusFor(%s) = %d, want %d", tc.kind, got, tc.want)
		}
	}
	// KindAuth must never be 401: that status is reserved for our own bearer, and
	// conflating them makes "your token is wrong" indistinguishable from
	// "mino cannot reach GitHub".
	if statusFor(errs.New(errs.KindAuth, "no github token")) == http.StatusUnauthorized {
		t.Error("KindAuth mapped to 401, which collides with a bad bearer token")
	}
}

func TestExplicitStatusBeatsTheKind(t *testing.T) {
	// KindUsage alone cannot distinguish 400 from 404/403/409, so handlers pin
	// the status explicitly. Wrapping must not disturb the kind in the body.
	for _, want := range []int{http.StatusNotFound, http.StatusForbidden, http.StatusConflict} {
		err := withStatus(want, errs.New(errs.KindUsage, "x").WithHint("h"))
		if got := statusFor(err); got != want {
			t.Errorf("statusFor(withStatus(%d, ...)) = %d, want %d", want, got, want)
		}
		if got := errs.KindOf(err); got != errs.KindUsage {
			t.Errorf("KindOf = %q, want usage; wrapping lost the kind the body reports", got)
		}
		if got := errs.Hint(err); got != "h" {
			t.Errorf("Hint = %q, want h; wrapping lost the hint", got)
		}
		if got := err.Error(); got != "x" {
			t.Errorf("Error() = %q, want x; the status must not leak into the message", got)
		}
	}
}
