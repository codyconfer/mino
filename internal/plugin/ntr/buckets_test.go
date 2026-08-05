package ntr

import (
	"context"
	"strings"
	"testing"
	"time"
)

func openStore(t *testing.T, home, role string) *Store {
	t.Helper()
	st, err := Open(context.Background(), home, role)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestBucketCRUD(t *testing.T) {
	ctx := context.Background()
	st := openStore(t, t.TempDir(), "work")

	b, err := st.CreateBucket(ctx, "escalations", BucketKindUser, "")
	if err != nil || b.ID == 0 {
		t.Fatalf("CreateBucket = %+v err=%v", b, err)
	}
	if b.Kind != BucketKindUser || b.Anchored() {
		t.Fatalf("kind = %q anchored = %v, want user and not anchored", b.Kind, b.Anchored())
	}

	list, err := st.ListBuckets(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListBuckets = %v err=%v", list, err)
	}
	if list[0].Name != "escalations" || list[0].Members != 0 {
		t.Fatalf("bucket = %+v, want escalations with 0 members", list[0])
	}

	if err := st.RenameBucket(ctx, b.ID, "pages"); err != nil {
		t.Fatal(err)
	}
	got, ok, err := st.Bucket(ctx, b.ID)
	if err != nil || !ok || got.Name != "pages" {
		t.Fatalf("Bucket = %+v ok=%v err=%v, want name pages", got, ok, err)
	}

	if err := st.DeleteBucket(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := st.Bucket(ctx, b.ID); ok {
		t.Fatal("bucket survived DeleteBucket")
	}
}

func TestBucketUnknownIDRenameReports(t *testing.T) {
	ctx := context.Background()
	st := openStore(t, t.TempDir(), "work")

	err := st.RenameBucket(ctx, 999, "nope")
	if err == nil || !strings.Contains(err.Error(), "bucket #999 no longer exists") {
		t.Fatalf("RenameBucket err = %v, want a no-longer-exists error", err)
	}
}

func TestBucketRoleScoped(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	work := openStore(t, home, "work")
	other := openStore(t, home, "other")

	b, err := work.CreateBucket(ctx, "mine", BucketKindUser, "")
	if err != nil {
		t.Fatal(err)
	}

	list, err := other.ListBuckets(ctx)
	if err != nil || len(list) != 0 {
		t.Fatalf("other role sees %v err=%v, want none", list, err)
	}
	if _, ok, _ := other.Bucket(ctx, b.ID); ok {
		t.Fatal("other role read another role's bucket")
	}
	if err := other.RenameBucket(ctx, b.ID, "theirs"); err == nil {
		t.Fatal("other role renamed another role's bucket")
	}

	if err := other.DeleteBucket(ctx, b.ID); err != nil {
		t.Fatalf("DeleteBucket across roles = %v, want a no-op", err)
	}
	if _, ok, _ := work.Bucket(ctx, b.ID); !ok {
		t.Fatal("another role's delete removed the bucket")
	}
}

func TestEnsureAnchorBucketReusesOneRow(t *testing.T) {
	ctx := context.Background()
	st := openStore(t, t.TempDir(), "work")
	const url = "https://github.com/o/r/pull/1"

	first, err := st.EnsureAnchorBucket(ctx, BucketKindItem, url, "PR #1")
	if err != nil {
		t.Fatal(err)
	}
	second, err := st.EnsureAnchorBucket(ctx, BucketKindItem, url, "PR #1 again")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("ensured two buckets: %d and %d", first.ID, second.ID)
	}
	if !first.Anchored() || first.Anchor != url {
		t.Fatalf("bucket = %+v, want anchored to %s", first, url)
	}
	list, err := st.ListBuckets(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListBuckets = %v err=%v, want one", list, err)
	}
}

func TestEnsureAnchorBucketSeparatesKinds(t *testing.T) {
	ctx := context.Background()
	st := openStore(t, t.TempDir(), "work")

	item, err := st.EnsureAnchorBucket(ctx, BucketKindItem, "x", "item x")
	if err != nil {
		t.Fatal(err)
	}
	run, err := st.EnsureAnchorBucket(ctx, BucketKindRun, "x", "run x")
	if err != nil {
		t.Fatal(err)
	}
	if item.ID == run.ID {
		t.Fatal("an item anchor and a run anchor collapsed into one bucket")
	}
}

func TestAddMemberIsIdempotent(t *testing.T) {
	ctx := context.Background()
	st := openStore(t, t.TempDir(), "work")

	b, err := st.CreateBucket(ctx, "b", BucketKindUser, "")
	if err != nil {
		t.Fatal(err)
	}
	n, err := st.CreateNote(ctx, "note", "")
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := st.AddMember(ctx, b.ID, n.ID, kindNote); err != nil {
			t.Fatal(err)
		}
	}
	got, _, err := st.Bucket(ctx, b.ID)
	if err != nil || got.Members != 1 {
		t.Fatalf("members = %d err=%v, want 1", got.Members, err)
	}
}

func TestAddMemberUnknownBucketReports(t *testing.T) {
	ctx := context.Background()
	st := openStore(t, t.TempDir(), "work")

	n, err := st.CreateNote(ctx, "note", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AddMember(ctx, 4242, n.ID, kindNote); err == nil {
		t.Fatal("AddMember into an unknown bucket succeeded")
	}
}

func TestBucketRecordsSpansAllKinds(t *testing.T) {
	ctx := context.Background()
	st := openStore(t, t.TempDir(), "work")

	b, err := st.CreateBucket(ctx, "b", BucketKindUser, "")
	if err != nil {
		t.Fatal(err)
	}
	due := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	n, _ := st.CreateNote(ctx, "the note", "the body")
	tk, _ := st.CreateTask(ctx, "the task", due)
	rm, _ := st.CreateReminder(ctx, "the reminder", due)
	for id, kind := range map[int64]string{n.ID: kindNote, tk.ID: kindTask, rm.ID: kindReminder} {
		if err := st.AddMember(ctx, b.ID, id, kind); err != nil {
			t.Fatal(err)
		}
	}

	recs, err := st.bucketRecords(ctx, b.ID)
	if err != nil || len(recs) != 3 {
		t.Fatalf("bucketRecords = %v err=%v, want 3", recs, err)
	}

	kinds := []string{recs[0].Kind, recs[1].Kind, recs[2].Kind}
	want := []string{kindNote, kindTask, kindReminder}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("kinds = %v, want %v", kinds, want)
		}
	}
	if recs[0].Title != "the note" || recs[0].Body != "the body" {
		t.Fatalf("note record = %+v", recs[0])
	}
	if !recs[0].Due.IsZero() {
		t.Fatalf("note carried a due: %v", recs[0].Due)
	}
	if recs[1].Due.IsZero() || recs[2].Due.IsZero() {
		t.Fatalf("task/reminder lost their due: %+v %+v", recs[1], recs[2])
	}
}

func TestBucketRecordsExcludesOtherRoles(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	work := openStore(t, home, "work")
	other := openStore(t, home, "other")

	b, err := work.CreateBucket(ctx, "b", BucketKindUser, "")
	if err != nil {
		t.Fatal(err)
	}
	mine, _ := work.CreateNote(ctx, "mine", "")
	theirs, _ := other.CreateNote(ctx, "theirs", "")
	if err := work.AddMember(ctx, b.ID, mine.ID, kindNote); err != nil {
		t.Fatal(err)
	}

	if err := work.db.Exec(ctx,
		`INSERT INTO bucket_members (bucket_id, record_id, record_kind, added_at) VALUES (?, ?, ?, ?)`,
		b.ID, theirs.ID, kindNote, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	recs, err := work.bucketRecords(ctx, b.ID)
	if err != nil || len(recs) != 1 || recs[0].Title != "mine" {
		t.Fatalf("bucketRecords = %v err=%v, want only mine", recs, err)
	}
}

func TestRemoveMemberKeepsTheRecord(t *testing.T) {
	ctx := context.Background()
	st := openStore(t, t.TempDir(), "work")

	b, _ := st.CreateBucket(ctx, "b", BucketKindUser, "")
	n, _ := st.CreateNote(ctx, "keep me", "")
	if err := st.AddMember(ctx, b.ID, n.ID, kindNote); err != nil {
		t.Fatal(err)
	}
	if err := st.RemoveMember(ctx, b.ID, n.ID); err != nil {
		t.Fatal(err)
	}

	recs, _ := st.bucketRecords(ctx, b.ID)
	if len(recs) != 0 {
		t.Fatalf("bucketRecords = %v, want empty", recs)
	}
	notes, _ := st.ListNotes(ctx)
	if len(notes) != 1 {
		t.Fatalf("ListNotes = %v, want the note to survive unfiling", notes)
	}
}

func TestDeleteBucketKeepsRecords(t *testing.T) {
	ctx := context.Background()
	st := openStore(t, t.TempDir(), "work")

	b, _ := st.CreateBucket(ctx, "b", BucketKindUser, "")
	n, _ := st.CreateNote(ctx, "survivor", "")
	if err := st.AddMember(ctx, b.ID, n.ID, kindNote); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteBucket(ctx, b.ID); err != nil {
		t.Fatal(err)
	}

	notes, _ := st.ListNotes(ctx)
	if len(notes) != 1 {
		t.Fatalf("ListNotes = %v, want the note to survive", notes)
	}
	buckets, _ := st.BucketsFor(ctx, n.ID)
	if len(buckets) != 0 {
		t.Fatalf("BucketsFor = %v, want no orphan membership", buckets)
	}
}

func TestDeletingARecordPurgesMemberships(t *testing.T) {
	ctx := context.Background()
	st := openStore(t, t.TempDir(), "work")

	b, _ := st.CreateBucket(ctx, "b", BucketKindUser, "")
	n, _ := st.CreateNote(ctx, "note", "")
	tk, _ := st.CreateTask(ctx, "task", time.Time{})
	rm, _ := st.CreateReminder(ctx, "reminder", time.Now().Add(time.Hour))
	for id, kind := range map[int64]string{n.ID: kindNote, tk.ID: kindTask, rm.ID: kindReminder} {
		if err := st.AddMember(ctx, b.ID, id, kind); err != nil {
			t.Fatal(err)
		}
	}

	for _, del := range []func() error{
		func() error { return st.DeleteNote(ctx, n.ID) },
		func() error { return st.DeleteTask(ctx, tk.ID) },
		func() error { return st.DeleteReminder(ctx, rm.ID) },
	} {
		if err := del(); err != nil {
			t.Fatal(err)
		}
	}

	got, _, err := st.Bucket(ctx, b.ID)
	if err != nil || got.Members != 0 {
		t.Fatalf("members = %d err=%v, want 0 after deleting every record", got.Members, err)
	}
}

func TestBucketsForListsEveryBucket(t *testing.T) {
	ctx := context.Background()
	st := openStore(t, t.TempDir(), "work")

	user, _ := st.CreateBucket(ctx, "escalations", BucketKindUser, "")
	item, _ := st.EnsureAnchorBucket(ctx, BucketKindItem, "https://x/1", "PR #1")
	n, _ := st.CreateNote(ctx, "note", "")
	for _, b := range []Bucket{user, item} {
		if err := st.AddMember(ctx, b.ID, n.ID, kindNote); err != nil {
			t.Fatal(err)
		}
	}

	got, err := st.BucketsFor(ctx, n.ID)
	if err != nil || len(got) != 2 {
		t.Fatalf("BucketsFor = %v err=%v, want 2", got, err)
	}
}

func TestAnchorCountsSumsAndOmits(t *testing.T) {
	ctx := context.Background()
	st := openStore(t, t.TempDir(), "work")

	const filed = "https://x/1"
	const bare = "https://x/2"
	b, _ := st.EnsureAnchorBucket(ctx, BucketKindItem, filed, "one")
	n, _ := st.CreateNote(ctx, "a", "")
	tk, _ := st.CreateTask(ctx, "b", time.Time{})
	if err := st.AddMember(ctx, b.ID, n.ID, kindNote); err != nil {
		t.Fatal(err)
	}
	if err := st.AddMember(ctx, b.ID, tk.ID, kindTask); err != nil {
		t.Fatal(err)
	}

	counts, err := st.AnchorCounts(ctx, BucketKindItem, []string{filed, bare, "", filed})
	if err != nil {
		t.Fatal(err)
	}
	if counts[filed] != 2 {
		t.Fatalf("counts[%s] = %d, want 2", filed, counts[filed])
	}
	if _, ok := counts[bare]; ok {
		t.Fatalf("counts included an unfiled anchor: %v", counts)
	}
}

func TestAnchorCountsSumsAcrossDuplicateAnchorBuckets(t *testing.T) {
	ctx := context.Background()
	st := openStore(t, t.TempDir(), "work")
	const url = "https://x/1"

	first, _ := st.CreateBucket(ctx, "one", BucketKindItem, url)
	second, _ := st.CreateBucket(ctx, "two", BucketKindItem, url)
	a, _ := st.CreateNote(ctx, "a", "")
	b, _ := st.CreateNote(ctx, "b", "")
	if err := st.AddMember(ctx, first.ID, a.ID, kindNote); err != nil {
		t.Fatal(err)
	}
	if err := st.AddMember(ctx, second.ID, b.ID, kindNote); err != nil {
		t.Fatal(err)
	}

	counts, err := st.AnchorCounts(ctx, BucketKindItem, []string{url})
	if err != nil || counts[url] != 2 {
		t.Fatalf("counts[%s] = %d err=%v, want 2", url, counts[url], err)
	}

	got, ok, err := st.BucketByAnchor(ctx, BucketKindItem, url)
	if err != nil || !ok || got.ID != first.ID {
		t.Fatalf("BucketByAnchor = %+v ok=%v err=%v, want id %d", got, ok, err, first.ID)
	}
}

func TestAnchorCountsChunksWideInput(t *testing.T) {
	ctx := context.Background()
	st := openStore(t, t.TempDir(), "work")

	anchors := make([]string, 0, anchorCountChunk*2+5)
	for i := range anchorCountChunk*2 + 5 {
		anchors = append(anchors, "https://x/"+strings.Repeat("0", i%3)+string(rune('a'+i%26))+RunAnchor(int64(i)))
	}
	b, _ := st.EnsureAnchorBucket(ctx, BucketKindItem, anchors[len(anchors)-1], "last")
	n, _ := st.CreateNote(ctx, "a", "")
	if err := st.AddMember(ctx, b.ID, n.ID, kindNote); err != nil {
		t.Fatal(err)
	}

	counts, err := st.AnchorCounts(ctx, BucketKindItem, anchors)
	if err != nil {
		t.Fatal(err)
	}
	if counts[anchors[len(anchors)-1]] != 1 {
		t.Fatalf("counts = %v, want the last anchor to survive chunking", counts)
	}
}

func TestBucketAndRecordIDsNeverCollide(t *testing.T) {
	ctx := context.Background()
	st := openStore(t, t.TempDir(), "work")

	seen := make(map[int64]string)
	add := func(id int64, what string) {
		t.Helper()
		if prev, ok := seen[id]; ok {
			t.Fatalf("id %d used by both %s and %s", id, prev, what)
		}
		seen[id] = what
	}
	for i := range 3 {
		b, err := st.CreateBucket(ctx, "b", BucketKindUser, "")
		if err != nil {
			t.Fatal(err)
		}
		add(b.ID, "bucket")
		n, err := st.CreateNote(ctx, "n", "")
		if err != nil {
			t.Fatal(err)
		}
		add(n.ID, "note")
		tk, err := st.CreateTask(ctx, "t", time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		add(tk.ID, "task")
		rm, err := st.CreateReminder(ctx, "r", time.Now().Add(time.Duration(i+1)*time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		add(rm.ID, "reminder")
	}
}

func TestRecordKindResolvesEachTable(t *testing.T) {
	ctx := context.Background()
	st := openStore(t, t.TempDir(), "work")

	n, _ := st.CreateNote(ctx, "n", "")
	tk, _ := st.CreateTask(ctx, "t", time.Time{})
	rm, _ := st.CreateReminder(ctx, "r", time.Now().Add(time.Hour))
	for id, want := range map[int64]string{n.ID: kindNote, tk.ID: kindTask, rm.ID: kindReminder} {
		got, ok, err := st.recordKind(ctx, id)
		if err != nil || !ok || got != want {
			t.Fatalf("recordKind(%d) = %q ok=%v err=%v, want %q", id, got, ok, err, want)
		}
	}
	if _, ok, err := st.recordKind(ctx, 9999); ok || err != nil {
		t.Fatalf("recordKind(unknown) = ok=%v err=%v, want absent", ok, err)
	}
}

func TestFiledCountsAndRunFiledCount(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	st := openStore(t, home, "work")

	const url = "https://github.com/o/r/pull/7"
	item, _ := st.EnsureAnchorBucket(ctx, BucketKindItem, url, "PR #7")
	run, _ := st.EnsureAnchorBucket(ctx, BucketKindRun, RunAnchor(184), "run #184")
	n, _ := st.CreateNote(ctx, "a", "")
	tk, _ := st.CreateTask(ctx, "b", time.Time{})
	if err := st.AddMember(ctx, item.ID, n.ID, kindNote); err != nil {
		t.Fatal(err)
	}
	if err := st.AddMember(ctx, run.ID, tk.ID, kindTask); err != nil {
		t.Fatal(err)
	}

	counts, err := FiledCounts(home, "work", []string{url, "https://elsewhere"})
	if err != nil {
		t.Fatal(err)
	}
	if counts[url] != 1 || len(counts) != 1 {
		t.Fatalf("FiledCounts = %v, want only %s at 1", counts, url)
	}

	if _, ok := counts[RunAnchor(184)]; ok {
		t.Fatalf("FiledCounts leaked a run anchor: %v", counts)
	}

	n184, err := RunFiledCount(home, "work", 184)
	if err != nil || n184 != 1 {
		t.Fatalf("RunFiledCount(184) = %d err=%v, want 1", n184, err)
	}
	n0, err := RunFiledCount(home, "work", 999)
	if err != nil || n0 != 0 {
		t.Fatalf("RunFiledCount(999) = %d err=%v, want 0", n0, err)
	}
}
