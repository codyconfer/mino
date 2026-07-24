package views

import (
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/codyconfer/viewkit/forms"
	"github.com/codyconfer/viewkit/keys"
	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/theme"

	"gopkg.in/yaml.v3"

	sconfig "github.com/codyconfer/sisyphus/config"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/deck"
	"github.com/codyconfer/munin/internal/filter"
	"github.com/codyconfer/munin/internal/keymap"
	"github.com/codyconfer/munin/internal/render/glyph"
	"github.com/codyconfer/munin/internal/signals"
)

type directiveKind int

const (
	directiveQuery directiveKind = iota
	directiveFilter
	directiveFlight
	directiveRole
)

func directiveLabel(k directiveKind) string {
	switch k {
	case directiveQuery:
		return "Queries"
	case directiveFilter:
		return "Filters"
	case directiveFlight:
		return "Flights"
	case directiveRole:
		return "Roles"
	}
	return "?"
}

func directiveSingular(k directiveKind) string {
	switch k {
	case directiveQuery:
		return "query"
	case directiveFilter:
		return "filter"
	case directiveFlight:
		return "flight"
	case directiveRole:
		return "role"
	}
	return "item"
}

func directiveDir(k directiveKind) string {
	switch k {
	case directiveQuery:
		return config.DirQueries
	case directiveFilter:
		return config.DirFilters
	case directiveFlight:
		return config.DirFlights
	case directiveRole:
		return config.DirRoles
	}
	return ""
}

func directiveRunnable(k directiveKind) bool {
	return k == directiveQuery || k == directiveFlight
}

func (kit *Kit) directiveNames(k directiveKind) []string {
	st := kit.d.App.Directives
	switch k {
	case directiveQuery:
		return st.QueryNames()
	case directiveFilter:
		return st.FilterNames()
	case directiveFlight:
		return st.FlightNames()
	case directiveRole:
		return st.RoleNames()
	}
	return nil
}

func (kit *Kit) directiveItem(k directiveKind, name string) any {
	st := kit.d.App.Directives
	switch k {
	case directiveQuery:
		return st.Queries[name]
	case directiveFilter:
		return st.Filters[name]
	case directiveFlight:
		return st.Flights[name]
	case directiveRole:
		return st.Roles[name]
	}
	return nil
}

func (kit *Kit) directiveCtx(k directiveKind) [][2]string {
	return append(kit.menuCtx(), [2]string{"directive", directiveLabel(k)})
}

func (kit *Kit) directiveItemCtx(k directiveKind, name string) [][2]string {
	return append(kit.directiveCtx(k), [2]string{"item", name})
}

func directiveSplit(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func directiveStr(vals map[string]any, key string) string {
	if v, ok := vals[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func (kit *Kit) directivesMenu() deck.View {
	pick := func(k directiveKind) deck.MenuItem {
		return deck.MenuItem{
			Label: directiveLabel(k),
			Desc:  "browse saved " + strings.ToLower(directiveLabel(k)),
			Do:    func(a *deck.State) tea.Cmd { return a.Push(kit.directiveBrowser(k)) },
		}
	}
	return deck.NewMenu("directives", kit.menuCtx(),
		pick(directiveQuery),
		pick(directiveFilter),
		pick(directiveFlight),
		pick(directiveRole),
	)
}

func (kit *Kit) directiveBrowser(k directiveKind) deck.View {
	var items []deck.MenuItem
	for _, n := range kit.directiveNames(k) {
		n := n
		items = append(items, deck.MenuItem{
			Label: n,
			Do:    func(a *deck.State) tea.Cmd { return a.Push(kit.directiveActionsMenu(k, n)) },
		})
	}
	if len(items) == 0 {
		items = append(items, deck.MenuItem{Label: "(none)", Desc: "no saved " + strings.ToLower(directiveLabel(k))})
	}
	items = append(items, deck.MenuItem{
		Label: "＋ New",
		Desc:  "create a " + directiveSingular(k),
		Do:    func(a *deck.State) tea.Cmd { return a.Push(kit.newDirectiveForm(k, "")) },
	})
	return deck.NewMenu(directiveLabel(k), kit.directiveCtx(k), items...)
}

func (kit *Kit) directiveActionsMenu(k directiveKind, name string) deck.View {
	ctx := kit.directiveItemCtx(k, name)
	items := []deck.MenuItem{
		{Label: "View", Desc: "show YAML", Do: func(a *deck.State) tea.Cmd {
			return a.Push(kit.directiveViewContent(k, name))
		}},
	}
	if directiveRunnable(k) {
		items = append(items, deck.MenuItem{Label: "Run", Desc: "fetch and render results", Do: func(a *deck.State) tea.Cmd {
			return a.Push(kit.directiveRunContent(k, name))
		}})
	}
	items = append(items,
		deck.MenuItem{Label: "Validate", Desc: "check for problems", Do: func(a *deck.State) tea.Cmd {
			return a.Push(kit.directiveValidateContent(k, name))
		}},
		deck.MenuItem{Label: "Edit", Desc: "modify and save", Do: func(a *deck.State) tea.Cmd {
			return a.Push(kit.newDirectiveForm(k, name))
		}},
		deck.MenuItem{Label: "Delete", Desc: "remove the file", Do: func(a *deck.State) tea.Cmd {
			return a.Push(kit.directiveDeleteConfirm(k, name))
		}},
	)
	return deck.NewMenu(directiveSingular(k)+": "+name, ctx, items...)
}

func (kit *Kit) directiveViewContent(k directiveKind, name string) deck.View {
	return deck.NewContent("view: "+name, kit.directiveItemCtx(k, name), nil, func() string {
		data, err := yaml.Marshal(kit.directiveItem(k, name))
		if err != nil {
			return theme.Cur().Cant.Render("error: " + err.Error())
		}
		body := strings.TrimRight(string(data), "\n")
		return layout.NewFrame(theme.BodyWidth).Panel(name, strings.Split(body, "\n")...)
	})
}

func (kit *Kit) directiveRunContent(k directiveKind, name string) deck.View {
	return deck.NewResults("run: "+name, kit.directiveItemCtx(k, name), func() []signals.Section {
		switch k {
		case directiveQuery:
			return kit.d.FetchQuery(name)
		case directiveFlight:
			return kit.d.FetchFlight(name)
		}
		return nil
	})
}

func (kit *Kit) directiveValidateContent(k directiveKind, name string) deck.View {
	return deck.NewContent("validate: "+name, kit.directiveItemCtx(k, name), nil, func() string {
		th := theme.Cur()
		if k == directiveFilter {
			return layout.NewFrame(theme.BodyWidth).Panel("validation",
				th.Dim.Render("no dedicated validation for filters"))
		}
		var findings []Finding
		switch k {
		case directiveQuery:
			findings = kit.d.Verify("queries")
		case directiveFlight:
			findings = kit.d.Verify("flights")
		case directiveRole:
			findings = kit.d.Verify("roles")
		}
		var lines []string
		for _, f := range findings {
			if f.Name != name {
				continue
			}
			lines = append(lines, directiveFindingLine(f))
			if f.Msg != "" {
				lines = append(lines, "    "+th.Dim.Render(f.Msg))
			}
		}
		if len(lines) == 0 {
			lines = append(lines, th.Dim.Render("(no findings)"))
		}
		return layout.NewFrame(theme.BodyWidth).Panel("validation", lines...)
	})
}

func directiveFindingLine(f Finding) string {
	th := theme.Cur()
	var mark string
	switch {
	case !f.OK:
		mark = th.Cant.Render(glyph.Cross())
	case f.Warn:
		mark = th.Dim.Render(glyph.Warn())
	default:
		mark = th.Can.Render(glyph.Check())
	}
	return mark + " " + th.Val.Render(f.Name)
}

func (kit *Kit) directiveDeleteConfirm(k directiveKind, name string) deck.View {
	ctx := kit.directiveItemCtx(k, name)
	return deck.NewMenu("delete "+name+"?", ctx,
		deck.MenuItem{Label: "No, keep it", Do: func(a *deck.State) tea.Cmd { return a.Pop() }},
		deck.MenuItem{Label: "Yes, delete", Desc: "remove the file", Do: func(a *deck.State) tea.Cmd {
			return a.Push(deck.NewMessage("delete", kit.directiveDeleteFile(k, name), ctx))
		}},
	)
}

func (kit *Kit) directiveDeleteFile(k directiveKind, name string) string {
	dir := directiveDir(k)
	removed, _ := sconfig.RemoveFiles(filepath.Join(kit.d.App.Cfg.Home, dir), name, []string{".yaml", ".yml", ".json"})
	if len(removed) == 0 {
		return "no file found for " + name + " under " + dir + "/.\n\n" +
			"It may exist only in DuckDB (the source of truth); use\n" +
			"`munin export " + dir + "` to write files first."
	}
	return "removed:\n  " + strings.Join(removed, "\n  ") + "\n\n" +
		"DuckDB remains the source of truth; this takes effect after\n" +
		"reconcile: run `munin import " + dir + "` or restart munin."
}

type directiveFormView struct {
	kit  *Kit
	kind directiveKind
	orig string
	form *forms.Form
	ctx  [][2]string
}

func (kit *Kit) newDirectiveForm(k directiveKind, orig string) deck.View {
	return &directiveFormView{
		kit:  kit,
		kind: k,
		orig: orig,
		form: forms.NewForm(kit.directiveFormFields(k, orig)...),
		ctx:  kit.directiveItemCtx(k, orig),
	}
}

func (kit *Kit) directiveFormFields(k directiveKind, name string) []forms.Field {
	st := kit.d.App.Directives
	text := func(key, label, val string) forms.Field {
		return forms.Field{Key: key, Label: label, Kind: forms.FieldText, Text: val}
	}
	switch k {
	case directiveQuery:
		q := st.Queries[name]
		var refs []string
		for _, qf := range q.Filters {
			if qf.Ref != "" {
				refs = append(refs, qf.Ref)
			}
		}
		return []forms.Field{
			text("name", "name", q.Name),
			text("signal", "signal", q.Signal),
			text("query", "query param", q.Params["query"]),
			text("filters", "filters (comma-sep)", strings.Join(refs, ", ")),
		}
	case directiveFilter:
		f := st.Filters[name]
		var r filter.Rule
		if len(f.Rules) > 0 {
			r = f.Rules[0]
		}
		return []forms.Field{
			text("name", "name", f.Name),
			text("field", "field", r.Field),
			text("include", "include regex", r.Include),
			text("exclude", "exclude regex", r.Exclude),
		}
	case directiveFlight:
		fl := st.Flights[name]
		return []forms.Field{
			text("name", "name", fl.Name),
			text("queries", "queries (comma-sep)", strings.Join(fl.Queries, ", ")),
		}
	case directiveRole:
		rd := st.Roles[name]
		return []forms.Field{
			text("name", "name", rd.Name),
			text("flights", "flights (comma-sep)", strings.Join(rd.Flights, ", ")),
			text("queries", "queries (comma-sep)", strings.Join(rd.Queries, ", ")),
			text("filters", "filters (comma-sep)", strings.Join(rd.Filters, ", ")),
		}
	}
	return nil
}

func (v *directiveFormView) Title() string {
	if v.orig == "" {
		return "new " + directiveSingular(v.kind)
	}
	return "edit " + directiveSingular(v.kind)
}

func (v *directiveFormView) Init() tea.Cmd        { return nil }
func (v *directiveFormView) Context() [][2]string { return v.ctx }
func (v *directiveFormView) Hints() [][2]string {
	return [][2]string{{"↑/↓", "field"}, {"←/→", "adjust"}, {"ctrl+s", "save"}, {"esc", "cancel"}}
}

func (v *directiveFormView) Update(a *deck.State, msg tea.Msg) tea.Cmd {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil
	}
	act, ok := keymap.Form().Action(key.String())
	if !ok {
		if key.String() == " " {
			v.form.Insert(" ")
		} else if key.Type == tea.KeyRunes {
			v.form.Insert(string(key.Runes))
		}
		return nil
	}
	switch act {
	case keys.Cancel:
		return a.Pop()
	case keymap.Save:
		return v.submit(a)
	default:
		v.form.Handle(act)
	}
	return nil
}

func (v *directiveFormView) Body(width, _ int) string {
	return v.form.Render(layout.NewFrame(width), v.Title())
}

func (v *directiveFormView) submit(a *deck.State) tea.Cmd {
	vals := v.form.Values()
	name := directiveStr(vals, "name")
	if name == "" {
		return a.Push(deck.NewMessage("save failed", "name is required", v.ctx))
	}

	var item any
	switch v.kind {
	case directiveQuery:
		q := config.Query{Name: name, Signal: directiveStr(vals, "signal")}
		if qp := directiveStr(vals, "query"); qp != "" {
			q.Params = map[string]string{"query": qp}
		}
		for _, ref := range directiveSplit(directiveStr(vals, "filters")) {
			q.Filters = append(q.Filters, config.QueryFilter{Ref: ref})
		}
		item = q
	case directiveFilter:
		f := filter.Filter{Name: name}
		r := filter.Rule{
			Field:   directiveStr(vals, "field"),
			Include: directiveStr(vals, "include"),
			Exclude: directiveStr(vals, "exclude"),
		}
		if r.Field != "" || r.Include != "" || r.Exclude != "" {
			f.Rules = []filter.Rule{r}
		}
		item = f
	case directiveFlight:
		item = config.Flight{
			Name:    name,
			Queries: directiveSplit(directiveStr(vals, "queries")),
		}
	case directiveRole:
		item = config.RoleDef{
			Name:    name,
			Flights: directiveSplit(directiveStr(vals, "flights")),
			Queries: directiveSplit(directiveStr(vals, "queries")),
			Filters: directiveSplit(directiveStr(vals, "filters")),
		}
	}

	summary, err := v.kit.directiveWriteFile(v.kind, name, item)
	if err != nil {
		return a.Push(deck.NewMessage("save failed", err.Error(), v.ctx))
	}
	return a.Push(deck.NewMessage("saved", summary, v.ctx))
}

func (kit *Kit) directiveWriteFile(k directiveKind, name string, item any) (string, error) {
	data, err := yaml.Marshal(item)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(kit.d.App.Cfg.Home, directiveDir(k))
	path, err := sconfig.WriteItem(dir, name+".yaml", data)
	if err != nil {
		return "", err
	}
	return "wrote " + path + "\n\n" +
		"DuckDB is the source of truth; this file takes effect after\n" +
		"reconcile: run `munin import " + directiveDir(k) + "` or restart munin.", nil
}
