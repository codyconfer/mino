package ntr

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/codyconfer/sisyphus/store"

	"github.com/codyconfer/munin/internal/config"
)

const dbName = "ntr.duckdb"

const schema = `
CREATE TABLE IF NOT EXISTS notes (
  id INTEGER PRIMARY KEY,
  role VARCHAR,
  title VARCHAR,
  body VARCHAR,
  created_at TIMESTAMP,
  updated_at TIMESTAMP
);
CREATE TABLE IF NOT EXISTS tasks (
  id INTEGER PRIMARY KEY,
  role VARCHAR,
  title VARCHAR,
  done BOOLEAN,
  due TIMESTAMP,
  created_at TIMESTAMP,
  updated_at TIMESTAMP
);
CREATE TABLE IF NOT EXISTS reminders (
  id INTEGER PRIMARY KEY,
  role VARCHAR,
  title VARCHAR,
  due TIMESTAMP,
  done BOOLEAN,
  created_at TIMESTAMP
);
CREATE SEQUENCE IF NOT EXISTS ntr_id_seq;
`

// Store is the role-namespaced NTR DuckDB.
type Store struct {
	db   *store.DB
	role string
}

func Open(ctx context.Context, home, role string) (*Store, error) {
	if role == "" {
		role = "default"
	}
	path := config.DataPath(home, dbName)
	db, err := store.Open(ctx, path, schema)
	if err != nil {
		return nil, err
	}
	return &Store{db: db, role: role}, nil
}

func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) nextID(ctx context.Context) (int64, error) {
	res, err := s.db.Query(ctx, `SELECT nextval('ntr_id_seq')`)
	if err != nil || len(res.Rows) == 0 {
		return 0, fmt.Errorf("ntr id: %w", err)
	}
	return strconv.ParseInt(res.Rows[0][0], 10, 64)
}

type Note struct {
	ID    int64
	Title string
	Body  string
}

type Task struct {
	ID    int64
	Title string
	Done  bool
	Due   time.Time
}

type Reminder struct {
	ID    int64
	Title string
	Due   time.Time
	Done  bool
}

func (s *Store) CreateNote(ctx context.Context, title, body string) (Note, error) {
	id, err := s.nextID(ctx)
	if err != nil {
		return Note{}, err
	}
	now := time.Now().UTC()
	err = s.db.Exec(ctx,
		`INSERT INTO notes (id, role, title, body, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		id, s.role, title, body, now, now)
	return Note{ID: id, Title: title, Body: body}, err
}

func (s *Store) ListNotes(ctx context.Context) ([]Note, error) {
	res, err := s.db.Query(ctx, `SELECT id, title, body FROM notes WHERE role = ? ORDER BY id`, s.role)
	if err != nil {
		return nil, err
	}
	out := make([]Note, 0, len(res.Rows))
	for _, r := range res.Rows {
		id, _ := strconv.ParseInt(r[0], 10, 64)
		out = append(out, Note{ID: id, Title: r[1], Body: r[2]})
	}
	return out, nil
}

func (s *Store) DeleteNote(ctx context.Context, id int64) error {
	return s.db.Exec(ctx, `DELETE FROM notes WHERE id = ? AND role = ?`, id, s.role)
}

func (s *Store) CreateTask(ctx context.Context, title string, due time.Time) (Task, error) {
	id, err := s.nextID(ctx)
	if err != nil {
		return Task{}, err
	}
	now := time.Now().UTC()
	err = s.db.Exec(ctx,
		`INSERT INTO tasks (id, role, title, done, due, created_at, updated_at) VALUES (?, ?, ?, false, ?, ?, ?)`,
		id, s.role, title, nullTime(due), now, now)
	return Task{ID: id, Title: title, Due: due}, err
}

func (s *Store) ListTasks(ctx context.Context, includeDone bool) ([]Task, error) {
	q := `SELECT id, title, done, due FROM tasks WHERE role = ?`
	if !includeDone {
		q += ` AND done = false`
	}
	q += ` ORDER BY id`
	res, err := s.db.Query(ctx, q, s.role)
	if err != nil {
		return nil, err
	}
	out := make([]Task, 0, len(res.Rows))
	for _, r := range res.Rows {
		id, _ := strconv.ParseInt(r[0], 10, 64)
		out = append(out, Task{ID: id, Title: r[1], Done: truthy(r[2]), Due: parseTime(r[3])})
	}
	return out, nil
}

func (s *Store) SetTaskDone(ctx context.Context, id int64, done bool) error {
	return s.db.Exec(ctx, `UPDATE tasks SET done = ?, updated_at = ? WHERE id = ? AND role = ?`,
		done, time.Now().UTC(), id, s.role)
}

func (s *Store) DeleteTask(ctx context.Context, id int64) error {
	return s.db.Exec(ctx, `DELETE FROM tasks WHERE id = ? AND role = ?`, id, s.role)
}

func (s *Store) CreateReminder(ctx context.Context, title string, due time.Time) (Reminder, error) {
	id, err := s.nextID(ctx)
	if err != nil {
		return Reminder{}, err
	}
	err = s.db.Exec(ctx,
		`INSERT INTO reminders (id, role, title, due, done, created_at) VALUES (?, ?, ?, ?, false, ?)`,
		id, s.role, title, due.UTC(), time.Now().UTC())
	return Reminder{ID: id, Title: title, Due: due}, err
}

func (s *Store) DueReminders(ctx context.Context, now time.Time) ([]Reminder, error) {
	res, err := s.db.Query(ctx,
		`SELECT id, title, due, done FROM reminders WHERE role = ? AND done = false AND due <= ? ORDER BY due`,
		s.role, now.UTC())
	if err != nil {
		return nil, err
	}
	out := make([]Reminder, 0, len(res.Rows))
	for _, r := range res.Rows {
		id, _ := strconv.ParseInt(r[0], 10, 64)
		out = append(out, Reminder{ID: id, Title: r[1], Due: parseTime(r[2]), Done: truthy(r[3])})
	}
	return out, nil
}

// ListReminders returns open (or all) reminders ordered by due time.
func (s *Store) ListReminders(ctx context.Context, includeDone bool) ([]Reminder, error) {
	q := `SELECT id, title, due, done FROM reminders WHERE role = ?`
	if !includeDone {
		q += ` AND done = false`
	}
	q += ` ORDER BY due, id`
	res, err := s.db.Query(ctx, q, s.role)
	if err != nil {
		return nil, err
	}
	out := make([]Reminder, 0, len(res.Rows))
	for _, r := range res.Rows {
		id, _ := strconv.ParseInt(r[0], 10, 64)
		out = append(out, Reminder{ID: id, Title: r[1], Due: parseTime(r[2]), Done: truthy(r[3])})
	}
	return out, nil
}

func (s *Store) MarkReminderDone(ctx context.Context, id int64) error {
	return s.db.Exec(ctx, `UPDATE reminders SET done = true WHERE id = ? AND role = ?`, id, s.role)
}

func (s *Store) DueTodayCount(ctx context.Context, now time.Time) (int, error) {
	loc := now.Location()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	end := start.Add(24 * time.Hour)
	res, err := s.db.Query(ctx,
		`SELECT count(*) FROM reminders WHERE role = ? AND done = false AND due >= ? AND due < ?`,
		s.role, start, end)
	if err != nil || len(res.Rows) == 0 {
		return 0, err
	}
	n, _ := strconv.Atoi(res.Rows[0][0])
	return n, nil
}

func (s *Store) UpdateNote(ctx context.Context, id int64, title, body string) error {
	return s.db.Exec(ctx,
		`UPDATE notes SET title = ?, body = ?, updated_at = ? WHERE id = ? AND role = ?`,
		title, body, time.Now().UTC(), id, s.role)
}

func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC()
}

func parseTime(s string) time.Time {
	if s == "" || s == "NULL" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func truthy(s string) bool {
	switch strings.ToLower(s) {
	case "true", "t", "1":
		return true
	}
	return false
}
