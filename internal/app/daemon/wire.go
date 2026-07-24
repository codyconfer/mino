package daemon

import (
	"encoding/json"
	"time"

	"github.com/codyconfer/munin/internal/signals"
)

type WireEvent struct {
	Source  string      `json:"source"`
	At      time.Time   `json:"at"`
	Section WireSection `json:"section"`
}

type WireSection = signals.Section

func Encode(ev signals.Event) ([]byte, error) {
	return json.Marshal(WireEvent{
		Source:  ev.Source,
		At:      ev.At,
		Section: ev.Section,
	})
}

func Decode(b []byte) (signals.Event, error) {
	var w WireEvent
	if err := json.Unmarshal(b, &w); err != nil {
		return signals.Event{}, err
	}
	return signals.Event{Source: w.Source, At: w.At, Section: w.Section}, nil
}
