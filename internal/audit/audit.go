package audit

import (
	"time"

	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/signals"
	"github.com/codyconfer/sisyphus/journal"
)

type Store struct {
	j *journal.Store
}

func Open(path string) (*Store, error) {
	j, err := journal.Open(path)
	if err != nil {
		return nil, errs.Wrap(errs.KindStore, err, "open audit store")
	}
	return &Store{j: j}, nil
}

func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	if err := s.j.Close(); err != nil {
		return errs.Wrap(errs.KindStore, err, "close audit store")
	}
	return nil
}

func (s *Store) StartFlight(name, role string) int64 {
	if s == nil {
		return 0
	}
	id, _ := s.j.Begin("flight", name, roleAttrs(role))
	return id
}

func (s *Store) FinishFlight(id int64) {
	if s == nil {
		return
	}
	_ = s.j.RollUp(id)
}

func (s *Store) RecordQuery(parentID int64, label, role string, started, finished time.Time, sections []signals.Section) {
	if s == nil {
		return
	}
	_, _ = s.j.Add(runFor(parentID, "query", label, role, started, finished, sections), recordsFor(sections))
}

func (s *Store) RecordAction(label, role string, started, finished time.Time, sections []signals.Section) {
	if s == nil {
		return
	}
	_, _ = s.j.Add(runFor(0, "write", label, role, started, finished, sections), recordsFor(sections))
}

func runFor(parentID int64, kind, name, role string, started, finished time.Time, sections []signals.Section) journal.Run {
	count, errText := 0, ""
	for _, sec := range sections {
		count += len(sec.Items)
		if sec.Err != nil && errText == "" {
			errText = sec.Err.Error()
		}
	}
	return journal.Run{
		ParentID: parentID,
		Kind:     kind,
		Name:     name,
		Started:  started,
		Finished: finished,
		Count:    count,
		Error:    errText,
		Attrs:    roleAttrs(role),
	}
}

func recordsFor(sections []signals.Section) []journal.Record {
	var recs []journal.Record
	for _, sec := range sections {
		for _, it := range sec.Items {
			recs = append(recs, journal.Record{
				Ts: it.Timestamp,
				Attrs: map[string]string{
					"signal":   sec.Signal,
					"kind":     it.Kind,
					"title":    it.Title,
					"subtitle": it.Subtitle,
					"url":      it.URL,
				},
			})
		}
	}
	return recs
}

func roleAttrs(role string) map[string]string {
	if role == "" {
		return nil
	}
	return map[string]string{"role": role}
}
