package github

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

const nodeCreatedAt = "2026-06-01T00:00:00Z"

func commentCreatedAt(i int) string {
	return fmt.Sprintf("2026-07-%02dT00:00:00Z", i+1)
}

func commentNodes(t *testing.T, authors ...[2]string) searchNode {
	t.Helper()
	nodes := make([]string, 0, len(authors))
	for i, a := range authors {
		at := `"createdAt":"` + commentCreatedAt(i) + `",`
		if a[0] == "" && a[1] == "" {
			nodes = append(nodes, `{`+at+`"author":null}`)
			continue
		}
		nodes = append(nodes, `{`+at+`"author":{"login":"`+a[0]+`","__typename":"`+a[1]+`"}}`)
	}
	raw := `{"author":{"login":"reporter"},"createdAt":"` + nodeCreatedAt + `",` +
		`"comments":{"nodes":[` + strings.Join(nodes, ",") + `]}}`
	var n searchNode
	if err := json.Unmarshal([]byte(raw), &n); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return n
}

func TestLastResponder(t *testing.T) {
	cases := []struct {
		name   string
		node   searchNode
		want   string
		wantAt string
	}{
		{
			name:   "last human comment wins",
			node:   commentNodes(t, [2]string{"alice", "User"}, [2]string{"custuser", "User"}),
			want:   "custuser",
			wantAt: commentCreatedAt(1),
		},
		{
			name:   "trailing bot skipped",
			node:   commentNodes(t, [2]string{"alice", "User"}, [2]string{"dependabot[bot]", "Bot"}),
			want:   "alice",
			wantAt: commentCreatedAt(0),
		},
		{
			name:   "all bots falls back to author",
			node:   commentNodes(t, [2]string{"renovate-bot", "User"}, [2]string{"gh-actions", "Bot"}),
			want:   "reporter",
			wantAt: nodeCreatedAt,
		},
		{
			name:   "no comments falls back to author",
			node:   commentNodes(t),
			want:   "reporter",
			wantAt: nodeCreatedAt,
		},
		{
			name:   "deleted comment author skipped",
			node:   commentNodes(t, [2]string{"alice", "User"}, [2]string{"", ""}),
			want:   "alice",
			wantAt: commentCreatedAt(0),
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, at := lastResponder(c.node)
			if got != c.want {
				t.Errorf("lastResponder = %q, want %q", got, c.want)
			}
			if gotAt := at.Format(time.RFC3339); gotAt != c.wantAt {
				t.Errorf("lastResponder time = %q, want %q", gotAt, c.wantAt)
			}
		})
	}
}

func TestLastResponderNoAuthor(t *testing.T) {
	var n searchNode
	got, at := lastResponder(n)
	if got != "" {
		t.Errorf("lastResponder = %q, want empty", got)
	}
	if !at.IsZero() {
		t.Errorf("lastResponder time = %v, want zero", at)
	}
}

func TestLastResponderMissingCommentTime(t *testing.T) {
	var n searchNode
	if err := json.Unmarshal([]byte(`{"comments":{"nodes":[{"author":{"login":"alice","__typename":"User"}}]}}`), &n); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got, at := lastResponder(n)
	if got != "alice" {
		t.Errorf("lastResponder = %q, want alice", got)
	}
	if !at.IsZero() {
		t.Errorf("lastResponder time = %v, want zero", at)
	}
}

func TestIsBotLogin(t *testing.T) {
	cases := []struct {
		login    string
		typeName string
		want     bool
	}{
		{login: "dependabot[bot]", typeName: "User", want: true},
		{login: "someone", typeName: "Bot", want: true},
		{login: "renovate-bot", typeName: "User", want: true},
		{login: "grafana_bot", typeName: "User", want: true},
		{login: "abbott", typeName: "User"},
		{login: "botanist", typeName: "User"},
		{login: "alice", typeName: "User"},
	}
	for _, c := range cases {
		if got := isBotLogin(c.login, c.typeName); got != c.want {
			t.Errorf("isBotLogin(%q, %q) = %v, want %v", c.login, c.typeName, got, c.want)
		}
	}
}
