package ntr

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/codyconfer/mino/internal/plugin"
	"github.com/codyconfer/mino/internal/testenv"
)

func TestRemindActionsServiceOnly(t *testing.T) {
	for _, name := range []string{"remind.add", "remind.done"} {
		d, ok := plugin.ByKind(plugin.KindAction, SignalName+"/"+name)
		if !ok {
			t.Fatalf("missing action descriptor %s", name)
		}
		if !d.ServiceOnly {
			t.Fatalf("%s ServiceOnly = false, want true", name)
		}
	}
	for _, name := range []string{"note.add", "task.add"} {
		d, ok := plugin.ByKind(plugin.KindAction, SignalName+"/"+name)
		if !ok {
			t.Fatalf("missing action descriptor %s", name)
		}
		if d.ServiceOnly {
			t.Fatalf("%s unexpectedly ServiceOnly", name)
		}
	}
}

func TestCapActionCRUDParity(t *testing.T) {
	testenv.Isolate(t)
	plugin.LoadEnabled()

	want := []string{
		"bucket.add", "bucket.file", "bucket.rename", "bucket.rm", "bucket.unfile",
		"note.add", "note.rm", "note.update",
		"remind.add", "remind.done",
		"task.add", "task.done", "task.rm", "task.undo",
	}
	got := plugin.ActionsFor(SignalName)
	if len(got) != len(want) {
		names := make([]string, len(got))
		for i, a := range got {
			names[i] = a.Name
		}
		t.Fatalf("ActionsFor(ntr) = %v, want %v", names, want)
	}
	for i, a := range got {
		if a.Name != want[i] {
			t.Fatalf("action[%d] = %q, want %q", i, a.Name, want[i])
		}
	}

	ctx := context.Background()
	home := t.TempDir()
	role := "work"
	base := map[string]string{"home": home, "role": role}

	run := func(name string, extra map[string]string) {
		t.Helper()
		params := map[string]string{}
		for k, v := range base {
			params[k] = v
		}
		for k, v := range extra {
			params[k] = v
		}
		if err := plugin.RunAction(ctx, SignalName, name, params); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}

	run("note.add", map[string]string{"title": "idea", "body": "v1"})
	st, err := Open(ctx, home, role)
	if err != nil {
		t.Fatal(err)
	}
	notes, err := st.ListNotes(ctx)
	if err != nil || len(notes) != 1 {
		st.Close()
		t.Fatalf("notes after add = %v err=%v", notes, err)
	}
	noteID := notes[0].ID
	st.Close()

	run("note.update", map[string]string{"id": fmt.Sprint(noteID), "title": "idea2", "body": "v2"})
	st, err = Open(ctx, home, role)
	if err != nil {
		t.Fatal(err)
	}
	notes, err = st.ListNotes(ctx)
	if err != nil || len(notes) != 1 || notes[0].Title != "idea2" || notes[0].Body != "v2" {
		st.Close()
		t.Fatalf("notes after update = %+v err=%v", notes, err)
	}
	st.Close()

	run("task.add", map[string]string{"title": "ship"})
	st, err = Open(ctx, home, role)
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := st.ListTasks(ctx, true)
	if err != nil || len(tasks) != 1 {
		st.Close()
		t.Fatalf("tasks after add = %v err=%v", tasks, err)
	}
	taskID := tasks[0].ID
	st.Close()

	run("task.done", map[string]string{"id": fmt.Sprint(taskID)})
	run("task.undo", map[string]string{"id": fmt.Sprint(taskID)})
	st, err = Open(ctx, home, role)
	if err != nil {
		t.Fatal(err)
	}
	tasks, err = st.ListTasks(ctx, false)
	if err != nil || len(tasks) != 1 || tasks[0].Done {
		st.Close()
		t.Fatalf("tasks after undo = %+v err=%v", tasks, err)
	}
	st.Close()

	run("remind.add", map[string]string{"title": "ping", "in": "1h"})
	st, err = Open(ctx, home, role)
	if err != nil {
		t.Fatal(err)
	}
	rems, err := st.ListReminders(ctx, false)
	if err != nil || len(rems) != 1 {
		st.Close()
		t.Fatalf("reminders after add = %v err=%v", rems, err)
	}
	remID := rems[0].ID
	st.Close()

	run("remind.done", map[string]string{"id": fmt.Sprint(remID)})
	st, err = Open(ctx, home, role)
	if err != nil {
		t.Fatal(err)
	}
	rems, err = st.ListReminders(ctx, false)
	st.Close()
	if err != nil || len(rems) != 0 {
		t.Fatalf("open reminders after done = %v err=%v", rems, err)
	}

	run("task.rm", map[string]string{"id": fmt.Sprint(taskID)})
	run("note.rm", map[string]string{"id": fmt.Sprint(noteID)})
	st, err = Open(ctx, home, role)
	if err != nil {
		t.Fatal(err)
	}
	notes, err = st.ListNotes(ctx)
	tasks, err2 := st.ListTasks(ctx, true)
	st.Close()
	if err != nil || err2 != nil || len(notes) != 0 || len(tasks) != 0 {
		t.Fatalf("after rm notes=%v tasks=%v err=%v/%v", notes, tasks, err, err2)
	}
}

func bucketAction(t *testing.T, name string, params map[string]string) error {
	t.Helper()
	if _, ok := plugin.LookupAction(SignalName, name); !ok {
		t.Fatalf("no action %s", name)
	}
	return plugin.RunAction(context.Background(), SignalName, name, params)
}

func TestBucketActionsRoundTrip(t *testing.T) {
	testenv.Isolate(t)
	plugin.LoadEnabled()
	ctx := context.Background()
	home := t.TempDir()
	base := func(extra map[string]string) map[string]string {
		p := map[string]string{"home": home, "role": "work"}
		for k, v := range extra {
			p[k] = v
		}
		return p
	}
	st := openStore(t, home, "work")

	if err := bucketAction(t, "bucket.add", base(map[string]string{"name": "escalations"})); err != nil {
		t.Fatal(err)
	}
	bs, _ := st.ListBuckets(ctx)
	if len(bs) != 1 || bs[0].Name != "escalations" || bs[0].Kind != BucketKindUser {
		t.Fatalf("ListBuckets = %v, want one user bucket", bs)
	}
	id := strconv.FormatInt(bs[0].ID, 10)

	if err := bucketAction(t, "bucket.rename", base(map[string]string{"id": id, "name": "pages"})); err != nil {
		t.Fatal(err)
	}
	got, _, _ := st.Bucket(ctx, bs[0].ID)
	if got.Name != "pages" {
		t.Fatalf("name = %q, want pages", got.Name)
	}

	if err := bucketAction(t, "note.add", base(map[string]string{"title": "a note"})); err != nil {
		t.Fatal(err)
	}
	notes, _ := st.ListNotes(ctx)
	noteID := strconv.FormatInt(notes[0].ID, 10)

	if err := bucketAction(t, "bucket.file", base(map[string]string{"id": noteID, "bucket": id})); err != nil {
		t.Fatal(err)
	}
	recs, _ := st.bucketRecords(ctx, bs[0].ID)
	if len(recs) != 1 {
		t.Fatalf("bucketRecords = %v, want the note filed", recs)
	}

	if err := bucketAction(t, "bucket.unfile", base(map[string]string{"id": noteID, "bucket": id})); err != nil {
		t.Fatal(err)
	}
	recs, _ = st.bucketRecords(ctx, bs[0].ID)
	if len(recs) != 0 {
		t.Fatalf("bucketRecords = %v, want it unfiled", recs)
	}

	if err := bucketAction(t, "bucket.rm", base(map[string]string{"id": id})); err != nil {
		t.Fatal(err)
	}
	bs, _ = st.ListBuckets(ctx)
	if len(bs) != 0 {
		t.Fatalf("ListBuckets = %v, want none", bs)
	}
	if notes, _ := st.ListNotes(ctx); len(notes) != 1 {
		t.Fatalf("ListNotes = %v, want the note to outlive its bucket", notes)
	}
}

func TestBucketFileEnsuresAnAnchorBucket(t *testing.T) {
	testenv.Isolate(t)
	plugin.LoadEnabled()
	ctx := context.Background()
	home := t.TempDir()
	st := openStore(t, home, "work")
	n, err := st.CreateNote(ctx, "about the PR", "")
	if err != nil {
		t.Fatal(err)
	}
	const url = "https://github.com/o/r/pull/7"

	params := map[string]string{
		"home": home, "role": "work",
		"id": strconv.FormatInt(n.ID, 10), "anchor": url, "name": "PR #7",
	}
	if err := bucketAction(t, "bucket.file", params); err != nil {
		t.Fatal(err)
	}
	b, ok, err := st.BucketByAnchor(ctx, BucketKindItem, url)
	if err != nil || !ok {
		t.Fatalf("BucketByAnchor = %+v ok=%v err=%v, want it ensured", b, ok, err)
	}
	if b.Name != "PR #7" || b.Kind != BucketKindItem {
		t.Fatalf("bucket = %+v, want an item bucket named PR #7", b)
	}
	counts, _ := st.AnchorCounts(ctx, BucketKindItem, []string{url})
	if counts[url] != 1 {
		t.Fatalf("AnchorCounts = %v, want 1", counts)
	}

	if err := bucketAction(t, "bucket.file", params); err != nil {
		t.Fatalf("filing twice = %v, want a no-op", err)
	}
	if bs, _ := st.ListBuckets(ctx); len(bs) != 1 {
		t.Fatalf("ListBuckets = %v, want the anchor bucket reused", bs)
	}
}

func TestBucketFileAcceptsARunAnchor(t *testing.T) {
	testenv.Isolate(t)
	plugin.LoadEnabled()
	ctx := context.Background()
	home := t.TempDir()
	st := openStore(t, home, "work")
	tk, _ := st.CreateTask(ctx, "follow up", time.Time{})

	err := bucketAction(t, "bucket.file", map[string]string{
		"home": home, "role": "work",
		"id": strconv.FormatInt(tk.ID, 10), "anchor": RunAnchor(184), "anchor_kind": BucketKindRun,
	})
	if err != nil {
		t.Fatal(err)
	}
	n, err := RunFiledCount(home, "work", 184)
	if err != nil || n != 1 {
		t.Fatalf("RunFiledCount = %d err=%v, want 1", n, err)
	}
}

func TestBucketFileRejectsBadInput(t *testing.T) {
	testenv.Isolate(t)
	plugin.LoadEnabled()
	home := t.TempDir()
	st := openStore(t, home, "work")
	n, _ := st.CreateNote(context.Background(), "a", "")
	id := strconv.FormatInt(n.ID, 10)

	cases := map[string]map[string]string{
		"no bucket or anchor": {"home": home, "role": "work", "id": id},
		"unknown record":      {"home": home, "role": "work", "id": "9999", "anchor": "https://x/1"},
		"bad anchor_kind":     {"home": home, "role": "work", "id": id, "anchor": "https://x/1", "anchor_kind": "nope"},
		"no home":             {"role": "work", "id": id, "anchor": "https://x/1"},
		"no id":               {"home": home, "role": "work", "anchor": "https://x/1"},
	}
	for name, params := range cases {
		if err := bucketAction(t, "bucket.file", params); err == nil {
			t.Errorf("%s: bucket.file succeeded, want an error", name)
		}
	}
}

func TestAddActionsFileWhenGivenABucket(t *testing.T) {
	testenv.Isolate(t)
	plugin.LoadEnabled()
	ctx := context.Background()
	home := t.TempDir()
	st := openStore(t, home, "work")
	b, err := st.CreateBucket(ctx, "shift", BucketKindUser, "")
	if err != nil {
		t.Fatal(err)
	}
	bucket := strconv.FormatInt(b.ID, 10)

	if err := bucketAction(t, "note.add", map[string]string{
		"home": home, "role": "work", "title": "a note", "bucket": bucket,
	}); err != nil {
		t.Fatal(err)
	}
	if err := bucketAction(t, "task.add", map[string]string{
		"home": home, "role": "work", "title": "a task", "bucket": bucket,
	}); err != nil {
		t.Fatal(err)
	}
	if err := bucketAction(t, "remind.add", map[string]string{
		"home": home, "role": "work", "title": "a reminder", "in": "10m", "bucket": bucket,
	}); err != nil {
		t.Fatal(err)
	}

	recs, err := st.bucketRecords(ctx, b.ID)
	if err != nil || len(recs) != 3 {
		t.Fatalf("bucketRecords = %v err=%v, want all three filed", recs, err)
	}
}

func TestAddActionsWithoutABucketFileNothing(t *testing.T) {
	testenv.Isolate(t)
	plugin.LoadEnabled()
	ctx := context.Background()
	home := t.TempDir()
	st := openStore(t, home, "work")

	if err := bucketAction(t, "note.add", map[string]string{
		"home": home, "role": "work", "title": "a note",
	}); err != nil {
		t.Fatal(err)
	}
	if bs, _ := st.ListBuckets(ctx); len(bs) != 0 {
		t.Fatalf("ListBuckets = %v, want none from a plain note.add", bs)
	}
}

func TestAddActionsAcceptAnAnchor(t *testing.T) {
	testenv.Isolate(t)
	plugin.LoadEnabled()
	home := t.TempDir()
	openStore(t, home, "work")
	const url = "https://github.com/o/r/pull/9"

	if err := bucketAction(t, "note.add", map[string]string{
		"home": home, "role": "work", "title": "about it", "anchor": url, "name": "PR #9",
	}); err != nil {
		t.Fatal(err)
	}
	counts, err := FiledCounts(home, "work", []string{url})
	if err != nil || counts[url] != 1 {
		t.Fatalf("FiledCounts = %v err=%v, want 1", counts, err)
	}
}

func TestBucketActionsAreNotServiceOnly(t *testing.T) {
	testenv.Isolate(t)
	plugin.LoadEnabled()
	for _, name := range []string{"bucket.add", "bucket.rename", "bucket.rm", "bucket.file", "bucket.unfile"} {
		d, ok := plugin.ByKind(plugin.KindAction, SignalName+"/"+name)
		if !ok {
			t.Fatalf("missing descriptor for %s", name)
		}
		if d.ServiceOnly {
			t.Errorf("%s is ServiceOnly; buckets must work without a daemon", name)
		}
	}
}
