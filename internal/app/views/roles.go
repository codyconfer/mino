package views

import (
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/codyconfer/viewkit/forms"

	vkdeck "github.com/codyconfer/viewkit/deck"

	"github.com/codyconfer/munin/internal/app"
	"github.com/codyconfer/munin/internal/app/suggest"
	"github.com/codyconfer/munin/internal/app/verify"
	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/render/glyph"
	"github.com/codyconfer/munin/internal/signals"
)

func (kit *Kit) rolesCtx() [][2]string {
	return append(kit.menuCtx(), [2]string{"directive", "Roles"})
}

func (kit *Kit) Roles() vkdeck.View {
	items := []vkdeck.MenuItem{{
		Label: "New",
		Desc:  "compose, dry-run, and save a new role",
		Icon:  glyph.Builder(),
		Do:    func(a *vkdeck.Model) tea.Cmd { return a.Push(kit.RoleBuilder()) },
	}}
	for _, n := range kit.d.App.Directives.RoleNames() {
		items = append(items, vkdeck.MenuItem{
			Label: n,
			Desc:  roleSummary(kit.d.App.Directives.Roles[n]),
			Do:    func(a *vkdeck.Model) tea.Cmd { return a.Push(kit.RoleEditor(n)) },
		})
	}
	return vkdeck.NewMenu("roles", kit.rolesCtx(), items...)
}

func roleSummary(rd config.RoleDef) string {
	var parts []string
	if rd.Home != "" {
		parts = append(parts, "home="+rd.Home)
	}
	if n := len(rd.Flights); n > 0 {
		parts = append(parts, "flights="+strconv.Itoa(n))
	}
	if n := len(rd.Queries); n > 0 {
		parts = append(parts, "queries="+strconv.Itoa(n))
	}
	if n := len(rd.Formatters); n > 0 {
		parts = append(parts, "formatters="+strconv.Itoa(n))
	}
	if len(parts) == 0 {
		return "empty"
	}
	return strings.Join(parts, "  ")
}

type roleView struct {
	*editorShell

	kit  *Kit
	orig string
	base config.RoleDef
}

func (kit *Kit) RoleBuilder() vkdeck.View { return kit.newRoleView("", config.RoleDef{}) }

func (kit *Kit) RoleEditor(name string) vkdeck.View {
	return kit.newRoleView(name, kit.d.App.Directives.Roles[name])
}

func (kit *Kit) newRoleView(orig string, base config.RoleDef) *roleView {
	v := &roleView{kit: kit, orig: orig, base: base}
	v.editorShell = newEditorShell(v, map[string]any{
		"name":       base.Name,
		"home":       base.Home,
		"flights":    strings.Join(base.Flights, ", "),
		"queries":    strings.Join(base.Queries, ", "),
		"formatters": strings.Join(base.Formatters, ", "),
	})
	return v
}

func (v *roleView) editorKind() string { return "role" }

func (v *roleView) editorTitle() string {
	if v.orig != "" {
		return "edit " + v.orig
	}
	return "build role"
}

func (v *roleView) editorCtx() [][2]string {
	ctx := v.kit.rolesCtx()
	if v.orig != "" {
		ctx = append(ctx, [2]string{"item", v.orig})
	}
	return ctx
}

func (v *roleView) editorSavedName() string { return v.orig }

func (v *roleView) editorFields(prev map[string]any) []forms.Field {
	return []forms.Field{
		{
			Key:     "home",
			Label:   "home flight (shown on the home screen)",
			Kind:    forms.FieldText,
			Text:    forms.Raw(prev, "home"),
			Suggest: suggest.Flights(v.kit.d.App),
		},
		{
			Key:     "flights",
			Label:   "flights (comma-sep)",
			Kind:    forms.FieldText,
			Text:    forms.Raw(prev, "flights"),
			Suggest: suggest.Flights(v.kit.d.App),
			Delim:   ",",
		},
		{
			Key:     "queries",
			Label:   "queries (comma-sep)",
			Kind:    forms.FieldText,
			Text:    forms.Raw(prev, "queries"),
			Suggest: suggest.Queries(v.kit.d.App),
			Delim:   ",",
		},
		{
			Key:     "formatters",
			Label:   "formatters (comma-sep)",
			Kind:    forms.FieldText,
			Text:    forms.Raw(prev, "formatters"),
			Suggest: suggest.Formatters(v.kit.d.App),
			Delim:   ",",
		},
		{Key: "name", Label: "name (required to save)", Kind: forms.FieldText, Text: forms.Raw(prev, "name")},
	}
}

func (v *roleView) editorSync() bool { return false }

func (v *roleView) editorSummary() string {
	rd := v.draft()
	var parts []string
	if rd.Name != "" {
		parts = append(parts, "name="+rd.Name)
	}
	if s := roleSummary(rd); s != "empty" {
		parts = append(parts, s)
	}
	if len(parts) == 0 {
		return "unsaved draft"
	}
	return strings.Join(parts, "  ")
}

func (v *roleView) draft() config.RoleDef {
	rd := v.base
	rd.Name = strings.TrimSpace(v.Value("name"))
	rd.Home = strings.TrimSpace(v.Value("home"))
	rd.Flights = directiveSplit(v.Value("flights"))
	rd.Queries = directiveSplit(v.Value("queries"))
	rd.Formatters = directiveSplit(v.Value("formatters"))
	return rd
}

func (v *roleView) role() (config.RoleDef, error) {
	rd := v.draft()
	st := v.kit.d.App.Directives

	var unknown []string
	if rd.Home != "" {
		if _, ok := st.Flights[rd.Home]; !ok {
			unknown = append(unknown, "home flight "+rd.Home)
		}
	}
	for _, n := range rd.Flights {
		if _, ok := st.Flights[n]; !ok {
			unknown = append(unknown, "flight "+n)
		}
	}
	for _, n := range rd.Queries {
		if _, ok := st.Queries[n]; !ok {
			unknown = append(unknown, "query "+n)
		}
	}
	for _, n := range rd.Formatters {
		if _, ok := st.Formatters[n]; !ok {
			unknown = append(unknown, "formatter "+n)
		}
	}
	if len(unknown) > 0 {
		return config.RoleDef{}, errs.Newf(errs.KindConfig, "references undefined: %s", strings.Join(unknown, ", ")).
			WithHint("flights: %s", strings.Join(st.FlightNames(), ", "))
	}
	return rd, nil
}

func (v *roleView) editorValue() (any, error) { return v.role() }

func (v *roleView) editorRun() (string, func() []signals.Section, error) {
	rd, err := v.role()
	if err != nil {
		return "", nil, err
	}
	home := rd.Home
	if home == "" && len(rd.Flights) > 0 {
		home = rd.Flights[0]
	}
	if home == "" {
		return "", nil, errs.New(errs.KindUsage, "this role lists no flights to run").
			WithHint("set a home flight, or add one to flights")
	}
	fl, ok := v.kit.d.App.Directives.Flights[home]
	if !ok {
		return "", nil, errs.Newf(errs.KindConfig, "unknown flight: %s", home)
	}
	fetch := v.kit.d.FetchFlightQueries
	if fetch == nil {
		return "", nil, errs.New(errs.KindInternal, "flight runs are unavailable in this session")
	}
	preview := v.kit.d.PreviewRole
	if preview == nil {
		return "", nil, errs.New(errs.KindInternal, "role dry-runs are unavailable in this session")
	}
	label := rd.Name
	if label == "" {
		label = editorAdhocLabel
	}
	return label, func() []signals.Section {
		var out []signals.Section
		steps := preview(rd, app.RolePreviewHold, func() app.RolePreviewStep {
			out = fetch(home, fl.Queries)
			n := 0
			for _, sec := range out {
				n += len(sec.Items)
			}
			return app.RolePreviewStep{Label: "flight: " + home, Detail: strconv.Itoa(n) + " items"}
		})
		return append([]signals.Section{rolePreviewSection(label, steps)}, out...)
	}, nil
}

func rolePreviewSection(label string, steps []app.RolePreviewStep) signals.Section {
	items := make([]signals.Item, 0, len(steps))
	for _, s := range steps {
		detail := s.Detail
		switch {
		case s.Err != nil:
			detail = s.Err.Error()
		case detail == "":
			detail = "ok"
		}
		items = append(items, signals.Item{Kind: "step", Title: s.Label, Subtitle: detail})
	}
	return signals.Section{Signal: "role", Title: "dry-run: " + label, Items: items}
}

func (v *roleView) editorVerify(val any) Finding {
	rd, _ := val.(config.RoleDef)
	name := rd.Name
	if name == "" {
		name = editorAdhocLabel
	}
	return verify.Role(v.kit.d.App.Directives, name, rd)
}

func (v *roleView) editorPersist(val any) (string, error) {
	rd, _ := val.(config.RoleDef)
	if rd.Name == "" {
		return "", errs.New(errs.KindUsage, "name is required to save")
	}
	if rd.Name != v.orig {
		if _, exists := v.kit.d.App.Directives.Roles[rd.Name]; exists {
			return "", errs.Newf(errs.KindUsage, "a role named %s already exists", rd.Name)
		}
	}
	rel := v.kit.d.App.Directives.Source(config.TypeRole, v.orig)
	summary, _, err := v.kit.saveDirective(config.TypeRole, rel, rd.Name, rd)
	if err != nil {
		return "", err
	}
	if rd.Name != v.orig && v.orig != "" {
		summary += editorRenameNote(v.orig, rel)
	}
	v.orig = rd.Name
	v.base = rd
	if err := v.kit.d.App.RefreshDirectives(config.ReconcileIgnore); err == nil {
		summary += "\nit is live in this session."
	}
	return summary, nil
}

func (v *roleView) editorRemove() string {
	return v.kit.deleteDirective(config.TypeRole, v.orig)
}
