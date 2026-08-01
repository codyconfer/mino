package views

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/codyconfer/sisyphus/duckfile"
	"github.com/codyconfer/viewkit/keys"
	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/panels"
	"github.com/codyconfer/viewkit/ui"

	vkdeck "github.com/codyconfer/viewkit/deck"

	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/keymap"
)

var auditvDBs = []string{"audit", "config", "tokens"}

var auditvDefaultSQL = map[string]string{
	"audit":  "SELECT name, kind, count(*) AS runs, coalesce(sum(count), 0) AS items FROM runs GROUP BY name, kind ORDER BY runs DESC",
	"config": "SELECT name, format, applied_at FROM store_current ORDER BY name",
	"tokens": "SELECT namespace, key, updated_at, expiry IS NOT NULL AS has_expiry FROM kv ORDER BY namespace, key",
}

const auditvTimeout = 30 * time.Second

type auditResult struct {
	cols []string
	rows [][]string
	err  string
	ran  bool
}

type auditRanMsg struct{ result auditResult }

type auditView struct {
	home    string
	dbIndex int
	sql     string
	result  auditResult
	running bool

	exec func(path, query string) auditResult

	keys   *keys.Map
	scroll vkdeck.ScrollBody
}

func (k *Kit) AuditQuery() vkdeck.View {
	return &auditView{home: k.d.App.Cfg.Home}
}

func (me *auditView) db() string { return auditvDBs[me.dbIndex] }

func (me *auditView) Title() string { return "query · " + me.db() }

func (me *auditView) Init() tea.Cmd { return nil }

func (me *auditView) Context(scope *ui.Scope) []keys.Hint {
	cues := []keys.Hint{{Key: "db", Label: me.db()}}
	if me.running {
		cues = append(cues, keys.Hint{Key: "state", Label: "running"})
	}
	return cues
}

func (me *auditView) Hints(scope *ui.Scope) []keys.Hint {
	return []keys.Hint{{Key: "←/→", Label: "db"}, {Key: "enter", Label: "run"}, {Key: "↑/↓", Label: "scroll"}}
}

func (me *auditView) Update(a *vkdeck.Model, msg tea.Msg) tea.Cmd {
	switch m := msg.(type) {
	case auditRanMsg:
		me.running = false
		me.result = m.result
		me.scroll.Offset = 0
		return nil
	case tea.KeyMsg:
		if me.keys == nil {
			me.keys = keymap.Form(modelScope(a).Keys, keys.Binding{Keys: []string{"tab"}, Action: keys.Right})
		}
		act, ok := me.keys.Action(m.String())
		if !ok {
			if m.String() == " " {
				me.sql += " "
			} else if m.Type == tea.KeyRunes {
				me.sql += string(m.Runes)
			}
			return nil
		}
		switch act {
		case keys.Cancel:
			return a.Pop()
		case keys.Left:
			me.cycle(-1)
			return nil
		case keys.Right:
			me.cycle(1)
			return nil
		case keys.Confirm:
			return me.run()
		case keys.Erase:
			if r := []rune(me.sql); len(r) > 0 {
				me.sql = string(r[:len(r)-1])
			}
			return nil
		case keys.Up, keys.Down, keys.PageUp, keys.PageDown:
			me.scroll.Handle(act)
		}
	}
	return nil
}

func (me *auditView) cycle(delta int) {
	n := len(auditvDBs)
	me.dbIndex = ((me.dbIndex+delta)%n + n) % n
}

func (me *auditView) defaultSQL() string {
	if q, ok := auditvDefaultSQL[me.db()]; ok {
		return q
	}
	return auditvDefaultSQL["audit"]
}

func (me *auditView) run() tea.Cmd {
	if me.running {
		return nil
	}
	query := strings.TrimSpace(me.sql)
	if query == "" {
		query = me.defaultSQL()
	}
	me.scroll.Offset = 0
	if !auditvReadOnly(query) {
		me.result = auditResult{err: "only read-only statements are allowed (select/with/pragma/describe/show)", ran: true}
		return nil
	}
	me.running = true
	me.result = auditResult{}
	exec, path := me.execFn(), me.dbPath()
	return func() tea.Msg { return auditRanMsg{result: exec(path, query)} }
}

func auditvReadOnly(query string) bool {
	lower := strings.ToLower(strings.TrimSpace(query))
	for _, p := range []string{"select", "with", "pragma", "describe", "show"} {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	return false
}

func (me *auditView) dbPath() string {
	return config.DataPath(me.home, me.db()+".duckdb")
}

func (me *auditView) execFn() func(path, query string) auditResult {
	if me.exec != nil {
		return me.exec
	}
	return auditExec
}

func auditExec(path, query string) auditResult {
	ctx, cancel := context.WithTimeout(context.Background(), auditvTimeout)
	defer cancel()
	res, err := duckfile.Query(ctx, path, query)
	if err != nil {
		return auditResult{err: err.Error(), ran: true}
	}
	return auditResult{cols: res.Columns, rows: res.Rows, ran: true}
}

func (me *auditView) results(f layout.Frame) string {
	th := f.Theme()
	switch {
	case me.running:
		return th.Dim.Render("running…")
	case me.result.err != "":
		return th.Cant.Render(me.result.err)
	case !me.result.ran:
		return th.Dim.Render("press enter to run")
	}
	return panels.Table(f, me.result.cols, me.result.rows)
}

func (me *auditView) Body(f layout.Frame) string {
	th := f.Theme()
	height := f.Height
	f = f.WithWidth(f.Width)

	tabs := make([]string, len(auditvDBs))
	for i, name := range auditvDBs {
		if i == me.dbIndex {
			tabs[i] = th.Accent.Render("[" + name + "]")
		} else {
			tabs[i] = th.Dim.Render(" " + name + " ")
		}
	}
	selector := f.Row("db", strings.Join(tabs, th.Dim.Render(" ")))

	shown := me.sql
	if strings.TrimSpace(shown) == "" {
		shown = th.Dim.Render(me.defaultSQL())
	} else {
		shown = th.Val.Render(shown)
	}
	editor := f.Row("sql", shown+th.Accent.Render("▉"))

	head := layout.StackTight(selector, editor, f.Rule())
	results := me.scroll.View(f, me.results(f), height-layout.CountLines(head))

	return layout.StackTight(head, results)
}
