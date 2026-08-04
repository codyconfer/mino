package serve

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/signals/active"
)

func TestSourcesExplainsWhyEachQueryWasSkipped(t *testing.T) {
	s, _ := testServer(t, t.TempDir())
	s.Directives = &config.Directives{
		Flights: map[string]config.Flight{
			"rt": {Name: "rt", Queries: []string{"ghost", "notes"}},
		},
		Queries: map[string]config.Query{
			"notes": {Name: "notes", Signal: "nosuchsignal"},
		},
	}

	_, err := s.sources(context.Background(), "rt", time.Minute, active.NewState(nil))
	if err == nil {
		t.Fatal("sources succeeded with no openable stream")
	}
	hint := errs.Hint(err)
	for _, want := range []string{"ghost", "notes"} {
		if !strings.Contains(hint, want) {
			t.Errorf("hint does not account for query %q: %q\nEvery skip reason is already known and was "+
				"being discarded at debug level, leaving the operator with a generic \"no realtime "+
				"signals\" line and no way to tell which query failed or why", want, hint)
		}
	}
}
