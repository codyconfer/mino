package argocd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/codyconfer/mino/plugin"
)

type buildCtx struct {
	params   map[string]string
	settings map[string]any
	token    string
}

func (b buildCtx) Params() map[string]string { return b.params }

func (b buildCtx) Home() string { return "" }

func (b buildCtx) Role() string { return "" }

func (b buildCtx) Settings(ns string) map[string]any {
	if ns != SignalName {
		return nil
	}
	return b.settings
}

func (b buildCtx) Credentials() plugin.CredentialStore { return nil }

func (b buildCtx) GetToken(_ context.Context, _ string) (string, string, bool, error) {
	if b.token == "" {
		return "", "", false, nil
	}
	return b.token, "", true, nil
}

type staticTokens struct {
	token string
	scope string
	err   error
}

func (s staticTokens) Get(context.Context, string) (string, string, bool, error) {
	if s.err != nil {
		return "", "", false, s.err
	}
	if s.token == "" {
		return "", "", false, nil
	}
	return s.token, s.scope, true, nil
}

type recordedRequest struct {
	path  string
	query url.Values
	auth  string
}

type fakeServer struct {
	t        *testing.T
	srv      *httptest.Server
	mu       sync.Mutex
	requests []recordedRequest
	handler  func(w http.ResponseWriter, r *http.Request)
}

func newFakeServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) *fakeServer {
	t.Helper()
	fs := &fakeServer{t: t, handler: handler}
	fs.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fs.mu.Lock()
		fs.requests = append(fs.requests, recordedRequest{
			path:  r.URL.Path,
			query: r.URL.Query(),
			auth:  r.Header.Get("Authorization"),
		})
		fs.mu.Unlock()
		fs.handler(w, r)
	}))
	t.Cleanup(fs.srv.Close)
	return fs
}

func (f *fakeServer) config() Config {
	return Config{ServerURL: f.srv.URL, TokenEnv: DefaultTokenEnv, Max: defaultMax, GroupBy: groupByNone}
}

func (f *fakeServer) signal(cfg Config) *Signal {
	s := New(cfg, staticTokens{token: "test-token"})
	s.client.HTTP = f.srv.Client()
	return s
}

func (f *fakeServer) client(cfg Config) *Client {
	c := NewClient(cfg, staticTokens{token: "test-token"})
	c.HTTP = f.srv.Client()
	return c
}

func (f *fakeServer) last() recordedRequest {
	f.t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.requests) == 0 {
		f.t.Fatal("no request reached the server")
	}
	return f.requests[len(f.requests)-1]
}

func (f *fakeServer) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

func (f *fakeServer) at(i int) recordedRequest {
	f.t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if i >= len(f.requests) {
		f.t.Fatalf("wanted request %d but only %d reached the server", i, len(f.requests))
	}
	return f.requests[i]
}

func serveJSON(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
