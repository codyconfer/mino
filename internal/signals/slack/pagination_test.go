package slack

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func conversationsServer(t *testing.T, calls *atomic.Int64, cursorFor func(n int64) string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/conversations.list", func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		resp := map[string]any{
			"ok": true,
			"channels": []map[string]any{
				{"id": fmt.Sprintf("C%03d", n), "name": fmt.Sprintf("other-%d", n)},
			},
			"response_metadata": map[string]string{"next_cursor": cursorFor(n)},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func newTestSignal(t *testing.T, url, token, channel string) *slackSignal {
	t.Helper()
	resetChannelCache()
	t.Cleanup(resetChannelCache)
	s := New(token, channel, 10).(*slackSignal)
	s.apiURL = url + "/"
	return s
}

func TestResolveChannelStopsOnNonAdvancingCursor(t *testing.T) {
	var calls atomic.Int64
	srv := conversationsServer(t, &calls, func(n int64) string {
		if n > 500 {
			return ""
		}
		return "STUCK"
	})

	s := newTestSignal(t, srv.URL, "tok-stuck", "eng")
	type result struct {
		err error
	}
	done := make(chan result, 1)
	go func() {
		_, _, err := s.resolveChannel(context.Background(), s.client())
		done <- result{err: err}
	}()

	select {
	case got := <-done:
		if got.err == nil {
			t.Fatal("resolveChannel found a channel that does not exist")
		}
		if n := calls.Load(); n > 4 {
			t.Fatalf("conversations.list called %d times against a non-advancing cursor: the walk spins until the flight deadline", n)
		}
		if !strings.Contains(got.err.Error(), "cursor") {
			t.Fatalf("error does not name the pagination fault: %v", got.err)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("resolveChannel never returned: %d calls against a non-advancing cursor", calls.Load())
	}
}

func TestResolveChannelBoundsPageCount(t *testing.T) {
	var calls atomic.Int64
	srv := conversationsServer(t, &calls, func(n int64) string {
		return fmt.Sprintf("cursor-%d", n)
	})

	s := newTestSignal(t, srv.URL, "tok-bound", "eng")
	done := make(chan error, 1)
	go func() {
		_, _, err := s.resolveChannel(context.Background(), s.client())
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected an error after exhausting the page bound")
		}
		if n := calls.Load(); n != maxListPages {
			t.Fatalf("conversations.list called %d times, want the %d page bound", n, maxListPages)
		}
	case <-time.After(20 * time.Second):
		t.Fatalf("resolveChannel never returned: %d calls with an always-advancing cursor", calls.Load())
	}
}

func TestResolveChannelMemoizesID(t *testing.T) {
	var calls atomic.Int64
	mux := http.NewServeMux()
	mux.HandleFunc("/conversations.list", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		resp := map[string]any{
			"ok": true,
			"channels": []map[string]any{
				{"id": "C777", "name": "eng"},
			},
			"response_metadata": map[string]string{"next_cursor": ""},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	s := newTestSignal(t, srv.URL, "tok-memo", "#eng")
	for i := 0; i < 3; i++ {
		id, name, err := s.resolveChannel(context.Background(), s.client())
		if err != nil || id != "C777" || name != "eng" {
			t.Fatalf("resolve #%d = (%q, %q, %v)", i, id, name, err)
		}
	}
	next := New("tok-memo", "#eng", 10).(*slackSignal)
	next.apiURL = srv.URL + "/"
	if id, _, err := next.resolveChannel(context.Background(), next.client()); err != nil || id != "C777" {
		t.Fatalf("fresh signal resolve = (%q, %v)", id, err)
	}

	if n := calls.Load(); n != 1 {
		t.Fatalf("conversations.list called %d times, want 1: the name->ID walk is not memoized across flights", n)
	}
}
