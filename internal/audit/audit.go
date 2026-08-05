package audit

import (
	"context"
	"sync"
	"time"

	"github.com/codyconfer/sisyphus/journal"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/log"
	"github.com/codyconfer/mino/internal/signals"
)

type Store struct {
	path string

	mu     sync.Mutex
	j      *journal.Store
	opened bool
	closed bool
}

// New returns a store that opens path on first use.
func New(path string) *Store { return &Store{path: path} }

func Open(ctx context.Context, path string) (*Store, error) {
	j, err := journal.Open(ctx, path)
	if err != nil {
		return nil, errs.Wrap(errs.KindStore, err, "open audit store")
	}
	return &Store{path: path, j: j, opened: true}, nil
}

// journal opens the backing journal on first use, or nil when unavailable.
func (s *Store) journal(ctx context.Context) *journal.Store {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		log.Debugf("audit: dropping work issued after close")
		return nil
	}
	if s.opened {
		return s.j
	}
	s.opened = true
	if s.path == "" {
		return nil
	}
	j, err := journal.Open(ctx, s.path)
	if err != nil {
		log.Debugf("audit disabled: %v", err)
		return nil
	}
	s.j = j
	return j
}

func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	if s.j == nil {
		return nil
	}
	j := s.j
	s.j = nil
	if err := j.Close(); err != nil {
		return errs.Wrap(errs.KindStore, err, "close audit store")
	}
	return nil
}

func (s *Store) StartFlight(name, role string) int64 {
	return s.StartFlightContext(context.Background(), name, role)
}

func (s *Store) StartFlightContext(ctx context.Context, name, role string) int64 {
	j := s.journal(ctx)
	if j == nil {
		return 0
	}
	id, _ := j.StartRun(ctx, "flight", name, roleAttrs(role))
	return id
}

func (s *Store) FinishFlight(id int64) {
	s.FinishFlightContext(context.Background(), id)
}

func (s *Store) FinishFlightContext(ctx context.Context, id int64) {
	j := s.journal(ctx)
	if j == nil {
		return
	}
	_ = j.FinishRun(ctx, id)
}

// QueryRun is one recorded query, buffered so a flight can flush them together.
type QueryRun struct {
	ParentID          int64
	Label, Role       string
	Started, Finished time.Time
	Sections          []signals.Section
}

func (s *Store) RecordQuery(parentID int64, label, role string, started, finished time.Time, sections []signals.Section) {
	s.RecordQueryContext(context.Background(), parentID, label, role, started, finished, sections)
}

func (s *Store) RecordQueryContext(ctx context.Context, parentID int64, label, role string, started, finished time.Time, sections []signals.Section) {
	s.RecordQueriesContext(ctx, []QueryRun{{
		ParentID: parentID, Label: label, Role: role,
		Started: started, Finished: finished, Sections: sections,
	}})
}

// RecordQueriesContext writes every buffered query against a single open journal.
func (s *Store) RecordQueriesContext(ctx context.Context, runs []QueryRun) {
	if len(runs) == 0 {
		return
	}
	j := s.journal(ctx)
	if j == nil {
		return
	}
	for _, r := range runs {
		_, _ = j.Add(ctx, runFor(r.ParentID, "query", r.Label, r.Role, r.Started, r.Finished, r.Sections), recordsFor(r.Sections))
	}
}

func (s *Store) RecordAction(label, role string, started, finished time.Time, sections []signals.Section) {
	s.RecordActionContext(context.Background(), label, role, started, finished, sections)
}

func (s *Store) RecordActionContext(ctx context.Context, label, role string, started, finished time.Time, sections []signals.Section) {
	j := s.journal(ctx)
	if j == nil {
		return
	}
	_, _ = j.Add(ctx, runFor(0, "write", label, role, started, finished, sections), recordsFor(sections))
}

func (s *Store) RecordAuth(event, role string, attrs map[string]string) {
	s.RecordAuthContext(context.Background(), event, role, attrs)
}

func (s *Store) RecordAuthContext(ctx context.Context, event, role string, attrs map[string]string) {
	j := s.journal(ctx)
	if j == nil {
		return
	}
	merged := make(map[string]string, len(attrs)+1)
	for k, v := range attrs {
		if v != "" {
			merged[k] = v
		}
	}
	if role != "" {
		merged["role"] = role
	}
	now := time.Now()
	_, _ = j.Add(ctx, journal.Run{
		Kind:     "auth",
		Name:     event,
		Started:  now,
		Finished: now,
		Attrs:    merged,
	}, nil)
}

func (s *Store) Delete(id int64) error {
	return s.DeleteContext(context.Background(), id)
}

func (s *Store) DeleteContext(ctx context.Context, id int64) error {
	j := s.journal(ctx)
	if j == nil {
		return errs.New(errs.KindStore, "audit is disabled")
	}
	if err := j.Delete(ctx, id); err != nil {
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
