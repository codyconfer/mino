package daemon

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	sysdaemon "github.com/codyconfer/sisyphus/daemon"

	"github.com/codyconfer/munin/internal/signals"
)

func TestWireEventRoundTrip(t *testing.T) {
	ev := signals.Event{
		Source: "github",
		At:     time.Now(),
		Section: signals.Section{
			Signal: "github",
			Title:  "github",
			Items:  []signals.Item{{Kind: "mention", Title: "hi", Meta: map[string]string{"id": "1"}}},
		},
	}
	b, err := Encode(ev)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != "github" || got.Section.Title != "github" || len(got.Section.Items) != 1 || got.Section.Items[0].Title != "hi" || got.Section.Items[0].Meta["id"] != "1" {
		t.Fatalf("round trip mismatch: %+v", got)
	}
}

func TestWireEventErrorPreserved(t *testing.T) {
	ev := signals.Event{Source: "x", Section: signals.Section{Signal: "x", Err: errors.New("boom")}}
	b, err := Encode(ev)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if got.Section.Err == nil || got.Section.Err.Error() != "boom" {
		t.Fatalf("error not preserved: %+v", got.Section.Err)
	}
}

func TestServeSocketDeliversToClients(t *testing.T) {
	path := filepath.Join(t.TempDir(), "serve.sock")
	ln, err := sysdaemon.Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	subj := sysdaemon.NewSubject[signals.Event]()
	defer subj.Close()
	go sysdaemon.Broadcast(t.Context(), ln, subj, 8, Encode)

	a, err := sysdaemon.Dial(t.Context(), path, Decode)
	if err != nil {
		t.Fatal(err)
	}
	b, err := sysdaemon.Dial(t.Context(), path, Decode)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)

	subj.Next(signals.Event{
		Source:  "demo",
		Section: signals.Section{Signal: "demo", Title: "demo", Items: []signals.Item{{Title: "hello"}}},
	})

	for _, ch := range []<-chan signals.Event{a, b} {
		select {
		case got := <-ch:
			if got.Source != "demo" || len(got.Section.Items) != 1 || got.Section.Items[0].Title != "hello" {
				t.Fatalf("bad event: %+v", got)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("client received nothing")
		}
	}
}
