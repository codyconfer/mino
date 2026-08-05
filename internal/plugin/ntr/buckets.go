package ntr

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/codyconfer/sisyphus/tabular"
)

const (
	BucketKindUser = "user"
	BucketKindItem = "item"
	BucketKindRun  = "run"
)

const anchorCountChunk = 200

type Bucket struct {
	ID      int64
	Name    string
	Kind    string
	Anchor  string
	Members int
	Created time.Time
	Updated time.Time
}

func (b Bucket) Anchored() bool { return b.Kind == BucketKindItem || b.Kind == BucketKindRun }

func RunAnchor(id int64) string { return "run:" + strconv.FormatInt(id, 10) }

func bucketKind(kind string) string {
	switch kind {
	case BucketKindItem, BucketKindRun:
		return kind
	default:
		return BucketKindUser
	}
}

func (s *Store) CreateBucket(ctx context.Context, name, kind, anchor string) (Bucket, error) {
	id, err := s.nextID(ctx)
	if err != nil {
		return Bucket{}, err
	}
	kind = bucketKind(kind)
	now := time.Now().UTC()
	err = s.db.Exec(ctx,
		`INSERT INTO buckets (id, role, name, kind, anchor, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, s.role, name, kind, anchor, now, now)
	return Bucket{ID: id, Name: name, Kind: kind, Anchor: anchor, Created: now, Updated: now}, err
}

const bucketCols = `b.id, b.name, b.kind, b.anchor, b.created_at, b.updated_at,
  (SELECT count(*) FROM bucket_members m WHERE m.bucket_id = b.id)`

func scanBucket(r []string) Bucket {
	row := tabular.Row(r)
	id, _ := row.Int64(0)
	created, _ := row.Time(4)
	updated, _ := row.Time(5)
	members, _ := row.Int64(6)
	return Bucket{
		ID:      id,
		Name:    row.Str(1),
		Kind:    row.Str(2),
		Anchor:  row.Str(3),
		Members: int(members),
		Created: created,
		Updated: updated,
	}
}

func (s *Store) ListBuckets(ctx context.Context) ([]Bucket, error) {
	res, err := s.db.Query(ctx,
		`SELECT `+bucketCols+` FROM buckets b WHERE b.role = ? ORDER BY b.updated_at DESC, b.id`, s.role)
	if err != nil {
		return nil, err
	}
	out := make([]Bucket, 0, len(res.Rows))
	for _, r := range res.Rows {
		out = append(out, scanBucket(r))
	}
	return out, nil
}

func (s *Store) Bucket(ctx context.Context, id int64) (Bucket, bool, error) {
	res, err := s.db.Query(ctx,
		`SELECT `+bucketCols+` FROM buckets b WHERE b.id = ? AND b.role = ?`, id, s.role)
	if err != nil || len(res.Rows) == 0 {
		return Bucket{}, false, err
	}
	return scanBucket(res.Rows[0]), true, nil
}

func (s *Store) BucketByAnchor(ctx context.Context, kind, anchor string) (Bucket, bool, error) {
	if strings.TrimSpace(anchor) == "" {
		return Bucket{}, false, nil
	}
	res, err := s.db.Query(ctx,
		`SELECT `+bucketCols+` FROM buckets b
		  WHERE b.role = ? AND b.kind = ? AND b.anchor = ? ORDER BY b.id LIMIT 1`,
		s.role, bucketKind(kind), anchor)
	if err != nil || len(res.Rows) == 0 {
		return Bucket{}, false, err
	}
	return scanBucket(res.Rows[0]), true, nil
}

func (s *Store) EnsureAnchorBucket(ctx context.Context, kind, anchor, name string) (Bucket, error) {
	b, ok, err := s.BucketByAnchor(ctx, kind, anchor)
	if err != nil {
		return Bucket{}, err
	}
	if ok {
		return b, nil
	}
	if strings.TrimSpace(name) == "" {
		name = anchor
	}
	return s.CreateBucket(ctx, name, kind, anchor)
}

func (s *Store) RenameBucket(ctx context.Context, id int64, name string) error {
	if err := s.exists(ctx, "buckets", "bucket", id); err != nil {
		return err
	}
	return s.db.Exec(ctx, `UPDATE buckets SET name = ?, updated_at = ? WHERE id = ? AND role = ?`,
		name, time.Now().UTC(), id, s.role)
}

func (s *Store) DeleteBucket(ctx context.Context, id int64) error {
	res, err := s.db.Query(ctx, `SELECT 1 FROM buckets WHERE id = ? AND role = ?`, id, s.role)
	if err != nil {
		return err
	}
	if len(res.Rows) == 0 {
		return nil
	}
	if err := s.db.Exec(ctx, `DELETE FROM bucket_members WHERE bucket_id = ?`, id); err != nil {
		return err
	}
	return s.db.Exec(ctx, `DELETE FROM buckets WHERE id = ? AND role = ?`, id, s.role)
}

func (s *Store) touchBucket(ctx context.Context, id int64) error {
	return s.db.Exec(ctx, `UPDATE buckets SET updated_at = ? WHERE id = ? AND role = ?`,
		time.Now().UTC(), id, s.role)
}

func (s *Store) AddMember(ctx context.Context, bucketID, recordID int64, kind string) error {
	if err := s.exists(ctx, "buckets", "bucket", bucketID); err != nil {
		return err
	}
	err := s.db.Exec(ctx,
		`INSERT INTO bucket_members (bucket_id, record_id, record_kind, added_at)
		 SELECT ?, ?, ?, ?
		  WHERE NOT EXISTS (SELECT 1 FROM bucket_members WHERE bucket_id = ? AND record_id = ?)`,
		bucketID, recordID, kind, time.Now().UTC(), bucketID, recordID)
	if err != nil {
		return err
	}
	return s.touchBucket(ctx, bucketID)
}

func (s *Store) RemoveMember(ctx context.Context, bucketID, recordID int64) error {
	if err := s.exists(ctx, "buckets", "bucket", bucketID); err != nil {
		return err
	}
	if err := s.db.Exec(ctx, `DELETE FROM bucket_members WHERE bucket_id = ? AND record_id = ?`,
		bucketID, recordID); err != nil {
		return err
	}
	return s.touchBucket(ctx, bucketID)
}

func (s *Store) BucketsFor(ctx context.Context, recordID int64) ([]Bucket, error) {
	res, err := s.db.Query(ctx,
		`SELECT `+bucketCols+` FROM buckets b JOIN bucket_members m ON m.bucket_id = b.id
		  WHERE b.role = ? AND m.record_id = ? ORDER BY b.id`, s.role, recordID)
	if err != nil {
		return nil, err
	}
	out := make([]Bucket, 0, len(res.Rows))
	for _, r := range res.Rows {
		out = append(out, scanBucket(r))
	}
	return out, nil
}

func (s *Store) AnchorCounts(ctx context.Context, kind string, anchors []string) (map[string]int, error) {
	out := make(map[string]int)
	want := make([]string, 0, len(anchors))
	seen := make(map[string]bool, len(anchors))
	for _, a := range anchors {
		if a = strings.TrimSpace(a); a == "" || seen[a] {
			continue
		}
		seen[a] = true
		want = append(want, a)
	}
	kind = bucketKind(kind)
	for len(want) > 0 {
		chunk := want
		if len(chunk) > anchorCountChunk {
			chunk = chunk[:anchorCountChunk]
		}
		want = want[len(chunk):]
		args := make([]any, 0, len(chunk)+2)
		args = append(args, s.role, kind)
		for _, a := range chunk {
			args = append(args, a)
		}
		q := `SELECT b.anchor, count(*) FROM buckets b JOIN bucket_members m ON m.bucket_id = b.id
		       WHERE b.role = ? AND b.kind = ? AND b.anchor IN (` +
			strings.TrimSuffix(strings.Repeat("?,", len(chunk)), ",") + `) GROUP BY b.anchor`
		res, err := s.db.Query(ctx, q, args...)
		if err != nil {
			return nil, err
		}
		for _, r := range res.Rows {
			row := tabular.Row(r)
			n, _ := row.Int64(1)
			out[row.Str(0)] += int(n)
		}
	}
	return out, nil
}

func (s *Store) bucketRecords(ctx context.Context, bucketID int64) ([]record, error) {
	res, err := s.db.Query(ctx,
		`SELECT 'note' AS kind, n.id, n.title, n.body, CAST(NULL AS TIMESTAMP) AS due, false AS done
		   FROM bucket_members m JOIN notes n ON n.id = m.record_id
		  WHERE m.bucket_id = ? AND n.role = ?
		 UNION ALL
		 SELECT 'task', t.id, t.title, '', t.due, t.done
		   FROM bucket_members m JOIN tasks t ON t.id = m.record_id
		  WHERE m.bucket_id = ? AND t.role = ?
		 UNION ALL
		 SELECT 'reminder', r.id, r.title, '', r.due, r.done
		   FROM bucket_members m JOIN reminders r ON r.id = m.record_id
		  WHERE m.bucket_id = ? AND r.role = ?
		 ORDER BY 2`,
		bucketID, s.role, bucketID, s.role, bucketID, s.role)
	if err != nil {
		return nil, err
	}
	out := make([]record, 0, len(res.Rows))
	for _, r := range res.Rows {
		row := tabular.Row(r)
		id, _ := row.Int64(1)
		due, _ := row.Time(4)
		out = append(out, record{
			Kind:  row.Str(0),
			ID:    id,
			Title: row.Str(2),
			Body:  row.Str(3),
			Due:   due,
			Done:  row.Bool(5),
		})
	}
	return out, nil
}

func (s *Store) recordKind(ctx context.Context, id int64) (string, bool, error) {
	for _, t := range []struct{ table, kind string }{
		{"notes", kindNote},
		{"tasks", kindTask},
		{"reminders", kindReminder},
	} {
		res, err := s.db.Query(ctx,
			`SELECT 1 FROM `+t.table+` WHERE id = ? AND role = ?`, id, s.role)
		if err != nil {
			return "", false, err
		}
		if len(res.Rows) > 0 {
			return t.kind, true, nil
		}
	}
	return "", false, nil
}

func (s *Store) forgetRecord(ctx context.Context, id int64) error {
	return s.db.Exec(ctx,
		`DELETE FROM bucket_members WHERE record_id = ?
		   AND bucket_id IN (SELECT id FROM buckets WHERE role = ?)`, id, s.role)
}

func FiledCounts(home, role string, urls []string) (map[string]int, error) {
	var out map[string]int
	err := withStore(home, role, recordReadTimeout, func(ctx context.Context, st *Store) error {
		counts, err := st.AnchorCounts(ctx, BucketKindItem, urls)
		if err != nil {
			return err
		}
		out = counts
		return nil
	})
	return out, err
}

func RunFiledCount(home, role string, id int64) (int, error) {
	var n int
	err := withStore(home, role, recordReadTimeout, func(ctx context.Context, st *Store) error {
		counts, err := st.AnchorCounts(ctx, BucketKindRun, []string{RunAnchor(id)})
		if err != nil {
			return err
		}
		n = counts[RunAnchor(id)]
		return nil
	})
	return n, err
}
