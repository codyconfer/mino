package slack

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

type fetchServer struct {
	srv       *httptest.Server
	userCalls atomic.Int64
	histCalls atomic.Int64
	failFor   string
}

func newFetchServer(t *testing.T) *fetchServer {
	t.Helper()
	fs := &fetchServer{}
	mux := http.NewServeMux()

	mux.HandleFunc("/auth.test", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"ok": true, "url": "https://myorg.slack.com/", "user_id": "UME"})
	})
	mux.HandleFunc("/conversations.list", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.FormValue("types"), "im") {
			writeJSON(w, map[string]any{"ok": true, "channels": []map[string]any{
				{"id": "D001", "user": "UBOB", "is_im": true},
			}})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "channels": []map[string]any{
			{"id": "C001", "name": "eng"},
			{"id": "C002", "name": "alerts"},
		}})
	})
	mux.HandleFunc("/users.info", func(w http.ResponseWriter, _ *http.Request) {
		fs.userCalls.Add(1)
		writeJSON(w, map[string]any{"ok": true, "users": []map[string]any{
			{"id": "UA", "name": "ada", "profile": map[string]any{"display_name": "ada"}},
			{"id": "UB", "name": "bob", "profile": map[string]any{"display_name": "bob"}},
			{"id": "UBOB", "name": "bob", "profile": map[string]any{"display_name": "bob"}},
		}})
	})
	mux.HandleFunc("/conversations.history", func(w http.ResponseWriter, r *http.Request) {
		fs.histCalls.Add(1)
		channel := r.FormValue("channel")
		if fs.failFor != "" && channel == fs.failFor {
			writeJSON(w, map[string]any{"ok": false, "error": "channel_not_found"})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "messages": []map[string]any{
			{"type": "message", "user": "UA", "text": "hello from " + channel + " cc <@UB>", "ts": "1700000000.000001", "reply_count": 2},
			{"type": "message", "user": "UB", "text": "second", "ts": "1700000001.000002"},
		}})
	})
	mux.HandleFunc("/search.messages", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"ok": true, "messages": map[string]any{
			"total": 1,
			"matches": []map[string]any{{
				"type": "message", "user": "UA", "text": "you were mentioned <@UME>",
				"ts": "1700000002.000003", "permalink": "https://myorg.slack.com/archives/C001/p1700000002000003",
				"channel": map[string]any{"id": "C001", "name": "eng"},
			}},
		}})
	})

	fs.srv = httptest.NewServer(mux)
	t.Cleanup(fs.srv.Close)
	return fs
}

func newFetchSignal(t *testing.T, url string, sp Spec) *slackSignal {
	t.Helper()
	resetCaches()
	t.Cleanup(resetCaches)
	sp.Token = "tok-fetch"
	sp.ResolveNames = true
	s := NewSpec(sp).(*slackSignal)
	s.apiURL = url + "/"
	return s
}

func TestFetchSingleChannelShape(t *testing.T) {
	fs := newFetchServer(t)
	s := newFetchSignal(t, fs.srv.URL, Spec{Channels: []string{"eng"}})

	secs, err := s.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch = %v", err)
	}
	if len(secs) != 1 {
		t.Fatalf("sections = %d, want 1", len(secs))
	}
	sec := secs[0]
	if sec.Signal != SignalName || sec.Title != "#eng" {
		t.Fatalf("section = %q/%q, want slack/#eng", sec.Signal, sec.Title)
	}
	if len(sec.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(sec.Items))
	}

	it := sec.Items[0]
	if it.Kind != "message" {
		t.Errorf("Kind = %q", it.Kind)
	}
	if it.URL != "https://myorg.slack.com/archives/C001/p1700000000000001" {
		t.Errorf("URL = %q, want a constructed permalink", it.URL)
	}
	if it.Meta["channel_id"] != "C001" {
		t.Errorf("Meta[channel_id] = %q, want C001 so Detail needs no channel walk", it.Meta["channel_id"])
	}
	if it.Meta["ts"] != "1700000000.000001" {
		t.Errorf("Meta[ts] = %q, want the full-precision ts", it.Meta["ts"])
	}
	if it.Meta["user"] != "UA" {
		t.Errorf("Meta[user] = %q, want the raw id kept for back-compat", it.Meta["user"])
	}
	if it.Meta["author"] != "ada" {
		t.Errorf("Meta[author] = %q, want the display name; the bundled no-bots filter keys on meta.author", it.Meta["author"])
	}
	if it.Meta["reply_count"] != "2" {
		t.Errorf("Meta[reply_count] = %q, want 2", it.Meta["reply_count"])
	}
	if !strings.Contains(it.Body, "@bob") {
		t.Errorf("Body = %q, want <@UB> unfurled", it.Body)
	}
}

func TestFetchMultiChannelOneSectionEach(t *testing.T) {
	fs := newFetchServer(t)
	s := newFetchSignal(t, fs.srv.URL, Spec{Channels: []string{"eng", "alerts"}})

	secs, err := s.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch = %v", err)
	}
	if len(secs) != 2 {
		t.Fatalf("sections = %d, want one per channel", len(secs))
	}
	if secs[0].Title != "#eng" || secs[1].Title != "#alerts" {
		t.Fatalf("titles = %q, %q; want request order preserved", secs[0].Title, secs[1].Title)
	}
}

func TestFetchPartialFailureCarriesSectionErr(t *testing.T) {
	fs := newFetchServer(t)
	fs.failFor = "C002"
	s := newFetchSignal(t, fs.srv.URL, Spec{Channels: []string{"eng", "alerts"}})

	secs, err := s.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch = %v; one bad channel must not fail the whole multi-surface query", err)
	}
	if len(secs) != 2 {
		t.Fatalf("sections = %d, want 2", len(secs))
	}
	if len(secs[0].Items) == 0 {
		t.Error("the healthy channel returned no items")
	}
	if secs[1].Err == nil {
		t.Error("the failing channel carries no Section.Err")
	}
}

func TestFetchSingleSurfaceFailureIsAnError(t *testing.T) {
	fs := newFetchServer(t)
	fs.failFor = "C001"
	s := newFetchSignal(t, fs.srv.URL, Spec{Channels: []string{"eng"}})

	if _, err := s.Fetch(context.Background()); err == nil {
		t.Fatal("a single-surface failure must return an error, matching today's `--channel bad` behaviour")
	}
}

func TestFetchBatchesUserLookups(t *testing.T) {
	fs := newFetchServer(t)
	s := newFetchSignal(t, fs.srv.URL, Spec{Channels: []string{"eng", "alerts"}})

	if _, err := s.Fetch(context.Background()); err != nil {
		t.Fatalf("Fetch = %v", err)
	}
	if n := fs.userCalls.Load(); n != 1 {
		t.Fatalf("users.info called %d times, want 1: names must be batched and cached across surfaces", n)
	}
}

func TestFetchMentionsUsesSearch(t *testing.T) {
	fs := newFetchServer(t)
	s := newFetchSignal(t, fs.srv.URL, Spec{Mentions: true})

	secs, err := s.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch = %v", err)
	}
	if len(secs) != 1 || secs[0].Title != "mentions" {
		t.Fatalf("sections = %+v, want one titled mentions", secs)
	}
	if len(secs[0].Items) != 1 {
		t.Fatalf("items = %d, want 1", len(secs[0].Items))
	}
	it := secs[0].Items[0]
	if it.Kind != "mention" {
		t.Errorf("Kind = %q, want mention", it.Kind)
	}
	if it.URL != "https://myorg.slack.com/archives/C001/p1700000002000003" {
		t.Errorf("URL = %q, want the permalink the search response already carries", it.URL)
	}
	if it.Meta["channel_id"] != "C001" {
		t.Errorf("Meta[channel_id] = %q, want it from the search match", it.Meta["channel_id"])
	}
}

func TestFetchDMs(t *testing.T) {
	fs := newFetchServer(t)
	s := newFetchSignal(t, fs.srv.URL, Spec{DMs: 3})

	secs, err := s.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch = %v", err)
	}
	if len(secs) != 1 || secs[0].Title != "dms" {
		t.Fatalf("sections = %+v, want a single dms section", secs)
	}
	if len(secs[0].Items) == 0 {
		t.Fatal("no DM items")
	}
	if got := secs[0].Items[0].Meta["channel_id"]; got != "D001" {
		t.Fatalf("Meta[channel_id] = %q, want the DM conversation id", got)
	}
}

func TestFetchCombinesSurfaces(t *testing.T) {
	fs := newFetchServer(t)
	s := newFetchSignal(t, fs.srv.URL, Spec{Channels: []string{"eng"}, Mentions: true, DMs: 2})

	secs, err := s.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch = %v", err)
	}
	if len(secs) != 3 {
		t.Fatalf("sections = %d, want channel + mentions + dms in one query", len(secs))
	}
}

func TestFetchWithoutWorkspaceStillReturnsItems(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/auth.test", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"ok": false, "error": "invalid_auth"})
	})
	mux.HandleFunc("/conversations.history", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"ok": true, "messages": []map[string]any{
			{"type": "message", "user": "UA", "text": "still readable", "ts": "1700000000.000001"},
		}})
	})
	mux.HandleFunc("/users.info", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"ok": true, "users": []map[string]any{}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	s := newFetchSignal(t, srv.URL, Spec{Channels: []string{"C001"}})
	secs, err := s.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch = %v; a failed auth.test must cost permalinks, not items", err)
	}
	if len(secs) != 1 || len(secs[0].Items) != 1 {
		t.Fatalf("sections = %+v", secs)
	}
	if secs[0].Items[0].URL != "" {
		t.Errorf("URL = %q, want empty when the workspace host is unknown", secs[0].Items[0].URL)
	}
}

func TestFetchWorkspaceOverrideSkipsAuthTest(t *testing.T) {
	var authCalls atomic.Int64
	mux := http.NewServeMux()
	mux.HandleFunc("/auth.test", func(w http.ResponseWriter, _ *http.Request) {
		authCalls.Add(1)
		writeJSON(w, map[string]any{"ok": true, "url": "https://other.slack.com/", "user_id": "UME"})
	})
	mux.HandleFunc("/conversations.history", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"ok": true, "messages": []map[string]any{
			{"type": "message", "user": "UA", "text": "hi", "ts": "1700000000.000001"},
		}})
	})
	mux.HandleFunc("/users.info", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"ok": true, "users": []map[string]any{}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	s := newFetchSignal(t, srv.URL, Spec{Channels: []string{"C001"}, Workspace: "pinned.slack.com"})
	secs, err := s.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch = %v", err)
	}
	if authCalls.Load() != 0 {
		t.Fatalf("auth.test called %d times; a configured workspace should skip it", authCalls.Load())
	}
	if got := secs[0].Items[0].URL; !strings.HasPrefix(got, "https://pinned.slack.com/") {
		t.Fatalf("URL = %q, want the configured workspace host", got)
	}
}

func TestResolveNamesDisabledSkipsLookup(t *testing.T) {
	fs := newFetchServer(t)
	resetCaches()
	t.Cleanup(resetCaches)
	s := NewSpec(Spec{Token: "tok-noname", Channels: []string{"eng"}, ResolveNames: false}).(*slackSignal)
	s.apiURL = fs.srv.URL + "/"

	secs, err := s.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch = %v", err)
	}
	if fs.userCalls.Load() != 0 {
		t.Fatalf("users.info called %d times with resolve_names off", fs.userCalls.Load())
	}
	if got := secs[0].Items[0].Meta["author"]; got != "UA" {
		t.Fatalf("Meta[author] = %q, want the raw id as a stable fallback", got)
	}
}
