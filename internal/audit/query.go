package audit

import (
	"context"
	"errors"
	"time"

	"github.com/codyconfer/sisyphus/journal"

	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/signals"
)

type AuditRow struct {
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

func (s *Store) RecentEntries(limit int) ([]AuditRow, error) {
	if s == nil {
		return nil, nil
	}
	runs, err := s.j.Recent(context.Background(), limit)
	if err != nil {
		return toAuditRows(runs), errs.Wrap(errs.KindStore, err, "list recent runs")
	}
	return toAuditRows(runs), nil
}

func (s *Store) Children(parentID int64) ([]AuditRow, error) {
	if s == nil {
		return nil, nil
	}
	runs, err := s.j.Children(context.Background(), parentID)
	if err != nil {
		return toAuditRows(runs), errs.Wrap(errs.KindStore, err, "list child runs")
	}
	return toAuditRows(runs), nil
}

func (s *Store) Entry(id int64) (AuditRow, bool, error) {
	if s == nil {
		return AuditRow{}, false, nil
	}
	r, ok, err := s.j.Get(context.Background(), id)
	if err != nil {
		return AuditRow{}, ok, errs.Wrap(errs.KindStore, err, "get run")
	}
	if !ok {
		return AuditRow{}, false, nil
	}
	return toAuditRow(r), true, nil
}

func (s *Store) Items(runID int64) ([]IntelRow, error) {
	if s == nil {
		return nil, nil
	}
	recs, err := s.j.Records(context.Background(), runID)
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

// Sections reconstructs signals.Section slices for a recorded run, matching
// CLI history show: flights expand child queries; other kinds load their items.
func (s *Store) Sections(id int64) ([]signals.Section, error) {
	if s == nil {
		return nil, nil
	}
	run, ok, err := s.Entry(id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errs.Newf(errs.KindUsage, "no run with id %d", id)
	}
	if run.Kind == "flight" {
		children, err := s.Children(id)
		if err != nil {
			return nil, err
		}
		var out []signals.Section
		for _, ch := range children {
			secs, err := s.sectionsForRun(ch)
			if err != nil {
				return nil, err
			}
			out = append(out, secs...)
		}
		return out, nil
	}
	return s.sectionsForRun(run)
}

func (s *Store) sectionsForRun(run AuditRow) ([]signals.Section, error) {
	items, err := s.Items(run.ID)
	if err != nil {
		return nil, err
	}
	return intelToSections(run, items), nil
}

func intelToSections(run AuditRow, items []IntelRow) []signals.Section {
	if len(items) == 0 {
		sec := signals.Section{Signal: run.Name, Title: run.Name}
		if run.Error != "" {
			sec.Err = errors.New(run.Error)
		}
		return []signals.Section{sec}
	}
	order := make([]string, 0)
	bySignal := make(map[string]*signals.Section)
	for _, it := range items {
		sig := it.Signal
		if sig == "" {
			sig = run.Name
		}
		sec, ok := bySignal[sig]
		if !ok {
			sec = &signals.Section{Signal: sig, Title: sig}
			bySignal[sig] = sec
			order = append(order, sig)
		}
		sec.Items = append(sec.Items, signals.Item{
			Kind:      it.Kind,
			Title:     it.Title,
			Subtitle:  it.Subtitle,
			URL:       it.URL,
			Timestamp: it.Ts,
		})
	}
	if run.Error != "" {
		bySignal[order[0]].Err = errors.New(run.Error)
	}
	out := make([]signals.Section, 0, len(order))
	for _, sig := range order {
		out = append(out, *bySignal[sig])
	}
	return out
}

func toAuditRows(runs []journal.Run) []AuditRow {
	out := make([]AuditRow, 0, len(runs))
	for _, r := range runs {
		out = append(out, toAuditRow(r))
	}
	return out
}

func toAuditRow(r journal.Run) AuditRow {
	return AuditRow{
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
