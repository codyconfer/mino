package slack

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/codyconfer/mino/plugin"
)

type threadServer struct {
	srv       *httptest.Server
	replies   atomic.Int64
	listCalls atomic.Int64
	userCalls atomic.Int64
}

func newThreadServer(t *testing.T) *threadServer {
	t.Helper()
	ts := &threadServer{}
	mux := http.NewServeMux()

	mux.HandleFunc("/auth.test", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"ok": true, "url": "https://myorg.slack.com/", "user_id": "UME", "team_id": "T1",
		})
	})
	mux.HandleFunc("/conversations.list", func(w http.ResponseWriter, _ *http.Request) {
		ts.listCalls.Add(1)
		writeJSON(w, map[string]any{"ok": true, "channels": []any{}})
	})
	mux.HandleFunc("/users.info", func(w http.ResponseWriter, _ *http.Request) {
		ts.userCalls.Add(1)
		writeJSON(w, map[string]any{"ok": true, "users": []map[string]any{
			{"id": "UROOT", "name": "ada", "real_name": "Ada L", "profile": map[string]any{"display_name": "ada"}},
			{"id": "UREPLY", "name": "grace", "real_name": "Grace H", "profile": map[string]any{"display_name": "grace"}},
		}})
	})
	mux.HandleFunc("/conversations.replies", func(w http.ResponseWriter, _ *http.Request) {
		ts.replies.Add(1)
		writeJSON(w, map[string]any{
			"ok": true,
			"messages": []map[string]any{
				{
					"type": "message", "user": "UROOT", "text": "deploy is stuck cc <@UREPLY>",
					"ts": "1700000000.000001", "thread_ts": "1700000000.000001", "reply_count": 2,
					"reactions": []map[string]any{
						{"name": "eyes", "count": 3}, {"name": "tada", "count": 1},
					},
				},
				{"type": "message", "user": "UREPLY", "text": "looking now", "ts": "1700000005.000002", "thread_ts": "1700000000.000001"},
				{"type": "message", "user": "UROOT", "text": "fixed", "ts": "1700000009.000003", "thread_ts": "1700000000.000001"},
			},
		})
	})

	ts.srv = httptest.NewServer(mux)
	t.Cleanup(ts.srv.Close)
	return ts
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func newDetailSignal(t *testing.T, url string) *slackSignal {
	t.Helper()
	resetCaches()
	t.Cleanup(resetCaches)
	s := NewSpec(Spec{Token: "tok-detail", ResolveNames: true}).(*slackSignal)
	s.apiURL = url + "/"
	return s
}

func TestDetailFromURLOnlyItem(t *testing.T) {
	ts := newThreadServer(t)
	s := newDetailSignal(t, ts.srv.URL)

	it := plugin.Item{URL: "https://myorg.slack.com/archives/C0123ABC/p1700000000000001"}
	d, err := s.Detail(context.Background(), it)
	if err != nil {
		t.Fatalf("Detail from a URL-only item = %v; cmd/show.go builds the item with nothing but a URL, so this is the CLI path", err)
	}
	if !strings.Contains(d.Title, "deploy is stuck") {
		t.Fatalf("Title = %q, want the root message text", d.Title)
	}
	if ts.listCalls.Load() != 0 {
		t.Fatalf("conversations.list called %d times; Detail must resolve from the permalink, never walk the channel list", ts.listCalls.Load())
	}
	if ts.replies.Load() != 1 {
		t.Fatalf("conversations.replies called %d times, want 1", ts.replies.Load())
	}
}

func TestDetailPrefersMetaOverURL(t *testing.T) {
	ts := newThreadServer(t)
	s := newDetailSignal(t, ts.srv.URL)

	it := plugin.Item{
		URL: "https://myorg.slack.com/archives/CWRONG/p1700000000000009",
		Meta: map[string]string{
			"channel_id":   "C0123ABC",
			"ts":           "1700000005.000002",
			"thread_ts":    "1700000000.000001",
			"channel_name": "eng",
		},
	}
	d, err := s.Detail(context.Background(), it)
	if err != nil {
		t.Fatalf("Detail = %v", err)
	}
	if got := rowValue(d.Rows, "channel"); got != "#eng" {
		t.Fatalf("channel row = %q, want #eng from Meta", got)
	}
}

func TestDetailResolvesThreadRootFromReply(t *testing.T) {
	ts := newThreadServer(t)
	s := newDetailSignal(t, ts.srv.URL)

	it := plugin.Item{Meta: map[string]string{
		"channel_id": "C0123ABC",
		"ts":         "1700000005.000002",
		"thread_ts":  "1700000000.000001",
	}}
	d, err := s.Detail(context.Background(), it)
	if err != nil {
		t.Fatalf("Detail = %v", err)
	}
	if !strings.Contains(d.Title, "deploy is stuck") {
		t.Fatalf("Title = %q, want the thread root, not the reply", d.Title)
	}
}

func TestDetailShapesThread(t *testing.T) {
	ts := newThreadServer(t)
	s := newDetailSignal(t, ts.srv.URL)

	it := plugin.Item{Meta: map[string]string{"channel_id": "C0123ABC", "ts": "1700000000.000001"}}
	d, err := s.Detail(context.Background(), it)
	if err != nil {
		t.Fatalf("Detail = %v", err)
	}

	if d.URL != "https://myorg.slack.com/archives/C0123ABC/p1700000000000001" {
		t.Fatalf("URL = %q, want a permalink built from the resolved workspace host", d.URL)
	}
	if got := rowValue(d.Rows, "author"); got != "ada" {
		t.Fatalf("author row = %q, want the resolved display name", got)
	}
	if got := rowValue(d.Rows, "replies"); got != "2" {
		t.Fatalf("replies row = %q, want 2", got)
	}
	if !strings.Contains(d.Body, "@grace") {
		t.Fatalf("Body = %q, want <@UREPLY> unfurled to @grace", d.Body)
	}

	var chips []string
	for _, c := range d.Chips {
		chips = append(chips, c.Label)
	}
	joined := strings.Join(chips, "|")
	if !strings.Contains(joined, ":eyes: 3") || !strings.Contains(joined, "2 replies") {
		t.Fatalf("chips = %v, want reaction and reply chips", chips)
	}

	if len(d.Sections) == 0 {
		t.Fatal("no thread section")
	}
	thread := d.Sections[0]
	if thread.Title != "thread (2)" {
		t.Fatalf("section title = %q, want thread (2)", thread.Title)
	}
	if len(thread.Lines) != 2 {
		t.Fatalf("thread lines = %d, want 2 replies", len(thread.Lines))
	}
	if !strings.Contains(thread.Lines[0], "grace") || !strings.Contains(thread.Lines[0], "looking now") {
		t.Fatalf("reply line = %q, want author and text", thread.Lines[0])
	}
}

func TestRefFromItemErrorsWithHint(t *testing.T) {
	_, err := refFromItem(plugin.Item{Title: "orphan"})
	if err == nil {
		t.Fatal("refFromItem accepted an item with no ref")
	}
	hinted, ok := err.(interface{ Hint() string })
	if !ok || hinted.Hint() == "" {
		t.Fatalf("error carries no hint: %v", err)
	}
	if !strings.Contains(hinted.Hint(), "permalink") {
		t.Fatalf("hint does not tell the user what to pass: %q", hinted.Hint())
	}
}

func TestRefFromItemUsesMeta(t *testing.T) {
	ref, err := refFromItem(plugin.Item{Meta: map[string]string{
		"channel_id": "C9", "ts": "1700000000.000001", "thread_ts": "1699999999.000001",
	}})
	if err != nil {
		t.Fatalf("refFromItem = %v", err)
	}
	if ref.ChannelID != "C9" || ref.TS != "1700000000.000001" || ref.ThreadTS != "1699999999.000001" {
		t.Fatalf("ref = %+v", ref)
	}
}

func rowValue(rows [][2]string, key string) string {
	for _, r := range rows {
		if r[0] == key {
			return r[1]
		}
	}
	return ""
}
