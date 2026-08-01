package views

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/codyconfer/viewkit/keys"
	"github.com/codyconfer/viewkit/layout"
)

func TestAuditSlowQueryDoesNotBlockUpdate(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})

	me := &auditView{
		home: t.TempDir(),
		sql:  "SELECT count(*) FROM runs a, runs b",
		exec: func(string, string) auditResult {
			<-release
			return auditResult{ran: true, cols: []string{"n"}, rows: [][]string{{"1"}}}
		},
	}

	returned := make(chan tea.Cmd, 1)
	go func() { returned <- me.Update(nil, tea.KeyMsg{Type: tea.KeyEnter}) }()

	var cmd tea.Cmd
	select {
	case cmd = <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("Update did not return: the query is still running on the bubbletea goroutine")
	}
	if cmd == nil {
		t.Fatal("enter returned no command to run the query with")
	}
	if !me.running {
		t.Fatal("the view does not report a running query, so no progress can be drawn")
	}
	if body := me.Body(120, 20); !strings.Contains(body, "running") {
		t.Fatalf("body draws no running state:\n%s", body)
	}
	if got := me.Context(); len(got) < 2 || got[1] != (keys.Hint{Key: "state", Label: "running"}) {
		t.Errorf("context does not advertise the run: %v", got)
	}

	msgs := make(chan tea.Msg, 1)
	go func() { msgs <- cmd() }()
	select {
	case msg := <-msgs:
		t.Fatalf("the command finished before the store released: %T", msg)
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	var msg tea.Msg
	select {
	case msg = <-msgs:
	case <-time.After(5 * time.Second):
		t.Fatal("the query command never delivered a message")
	}
	ran, ok := msg.(auditRanMsg)
	if !ok {
		t.Fatalf("query command produced %T, want auditRanMsg", msg)
	}

	me.Update(nil, ran)
	if me.running {
		t.Error("the running flag survived the result")
	}
	if !me.result.ran || len(me.result.rows) != 1 {
		t.Fatalf("result not applied: %+v", me.result)
	}
	if out := me.results(layout.NewFrame(120)); !strings.Contains(out, "n") {
		t.Fatalf("results not rendered after the run:\n%s", out)
	}
}

func TestAuditIgnoresEnterWhileRunning(t *testing.T) {
	calls := 0
	me := &auditView{home: t.TempDir(), sql: "SELECT 1", exec: func(string, string) auditResult {
		calls++
		return auditResult{ran: true}
	}}
	first := me.run()
	if first == nil {
		t.Fatal("the first run produced no command")
	}
	if second := me.Update(nil, tea.KeyMsg{Type: tea.KeyEnter}); second != nil {
		t.Fatal("enter started a second query while one was already running")
	}
	if calls != 0 {
		t.Fatalf("the store was hit %d times before any command ran", calls)
	}
}
