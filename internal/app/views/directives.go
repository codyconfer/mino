package views

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/codyconfer/viewkit/forms"
	"github.com/codyconfer/viewkit/keys"
	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/theme"

	"gopkg.in/yaml.v3"

	vkdeck "github.com/codyconfer/viewkit/deck"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/keymap"
	"github.com/codyconfer/munin/internal/render/glyph"
)

type directiveKind int

const (
	directiveRole directiveKind = iota
)

func directiveLabel(k directiveKind) string {
	if k == directiveRole {
		return "Roles"
	}
	return "?"
}

func directiveSingular(k directiveKind) string {
	if k == directiveRole {
		return "role"
	}
	return "item"
}

func directiveKey(k directiveKind) string {
	if k == directiveRole {
		return config.KindRoles
	}
	return ""
}

func (kit *Kit) directiveNames(k directiveKind) []string {
	if k == directiveRole {
		return kit.d.App.Directives.RoleNames()
	}
	return nil
}

func (kit *Kit) directiveItem(k directiveKind, name string) any {
	if k == directiveRole {
		return kit.d.App.Directives.Roles[name]
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

func (kit *Kit) directivesMenu() vkdeck.View {
	pick := func(k directiveKind) vkdeck.MenuItem {
		return vkdeck.MenuItem{
			Label: directiveLabel(k),
			Desc:  "browse saved " + strings.ToLower(directiveLabel(k)),
			Do:    func(a *vkdeck.Model) tea.Cmd { return a.Push(kit.directiveBrowser(k)) },
		}
	}
	return vkdeck.NewMenu("directives", kit.menuCtx(), pick(directiveRole))
}

func (kit *Kit) directiveBrowser(k directiveKind) vkdeck.View {
	var items []vkdeck.MenuItem
	for _, n := range kit.directiveNames(k) {
		n := n
		items = append(items, vkdeck.MenuItem{
			Label: n,
			Do:    func(a *vkdeck.Model) tea.Cmd { return a.Push(kit.directiveActionsMenu(k, n)) },
		})
	}
	if len(items) == 0 {
		items = append(items, vkdeck.MenuItem{Label: "(none)", Desc: "no saved " + strings.ToLower(directiveLabel(k))})
	}
	items = append(items, vkdeck.MenuItem{
		Label: "Create",
		Desc:  "create a " + directiveSingular(k),
		Do:    func(a *vkdeck.Model) tea.Cmd { return a.Push(kit.newDirectiveForm(k, "")) },
	})
	return vkdeck.NewMenu(directiveLabel(k), kit.directiveCtx(k), items...)
}

func (kit *Kit) directiveActionsMenu(k directiveKind, name string) vkdeck.View {
	ctx := kit.directiveItemCtx(k, name)
	items := []vkdeck.MenuItem{
		{Label: "View", Desc: "show YAML", Do: func(a *vkdeck.Model) tea.Cmd {
			return a.Push(kit.directiveViewContent(k, name))
		}},
	}
	items = append(items,
		vkdeck.MenuItem{Label: "Validate", Desc: "check for problems", Do: func(a *vkdeck.Model) tea.Cmd {
			return a.Push(kit.directiveValidateContent(k, name))
		}},
		vkdeck.MenuItem{Label: "Edit", Desc: "modify and save", Do: func(a *vkdeck.Model) tea.Cmd {
			return a.Push(kit.newDirectiveForm(k, name))
		}},
		vkdeck.MenuItem{Label: "Delete", Desc: "remove the file", Do: func(a *vkdeck.Model) tea.Cmd {
			return a.Push(kit.directiveDeleteConfirm(k, name))
		}},
	)
	return vkdeck.NewMenu(directiveSingular(k)+": "+name, ctx, items...)
}

func (kit *Kit) directiveViewContent(k directiveKind, name string) vkdeck.View {
	return vkdeck.NewScroll("view: "+name, kit.directiveItemCtx(k, name), nil, func() string {
		data, err := yaml.Marshal(kit.directiveItem(k, name))
		if err != nil {
			return theme.Cur().Cant.Render("error: " + err.Error())
		}
		body := strings.TrimRight(string(data), "\n")
		return layout.NewFrame(theme.BodyWidth).Panel(name, strings.Split(body, "\n")...)
	})
}

func (kit *Kit) directiveValidateContent(k directiveKind, name string) vkdeck.View {
	return vkdeck.NewScroll("validate: "+name, kit.directiveItemCtx(k, name), nil, func() string {
		th := theme.Cur()
		findings := kit.d.Verify(config.KindRoles)
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

func (kit *Kit) directiveDeleteConfirm(k directiveKind, name string) vkdeck.View {
	ctx := kit.directiveItemCtx(k, name)
	return vkdeck.NewMenu("delete "+name+"?", ctx,
		vkdeck.MenuItem{Label: "No, keep it", Do: func(a *vkdeck.Model) tea.Cmd { return a.Pop() }},
		vkdeck.MenuItem{Label: "Yes, delete", Desc: "remove the file", Do: func(a *vkdeck.Model) tea.Cmd {
			return a.Push(vkdeck.NewMessage("delete", kit.directiveDeleteFile(k, name), ctx))
		}},
	)
}

func (kit *Kit) directiveDeleteFile(k directiveKind, name string) string {
	key := directiveKey(k)
	home := kit.d.App.Cfg.Home
	removed, err := config.RemoveCollectionItem(home, key, name)
	if err != nil {
		return err.Error()
	}
	if len(removed) == 0 {
		return "no file found for " + name + " in " + config.CollectionDir(home, key) + ".\n\n" +
			"It may exist only in DuckDB (the source of truth); use\n" +
			"`munin export " + key + "` to write files first."
	}
	return "removed:\n  " + strings.Join(removed, "\n  ") + "\n\n" +
		"DuckDB remains the source of truth; this takes effect after\n" +
		"reconcile: run `munin import " + key + "` or restart munin."
}

type directiveFormView struct {
	kit  *Kit
	kind directiveKind
	orig string
	form *forms.Form
	ctx  [][2]string
}

func (kit *Kit) newDirectiveForm(k directiveKind, orig string) vkdeck.View {
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
	case directiveRole:
		rd := st.Roles[name]
		return []forms.Field{
			text("name", "name", rd.Name),
			text("flights", "flights (comma-sep)", strings.Join(rd.Flights, ", ")),
			text("queries", "queries (comma-sep)", strings.Join(rd.Queries, ", ")),
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

func (v *directiveFormView) Update(a *vkdeck.Model, msg tea.Msg) tea.Cmd {
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

func (v *directiveFormView) submit(a *vkdeck.Model) tea.Cmd {
	vals := v.form.Values()
	name := forms.Str(vals, "name")
	if name == "" {
		return a.Push(vkdeck.NewMessage("save failed", "name is required", v.ctx))
	}

	var item any
	switch v.kind {
	case directiveRole:
		item = config.RoleDef{
			Name:    name,
			Flights: directiveSplit(forms.Str(vals, "flights")),
			Queries: directiveSplit(forms.Str(vals, "queries")),
		}
	}

	summary, err := v.kit.directiveWriteFile(v.kind, name, item)
	if err != nil {
		return a.Push(vkdeck.NewMessage("save failed", err.Error(), v.ctx))
	}
	return a.Push(vkdeck.NewMessage("saved", summary, v.ctx))
}

func (kit *Kit) directiveWriteFile(k directiveKind, name string, item any) (string, error) {
	key := directiveKey(k)
	path, stored, err := config.SaveCollectionItem(kit.d.App.Mgr, kit.d.App.Cfg.Home, key, name, item)
	if err != nil {
		return "", err
	}
	if !stored {
		return "wrote " + path + "\n\n" +
			"the config store is unavailable, so this file takes effect after\n" +
			"reconcile: run `munin import " + key + "` or restart munin.", nil
	}
	summary := "wrote " + path + "\nimported the " + key + " collection into DuckDB."
	if err := kit.d.App.RefreshDirectives(config.ReconcileIgnore); err != nil {
		return summary + "\n\nreload failed, so it is not live yet: " + err.Error(), nil
	}
	return summary + "\nit is live in this session.", nil
}
