package plugin

import (
	"encoding/json"
	"errors"
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

type wireSection struct {
	Signal string `json:"signal"`
	Title  string `json:"title"`
	Items  []Item `json:"items"`
	Err    string `json:"err,omitempty"`
}

func (s Section) MarshalJSON() ([]byte, error) {
	return json.Marshal(wireSection{
		Signal: s.Signal,
		Title:  s.Title,
		Items:  s.Items,
		Err:    s.ErrString(),
	})
}

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

type Event struct {
	Source  string
	Section Section
	At      time.Time
}
