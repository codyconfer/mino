package audit

import (
	"context"
	"time"

	"github.com/codyconfer/sisyphus/journal"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/signals"
)

type Store struct {
	j *journal.Store
}

func Open(ctx context.Context, path string) (*Store, error) {
	j, err := journal.Open(ctx, path)
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
	return s.StartFlightContext(context.Background(), name, role)
}

func (s *Store) StartFlightContext(ctx context.Context, name, role string) int64 {
	if s == nil {
		return 0
	}
	id, _ := s.j.Begin(ctx, "flight", name, roleAttrs(role))
	return id
}

func (s *Store) FinishFlight(id int64) {
	s.FinishFlightContext(context.Background(), id)
}

func (s *Store) FinishFlightContext(ctx context.Context, id int64) {
	if s == nil {
		return
	}
	_ = s.j.RollUp(ctx, id)
}

func (s *Store) RecordQuery(parentID int64, label, role string, started, finished time.Time, sections []signals.Section) {
	s.RecordQueryContext(context.Background(), parentID, label, role, started, finished, sections)
}

func (s *Store) RecordQueryContext(ctx context.Context, parentID int64, label, role string, started, finished time.Time, sections []signals.Section) {
	if s == nil {
		return
	}
	_, _ = s.j.Add(ctx, runFor(parentID, "query", label, role, started, finished, sections), recordsFor(sections))
}

func (s *Store) RecordAction(label, role string, started, finished time.Time, sections []signals.Section) {
	s.RecordActionContext(context.Background(), label, role, started, finished, sections)
}

func (s *Store) RecordActionContext(ctx context.Context, label, role string, started, finished time.Time, sections []signals.Section) {
	if s == nil {
		return
	}
	_, _ = s.j.Add(ctx, runFor(0, "write", label, role, started, finished, sections), recordsFor(sections))
}

func (s *Store) Delete(id int64) error {
	return s.DeleteContext(context.Background(), id)
}

func (s *Store) DeleteContext(ctx context.Context, id int64) error {
	if s == nil {
		return errs.New(errs.KindStore, "audit is disabled")
	}
	if err := s.j.Delete(ctx, id); err != nil {
		return errs.Wrapf(errs.KindStore, err, "deleting run %d", id)
	}
	return nil
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
