package views

import (
	"errors"
	"strconv"
	"strings"

	"github.com/codyconfer/viewkit/forms"

	tea "github.com/charmbracelet/bubbletea"
	vkdeck "github.com/codyconfer/viewkit/deck"
	"github.com/codyconfer/viewkit/keys"

	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/render/glyph"
	"github.com/codyconfer/mino/internal/signals"
)

func (kit *Kit) duckDBCtx() []keys.Hint {
	return append(kit.menuCtx(), keys.Hint{Key: "tool", Label: "DuckDB"})
}

func (kit *Kit) DuckDB() vkdeck.View {
	items := []vkdeck.MenuItem{{
		Label:    "New",
		Desc:     "write, run, and save a read-only SQL query",
		Icon:     glyph.Builder(),
		OnSelect: func(a *vkdeck.Model) tea.Cmd { return a.Push(kit.DuckDBBuilder()) },
	}}
	d := kit.d.App.Dirs()
	for _, name := range d.DuckDBNames() {
		name := name
		query := d.DuckDB[name]
		items = append(items, vkdeck.MenuItem{
			Label:    name,
			Desc:     duckDBSummary(query),
			OnSelect: func(a *vkdeck.Model) tea.Cmd { return a.Push(kit.DuckDBEditor(name)) },
		})
	}
	return vkdeck.NewMenu("duckdb", kit.duckDBCtx(), items...)
}

func duckDBSummary(q config.DuckDBQuery) string {
	sql := strings.Join(strings.Fields(q.SQL), " ")
	if len(sql) > 72 {
		sql = sql[:69] + "…"
	}
	return q.Database + "  " + sql
}

type duckDBView struct {
	*editorShell

	kit     *Kit
	orig    string
	base    config.DuckDBQuery
	dbIndex int
	exec    func(path, query string) auditResult
}

func (kit *Kit) DuckDBBuilder() vkdeck.View {
	base := config.DuckDBQuery{Database: auditvDBs[0], SQL: auditvDefaultSQL[auditvDBs[0]]}
	return kit.newDuckDBView("", base)
}

func (kit *Kit) DuckDBEditor(name string) vkdeck.View {
	return kit.newDuckDBView(name, kit.d.App.Dirs().DuckDB[name])
}

func (kit *Kit) AuditQuery() vkdeck.View { return kit.DuckDBBuilder() }

func (kit *Kit) newDuckDBView(orig string, base config.DuckDBQuery) *duckDBView {
	v := &duckDBView{kit: kit, orig: orig, base: base, dbIndex: indexOfDBName(base.Database)}
	v.editorShell = newEditorShell(v, map[string]any{
		"database": base.Database,
		"sql":      base.SQL,
		"name":     base.Name,
	}, kit.scope().Keys)
	return v
}

func indexOfDBName(name string) int {
	for i, candidate := range auditvDBs {
		if candidate == name {
			return i
		}
	}
	return 0
}

func (v *duckDBView) editorKind() string { return "DuckDB query" }

func (v *duckDBView) editorTitle() string {
	if v.orig != "" {
		return "edit " + v.orig
	}
	return "build DuckDB query"
}

func (v *duckDBView) editorCtx() []keys.Hint {
	ctx := v.kit.duckDBCtx()
	if v.orig != "" {
		ctx = append(ctx, keys.Hint{Key: "item", Label: v.orig})
	}
	return append(ctx, keys.Hint{Key: "database", Label: v.database()})
}

func (v *duckDBView) editorSavedName() string { return v.orig }

func (v *duckDBView) editorFields(prev map[string]any) []forms.Field {
	return []forms.Field{
		{Key: "database", Label: "database", Kind: forms.FieldSelect, Options: auditvDBs, Selected: v.dbIndex},
		{Key: "sql", Label: "read-only SQL", Kind: forms.FieldMultiline, Text: forms.Raw(prev, "sql")},
		{Key: "name", Label: "name (required to save)", Kind: forms.FieldText, Text: forms.Raw(prev, "name")},
	}
}

func (v *duckDBView) editorSync() bool {
	idx := v.SelectedOf("database")
	if idx < 0 || idx == v.dbIndex {
		return false
	}
	v.dbIndex = idx
	return true
}

func (v *duckDBView) database() string {
	if v.dbIndex < 0 || v.dbIndex >= len(auditvDBs) {
		return auditvDBs[0]
	}
	return auditvDBs[v.dbIndex]
}

func (v *duckDBView) editorSummary() string {
	name := strings.TrimSpace(v.Value("name"))
	if name == "" {
		name = "unsaved draft"
	}
	return name + "  database=" + v.database()
}

func (v *duckDBView) query() config.DuckDBQuery {
	q := v.base
	q.Name = strings.TrimSpace(v.Value("name"))
	q.Type = config.TypeDuckDB
	q.Database = v.database()
	q.SQL = strings.TrimSpace(v.Value("sql"))
	return q
}

func (v *duckDBView) editorValue() (any, error) {
	q := v.query()
	if q.SQL == "" {
		return config.DuckDBQuery{}, errs.New(errs.KindConfig, "a DuckDB query needs SQL")
	}
	if !config.DuckDBReadOnly(q.SQL) {
		return config.DuckDBQuery{}, errs.New(errs.KindConfig, "only read-only statements are allowed").
			WithHint("use select, with, pragma, describe, or show")
	}
	return q, nil
}

func (v *duckDBView) editorRun() (string, func() []signals.Section, error) {
	val, err := v.editorValue()
	if err != nil {
		return "", nil, err
	}
	q := val.(config.DuckDBQuery)
	exec := v.exec
	if exec == nil {
		exec = auditExec
	}
	path := config.DataPath(v.kit.d.App.Cfg.Home, q.Database+".duckdb")
	label := q.Name
	if label == "" {
		label = editorAdhocLabel
	}
	return label, func() []signals.Section {
		return duckDBSections(q, exec(path, q.SQL))
	}, nil
}

func duckDBSections(q config.DuckDBQuery, result auditResult) []signals.Section {
	title := q.Name
	if title == "" {
		title = "ad-hoc"
	}
	title += " · " + q.Database
	if result.err != "" {
		return []signals.Section{{Signal: "duckdb", Title: title, Err: errors.New(result.err)}}
	}
	items := make([]signals.Item, 0, len(result.rows))
	for i, row := range result.rows {
		parts := make([]string, 0, len(row))
		for col, value := range row {
			key := "column " + strconv.Itoa(col+1)
			if col < len(result.cols) && result.cols[col] != "" {
				key = result.cols[col]
			}
			parts = append(parts, key+"="+value)
		}
		if len(parts) == 0 {
			parts = append(parts, "row "+strconv.Itoa(i+1))
		}
		items = append(items, signals.Item{Kind: "row", Title: strings.Join(parts, "  ")})
	}
	return []signals.Section{{
		Signal: "duckdb", Title: title, Items: items,
		Meta: map[string]string{"database": q.Database, "rows": strconv.Itoa(len(items))},
	}}
}

func (v *duckDBView) editorVerify(val any) Finding {
	q := val.(config.DuckDBQuery)
	name := q.Name
	if name == "" {
		name = editorAdhocLabel
	}
	return Finding{Name: name, OK: config.ValidDuckDBDatabase(q.Database) && q.SQL != "" && config.DuckDBReadOnly(q.SQL)}
}

func (v *duckDBView) editorPersist(val any) (string, error) {
	q := val.(config.DuckDBQuery)
	if q.Name == "" {
		return "", errs.New(errs.KindUsage, "name is required to save")
	}
	d := v.kit.d.App.Dirs()
	if q.Name != v.orig {
		if _, exists := d.DuckDB[q.Name]; exists {
			return "", errs.Newf(errs.KindUsage, "a DuckDB query named %s already exists", q.Name)
		}
	}
	rel := d.Source(config.TypeDuckDB, v.orig)
	summary, _, err := v.kit.saveDirective(config.TypeDuckDB, rel, q.Name, q)
	if err != nil {
		return "", err
	}
	if q.Name != v.orig && v.orig != "" {
		summary += editorRenameNote(v.orig, rel)
	}
	v.orig, v.base = q.Name, q
	if err := v.kit.d.App.RefreshDirectives(config.ReconcileIgnore); err == nil {
		summary += "\nit is live in this session."
	}
	return summary, nil
}

func (v *duckDBView) editorRemove() (string, error) {
	return v.kit.deleteDirective(config.TypeDuckDB, v.orig)
}
