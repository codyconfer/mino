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
