package serve

import (
	"strings"
	"testing"
	"time"

	sysdaemon "github.com/codyconfer/sisyphus/daemon"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/signals"
)

func oversizedEvent() signals.Event {
	return signals.Event{
		Source: "plugin",
		At:     time.Now(),
		Section: signals.Section{
			Signal: "plugin",
			Title:  "huge",
			Items: []signals.Item{
				{Kind: "note", Title: "big", Body: strings.Repeat("x", 5<<20)},
				{Kind: "note", Title: "small"},
			},
		},
	}
}

func TestEncodeBoundsOversizedSection(t *testing.T) {
	b, err := Encode(oversizedEvent())
	if err != nil {
		t.Fatal(err)
	}
	if len(b) > MaxFrameBytes {
		t.Fatalf("frame is %d bytes, want at most %d", len(b), MaxFrameBytes)
	}
	got, err := Decode(b)
	if err != nil {
		t.Fatalf("bounded frame does not decode: %v", err)
	}
	if got.Section.Signal != "plugin" {
		t.Errorf("signal = %q", got.Section.Signal)
	}
	if got.Section.Meta[truncatedKey] == "" {
		t.Errorf("bounded frame carries no %q diagnostic: %+v", truncatedKey, got.Section.Meta)
	}
}

func TestEncodeBoundsManySmallItems(t *testing.T) {
	items := make([]signals.Item, 40000)
	for i := range items {
		items[i] = signals.Item{Kind: "note", Title: strings.Repeat("t", 64)}
	}
	ev := signals.Event{Source: "plugin", At: time.Now(), Section: signals.Section{Signal: "plugin", Items: items}}
	b, err := Encode(ev)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) > MaxFrameBytes {
		t.Fatalf("frame is %d bytes, want at most %d", len(b), MaxFrameBytes)
	}
	got, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Section.Items) == 0 || len(got.Section.Items) >= len(items) {
		t.Fatalf("bounded items = %d, want a non-empty subset of %d", len(got.Section.Items), len(items))
	}
}

func TestEncodeLeavesSmallEventsAlone(t *testing.T) {
	ev := signals.Event{Source: "demo", At: time.Now(), Section: signals.Section{
		Signal: "demo",
		Items:  []signals.Item{{Kind: "note", Title: "hi", Meta: map[string]string{"id": "1"}}},
	}}
	b, err := Encode(ev)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Section.Items) != 1 || got.Section.Items[0].Title != "hi" || got.Section.Meta[truncatedKey] != "" {
		t.Fatalf("small event was altered: %+v", got.Section)
	}
}

func TestOversizedEventDoesNotKillTheStream(t *testing.T) {
	path := shortSocketPath(t)
	ln, err := sysdaemon.Listen(config.SocketPrefix, path)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	subj := sysdaemon.NewSubject[signals.Event]()
	defer subj.Close()
	go sysdaemon.Broadcast(t.Context(), ln, subj, 8, Encode)

	client, err := sysdaemon.Dial(t.Context(), config.SocketPrefix, path, Decode)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)

	subj.Next(oversizedEvent())
	select {
	case got := <-client:
		if got.Section.Signal != "plugin" {
			t.Fatalf("first event = %+v", got.Section)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("client never received the oversized event")
	}

	subj.Next(signals.Event{Source: "demo", At: time.Now(), Section: signals.Section{
		Signal: "demo",
		Items:  []signals.Item{{Title: "after"}},
	}})
	select {
	case got := <-client:
		if len(got.Section.Items) != 1 || got.Section.Items[0].Title != "after" {
			t.Fatalf("event after the oversized one = %+v", got.Section)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("event stream died after one oversized frame")
	}
}
