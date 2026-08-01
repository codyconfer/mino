package views

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/codyconfer/sisyphus/configdb"
	"github.com/codyconfer/sisyphus/kv"
	"github.com/codyconfer/viewkit/keys"
	"github.com/codyconfer/viewkit/layout"

	"github.com/codyconfer/mino/internal/audit"
	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/signals"
)

func auditRender(res auditResult) string {
	me := &auditView{result: res}
	return me.results(layout.NewFrame(120))
}

func TestAuditDefaultSQLMatchesSchemas(t *testing.T) {
	me := &auditView{}
	for i, db := range auditvDBs {
		me.dbIndex = i
		q := me.defaultSQL()
		if q == "" {
			t.Fatalf("defaultSQL(%q) empty", db)
		}
		if !auditvReadOnly(q) {
			t.Fatalf("defaultSQL(%q) not read-only: %s", db, q)
		}
	}
	if !strings.Contains(auditvDefaultSQL["audit"], "FROM runs") {
		t.Fatalf("audit default = %q", auditvDefaultSQL["audit"])
	}
	if !strings.Contains(auditvDefaultSQL["audit"], "sum(count)") {
		t.Fatalf("audit default should sum item counts: %q", auditvDefaultSQL["audit"])
	}
	if !strings.Contains(auditvDefaultSQL["config"], "FROM store_current") {
		t.Fatalf("config default = %q", auditvDefaultSQL["config"])
	}
	if !strings.Contains(auditvDefaultSQL["tokens"], "FROM kv") {
		t.Fatalf("tokens default = %q", auditvDefaultSQL["tokens"])
	}
}

func TestAuditQueryDefaultAgainstAuditDB(t *testing.T) {
	home := t.TempDir()
	st, err := audit.Open(context.Background(), config.DataPath(home, "audit.duckdb"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	start := time.Now()
	fid := st.StartFlight("morning", "triage")
	st.RecordQuery(fid, "incidents", "triage", start, time.Now(), []signals.Section{{
		Signal: "github",
		Items:  []signals.Item{{Kind: "pr", Title: "one", Timestamp: start}},
	}})
	st.FinishFlight(fid)
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	me := &auditView{home: home, dbIndex: 0}
	res := auditExec(me.dbPath(), me.defaultSQL())
	if res.err != "" {
		t.Fatalf("exec: %s", res.err)
	}
	out := auditRender(res)
	if !strings.Contains(out, "morning") && !strings.Contains(out, "incidents") {
		t.Fatalf("result missing run names: %q", out)
	}
	if !strings.Contains(out, "runs") || !strings.Contains(out, "items") {
		t.Fatalf("result missing aggregate columns: %q", out)
	}
}

func TestAuditQueryDefaultAgainstConfigDB(t *testing.T) {
	home := t.TempDir()
	db, err := configdb.Open(context.Background(), config.DataPath(home, "config.duckdb"))
	if err != nil {
		t.Fatalf("configdb.Open: %v", err)
	}
	if err := db.Import(context.Background(), "queries", []byte("name: demo\nsignal: demo\n"), "collection"); err != nil {
		t.Fatalf("Import: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	me := &auditView{home: home}
	me.dbIndex = indexOfDB("config")
	res := auditExec(me.dbPath(), me.defaultSQL())
	if res.err != "" {
		t.Fatalf("exec: %s", res.err)
	}
	if out := auditRender(res); !strings.Contains(out, "queries") {
		t.Fatalf("result missing store name: %q", out)
	}
}

func TestAuditQueryDefaultAgainstTokensDB(t *testing.T) {
	home := t.TempDir()
	store, err := kv.Open(context.Background(), config.DataPath(home, "tokens.duckdb"))
	if err != nil {
		t.Fatalf("kv.Open: %v", err)
	}
	if err := store.Put(context.Background(), "tokens", "github", "sealed", time.Time{}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	me := &auditView{home: home}
	me.dbIndex = indexOfDB("tokens")
	res := auditExec(me.dbPath(), me.defaultSQL())
	if res.err != "" {
		t.Fatalf("exec: %s", res.err)
	}
	out := auditRender(res)
	if !strings.Contains(out, "github") || !strings.Contains(out, "tokens") {
		t.Fatalf("result missing kv row: %q", out)
	}
	if strings.Contains(out, "sealed") {
		t.Fatalf("token value should not be selected: %q", out)
	}
}

func indexOfDB(name string) int {
	for i, n := range auditvDBs {
		if n == name {
			return i
		}
	}
	return 0
}

func auditAssertAligned(t *testing.T, out string) {
	t.Helper()
	lines := strings.Split(out, "\n")
	if len(lines) < 3 {
		t.Fatalf("table too short: %q", out)
	}
	want := lipgloss.Width(lines[0])
	for i, ln := range lines[:len(lines)-1] {
		if got := lipgloss.Width(ln); got != want {
			t.Fatalf("line %d width = %d, want %d (%q)", i, got, want, ln)
		}
	}
}

func TestAuditResultsRendersTable(t *testing.T) {
	out := auditRender(auditResult{
		ran:  true,
		cols: []string{"name", "kind", "runs"},
		rows: [][]string{{"morning", "triage", "3"}, {"evening", "review", "11"}},
	})
	for _, want := range []string{"name", "kind", "runs", "morning", "evening", "─┼─", " │ ", "(2 rows)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("table missing %q: %q", want, out)
		}
	}
	auditAssertAligned(t, out)
}

func TestAuditResultsWideRunesStayAligned(t *testing.T) {
	out := auditRender(auditResult{
		ran:  true,
		cols: []string{"name", "note"},
		rows: [][]string{
			{"ascii", "plain text"},
			{"日本語のながいなまえ", "レビュー待ち"},
			{"mixed 語", "tail"},
		},
	})
	auditAssertAligned(t, out)
}

func TestAuditResultsFlattensAndClamps(t *testing.T) {
	long := strings.Repeat("x", 200)
	out := auditRender(auditResult{
		ran:  true,
		cols: []string{"a", "b"},
		rows: [][]string{{"one\ntwo\tthree", long}},
	})
	if strings.Count(out, "\n") != 3 {
		t.Fatalf("cell newline leaked into table: %q", out)
	}
	if strings.Contains(out, long) {
		t.Fatalf("long cell was not clamped: %q", out)
	}
	auditAssertAligned(t, out)
}

func TestAuditResultsRaggedAndEmpty(t *testing.T) {
	auditAssertAligned(t, auditRender(auditResult{
		ran:  true,
		cols: []string{"a", "b", "c"},
		rows: [][]string{{"only"}},
	}))

	if out := auditRender(auditResult{ran: true}); !strings.Contains(out, "(no columns)") {
		t.Fatalf("empty columns = %q", out)
	}
	if out := auditRender(auditResult{ran: true, cols: []string{"a"}}); !strings.Contains(out, "(0 rows)") {
		t.Fatalf("empty rows = %q", out)
	}
}

func TestAuditResultsStates(t *testing.T) {
	if out := auditRender(auditResult{}); !strings.Contains(out, "press enter to run") {
		t.Fatalf("pre-run state = %q", out)
	}
	if out := auditRender(auditResult{ran: true, err: "boom"}); !strings.Contains(out, "boom") {
		t.Fatalf("error state = %q", out)
	}
}

func TestAuditNarrowFrameCapsColumns(t *testing.T) {
	res := auditResult{
		ran:  true,
		cols: []string{"a"},
		rows: [][]string{{strings.Repeat("y", 120)}},
	}
	me := &auditView{result: res}
	narrow := me.results(layout.NewFrame(24))
	wide := me.results(layout.NewFrame(200))
	nw := lipgloss.Width(strings.Split(narrow, "\n")[0])
	ww := lipgloss.Width(strings.Split(wide, "\n")[0])
	if nw > ww {
		t.Fatalf("narrow frame column width %d exceeds wide frame %d", nw, ww)
	}
	if ww != 40 {
		t.Fatalf("wide frame column width = %d, want the 40-cell cap", ww)
	}
}

func TestAuditRunRejectsWrites(t *testing.T) {
	me := &auditView{home: t.TempDir(), sql: "DELETE FROM runs"}
	me.run()
	if !strings.Contains(me.result.err, "read-only") {
		t.Fatalf("write statement not rejected: %+v", me.result)
	}
}

func TestAuditEnterDefersTheQueryToACommand(t *testing.T) {
	me := &auditView{home: t.TempDir(), sql: "SELECT 42 AS answer"}

	cmd := me.Update(nil, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter returned no command: the query must run off the bubbletea Update goroutine")
	}
	if me.result.ran {
		t.Fatalf("enter produced a result inside Update: %+v", me.result)
	}
}

func TestAuditBackspaceKeepsMultiByteRunesIntact(t *testing.T) {
	const typed = "select 'é🙂'"
	me := &auditView{}
	for _, r := range typed {
		me.Update(nil, runeKey(r))
	}
	if me.sql != typed {
		t.Fatalf("typed sql = %q, want %q", me.sql, typed)
	}

	for i, want := range []string{"select 'é🙂", "select 'é", "select '"} {
		me.Update(nil, tea.KeyMsg{Type: tea.KeyBackspace})
		if !utf8.ValidString(me.sql) {
			t.Fatalf("backspace %d left invalid utf-8: %q", i+1, me.sql)
		}
		if me.sql != want {
			t.Fatalf("backspace %d = %q, want %q", i+1, me.sql, want)
		}
	}
}

// auditTestHeight is the body height handed to Body in the scroll tests; the
// db selector, sql editor and rule take three of those lines.
const (
	auditTestHeight     = 20
	auditTestHeadLines  = 3
	auditTestViewHeight = auditTestHeight - auditTestHeadLines
)

func auditWindowRows() int {
	return max(layout.ViewportContentRows(auditTestViewHeight), 1)
}

func auditScrollView(rows int) *auditView {
	data := make([][]string, rows)
	for i := range data {
		data[i] = []string{fmt.Sprintf("r%02d", i), "value"}
	}
	me := &auditView{
		result: auditResult{ran: true, cols: []string{"id", "val"}, rows: data},
	}
	me.Body(layout.Frame{Width: 120, Height: auditTestHeight}) // prime the scroll row math from the body height
	return me
}

func auditScrollBy(me *auditView, delta int) {
	act := keys.Down
	if delta < 0 {
		act, delta = keys.Up, -delta
	}
	for range delta {
		me.scroll.Handle(act)
	}
}

func TestAuditScrollClamping(t *testing.T) {
	me := auditScrollView(50)
	total := layout.CountLines(me.results(layout.NewFrame(120)))
	window := auditWindowRows()
	if total != 53 {
		t.Fatalf("total lines = %d, want 53", total)
	}
	bottom := total - window
	if bottom <= 0 {
		t.Fatalf("window %d does not scroll for %d lines", window, total)
	}

	auditScrollBy(me, -1)
	if me.scroll.Offset != 0 {
		t.Fatalf("scroll up at top = %d, want 0", me.scroll.Offset)
	}

	auditScrollBy(me, 5)
	if me.scroll.Offset != 5 {
		t.Fatalf("mid scroll = %d, want 5", me.scroll.Offset)
	}
	first := strings.Split(layout.NewFrame(120).Viewport(me.results(layout.NewFrame(120)), auditTestViewHeight, me.scroll.Offset), "\n")[0]
	if !strings.Contains(first, "r03") {
		t.Fatalf("first visible line at offset 5 = %q", first)
	}

	auditScrollBy(me, 1000)
	if me.scroll.Offset != bottom {
		t.Fatalf("scroll down past end = %d, want %d", me.scroll.Offset, bottom)
	}
	auditScrollBy(me, 1)
	if me.scroll.Offset != bottom {
		t.Fatalf("scroll down at bottom = %d, want %d", me.scroll.Offset, bottom)
	}

	auditScrollBy(me, -1000)
	if me.scroll.Offset != 0 {
		t.Fatalf("scroll up past start = %d, want 0", me.scroll.Offset)
	}
}

func TestAuditScrollShorterThanWindow(t *testing.T) {
	me := auditScrollView(2)
	body := me.results(layout.NewFrame(120))
	if layout.CountLines(body) >= auditWindowRows() {
		t.Fatalf("fixture is not shorter than the window")
	}

	auditScrollBy(me, 1000)
	if me.scroll.Offset != 0 {
		t.Fatalf("offset = %d, want 0 for content shorter than window", me.scroll.Offset)
	}
	if got := layout.NewFrame(120).Viewport(body, auditTestViewHeight, me.scroll.Offset); got != body {
		t.Fatalf("short body was windowed:\n%q\nwant\n%q", got, body)
	}
}

func TestAuditScrollKeysAndRunReset(t *testing.T) {
	me := auditScrollView(50)
	me.sql = "DROP TABLE runs"

	for i := 0; i < 3; i++ {
		me.Update(nil, tea.KeyMsg{Type: tea.KeyDown})
	}
	if me.scroll.Offset != 3 {
		t.Fatalf("offset after three downs = %d, want 3", me.scroll.Offset)
	}
	me.Update(nil, tea.KeyMsg{Type: tea.KeyUp})
	if me.scroll.Offset != 2 {
		t.Fatalf("offset after up = %d, want 2", me.scroll.Offset)
	}
	me.Update(nil, tea.KeyMsg{Type: tea.KeyPgDown})
	if me.scroll.Offset != 2+auditWindowRows() {
		t.Fatalf("offset after pgdown = %d, want %d", me.scroll.Offset, 2+auditWindowRows())
	}
	me.Update(nil, tea.KeyMsg{Type: tea.KeyPgUp})
	if me.scroll.Offset != 2 {
		t.Fatalf("offset after pgup = %d, want 2", me.scroll.Offset)
	}

	me.Update(nil, tea.KeyMsg{Type: tea.KeyEnter})
	if me.scroll.Offset != 0 {
		t.Fatalf("offset after run = %d, want 0", me.scroll.Offset)
	}
	if !strings.Contains(me.result.err, "read-only") {
		t.Fatalf("run did not reject the write statement: %+v", me.result)
	}
}

func TestAuditScrollBeforeFirstRenderStaysPut(t *testing.T) {
	me := &auditView{result: auditResult{ran: true, cols: []string{"id"}, rows: [][]string{{"r00"}}}}
	me.scroll.Handle(keys.Down)
	if me.scroll.Offset != 0 {
		t.Fatalf("offset before first render = %d, want 0", me.scroll.Offset)
	}
}

func TestAuditTallerBodyClampsOffset(t *testing.T) {
	me := auditScrollView(50)
	auditScrollBy(me, 1000)
	tall := me.scroll.Offset
	me.Body(layout.Frame{Width: 120, Height: 200})
	auditScrollBy(me, 0)
	me.scroll.Handle(keys.Up)
	if me.scroll.Offset >= tall {
		t.Fatalf("offset %d not clamped after growing the body (was %d)", me.scroll.Offset, tall)
	}
	if me.scroll.Offset != 0 {
		t.Fatalf("offset = %d, want 0 once the body fits", me.scroll.Offset)
	}
}
