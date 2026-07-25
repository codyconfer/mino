package ntr

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestCLINotesRoundTrip(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	var buf bytes.Buffer
	if err := CLINotesAdd(ctx, &buf, home, "r", "hello", "body"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "created") {
		t.Fatalf("add out = %q", buf.String())
	}
	buf.Reset()
	if err := CLINotesList(ctx, &buf, home, "r"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "hello") {
		t.Fatalf("list = %q", buf.String())
	}
	id := strings.Fields(buf.String())[0]
	buf.Reset()
	if err := CLINotesUpdate(ctx, &buf, home, "r", id, "hi", "new body"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "updated") {
		t.Fatalf("update out = %q", buf.String())
	}
	buf.Reset()
	if err := CLINotesList(ctx, &buf, home, "r"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "hi") || strings.Contains(buf.String(), "hello") {
		t.Fatalf("list after update = %q", buf.String())
	}
	buf.Reset()
	if err := CLINotesRM(ctx, &buf, home, "r", id); err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	if err := CLINotesList(ctx, &buf, home, "r"); err != nil {
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
	if err := CLITasksAdd(ctx, &buf, home, "r", "ship"); err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	if err := CLITasksList(ctx, &buf, home, "r"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "ship") {
		t.Fatalf("list = %q", buf.String())
	}
	id := strings.Fields(buf.String())[0]

	buf.Reset()
	if err := CLITasksDone(ctx, &buf, home, "r", id); err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	if err := CLITasksList(ctx, &buf, home, "r"); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 0 {
		t.Fatalf("open list after done = %q", buf.String())
	}

	buf.Reset()
	if err := CLITasksUndo(ctx, &buf, home, "r", id); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "reopened") {
		t.Fatalf("undo out = %q", buf.String())
	}
	buf.Reset()
	if err := CLITasksList(ctx, &buf, home, "r"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "ship") {
		t.Fatalf("list after undo = %q", buf.String())
	}

	buf.Reset()
	if err := CLITasksRM(ctx, &buf, home, "r", id); err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	if err := CLITasksList(ctx, &buf, home, "r"); err != nil {
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
	if err := CLIRemindAdd(ctx, &buf, home, "r", "ping", "1h"); err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	if err := CLIRemindList(ctx, &buf, home, "r"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "ping") {
		t.Fatalf("list = %q", buf.String())
	}
	id := strings.Fields(buf.String())[0]
	buf.Reset()
	if err := CLIRemindDone(ctx, &buf, home, "r", id); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "done") {
		t.Fatalf("done out = %q", buf.String())
	}
	buf.Reset()
	if err := CLIRemindList(ctx, &buf, home, "r"); err != nil {
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
	if err := CLICatchUp(ctx, &buf, home, "r"); err != nil {
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
