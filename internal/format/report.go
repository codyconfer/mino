package format

import (
	"time"

	"github.com/codyconfer/munin/internal/signals"
)

type Report struct {
	Formatter string
	Name      string
	Kind      string
	Role      string
	Now       time.Time
	Queries   []Group
	Sections  []Section
	Items     []Item
	Count     int
	Errors    []string
}

type Group struct {
	Query, Title string
	Sections     []Section
	Items        []Item
	Count        int
}

type Section struct {
	Query, Signal, Title string
	Items                []Item
	Meta                 map[string]string
	Err                  string
	Count                int
}

type Item struct {
	Kind, Title, Subtitle, Body, URL string
	Timestamp                        time.Time
	Meta                             map[string]string
	Query, Signal                    string
}

type Input struct {
	Formatter, Name, Kind, Role string
	Now                         time.Time
	Groups                      []InputGroup
}

type InputGroup struct {
	Query, Title string
	Sections     []signals.Section
}

func Build(in Input) Report {
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}
	r := Report{
		Formatter: signals.CleanLine(in.Formatter),
		Name:      signals.CleanLine(in.Name),
		Kind:      signals.CleanLine(in.Kind),
		Role:      signals.CleanLine(in.Role),
		Now:       now,
	}
	for _, g := range in.Groups {
		group := Group{Query: signals.CleanLine(g.Query), Title: signals.CleanLine(g.Title)}
		for _, s := range g.Sections {
			sec := buildSection(group.Query, s)
			group.Sections = append(group.Sections, sec)
			group.Items = append(group.Items, sec.Items...)
			group.Count += sec.Count
			r.Sections = append(r.Sections, sec)
			r.Items = append(r.Items, sec.Items...)
			if sec.Err != "" {
				r.Errors = append(r.Errors, sec.Err)
			}
		}
		r.Count += group.Count
		r.Queries = append(r.Queries, group)
	}
	return r
}

func buildSection(query string, s signals.Section) Section {
	s = signals.CleanSection(s)
	sec := Section{
		Query:  query,
		Signal: s.Signal,
		Title:  s.Title,
		Meta:   s.Meta,
		Err:    signals.CleanLine(s.ErrString()),
	}
	for _, it := range s.Items {
		sec.Items = append(sec.Items, Item{
			Kind:      it.Kind,
			Title:     it.Title,
			Subtitle:  it.Subtitle,
			Body:      it.Body,
			URL:       it.URL,
			Timestamp: it.Timestamp,
			Meta:      it.Meta,
			Query:     query,
			Signal:    s.Signal,
		})
	}
	sec.Count = len(sec.Items)
	return sec
}
