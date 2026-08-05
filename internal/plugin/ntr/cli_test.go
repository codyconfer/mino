package ntr

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/codyconfer/viewkit/ui"
)

func TestCLINotesRoundTrip(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	var buf bytes.Buffer
	if err := CLINotesAdd(ctx, &buf, ui.Default(), home, "r", "hello", "body"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "created") {
		t.Fatalf("add out = %q", buf.String())
	}
	buf.Reset()
	if err := CLINotesList(ctx, &buf, ui.Default(), home, "r"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "hello") {
		t.Fatalf("list = %q", buf.String())
	}
	id := strings.Fields(buf.String())[0]
	buf.Reset()
	if err := CLINotesUpdate(ctx, &buf, ui.Default(), home, "r", id, "hi", "new body"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "updated") {
		t.Fatalf("update out = %q", buf.String())
	}
	buf.Reset()
	if err := CLINotesList(ctx, &buf, ui.Default(), home, "r"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "hi") || strings.Contains(buf.String(), "hello") {
		t.Fatalf("list after update = %q", buf.String())
	}
	buf.Reset()
	if err := CLINotesRM(ctx, &buf, ui.Default(), home, "r", id); err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	if err := CLINotesList(ctx, &buf, ui.Default(), home, "r"); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 0 {
		t.Fatalf("list after rm = %q", buf.String())
	}
}

func TestCLITasksCRUD(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	var buf bytes.Buffer
	if err := CLITasksAdd(ctx, &buf, ui.Default(), home, "r", "ship"); err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	if err := CLITasksList(ctx, &buf, ui.Default(), home, "r"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "ship") {
		t.Fatalf("list = %q", buf.String())
	}
	id := strings.Fields(buf.String())[0]

	buf.Reset()
	if err := CLITasksDone(ctx, &buf, ui.Default(), home, "r", id); err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	if err := CLITasksList(ctx, &buf, ui.Default(), home, "r"); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 0 {
		t.Fatalf("open list after done = %q", buf.String())
	}

	buf.Reset()
	if err := CLITasksUndo(ctx, &buf, ui.Default(), home, "r", id); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "reopened") {
		t.Fatalf("undo out = %q", buf.String())
	}
	buf.Reset()
	if err := CLITasksList(ctx, &buf, ui.Default(), home, "r"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "ship") {
		t.Fatalf("list after undo = %q", buf.String())
	}

	buf.Reset()
	if err := CLITasksRM(ctx, &buf, ui.Default(), home, "r", id); err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	if err := CLITasksList(ctx, &buf, ui.Default(), home, "r"); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 0 {
		t.Fatalf("list after rm = %q", buf.String())
	}
}

func TestCLIRemindDone(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	var buf bytes.Buffer
	if err := CLIRemindAdd(ctx, &buf, ui.Default(), home, "r", "ping", "1h"); err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	if err := CLIRemindList(ctx, &buf, ui.Default(), home, "r"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "ping") {
		t.Fatalf("list = %q", buf.String())
	}
	id := strings.Fields(buf.String())[0]
	buf.Reset()
	if err := CLIRemindDone(ctx, &buf, ui.Default(), home, "r", id); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "done") {
		t.Fatalf("done out = %q", buf.String())
	}
	buf.Reset()
	if err := CLIRemindList(ctx, &buf, ui.Default(), home, "r"); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 0 {
		t.Fatalf("open list after done = %q", buf.String())
	}
}

func TestCLICatchUpAck(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	st, err := Open(ctx, home, "r")
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.CreateReminder(ctx, "due", time.Now().UTC().Add(-time.Minute))
	st.Close()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := CLICatchUp(ctx, &buf, ui.Default(), home, "r"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "fired 1") {
		t.Fatalf("catch-up = %q", buf.String())
	}
	st, err = Open(ctx, home, "r")
	if err != nil {
		t.Fatal(err)
	}
	due, err := st.DueReminders(ctx, time.Now().UTC())
	st.Close()
	if err != nil || len(due) != 0 {
		t.Fatalf("due after catch-up = %v err=%v", due, err)
	}
}

func TestCLIBucketsRoundTrip(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	var out bytes.Buffer
	scope := testScope()

	if err := CLIBucketsAdd(ctx, &out, scope, home, "r", "escalations"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "created") {
		t.Fatalf("add output = %q, want a created notice", out.String())
	}

	out.Reset()
	if err := CLIBucketsList(ctx, &out, scope, home, "r"); err != nil {
		t.Fatal(err)
	}
	line := strings.TrimSpace(out.String())
	if !strings.Contains(line, "escalations") || !strings.Contains(line, "user") {
		t.Fatalf("list output = %q, want the user bucket", line)
	}
	id := strings.Split(line, "\t")[0]

	out.Reset()
	if err := CLIBucketsRename(ctx, &out, scope, home, "r", id, "pages"); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := CLIBucketsList(ctx, &out, scope, home, "r"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "pages") {
		t.Fatalf("list output = %q, want the renamed bucket", out.String())
	}

	out.Reset()
	if err := CLINotesAdd(ctx, &out, scope, home, "r", "a note", ""); err != nil {
		t.Fatal(err)
	}
	st := openStore(t, home, "r")
	notes, _ := st.ListNotes(ctx)
	noteID := strconv.FormatInt(notes[0].ID, 10)

	out.Reset()
	if err := CLIBucketsFile(ctx, &out, scope, home, "r", id, noteID); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "note") {
		t.Fatalf("file output = %q, want the inferred kind", out.String())
	}

	out.Reset()
	if err := CLIBucketsShow(ctx, &out, scope, home, "r", id); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "a note") {
		t.Fatalf("show output = %q, want the member listed", out.String())
	}

	out.Reset()
	if err := CLIBucketsFor(ctx, &out, scope, home, "r", noteID); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "pages") {
		t.Fatalf("for output = %q, want the bucket listed", out.String())
	}

	out.Reset()
	if err := CLIBucketsUnfile(ctx, &out, scope, home, "r", id, noteID); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "kept") {
		t.Fatalf("unfile output = %q, want it to say the record is kept", out.String())
	}

	out.Reset()
	if err := CLIBucketsRM(ctx, &out, scope, home, "r", id); err != nil {
		t.Fatal(err)
	}
	if notes, _ := st.ListNotes(ctx); len(notes) != 1 {
		t.Fatalf("ListNotes = %v, want the note to outlive the bucket", notes)
	}
}

func TestCLIAddInFilesAndPlainAddDoesNot(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	var out bytes.Buffer
	scope := testScope()
	st := openStore(t, home, "r")
	b, err := st.CreateBucket(ctx, "shift", BucketKindUser, "")
	if err != nil {
		t.Fatal(err)
	}
	id := strconv.FormatInt(b.ID, 10)

	if err := CLINotesAddIn(ctx, &out, scope, home, "r", "filed note", "", id); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "filed into bucket") {
		t.Fatalf("output = %q, want the filing reported", out.String())
	}
	if err := CLITasksAddIn(ctx, &out, scope, home, "r", "filed task", id); err != nil {
		t.Fatal(err)
	}
	if err := CLIRemindAddIn(ctx, &out, scope, home, "r", "filed reminder", "10m", id); err != nil {
		t.Fatal(err)
	}
	recs, err := st.bucketRecords(ctx, b.ID)
	if err != nil || len(recs) != 3 {
		t.Fatalf("bucketRecords = %v err=%v, want all three filed", recs, err)
	}

	if err := CLINotesAdd(ctx, &out, scope, home, "r", "loose note", ""); err != nil {
		t.Fatal(err)
	}
	recs, _ = st.bucketRecords(ctx, b.ID)
	if len(recs) != 3 {
		t.Fatalf("bucketRecords = %v, want the plain add to file nothing", recs)
	}
}

func TestCLIBucketsRejectsBadInput(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	var out bytes.Buffer
	scope := testScope()

	if err := CLIBucketsAdd(ctx, &out, scope, home, "r", "   "); err == nil {
		t.Error("CLIBucketsAdd accepted a blank name")
	}
	if err := CLIBucketsShow(ctx, &out, scope, home, "r", "9999"); err == nil {
		t.Error("CLIBucketsShow accepted an unknown bucket")
	}
	if err := CLIBucketsFile(ctx, &out, scope, home, "r", "1", "notanumber"); err == nil {
		t.Error("CLIBucketsFile accepted a non-numeric record id")
	}
	if err := CLINotesAddIn(ctx, &out, scope, home, "r", "t", "", "notanumber"); err == nil {
		t.Error("CLINotesAddIn accepted a non-numeric bucket")
	}
}
