package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/codyconfer/mino/internal/signals"
)

// pagedServer serves numbered rows, honouring per_page/page and the X- headers, so the
// collect loop is driven the way GitLab drives it.
type pagedServer struct {
	total     int
	withTotal bool
	requests  []string
}

func (p *pagedServer) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p.requests = append(p.requests, r.URL.RawQuery)
		q := r.URL.Query()
		perPage := atoiOr(t, q.Get("per_page"), 20)
		page := atoiOr(t, q.Get("page"), 1)

		start := (page - 1) * perPage
		end := min(start+perPage, p.total)
		rows := make([]map[string]any, 0, max(0, end-start))
		for i := start; i < end; i++ {
			rows = append(rows, map[string]any{"id": i})
		}
		if p.withTotal {
			w.Header().Set("X-Total", fmt.Sprint(p.total))
			w.Header().Set("X-Total-Pages", fmt.Sprint((p.total+perPage-1)/perPage))
		}
		if end < p.total {
			w.Header().Set("X-Next-Page", fmt.Sprint(page+1))
		}
		if err := json.NewEncoder(w).Encode(rows); err != nil {
			t.Error(err)
		}
	}
}

func atoiOr(t *testing.T, s string, def int) int {
	t.Helper()
	if s == "" {
		return def
	}
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		t.Fatalf("bad number %q", s)
	}
	return n
}

func countRows(body []byte, room int) (int, error) {
	var rows []json.RawMessage
	if err := json.Unmarshal(body, &rows); err != nil {
		return 0, err
	}
	return min(len(rows), room), nil
}

func TestCollectWalksNextPageUntilTheLimit(t *testing.T) {
	srv := &pagedServer{total: 57, withTotal: true}
	b := apiBackend(t, srv.handler(t))

	m, err := collect(context.Background(), b, "merge_requests", url.Values{}, 20, 50, countRows)
	if err != nil {
		t.Fatal(err)
	}
	if m.shown != 50 {
		t.Errorf("shown = %d, want the 50-item limit", m.shown)
	}
	if !m.hasTotal || m.total != 57 {
		t.Errorf("total = %d/%v, want 57 from X-Total", m.total, m.hasTotal)
	}
	if len(srv.requests) != 3 {
		t.Errorf("made %d requests (%v), want 3 pages of 20", len(srv.requests), srv.requests)
	}
}

func TestCollectStopsWhenTheServerReportsNoNextPage(t *testing.T) {
	srv := &pagedServer{total: 5, withTotal: true}
	b := apiBackend(t, srv.handler(t))

	m, err := collect(context.Background(), b, "merge_requests", url.Values{}, 20, 100, countRows)
	if err != nil {
		t.Fatal(err)
	}
	if m.shown != 5 || len(srv.requests) != 1 {
		t.Errorf("shown = %d over %d requests, want 5 in one", m.shown, len(srv.requests))
	}
	if m.truncated {
		t.Error("a complete result was marked truncated")
	}
}

func TestCollectDoesNotFabricateATotal(t *testing.T) {
	srv := &pagedServer{total: 5}
	b := apiBackend(t, srv.handler(t))

	m, err := collect(context.Background(), b, "merge_requests", url.Values{}, 20, 100, countRows)
	if err != nil {
		t.Fatal(err)
	}
	if m.hasTotal {
		t.Error("a total was reported with no X-Total header; GitLab omits it past 10,000 rows and a " +
			"zero would render as \"0 of 0\"")
	}
	meta := m.sectionMeta()
	if _, ok := meta["total"]; ok {
		t.Errorf("section meta = %v, want no total key", meta)
	}
	if _, ok := meta[signals.MetaMore]; ok {
		t.Errorf("section meta = %v, want no more key", meta)
	}
}

func TestCollectStopsAtTheHardPageCap(t *testing.T) {
	srv := &pagedServer{total: 10000, withTotal: true}
	b := apiBackend(t, srv.handler(t))

	m, err := collect(context.Background(), b, "merge_requests", url.Values{}, 5, 10000, countRows)
	if err != nil {
		t.Fatal(err)
	}
	if len(srv.requests) != maxPages {
		t.Errorf("made %d requests, want the %d-page cap", len(srv.requests), maxPages)
	}
	if !m.truncated {
		t.Error("hitting the page cap was not reported as truncated; silent truncation reads as " +
			"\"that is everything\"")
	}
	meta := m.sectionMeta()
	if meta[signals.MetaTruncated] != "true" || meta["truncated_reason"] == "" {
		t.Errorf("section meta = %v, want the truncation explained", meta)
	}
}

func TestSectionMetaReportsMore(t *testing.T) {
	m := pageMeta{shown: 20, total: 57, hasTotal: true}
	meta := m.sectionMeta()
	if meta["shown"] != "20" || meta["total"] != "57" || meta[signals.MetaMore] != "37" {
		t.Errorf("section meta = %v", meta)
	}
}

func TestNextPageHeaderParsing(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"0", 0},
		{"2", 2},
		{"not a number", 0},
		{" 3 ", 3},
	}
	for _, c := range cases {
		hdr := http.Header{}
		if c.in != "" {
			hdr.Set("X-Next-Page", c.in)
		}
		if got := nextPage(hdr); got != c.want {
			t.Errorf("nextPage(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestCLIBackendShortPageStopsPaging(t *testing.T) {
	fakeGLab(t, "#!/bin/sh\necho '[{\"id\":1}]'\n")

	m, err := collect(context.Background(), CLIBackend{}, "merge_requests", url.Values{}, 20, 100, countRows)
	if err != nil {
		t.Fatal(err)
	}
	if m.shown != 1 {
		t.Errorf("shown = %d, want 1; glab prints only the body, so a short page is the only paging "+
			"signal available", m.shown)
	}
}

func TestGitLabMessageFlattensEveryShape(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"string", `{"message":"404 Project Not Found"}`, "404 Project Not Found"},
		{"array", `{"message":["a","b"]}`, "a; b"},
		{"object", `{"message":{"base":["taken"]}}`, "base taken"},
		{"error and description", `{"error":"invalid_scope","error_description":"bad"}`, "invalid_scope: bad"},
		{"error and scope", `{"error":"insufficient_scope","scope":"read_api"}`, "insufficient_scope (scope read_api)"},
		{"not json", `plain text failure`, "plain text failure"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := gitlabMessage([]byte(c.body)); !strings.Contains(got, c.want) {
				t.Errorf("gitlabMessage(%s) = %q, want it to contain %q", c.body, got, c.want)
			}
		})
	}
}
