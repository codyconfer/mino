package httpapi

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/codyconfer/mino/internal/app/flight"
	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/signals"
)

func okSections() []signals.Section {
	return []signals.Section{{
		Signal: "demo",
		Title:  "Demo",
		Items:  []signals.Item{{Kind: "note", Title: "hello"}},
	}}
}

func TestFlightRunMatchesOJSONByteForByte(t *testing.T) {
	sections := okSections()
	d := fakeDeps()
	d.RunFlight = func(context.Context, string) ([]signals.Section, error) { return sections, nil }
	srv := testAPI(t, d)

	res := do(t, srv, "POST", "/api/v1/flights/default", testToken, nil)
	if res.status != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.status)
	}
	got := res.body

	var want bytes.Buffer
	if err := flight.Emit(&want, "json", "default", sections); err != nil {
		t.Fatalf("flight.Emit: %v", err)
	}
	// The API's whole contract is "same bytes as `mino fly -o json`". If someone
	// later "improves" the response shape, this is what stops it.
	if !bytes.Equal(got, want.Bytes()) {
		t.Errorf("response body diverged from -o json\n got: %s\nwant: %s", got, want.Bytes())
	}
	if res.header.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", res.header.Get("Content-Type"))
	}
}

func TestQueryRunMatchesOJSONByteForByte(t *testing.T) {
	sections := okSections()
	d := fakeDeps()
	d.RunQuery = func(context.Context, string) ([]signals.Section, error) { return sections, nil }
	srv := testAPI(t, d)

	res := do(t, srv, "POST", "/api/v1/queries/prs", testToken, nil)
	got := res.body

	var want bytes.Buffer
	if err := flight.Emit(&want, "json", "prs", sections); err != nil {
		t.Fatalf("flight.Emit: %v", err)
	}
	if !bytes.Equal(got, want.Bytes()) {
		t.Errorf("response body diverged from -o json\n got: %s\nwant: %s", got, want.Bytes())
	}
}

func TestUnknownFlightIs404AndUnknownQueryIs404(t *testing.T) {
	d := fakeDeps()
	d.FlightExists = func(string) bool { return false }
	d.QueryExists = func(string) bool { return false }
	srv := testAPI(t, d)

	for _, path := range []string{"/api/v1/flights/nope", "/api/v1/queries/nope"} {
		res := do(t, srv, "POST", path, testToken, nil)
		if res.status != http.StatusNotFound {
			t.Errorf("POST %s = %d, want 404", path, res.status)
		}
	}
}

func TestRoleInvisibleFlightIs403(t *testing.T) {
	d := fakeDeps()
	d.FlightVisible = func(string) bool { return false }
	srv := testAPI(t, d)

	res := do(t, srv, "POST", "/api/v1/flights/secret", testToken, nil)
	if res.status != http.StatusForbidden {
		t.Errorf("status = %d, want 403; the role scope must apply over http exactly as it does on the CLI",
			res.status)
	}
}

func TestPartialFailureIs200WithACountHeader(t *testing.T) {
	sections := []signals.Section{
		{Signal: "ok", Items: []signals.Item{{Title: "a"}}},
		{Signal: "bad", Err: errors.New("upstream exploded")},
	}
	d := fakeDeps()
	d.RunFlight = func(context.Context, string) ([]signals.Section, error) {
		return sections, flight.Failure(sections)
	}
	srv := testAPI(t, d)

	res := do(t, srv, "POST", "/api/v1/flights/default", testToken, nil)
	if res.status != http.StatusOK {
		t.Errorf("status = %d for a partly-failed flight, want 200; the CLI still prints partial results",
			res.status)
	}
	if got := res.header.Get("X-Mino-Sections-Failed"); got != "1/2" {
		t.Errorf("X-Mino-Sections-Failed = %q, want 1/2", got)
	}
	body := res.body
	if !strings.Contains(string(body), "upstream exploded") {
		t.Errorf("the per-section error is missing from the body, so a caller cannot see what failed:\n%s", body)
	}
}

func TestTotalFailureIs502ButStillCarriesTheBody(t *testing.T) {
	sections := []signals.Section{{Signal: "bad", Err: errors.New("nothing worked")}}
	d := fakeDeps()
	d.RunFlight = func(context.Context, string) ([]signals.Section, error) {
		return sections, flight.Failure(sections)
	}
	srv := testAPI(t, d)

	res := do(t, srv, "POST", "/api/v1/flights/default", testToken, nil)
	if res.status != http.StatusBadGateway {
		t.Errorf("status = %d when every source failed, want 502", res.status)
	}
	if got := res.header.Get("X-Mino-Outcome"); got != "failed" {
		t.Errorf("X-Mino-Outcome = %q, want failed", got)
	}
	body := res.body
	var want bytes.Buffer
	if err := flight.Emit(&want, "json", "default", sections); err != nil {
		t.Fatalf("flight.Emit: %v", err)
	}
	// Only the status changes on failure; the body stays the -o json shape so a
	// caller can always read the per-section detail.
	if !bytes.Equal(body, want.Bytes()) {
		t.Errorf("failure body diverged from -o json\n got: %s\nwant: %s", body, want.Bytes())
	}
}

func TestConcurrentRunsBeyondTheLimitGet429(t *testing.T) {
	release := make(chan struct{})
	var started sync.WaitGroup
	started.Add(2)
	d := fakeDeps()
	d.RunFlight = func(context.Context, string) ([]signals.Section, error) {
		started.Done()
		<-release
		return okSections(), nil
	}
	srv := testAPI(t, d) // MaxConcurrent: 2

	codes := make(chan int, 3)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res := do(t, srv, "POST", "/api/v1/flights/default", testToken, nil)
			codes <- res.status
		}()
	}
	started.Wait() // both slots are now held

	res := do(t, srv, "POST", "/api/v1/flights/default", testToken, nil)
	if res.status != http.StatusTooManyRequests {
		t.Errorf("third concurrent run = %d, want 429; without a limiter an unattended curl loop walks "+
			"straight into an upstream rate limit", res.status)
	}
	if res.header.Get("Retry-After") == "" {
		t.Error("no Retry-After on the 429, so a client cannot tell how long to back off")
	}

	close(release)
	wg.Wait()
	for range 2 {
		if got := <-codes; got != http.StatusOK {
			t.Errorf("an in-limit run got %d, want 200", got)
		}
	}
}

func TestActionRunsAndReports(t *testing.T) {
	var gotSignal, gotName string
	var gotParams map[string]string
	d := fakeDeps()
	d.RunAction = func(_ context.Context, signal, name string, params map[string]string) error {
		gotSignal, gotName, gotParams = signal, name, params
		return nil
	}
	srv := testAPI(t, d)

	res := do(t, srv, "POST", "/api/v1/actions/ntr/note.add", testToken,
		strings.NewReader(`{"params":{"title":"from curl"}}`))
	if res.status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", res.status, res.body)
	}
	if gotSignal != "ntr" || gotName != "note.add" {
		t.Errorf("dispatched %s/%s, want ntr/note.add", gotSignal, gotName)
	}
	if gotParams["title"] != "from curl" {
		t.Errorf("params = %v, want title=from curl", gotParams)
	}
	var body map[string]any
	res.decode(t, &body)
	if body["ok"] != true {
		t.Errorf("body = %v, want ok:true", body)
	}
}

func TestActionRejectsCallerSuppliedHomeAndRole(t *testing.T) {
	called := false
	d := fakeDeps()
	d.RunAction = func(context.Context, string, string, map[string]string) error {
		called = true
		return nil
	}
	srv := testAPI(t, d)

	for _, param := range []string{"home", "role"} {
		res := do(t, srv, "POST", "/api/v1/actions/ntr/note.add", testToken,
			strings.NewReader(`{"params":{"`+param+`":"/tmp/evil"}}`))
		if res.status != http.StatusBadRequest {
			t.Errorf("params.%s = %d, want 400; a caller-set home points a plugin's writes outside the "+
				"mino directory", param, res.status)
		}
	}
	if called {
		t.Error("the action ran despite a rejected param")
	}
}

func TestUnknownActionIs404AndDisabledSignalIs409(t *testing.T) {
	d := fakeDeps()
	d.ActionExists = func(string, string) bool { return false }
	srv := testAPI(t, d)
	// build.Action reports a missing action as errs.KindSignal, which would map to
	// a misleading 502; the handler checks existence first for this reason.
	if res := do(t, srv, "POST", "/api/v1/actions/ntr/nope", testToken, nil); res.status != http.StatusNotFound {
		t.Errorf("unknown action = %d, want 404", res.status)
	}

	d2 := fakeDeps()
	d2.SignalEnabled = func(string) bool { return false }
	srv2 := testAPI(t, d2)
	if res := do(t, srv2, "POST", "/api/v1/actions/ntr/note.add", testToken, nil); res.status != http.StatusConflict {
		t.Errorf("disabled signal = %d, want 409; it exists, it is just switched off", res.status)
	}
}

func TestSlowActionIs504(t *testing.T) {
	d := fakeDeps()
	d.Timeout = func() time.Duration { return 20 * time.Millisecond }
	d.RunAction = func(ctx context.Context, _, _ string, _ map[string]string) error {
		<-ctx.Done()
		return ctx.Err()
	}
	srv := testAPI(t, d)

	res := do(t, srv, "POST", "/api/v1/actions/ntr/note.add", testToken, nil)
	if res.status != http.StatusGatewayTimeout {
		t.Errorf("status = %d for a stalled action, want 504; the CLI runs actions unbounded but over "+
			"http that is a wedged connection and a leaked goroutine", res.status)
	}
}

func TestFailingActionIs502(t *testing.T) {
	d := fakeDeps()
	d.RunAction = func(context.Context, string, string, map[string]string) error {
		return errs.New(errs.KindSignal, "the plugin said no")
	}
	srv := testAPI(t, d)

	if res := do(t, srv, "POST", "/api/v1/actions/ntr/note.add", testToken, nil); res.status != http.StatusBadGateway {
		t.Errorf("status = %d for a failing action, want 502", res.status)
	}
}
