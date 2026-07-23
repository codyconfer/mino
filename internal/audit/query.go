package audit

import (
	"time"

	"github.com/codyconfer/sisyphus/journal"

	"github.com/codyconfer/munin/internal/errs"
)

type FlightRow struct {
	ID        int64
	Kind      string
	Name      string
	Role      string
	Started   time.Time
	Finished  time.Time
	ItemCount int
	Error     string
}

type IntelRow struct {
	Signal   string
	Kind     string
	Title    string
	Subtitle string
	URL      string
	Ts       time.Time
}

func (s *Store) RecentRuns(limit int) ([]FlightRow, error) {
	if s == nil {
		return nil, nil
	}
	runs, err := s.j.Recent(limit)
	if err != nil {
		return toFlightRows(runs), errs.Wrap(errs.KindStore, err, "list recent runs")
	}
	return toFlightRows(runs), nil
}

func (s *Store) ChildRuns(parentID int64) ([]FlightRow, error) {
	if s == nil {
		return nil, nil
	}
	runs, err := s.j.Children(parentID)
	if err != nil {
		return toFlightRows(runs), errs.Wrap(errs.KindStore, err, "list child runs")
	}
	return toFlightRows(runs), nil
}

func (s *Store) Run(id int64) (FlightRow, bool, error) {
	if s == nil {
		return FlightRow{}, false, nil
	}
	r, ok, err := s.j.Get(id)
	if err != nil {
		return FlightRow{}, ok, errs.Wrap(errs.KindStore, err, "get run")
	}
	if !ok {
		return FlightRow{}, false, nil
	}
	return toFlightRow(r), true, nil
}

func (s *Store) Items(runID int64) ([]IntelRow, error) {
	if s == nil {
		return nil, nil
	}
	recs, err := s.j.Records(runID)
	if err != nil {
		return nil, errs.Wrap(errs.KindStore, err, "read run items")
	}
	out := make([]IntelRow, 0, len(recs))
	for _, r := range recs {
		a := r.Attrs
		out = append(out, IntelRow{
			Signal:   a["signal"],
			Kind:     a["kind"],
			Title:    a["title"],
			Subtitle: a["subtitle"],
			URL:      a["url"],
			Ts:       r.Ts,
		})
	}
	return out, nil
}

func toFlightRows(runs []journal.Run) []FlightRow {
	out := make([]FlightRow, 0, len(runs))
	for _, r := range runs {
		out = append(out, toFlightRow(r))
	}
	return out
}

func toFlightRow(r journal.Run) FlightRow {
	return FlightRow{
		ID:        r.ID,
		Kind:      r.Kind,
		Name:      r.Name,
		Role:      r.Attrs["role"],
		Started:   r.Started,
		Finished:  r.Finished,
		ItemCount: r.Count,
		Error:     r.Error,
	}
}
