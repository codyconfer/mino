package ntr

import (
	"context"
	"testing"
	"time"
)

func TestParseTimeLayouts(t *testing.T) {
	want := time.Date(2026, 7, 29, 2, 10, 54, 875310000, time.UTC)
	whole := time.Date(2026, 7, 29, 2, 10, 54, 0, time.UTC)

	cases := []struct {
		name string
		in   string
		want time.Time
	}{
		{"duckdb go string", "2026-07-29 02:10:54.87531 +0000 UTC", want},
		{"duckdb go string whole second", "2026-07-29 02:10:54 +0000 UTC", whole},
		{"go string offset only", "2026-07-29 02:10:54.87531 +0000", want},
		{"go string non utc zone", "2026-07-28 22:10:54.87531 -0400 EDT", want},
		{"rfc3339nano", "2026-07-29T02:10:54.87531Z", want},
		{"rfc3339", "2026-07-29T02:10:54Z", whole},
		{"rfc3339 with offset", "2026-07-28T22:10:54-04:00", whole},
		{"naive", "2026-07-29 02:10:54", whole},
		{"date only", "2026-07-29", time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)},
		{"monotonic suffix", "2026-07-29 02:10:54.87531 +0000 UTC m=+7200.492691676", want},
		{"padded", "  2026-07-29 02:10:54.87531 +0000 UTC  ", want},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseTime(c.in)
			if !got.Equal(c.want) {
				t.Fatalf("parseTime(%q) = %v, want %v", c.in, got, c.want)
			}
			if got.Location() != time.UTC {
				t.Errorf("parseTime(%q) location = %v, want UTC", c.in, got.Location())
			}
		})
	}
}

func TestParseTimeZeroCases(t *testing.T) {
	for _, in := range []string{"", "   ", "NULL", "null", "Null", "not-a-time", "29/07/2026"} {
		if got := parseTime(in); !got.IsZero() {
			t.Errorf("parseTime(%q) = %v, want zero", in, got)
		}
	}
}

func TestStoreRoundTripsDueThroughDuckDB(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, t.TempDir(), "duetest")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	due := time.Date(2026, 7, 29, 2, 10, 54, 875310000, time.UTC)

	if _, err := st.CreateReminder(ctx, "remind me", due); err != nil {
		t.Fatal(err)
	}
	rs, err := st.ListReminders(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 1 {
		t.Fatalf("reminders = %d, want 1", len(rs))
	}
	if rs[0].Due.IsZero() {
		t.Fatal("ListReminders returned a zero Due; parseTime failed to decode DuckDB's format")
	}
	if !rs[0].Due.Equal(due) {
		t.Errorf("reminder Due = %v, want %v", rs[0].Due, due)
	}

	if _, err := st.CreateTask(ctx, "do it", due); err != nil {
		t.Fatal(err)
	}
	ts, err := st.ListTasks(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(ts) != 1 {
		t.Fatalf("tasks = %d, want 1", len(ts))
	}
	if ts[0].Due.IsZero() {
		t.Fatal("ListTasks returned a zero Due; parseTime failed to decode DuckDB's format")
	}
	if !ts[0].Due.Equal(due) {
		t.Errorf("task Due = %v, want %v", ts[0].Due, due)
	}
}

func TestStoreKeepsZeroDueZero(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, t.TempDir(), "nodue")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if _, err := st.CreateTask(ctx, "someday", time.Time{}); err != nil {
		t.Fatal(err)
	}
	ts, err := st.ListTasks(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(ts) != 1 {
		t.Fatalf("tasks = %d, want 1", len(ts))
	}
	if !ts[0].Due.IsZero() {
		t.Errorf("task with no due decoded as %v, want zero", ts[0].Due)
	}
}
