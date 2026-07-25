package demo

import (
	"context"
	"testing"

	"github.com/codyconfer/munin/internal/filter"
)

func TestFetchFilterDemoDropsBots(t *testing.T) {
	secs, err := Signal{}.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(secs) != 1 || len(secs[0].Items) != 2 {
		t.Fatalf("Fetch = %+v", secs)
	}

	c, err := filter.Compile(filter.Filter{
		Name: "demo",
		Rules: []filter.Rule{{
			Field:   "meta.author",
			Exclude: "(?i)bot$",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := c.Apply(secs[0].Items)
	if len(got) != 1 {
		t.Fatalf("filtered = %d items, want 1", len(got))
	}
	if got[0].Meta["author"] != "alice" {
		t.Fatalf("kept author = %q, want alice", got[0].Meta["author"])
	}
	if got[0].Title != "Deploy finished for api-gateway" {
		t.Fatalf("kept title = %q", got[0].Title)
	}
}
