package views

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/codyconfer/sisyphus/store"
	"github.com/codyconfer/viewkit/keys"
	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/panels"
	"github.com/codyconfer/viewkit/theme"

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

const (
	auditvChrome  = 10
	auditvTimeout = 30 * time.Second
)

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

	scroll layout.ScrollState
	height int
	ready  bool
	width  int
}

func (k *Kit) AuditQuery() vkdeck.View {
	return &auditView{home: k.d.App.Cfg.Home}
}

func (me *auditView) db() string { return auditvDBs[me.dbIndex] }

func (me *auditView) Title() string { return "query · " + me.db() }

func (me *auditView) Init() tea.Cmd { return nil }

func (me *auditView) Context() [][2]string {
	cues := [][2]string{{"db", me.db()}}
	if me.running {
		cues = append(cues, [2]string{"state", "running"})
	}
	return cues
}

func (me *auditView) Hints() [][2]string {
	return [][2]string{{"←/→", "db"}, {"enter", "run"}, {"↑/↓", "scroll"}}
}

func (me *auditView) Update(a *vkdeck.Model, msg tea.Msg) tea.Cmd {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		me.width = m.Width
		h := m.Height - auditvChrome
		if h < 1 {
			h = 1
		}
		me.height = h
		me.ready = true
		me.scrollBy(0)
		return nil
	case auditRanMsg:
		me.running = false
		me.result = m.result
		me.scroll.Offset = 0
		me.scrollBy(0)
		return nil
	case tea.KeyMsg:
		act, ok := keymap.Form(keys.Binding{Keys: []string{"tab"}, Action: keys.Right}).Action(m.String())
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
		case keys.Up:
			me.scrollBy(-1)
		case keys.Down:
			me.scrollBy(1)
		case keys.PageUp:
			me.scrollBy(-me.windowRows())
		case keys.PageDown:
			me.scrollBy(me.windowRows())
		}
	}
	return nil
}

func (me *auditView) windowRows() int {
	rows := layout.ViewportContentRows(me.height)
	if rows < 1 {
		rows = 1
	}
	return rows
}

func (me *auditView) scrollBy(delta int) {
	if !me.ready {
		return
	}
	me.scroll.Scroll(delta, layout.CountLines(me.results(me.frame())), me.windowRows())
}

func (me *auditView) frame() layout.Frame { return layout.NewFrame(me.width) }

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
	res, err := store.Query(ctx, path, query)
	if err != nil {
		return auditResult{err: err.Error(), ran: true}
	}
	return auditResult{cols: res.Columns, rows: res.Rows, ran: true}
}

func (me *auditView) results(f layout.Frame) string {
	th := theme.Cur()
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

func (me *auditView) Body(width, height int) string {
	th := theme.Cur()
	f := layout.NewFrame(width)

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

	results := me.results(f)
	if me.ready {
		results = layout.Viewport(results, me.height, me.scroll.Offset)
	}

	return layout.StackTight(selector, editor, f.Rule(), results)
}
