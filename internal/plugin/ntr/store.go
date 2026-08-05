package ntr

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/codyconfer/sisyphus/duckfile"
	"github.com/codyconfer/sisyphus/tabular"

	"github.com/codyconfer/mino/internal/config"
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
CREATE TABLE IF NOT EXISTS buckets (
  id INTEGER PRIMARY KEY,
  role VARCHAR,
  name VARCHAR,
  kind VARCHAR,
  anchor VARCHAR,
  created_at TIMESTAMP,
  updated_at TIMESTAMP
);
CREATE TABLE IF NOT EXISTS bucket_members (
  bucket_id INTEGER,
  record_id INTEGER,
  record_kind VARCHAR,
  added_at TIMESTAMP,
  PRIMARY KEY (bucket_id, record_id)
);
CREATE INDEX IF NOT EXISTS buckets_anchor_idx ON buckets (role, kind, anchor);
CREATE INDEX IF NOT EXISTS bucket_members_record_idx ON bucket_members (record_id);
CREATE SEQUENCE IF NOT EXISTS ntr_id_seq;
`

type Store struct {
	db   *duckfile.DB
	role string
}

func Open(ctx context.Context, home, role string) (*Store, error) {
	if role == "" {
		role = "default"
	}
	path := config.DataPath(home, dbName)
	db, err := duckfile.Open(ctx, path, schema)
	if err != nil {
		return nil, err
	}
	// Registration is explicit since duckfile.Open no longer registers paths
	// itself; without this the ntr database would drop out of `mino backup`.
	duckfile.RegisterBackupPath(path)
	return &Store{db: db, role: role}, nil
}

func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	return s.db.Close()
}

var ErrNoID = errors.New("ntr id: the id sequence returned no rows")

func (s *Store) nextID(ctx context.Context) (int64, error) {
	res, err := s.db.Query(ctx, `SELECT nextval('ntr_id_seq')`)
	if err != nil {
		return 0, fmt.Errorf("ntr id: %w", err)
	}
	if len(res.Rows) == 0 || len(res.Rows[0]) == 0 {
		return 0, ErrNoID
	}
	id, err := res.Row(0).Int64(0)
	if err != nil {
		return 0, fmt.Errorf("ntr id: %w", err)
	}
	return id, nil
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
		row := tabular.Row(r)
		id, _ := row.Int64(0)
		out = append(out, Note{ID: id, Title: row.Str(1), Body: row.Str(2)})
	}
	return out, nil
}

func (s *Store) DeleteNote(ctx context.Context, id int64) error {
	if err := s.db.Exec(ctx, `DELETE FROM notes WHERE id = ? AND role = ?`, id, s.role); err != nil {
		return err
	}
	return s.forgetRecord(ctx, id)
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
		row := tabular.Row(r)
		id, _ := row.Int64(0)
		due, _ := row.Time(3)
		out = append(out, Task{ID: id, Title: row.Str(1), Done: row.Bool(2), Due: due})
	}
	return out, nil
}

func (s *Store) SetTaskDone(ctx context.Context, id int64, done bool) error {
	return s.db.Exec(ctx, `UPDATE tasks SET done = ?, updated_at = ? WHERE id = ? AND role = ?`,
		done, time.Now().UTC(), id, s.role)
}

func (s *Store) DeleteTask(ctx context.Context, id int64) error {
	if err := s.db.Exec(ctx, `DELETE FROM tasks WHERE id = ? AND role = ?`, id, s.role); err != nil {
		return err
	}
	return s.forgetRecord(ctx, id)
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
		row := tabular.Row(r)
		id, _ := row.Int64(0)
		due, _ := row.Time(2)
		out = append(out, Reminder{ID: id, Title: row.Str(1), Due: due, Done: row.Bool(3)})
	}
	return out, nil
}

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
		row := tabular.Row(r)
		id, _ := row.Int64(0)
		due, _ := row.Time(2)
		out = append(out, Reminder{ID: id, Title: row.Str(1), Due: due, Done: row.Bool(3)})
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
	n, _ := res.Row(0).Int64(0)
	return int(n), nil
}

func (s *Store) UpdateNote(ctx context.Context, id int64, title, body string) error {
	return s.db.Exec(ctx,
		`UPDATE notes SET title = ?, body = ?, updated_at = ? WHERE id = ? AND role = ?`,
		title, body, time.Now().UTC(), id, s.role)
}

func (s *Store) exists(ctx context.Context, table, kind string, id int64) error {
	res, err := s.db.Query(ctx, fmt.Sprintf(`SELECT 1 FROM %s WHERE id = ? AND role = ?`, table), id, s.role)
	if err != nil {
		return err
	}
	if len(res.Rows) == 0 {
		return fmt.Errorf("%s #%d no longer exists", kind, id)
	}
	return nil
}

func (s *Store) UpdateTask(ctx context.Context, id int64, title string, done bool, due time.Time) error {
	if err := s.exists(ctx, "tasks", "task", id); err != nil {
		return err
	}
	return s.db.Exec(ctx,
		`UPDATE tasks SET title = ?, done = ?, due = ?, updated_at = ? WHERE id = ? AND role = ?`,
		title, done, nullTime(due), time.Now().UTC(), id, s.role)
}

func (s *Store) UpdateReminder(ctx context.Context, id int64, title string, due time.Time) error {
	if err := s.exists(ctx, "reminders", "reminder", id); err != nil {
		return err
	}
	return s.db.Exec(ctx,
		`UPDATE reminders SET title = ?, due = ? WHERE id = ? AND role = ?`,
		title, due.UTC(), id, s.role)
}

func (s *Store) DeleteReminder(ctx context.Context, id int64) error {
	if err := s.db.Exec(ctx, `DELETE FROM reminders WHERE id = ? AND role = ?`, id, s.role); err != nil {
		return err
	}
	return s.forgetRecord(ctx, id)
}

func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC()
}
