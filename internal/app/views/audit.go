package views

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/codyconfer/sisyphus/store"
	"github.com/codyconfer/viewkit/keys"
	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/panels"
	"github.com/codyconfer/viewkit/theme"

	vkdeck "github.com/codyconfer/viewkit/deck"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/keymap"
)

var auditvDBs = []string{"audit", "config", "tokens"}

var auditvDefaultSQL = map[string]string{
	"audit":  "SELECT name, kind, count(*) AS runs, coalesce(sum(count), 0) AS items FROM runs GROUP BY name, kind ORDER BY runs DESC",
	"config": "SELECT name, format, applied_at FROM store_current ORDER BY name",
	"tokens": "SELECT namespace, key, updated_at, expiry IS NOT NULL AS has_expiry FROM kv ORDER BY namespace, key",
}

const auditvChrome = 10

type auditResult struct {
	cols []string
	rows [][]string
	err  string
	ran  bool
}

type auditView struct {
	home    string
	dbIndex int
	sql     string
	result  auditResult

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
	return [][2]string{{"db", me.db()}}
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
			me.run()
			me.scroll.Offset = 0
			return nil
		case keys.Erase:
			if n := len(me.sql); n > 0 {
				me.sql = me.sql[:n-1]
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

func (me *auditView) run() {
	query := strings.TrimSpace(me.sql)
	if query == "" {
		query = me.defaultSQL()
	}
	if !auditvReadOnly(query) {
		me.result = auditResult{err: "only read-only statements are allowed (select/with/pragma/describe/show)", ran: true}
		return
	}
	res, err := me.exec(query)
	if err != nil {
		me.result = auditResult{err: err.Error(), ran: true}
		return
	}
	me.result = res
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

func (me *auditView) exec(query string) (auditResult, error) {
	path := config.DataPath(me.home, me.db()+".duckdb")
	res, err := store.Query(context.Background(), path, query)
	if err != nil {
		return auditResult{}, err
	}
	return auditResult{cols: res.Columns, rows: res.Rows, ran: true}, nil
}

func (me *auditView) results(f layout.Frame) string {
	th := theme.Cur()
	switch {
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
