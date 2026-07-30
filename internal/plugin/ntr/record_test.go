package ntr

import (
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func recordNow(t *testing.T) time.Time {
	t.Helper()
	now, err := time.Parse(time.RFC3339, "2026-07-29T16:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	return now
}

func TestRecordTextPerKind(t *testing.T) {
	now := recordNow(t)
	due := now.Add(2 * time.Hour)

	note := record{Kind: kindNote, ID: 12, Title: "idea", Body: "the body"}
	if got, want := note.text(), "idea\n\nthe body"; got != want {
		t.Errorf("note text = %q want %q", got, want)
	}
	if got, want := note.label(), "note #12"; got != want {
		t.Errorf("note label = %q want %q", got, want)
	}

	bare := record{Kind: kindNote, Title: "idea"}
	if got, want := bare.text(), "idea"; got != want {
		t.Errorf("empty body note text = %q want %q", got, want)
	}
	if got, want := bare.label(), "note #0"; got != want {
		t.Errorf("unsaved label = %q want %q", got, want)
	}

	task := record{Kind: kindTask, ID: 3, Title: "ship it", Done: true, Due: due}
	if got, want := task.text(), "[x] ship it  (due 2026-07-29 18:00Z)"; got != want {
		t.Errorf("task text = %q want %q", got, want)
	}
	if got, want := task.label(), "task #3"; got != want {
		t.Errorf("task label = %q want %q", got, want)
	}

	noDue := record{Kind: kindTask, ID: 4, Title: "someday"}
	if got, want := noDue.text(), "[ ] someday"; got != want {
		t.Errorf("no-due task text = %q want %q", got, want)
	}

	rem := record{Kind: kindReminder, ID: 8, Title: "ping bob", Due: due}
	if got, want := rem.text(), "ping bob  (due 2026-07-29 18:00Z)"; got != want {
		t.Errorf("reminder text = %q want %q", got, want)
	}
	if got, want := rem.label(), "reminder #8"; got != want {
		t.Errorf("reminder label = %q want %q", got, want)
	}
}

func TestRecordSummaryUnsavedDraft(t *testing.T) {
	if got, want := (record{}).summary(), "unsaved draft"; got != want {
		t.Errorf("empty summary = %q want %q", got, want)
	}
	if got, want := (record{Kind: kindTask}).summary(), "unsaved draft"; got != want {
		t.Errorf("kind-only summary = %q want %q", got, want)
	}

	now := recordNow(t)
	rec := record{Kind: kindTask, ID: 3, Title: "ship it", Due: now.Add(2 * time.Hour), Done: true}
	if got, want := rec.summary(), "title=ship it  due=2026-07-29 18:00Z  done"; got != want {
		t.Errorf("task summary = %q want %q", got, want)
	}

	note := record{Kind: kindNote, Title: "idea", Body: "abc"}
	if got, want := note.summary(), "title=idea  body=3 chars"; got != want {
		t.Errorf("note summary = %q want %q", got, want)
	}
}

func TestRecordPreviewYAML(t *testing.T) {
	now := recordNow(t)

	minimal := record{Kind: kindTask, Title: "ship it"}
	out, err := yaml.Marshal(minimal.preview())
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	for _, want := range []string{"kind: task", "title: ship it"} {
		if !strings.Contains(got, want) {
			t.Errorf("yaml %q missing %q", got, want)
		}
	}
	for _, unwanted := range []string{"id:", "body:", "due:", "done:"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("yaml %q should omit %q", got, unwanted)
		}
	}

	full := record{Kind: kindReminder, ID: 8, Title: "ping", Due: now.Add(2 * time.Hour), Done: true}
	out, err = yaml.Marshal(full.preview())
	if err != nil {
		t.Fatal(err)
	}
	got = string(out)
	for _, want := range []string{"kind: reminder", "id: 8", "due: \"2026-07-29T18:00:00Z\"", "done: true"} {
		if !strings.Contains(got, want) {
			t.Errorf("yaml %q missing %q", got, want)
		}
	}

	note := record{Kind: kindNote, ID: 12, Title: "idea", Body: "the body"}
	out, err = yaml.Marshal(note.preview())
	if err != nil {
		t.Fatal(err)
	}
	if got := string(out); !strings.Contains(got, "body: the body") {
		t.Errorf("note yaml %q missing body", got)
	}
}

func TestRecordCheckNeverEmpty(t *testing.T) {
	now := recordNow(t)
	recs := []record{
		{},
		{Kind: kindNote},
		{Kind: kindNote, ID: 1, Title: "idea", Body: "b"},
		{Kind: kindTask},
		{Kind: kindTask, ID: 2, Title: "ship", Due: now.Add(time.Hour), Done: true},
		{Kind: kindReminder},
		{Kind: kindReminder, ID: 3, Title: "ping", Due: now.Add(time.Hour)},
	}
	for _, rec := range recs {
		lines := rec.check(now)
		if len(lines) == 0 {
			t.Fatalf("check(%+v) returned no lines", rec)
		}
		if strings.TrimSpace(lines[0]) == "" {
			t.Errorf("check(%+v) first line blank", rec)
		}
	}
}

func TestRecordCheckReminderRules(t *testing.T) {
	now := recordNow(t)

	noDue := record{Kind: kindReminder, Title: "ping"}.check(now)
	joined := strings.Join(noDue, "\n")
	if !strings.Contains(joined, "no due") || !strings.Contains(joined, "never fires") {
		t.Errorf("no-due reminder check = %q", joined)
	}
	if !strings.Contains(joined, "will not work") {
		t.Errorf("no-due reminder should fail: %q", joined)
	}

	past := record{Kind: kindReminder, Title: "ping", Due: now.Add(-2 * time.Hour)}.check(now)
	joined = strings.Join(past, "\n")
	if !strings.Contains(joined, "2h ago") {
		t.Errorf("past reminder check missing relative time: %q", joined)
	}
	if !strings.Contains(joined, "next poll") || !strings.Contains(joined, "needs a look") {
		t.Errorf("past reminder should warn: %q", joined)
	}

	done := record{Kind: kindReminder, Title: "ping", Due: now.Add(2 * time.Hour), Done: true}.check(now)
	joined = strings.Join(done, "\n")
	if !strings.Contains(joined, "already done") || !strings.Contains(joined, "will not fire") {
		t.Errorf("done reminder check = %q", joined)
	}
	if !strings.Contains(joined, "needs a look") {
		t.Errorf("done reminder should warn: %q", joined)
	}

	future := record{Kind: kindReminder, Title: "ping", Due: now.Add(2 * time.Hour)}.check(now)
	joined = strings.Join(future, "\n")
	if !strings.Contains(joined, "fires 2026-07-29 18:00Z") || !strings.Contains(joined, "in 2h") {
		t.Errorf("future reminder check = %q", joined)
	}
	if !strings.Contains(joined, "looks good") {
		t.Errorf("future reminder should pass: %q", joined)
	}
}

func TestRecordCheckTaskDueOptional(t *testing.T) {
	now := recordNow(t)

	noDue := strings.Join(record{Kind: kindTask, Title: "someday"}.check(now), "\n")
	if !strings.Contains(noDue, "looks good") {
		t.Errorf("task without due should pass: %q", noDue)
	}
	if !strings.Contains(noDue, "no due") {
		t.Errorf("task without due check = %q", noDue)
	}

	withDue := strings.Join(record{Kind: kindTask, Title: "ship", Due: now.Add(2 * time.Hour)}.check(now), "\n")
	if !strings.Contains(withDue, "due 2026-07-29 18:00Z") || !strings.Contains(withDue, "in 2h") {
		t.Errorf("task with due check = %q", withDue)
	}
}

func TestRecordCheckNoteIgnoresDue(t *testing.T) {
	now := recordNow(t)
	lines := strings.Join(record{Kind: kindNote, Title: "idea", Due: now.Add(-time.Hour)}.check(now), "\n")
	if strings.Contains(lines, "due") {
		t.Errorf("note check should say nothing about due: %q", lines)
	}
}

func TestParseDueAt(t *testing.T) {
	now := recordNow(t)

	blank, err := parseDueAt("   ", now)
	if err != nil {
		t.Fatalf("blank due: %v", err)
	}
	if want := now.UTC().Add(time.Hour); !blank.Equal(want) {
		t.Errorf("blank due = %v want %v", blank, want)
	}

	rel, err := parseDueAt("+90m", now)
	if err != nil {
		t.Fatalf("relative due: %v", err)
	}
	if want := now.UTC().Add(90 * time.Minute); !rel.Equal(want) {
		t.Errorf("relative due = %v want %v", rel, want)
	}

	abs, err := parseDueAt("2026-08-01T09:00:00Z", now)
	if err != nil {
		t.Fatalf("absolute due: %v", err)
	}
	if want := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC); !abs.Equal(want) {
		t.Errorf("absolute due = %v want %v", abs, want)
	}

	if _, err := parseDueAt("not a time", now); err == nil {
		t.Fatal("bad due: want error")
	} else if !strings.HasPrefix(err.Error(), "due: ") {
		t.Errorf("bad due error = %q want due: prefix", err.Error())
	}
}
