package slack

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	slackapi "github.com/slack-go/slack"
)

func TestRetryConfigOverridesUnusableDefaults(t *testing.T) {
	cfg := retryConfig(2)
	if cfg.MaxRetries != 2 {
		t.Fatalf("MaxRetries = %d, want 2", cfg.MaxRetries)
	}
	if def := slackapi.DefaultRetryConfig().RetryAfterDuration; cfg.RetryAfterDuration >= def {
		t.Fatalf("RetryAfterDuration = %v, want less than the %v default which outlives a flight timeout",
			cfg.RetryAfterDuration, def)
	}
	if len(cfg.Handlers) == 0 {
		t.Fatal("no retry handlers installed, so 429s would not be retried")
	}
}

func TestFetchRetriesRateLimit(t *testing.T) {
	var hist atomic.Int64
	mux := http.NewServeMux()
	mux.HandleFunc("/auth.test", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"ok": true, "url": "https://myorg.slack.com/", "user_id": "UME"})
	})
	mux.HandleFunc("/users.info", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"ok": true, "users": []map[string]any{}})
	})
	mux.HandleFunc("/conversations.history", func(w http.ResponseWriter, _ *http.Request) {
		if hist.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "messages": []map[string]any{
			{"type": "message", "user": "UA", "text": "after the throttle", "ts": "1700000000.000001"},
		}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	s := newFetchSignal(t, srv.URL, Spec{Channels: []string{"C001"}, RetryMax: 2})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	secs, err := s.Fetch(ctx)
	if err != nil {
		t.Fatalf("Fetch = %v, want the 429 to be retried", err)
	}
	if n := hist.Load(); n != 2 {
		t.Fatalf("conversations.history called %d times, want 2: one 429 then one success", n)
	}
	if len(secs) != 1 || len(secs[0].Items) != 1 {
		t.Fatalf("sections = %+v", secs)
	}
}

func TestFetchGivesUpOnCancelledContextWhileThrottled(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/auth.test", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"ok": true, "url": "https://myorg.slack.com/", "user_id": "UME"})
	})
	mux.HandleFunc("/conversations.history", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	s := newFetchSignal(t, srv.URL, Spec{Channels: []string{"C001"}, RetryMax: 2})

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := s.Fetch(ctx)
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Fetch succeeded against a permanently throttled server")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Fetch ignored context cancellation and slept through the whole Retry-After")
	}
}
