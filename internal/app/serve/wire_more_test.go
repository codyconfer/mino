package serve

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/codyconfer/munin/internal/signals"
)

func manyItemEvent(items int) signals.Event {
	filler := strings.Repeat("x", 4096)
	sec := signals.Section{Signal: "probe", Title: "probe", Items: make([]signals.Item, items)}
	for i := range sec.Items {
		sec.Items[i] = signals.Item{Title: strconv.Itoa(i) + filler, Kind: "pr"}
	}
	return signals.Event{Source: "probe", At: time.Unix(0, 0), Section: sec}
}

func TestBoundEventReportsHowManyItemsItDropped(t *testing.T) {
	ev := manyItemEvent(600)
	raw, err := encodeEvent(ev)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) <= MaxFrameBytes {
		t.Fatalf("fixture is only %d bytes, under the %d byte limit, so the drop path never runs",
			len(raw), MaxFrameBytes)
	}

	out := boundEvent(ev, len(raw))
	if out.Section.Meta[truncatedKey] == "" {
		t.Fatal("a frame-truncated event carries no truncation marker")
	}
	more := out.Section.Meta[signals.MetaMore]
	if more == "" {
		t.Fatal("a frame-truncated event says it was truncated but not by how much; render's truncationCue " +
			"reads `more` for the magnitude, so without it the user sees a bare \"(truncated)\"")
	}
	n, err := strconv.Atoi(more)
	if err != nil {
		t.Fatalf("more = %q, want an item count", more)
	}
	if want := len(ev.Section.Items) - len(out.Section.Items); n != want {
		t.Errorf("more = %d, want %d dropped items", n, want)
	}
}

func TestBoundEventDoesNotOverwriteAnUpstreamMoreCount(t *testing.T) {
	ev := manyItemEvent(600)
	ev.Section.Meta = map[string]string{signals.MetaMore: "9000"}
	raw, err := encodeEvent(ev)
	if err != nil {
		t.Fatal(err)
	}

	out := boundEvent(ev, len(raw))
	if got := out.Section.Meta[signals.MetaMore]; got != "9000" {
		t.Errorf("more = %q; the signal's own count of unfetched upstream results must survive frame "+
			"truncation, because it means something different from the count munin dropped", got)
	}
}

func TestWireAndRenderAgreeOnTheTruncationKey(t *testing.T) {
	if truncatedKey != signals.MetaWireTruncated {
		t.Errorf("serve writes %q but the shared key is %q; render only renders a cue for the shared key, so "+
			"a divergence here silently drops the marker", truncatedKey, signals.MetaWireTruncated)
	}
}
