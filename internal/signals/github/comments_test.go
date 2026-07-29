package github

import (
	"encoding/json"
	"strings"
	"testing"
)

func commentNodes(t *testing.T, authors ...[2]string) searchNode {
	t.Helper()
	nodes := make([]string, 0, len(authors))
	for _, a := range authors {
		if a[0] == "" && a[1] == "" {
			nodes = append(nodes, `{"author":null}`)
			continue
		}
		nodes = append(nodes, `{"author":{"login":"`+a[0]+`","__typename":"`+a[1]+`"}}`)
	}
	raw := `{"author":{"login":"reporter"},"comments":{"nodes":[` + strings.Join(nodes, ",") + `]}}`
	var n searchNode
	if err := json.Unmarshal([]byte(raw), &n); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return n
}

func TestLastResponder(t *testing.T) {
	cases := []struct {
		name string
		node searchNode
		want string
	}{
		{
			name: "last human comment wins",
			node: commentNodes(t, [2]string{"alice", "User"}, [2]string{"custuser", "User"}),
			want: "custuser",
		},
		{
			name: "trailing bot skipped",
			node: commentNodes(t, [2]string{"alice", "User"}, [2]string{"dependabot[bot]", "Bot"}),
			want: "alice",
		},
		{
			name: "all bots falls back to author",
			node: commentNodes(t, [2]string{"renovate-bot", "User"}, [2]string{"gh-actions", "Bot"}),
			want: "reporter",
		},
		{
			name: "no comments falls back to author",
			node: commentNodes(t),
			want: "reporter",
		},
		{
			name: "deleted comment author skipped",
			node: commentNodes(t, [2]string{"alice", "User"}, [2]string{"", ""}),
			want: "alice",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := lastResponder(c.node); got != c.want {
				t.Errorf("lastResponder = %q, want %q", got, c.want)
			}
		})
	}
}

func TestLastResponderNoAuthor(t *testing.T) {
	var n searchNode
	if got := lastResponder(n); got != "" {
		t.Errorf("lastResponder = %q, want empty", got)
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
