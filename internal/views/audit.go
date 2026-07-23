package views

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	_ "github.com/marcboeker/go-duckdb/v2"

	"github.com/codyconfer/viewkit/keys"
	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/munin/internal/keymap"
	"github.com/codyconfer/munin/internal/tui"
)

var auditvDBs = []string{"audit", "config", "tokens"}

const auditvDefaultSQL = "SELECT name, kind, count(*) AS runs FROM runs GROUP BY name, kind ORDER BY runs DESC"

const auditvChrome = 10

const auditvMaxCell = 40

type auditView struct {
	home    string
	dbIndex int
	sql     string
	result  string

	vp    viewport.Model
	ready bool
	width int
}

func (k *Kit) AuditQuery() tui.View {
	return &auditView{home: k.d.Home()}
}

func (me *auditView) db() string { return auditvDBs[me.dbIndex] }

func (me *auditView) Title() string { return "audit · query" }

func (me *auditView) Init() tea.Cmd { return nil }

func (me *auditView) Context() [][2]string {
	return [][2]string{{"db", me.db()}}
}

func (me *auditView) Hints() [][2]string {
	return [][2]string{{"←/→", "db"}, {"enter", "run"}, {"↑/↓", "scroll"}}
}

func (me *auditView) Update(a *tui.App, msg tea.Msg) tea.Cmd {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		me.width = m.Width
		h := m.Height - auditvChrome
		if h < 1 {
			h = 1
		}
		if !me.ready {
			me.vp = viewport.New(m.Width, h)
			me.ready = true
		} else {
			me.vp.Width, me.vp.Height = m.Width, h
		}
		me.vp.SetContent(me.result)
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
			if me.ready {
				me.vp.SetContent(me.result)
				me.vp.GotoTop()
			}
			return nil
		case keys.Erase:
			if n := len(me.sql); n > 0 {
				me.sql = me.sql[:n-1]
			}
			return nil
		case keys.Up, keys.Down, keys.PageUp, keys.PageDown:
			if me.ready {
				var cmd tea.Cmd
				me.vp, cmd = me.vp.Update(msg)
				return cmd
			}
		}
	}
	return nil
}

func (me *auditView) cycle(delta int) {
	n := len(auditvDBs)
	me.dbIndex = ((me.dbIndex+delta)%n + n) % n
}

func (me *auditView) run() {
	th := theme.Cur()
	query := strings.TrimSpace(me.sql)
	if query == "" {
		query = auditvDefaultSQL
	}
	if !auditvReadOnly(query) {
		me.result = th.Cant.Render("only read-only statements are allowed (select/with/pragma/describe/show)")
		return
	}
	out, err := me.exec(query)
	if err != nil {
		me.result = th.Cant.Render(err.Error())
		return
	}
	me.result = out
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

func (me *auditView) exec(query string) (string, error) {
	path := filepath.Join(me.home, me.db()+".duckdb")
	db, err := sql.Open("duckdb", path)
	if err != nil {
		return "", err
	}
	db.SetMaxOpenConns(1)
	defer db.Close()

	rows, err := db.Query(query)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return "", err
	}

	var data [][]string
	for rows.Next() {
		cells := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range cells {
			ptrs[i] = &cells[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return "", err
		}
		row := make([]string, len(cols))
		for i, c := range cells {
			row[i] = auditvCell(c)
		}
		data = append(data, row)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return auditvTable(cols, data), nil
}

func auditvCell(v any) string {
	switch t := v.(type) {
	case nil:
		return "NULL"
	case []byte:
		return auditvOneLine(string(t))
	case string:
		return auditvOneLine(t)
	default:
		return auditvOneLine(fmt.Sprintf("%v", t))
	}
}

func auditvOneLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	return s
}

func auditvTable(cols []string, data [][]string) string {
	th := theme.Cur()
	if len(cols) == 0 {
		return th.Dim.Render("(no columns)")
	}

	widths := make([]int, len(cols))
	for i, c := range cols {
		widths[i] = auditvClamp(len(c))
	}
	for _, row := range data {
		for i, cell := range row {
			if w := auditvClamp(len(cell)); w > widths[i] {
				widths[i] = w
			}
		}
	}

	var b strings.Builder
	b.WriteString(th.Key.Render(auditvRow(cols, widths)))
	b.WriteString("\n")

	sep := make([]string, len(cols))
	for i, w := range widths {
		sep[i] = strings.Repeat("─", w)
	}
	b.WriteString(th.Dim.Render(strings.Join(sep, "─┼─")))
	b.WriteString("\n")

	if len(data) == 0 {
		b.WriteString(th.Dim.Render("(0 rows)"))
		return b.String()
	}
	for _, row := range data {
		b.WriteString(th.Val.Render(auditvRow(row, widths)))
		b.WriteString("\n")
	}
	b.WriteString(th.Dim.Render(fmt.Sprintf("(%d rows)", len(data))))
	return b.String()
}

func auditvClamp(n int) int {
	if n > auditvMaxCell {
		return auditvMaxCell
	}
	return n
}

func auditvRow(cells []string, widths []int) string {
	out := make([]string, len(widths))
	for i, w := range widths {
		val := ""
		if i < len(cells) {
			val = layout.Fit(cells[i], w)
		}
		pad := w - len([]rune(val))
		if pad < 0 {
			pad = 0
		}
		out[i] = val + strings.Repeat(" ", pad)
	}
	return strings.Join(out, " │ ")
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
		shown = th.Dim.Render(auditvDefaultSQL)
	} else {
		shown = th.Val.Render(shown)
	}
	editor := f.Row("sql", shown+th.Accent.Render("▉"))

	results := me.result
	if me.ready {
		results = me.vp.View()
	} else if results == "" {
		results = th.Dim.Render("press enter to run")
	}

	return layout.StackTight(selector, editor, f.Rule(), results)
}
