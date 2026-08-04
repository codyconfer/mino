package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/codyconfer/mino/internal/errs"
)

const testDeadline = 3 * time.Second

func noGHOnPath(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
	t.Setenv("GITHUB_TOKEN", "test-token")
	t.Setenv("GH_TOKEN", "")
	t.Setenv("HOME", t.TempDir())
}

func withShortTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	prev := sharedHTTPClient
	sharedHTTPClient = &http.Client{Transport: sharedTransport, Timeout: d}
	t.Cleanup(func() { sharedHTTPClient = prev })
}

func silentServer(t *testing.T) *httptest.Server {
	t.Helper()
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() {
		close(release)
		srv.Close()
	})
	return srv
}

func endlessServer(t *testing.T, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		chunk := []byte(strings.Repeat("a", 32<<10))
		for {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func withinDeadline(t *testing.T, what string, call func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- call() }()
	select {
	case err := <-done:
		return err
	case <-time.After(testDeadline):
		t.Fatalf("%s did not return within %s", what, testDeadline)
		return nil
	}
}

func TestAuthClientsHaveBoundedTimeouts(t *testing.T) {
	if got := HTTPClient().Timeout; got != HTTPTimeout {
		t.Errorf("shared client timeout = %s, want %s", got, HTTPTimeout)
	}
	if got := deviceFlowHTTPClient.Timeout; got != DeviceFlowTimeout {
		t.Errorf("device flow client timeout = %s, want %s", got, DeviceFlowTimeout)
	}
	if deviceFlowHTTPClient.Timeout < HTTPClient().Timeout {
		t.Error("the interactive device flow deserves at least the standard budget")
	}
	tr, ok := sharedTransport.(*http.Transport)
	if !ok {
		t.Fatalf("shared transport is %T", sharedTransport)
	}
	if tr.MaxIdleConns != httpMaxIdleConns || tr.MaxIdleConnsPerHost != httpMaxIdlePerHost {
		t.Errorf("transport pool = %d/%d, want %d/%d",
			tr.MaxIdleConns, tr.MaxIdleConnsPerHost, httpMaxIdleConns, httpMaxIdlePerHost)
	}
}

func TestGHAPIGetGivesUpOnASilentServer(t *testing.T) {
	noGHOnPath(t)
	withShortTimeout(t, 200*time.Millisecond)
	srv := silentServer(t)

	err := withinDeadline(t, "GHAPIGet", func() error {
		_, err := GHAPIGet(context.Background(), ambientSelection(t, srv.URL), "user")
		return err
	})
	if err == nil {
		t.Fatal("want a timeout error from a server that never responds")
	}
}

func TestGHAPIGetBoundsAnEndlessBody(t *testing.T) {
	noGHOnPath(t)
	srv := endlessServer(t, http.StatusOK)

	done := make(chan error, 1)
	go func() {
		_, err := GHAPIGet(context.Background(), ambientSelection(t, srv.URL), "user")
		done <- err
	}()
	var err error
	select {
	case err = <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("GHAPIGet never stopped reading: the body read is unbounded")
	}
	if err == nil {
		t.Fatal("want an oversize-body error")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("error = %v, want the size limit named", err)
	}
}

func ghAPIGetAgainst(t *testing.T, h http.HandlerFunc) error {
	t.Helper()
	noGHOnPath(t)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	_, err := GHAPIGet(context.Background(), ambientSelection(t, srv.URL), "user")
	if err == nil {
		t.Fatal("want an error")
	}
	return err
}

func TestGHAPIGetErrorsCarryNoTerminalEscapes(t *testing.T) {
	err := ghAPIGetAgainst(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("\x1b]0;pwned\a\x1b[2J\x1b[32mall checks passed\x1b[0m\x7f\nsecond line"))
	})
	msg := err.Error()
	for _, r := range msg {
		if r == 0x1b || r == 0x07 || r == 0x7f || r == '\n' || r == '\r' || r < 0x20 {
			t.Fatalf("error message carries control bytes: %q", msg)
		}
	}
	if !strings.Contains(msg, "all checks passed") {
		t.Fatalf("error message dropped the readable excerpt: %q", msg)
	}
	if rendered := errs.Render(err); strings.ContainsRune(rendered, 0x1b) {
		t.Fatalf("Render leaked escapes: %q", rendered)
	}
}

func TestGHAPIGetErrorsAreBounded(t *testing.T) {
	err := ghAPIGetAgainst(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		for i := 0; i < 64; i++ {
			if _, werr := w.Write([]byte(strings.Repeat("PADDING ", 4096))); werr != nil {
				return
			}
		}
	})
	if len(err.Error()) > 2048 {
		t.Fatalf("error message is %d bytes; a short excerpt is enough", len(err.Error()))
	}
}

func TestGHAPIGetClassifies401And403Distinctly(t *testing.T) {
	cases := []struct {
		name     string
		handler  http.HandlerFunc
		wantKind errs.Kind
		wantHint string
	}{
		{
			name: "401 unauthorized",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"message":"Bad credentials"}`))
			},
			wantKind: errs.KindAuth,
			wantHint: "mino login github",
		},
		{
			name: "403 rate limited",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("X-RateLimit-Remaining", "0")
				w.Header().Set("Retry-After", "60")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"message":"API rate limit exceeded"}`))
			},
			wantKind: errs.KindSignal,
			wantHint: "rate limit reached; retry after 1m0s",
		},
		{
			name: "429 rate limited",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"message":"too many requests"}`))
			},
			wantKind: errs.KindSignal,
			wantHint: "rate limit reached; retry in a few minutes",
		},
		{
			name: "403 saml enforcement",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"message":"Resource protected by organization SAML enforcement."}`))
			},
			wantKind: errs.KindAuth,
			wantHint: "SAML single sign-on",
		},
		{
			name: "403 ip allow list",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"message":"Although you appear to have the correct authorization credentials, the organization has an IP allow list enabled."}`))
			},
			wantKind: errs.KindAuth,
			wantHint: "IP allow list",
		},
		{
			name: "403 missing scope",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"message":"Must have admin rights to Repository."}`))
			},
			wantKind: errs.KindAuth,
			wantHint: "scopes",
		},
		{
			name: "404 not found",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"message":"Not Found"}`))
			},
			wantKind: errs.KindSignal,
			wantHint: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ghAPIGetAgainst(t, tc.handler)
			if got := errs.KindOf(err); got != tc.wantKind {
				t.Errorf("kind = %q, want %q (err %v, hint %q)", got, tc.wantKind, err, errs.Hint(err))
			}
			hint := errs.Hint(err)
			if tc.wantHint == "" {
				if hint != "" {
					t.Errorf("hint = %q, want none", hint)
				}
				return
			}
			if !strings.Contains(hint, tc.wantHint) {
				t.Errorf("hint = %q, want it to mention %q", hint, tc.wantHint)
			}
		})
	}
}

func TestGHAPIGetRateLimitHintDoesNotTellTheUserToReLogIn(t *testing.T) {
	err := ghAPIGetAgainst(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"API rate limit exceeded for user"}`))
	})
	if strings.Contains(errs.Hint(err), "mino login") {
		t.Fatalf("a rate-limited 403 must not send the user back through login: %q", errs.Hint(err))
	}
}

func TestGHAPIGetReturnsSmallBodiesUnchanged(t *testing.T) {
	noGHOnPath(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"login":"octocat"}`))
	}))
	t.Cleanup(srv.Close)
	body, err := GHAPIGet(context.Background(), ambientSelection(t, srv.URL), "user")
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"login":"octocat"}` {
		t.Fatalf("body = %q", body)
	}
}

func TestDeviceFlowTokenExchangeSurvivesASlowButBoundedServer(t *testing.T) {
	var polls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "code") && polls == 0 {
			_, _ = w.Write([]byte(`{"device_code":"dc","user_code":"uc","verification_uri":"https://example.invalid","interval":1,"expires_in":10}`))
			polls++
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"gho_slow","scope":"repo"}`))
	}))
	t.Cleanup(srv.Close)

	tok, err := runGitHubDeviceFlow(context.Background(), deviceFlowHTTPClient,
		srv.URL+"/login/device/code", srv.URL+"/login/oauth/access_token",
		"client-id", "repo", &strings.Builder{}, func(time.Duration) {})
	if err != nil {
		t.Fatalf("device flow: %v", err)
	}
	if tok != "gho_slow" {
		t.Fatalf("token = %q", tok)
	}
}

func TestScopeHintsCoverAppsAsWellAsTokens(t *testing.T) {
	// A GitHub App has installation permissions, not OAuth scopes, and
	// `gh auth refresh` does not apply to it. A hint that only ever says "scope"
	// sends an App operator to a setting that does not exist for them.
	for name, hint := range map[string]string{"githubScopeHint": githubScopeHint} {
		if !strings.Contains(hint, "scope") {
			t.Errorf("%s does not mention scopes, which is the fix for a token: %q", name, hint)
		}
		if !strings.Contains(hint, "permission") {
			t.Errorf("%s does not mention installation permissions, so an operator authenticated as a "+
				"GitHub App has no actionable fix: %q", name, hint)
		}
	}
}
