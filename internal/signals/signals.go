package signals

import (
	"context"
	"time"
)

type Item struct {
	Kind      string            `json:"kind"`
	Title     string            `json:"title"`
	Subtitle  string            `json:"subtitle,omitempty"`
	Body      string            `json:"body,omitempty"`
	URL       string            `json:"url,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
	Meta      map[string]string `json:"meta,omitempty"`
}

type Section struct {
	Signal string `json:"signal"`
	Title  string `json:"title"`
	Items  []Item `json:"items"`
	Err    error  `json:"-"`
}

func (s Section) ErrString() string {
	if s.Err == nil {
		return ""
	}
	return s.Err.Error()
}

type Signal interface {
	Name() string
	Fetch(ctx context.Context) ([]Section, error)
}

type ColdSignal = Signal

type Event struct {
	Source  string
	Section Section
	At      time.Time
}

type HotSignal interface {
	Name() string
	Stream(ctx context.Context) (<-chan Event, error)
	LatencyFloor() time.Duration
}
