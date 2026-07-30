package serve

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/codyconfer/munin/internal/log"
	"github.com/codyconfer/munin/internal/signals"
)

const (
	MaxFrameBytes = 1 << 20
	maxItemText   = 8 << 10
	maxMetaValue  = 2 << 10
	maxLabelText  = 1 << 10
	truncatedKey  = signals.MetaWireTruncated
	moreKey       = signals.MetaMore
)

type WireEvent struct {
	Source  string      `json:"source"`
	At      time.Time   `json:"at"`
	Section WireSection `json:"section"`
}

type WireSection = signals.Section

func Encode(ev signals.Event) ([]byte, error) {
	b, err := encodeEvent(ev)
	if err != nil {
		return nil, err
	}
	if len(b) <= MaxFrameBytes {
		return b, nil
	}
	bounded := boundEvent(ev, len(b))
	log.Warnf("serve: %s: %d byte event exceeds the %d byte frame limit; sending %d of %d items",
		ev.Section.Signal, len(b), MaxFrameBytes, len(bounded.Section.Items), len(ev.Section.Items))
	return encodeEvent(bounded)
}

func Decode(b []byte) (signals.Event, error) {
	var w WireEvent
	if err := json.Unmarshal(b, &w); err != nil {
		return signals.Event{}, err
	}
	return signals.Event{Source: w.Source, At: w.At, Section: w.Section}, nil
}

func encodeEvent(ev signals.Event) ([]byte, error) {
	return json.Marshal(WireEvent{
		Source:  ev.Source,
		At:      ev.At,
		Section: ev.Section,
	})
}

func boundEvent(ev signals.Event, size int) signals.Event {
	total := len(ev.Section.Items)
	items, clipped := clampItems(ev.Section.Items)
	dropped := 0
	for {
		out := ev
		out.Section.Items = items
		out.Section.Meta = withNote(ev.Section.Meta, frameNote(size, total, dropped, clipped), dropped)
		b, err := encodeEvent(out)
		if err == nil && len(b) <= MaxFrameBytes {
			return out
		}
		if len(items) == 0 {
			return diagnosticEvent(ev, size, total)
		}
		keep := len(items) / 2
		dropped += len(items) - keep
		items = items[:keep]
	}
}

func diagnosticEvent(ev signals.Event, size, total int) signals.Event {
	var clipped bool
	out := ev
	out.Source = clip(ev.Source, maxLabelText, &clipped)
	out.Section = signals.Section{
		Signal: clip(ev.Section.Signal, maxLabelText, &clipped),
		Title:  clip(ev.Section.Title, maxLabelText, &clipped),
		Meta:   withNote(nil, frameNote(size, total, total, true), total),
	}
	if ev.Section.Err != nil {
		out.Section.Err = errors.New(clip(ev.Section.Err.Error(), maxItemText, &clipped))
	}
	return out
}

func frameNote(size, total, dropped int, clipped bool) string {
	note := fmt.Sprintf("%d byte event exceeds the %d byte frame limit: dropped %d of %d items",
		size, MaxFrameBytes, dropped, total)
	if clipped {
		note += "; item text clipped"
	}
	return note
}

func withNote(meta map[string]string, note string, dropped int) map[string]string {
	out := make(map[string]string, len(meta)+2)
	for k, v := range meta {
		out[k] = v
	}
	out[truncatedKey] = note
	if dropped > 0 && out[moreKey] == "" {
		out[moreKey] = strconv.Itoa(dropped)
	}
	return out
}

func clampItems(items []signals.Item) ([]signals.Item, bool) {
	clipped := false
	out := make([]signals.Item, len(items))
	for i, it := range items {
		it.Kind = clip(it.Kind, maxLabelText, &clipped)
		it.Title = clip(it.Title, maxItemText, &clipped)
		it.Subtitle = clip(it.Subtitle, maxItemText, &clipped)
		it.Body = clip(it.Body, maxItemText, &clipped)
		it.URL = clip(it.URL, maxMetaValue, &clipped)
		it.Meta = clampMeta(it.Meta, &clipped)
		out[i] = it
	}
	return out, clipped
}

func clampMeta(meta map[string]string, clipped *bool) map[string]string {
	if meta == nil {
		return nil
	}
	out := make(map[string]string, len(meta))
	for k, v := range meta {
		out[clip(k, maxLabelText, clipped)] = clip(v, maxMetaValue, clipped)
	}
	return out
}

func clip(s string, limit int, clipped *bool) string {
	if len(s) <= limit {
		return s
	}
	*clipped = true
	cut := limit
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}
