package ntr

import (
	"strings"
	"testing"
	"time"

	vkdeck "github.com/codyconfer/viewkit/deck"
)

func TestParseDueRelative(t *testing.T) {
	got, err := parseDue("+2h")
	if err != nil {
		t.Fatal(err)
	}
	if d := time.Until(got); d < time.Hour || d > 3*time.Hour {
		t.Fatalf("until = %v", d)
	}
}

func TestParseDueBad(t *testing.T) {
	if _, err := parseDue("not-a-date"); err == nil {
		t.Fatal("expected error")
	}
}

func TestNTRViewsRegistered(t *testing.T) {
	for _, id := range []string{"ntr.home", "ntr.notes", "ntr.tasks", "ntr.reminders"} {
		if _, ok := vkdeck.LookupView(id); !ok {
			t.Fatalf("missing view %s (have %v)", id, vkdeck.ViewIDs())
		}
	}
	if !strings.Contains(strings.Join(vkdeck.ViewIDs(), ","), "ntr.") {
		t.Fatal(vkdeck.ViewIDs())
	}
}
