package demo

import (
	"context"
	"regexp"
	"testing"
)

func TestFetchYieldsExactlyOneBotAndOneHumanItem(t *testing.T) {
	secs, err := Signal{}.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(secs) != 1 || len(secs[0].Items) != 2 {
		t.Fatalf("Fetch = %+v", secs)
	}

	bot := regexp.MustCompile("(?i)bot$")
	var kept []int
	for i, it := range secs[0].Items {
		if !bot.MatchString(it.Meta["author"]) {
			kept = append(kept, i)
		}
	}
	if len(kept) != 1 {
		t.Fatalf("non-bot items = %d, want 1 so the filter demo has exactly one bot to drop", len(kept))
	}
	human := secs[0].Items[kept[0]]
	if human.Meta["author"] != "alice" {
		t.Fatalf("non-bot author = %q, want alice", human.Meta["author"])
	}
	if human.Title != "Deploy finished for api-gateway" {
		t.Fatalf("non-bot title = %q", human.Title)
	}
}
