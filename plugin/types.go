package plugin

import (
	"encoding/json"
	"errors"
	"time"
)

// Item is one row inside a [Section].
type Item struct {
	Kind      string            `json:"kind"`
	Title     string            `json:"title"`
	Subtitle  string            `json:"subtitle,omitempty"`
	Body      string            `json:"body,omitempty"`
	URL       string            `json:"url,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
	Meta      map[string]string `json:"meta,omitempty"`
}

// Section is a named bundle of items from a signal query or stream.
type Section struct {
	Signal string `json:"signal"`
	Title  string `json:"title"`
	Items  []Item `json:"items"`
	Err    error  `json:"-"`
}

// ErrString returns the section error message, or empty when nil.
func (s Section) ErrString() string {
	if s.Err == nil {
		return ""
	}
	return s.Err.Error()
}

type wireSection struct {
	Signal string `json:"signal"`
	Title  string `json:"title"`
	Items  []Item `json:"items"`
	Err    string `json:"err,omitempty"`
}

// MarshalJSON encodes Err as a string field.
func (s Section) MarshalJSON() ([]byte, error) {
	return json.Marshal(wireSection{
		Signal: s.Signal,
		Title:  s.Title,
		Items:  s.Items,
		Err:    s.ErrString(),
	})
}

// UnmarshalJSON decodes the string err field into Err.
func (s *Section) UnmarshalJSON(b []byte) error {
	var w wireSection
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	s.Signal = w.Signal
	s.Title = w.Title
	s.Items = w.Items
	if w.Err != "" {
		s.Err = errors.New(w.Err)
	} else {
		s.Err = nil
	}
	return nil
}

// Event is one stream emission from an active signal.
type Event struct {
	Source  string
	Section Section
	At      time.Time
}
