package ntr

import (
	"context"
	"fmt"
	"testing"

	"github.com/codyconfer/munin/internal/plugin"
	"github.com/codyconfer/munin/internal/testenv"
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
