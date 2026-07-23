package daemon

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/codyconfer/munin/internal/signals"
)

type WireEvent struct {
	Source  string      `json:"source"`
	At      time.Time   `json:"at"`
	Section WireSection `json:"section"`
}

type WireSection struct {
	Signal string         `json:"signal"`
	Title  string         `json:"title"`
	Items  []signals.Item `json:"items"`
	Err    string         `json:"err,omitempty"`
}

func Encode(ev signals.Event) ([]byte, error) {
	return json.Marshal(WireEvent{
		Source: ev.Source,
		At:     ev.At,
		Section: WireSection{
			Signal: ev.Section.Signal,
			Title:  ev.Section.Title,
			Items:  ev.Section.Items,
			Err:    ev.Section.ErrString(),
		},
	})
}

func Decode(b []byte) (signals.Event, error) {
	var w WireEvent
	if err := json.Unmarshal(b, &w); err != nil {
		return signals.Event{}, err
	}
	sec := signals.Section{Signal: w.Section.Signal, Title: w.Section.Title, Items: w.Section.Items}
	if w.Section.Err != "" {
		sec.Err = errors.New(w.Section.Err)
	}
	return signals.Event{Source: w.Source, At: w.At, Section: sec}, nil
}
