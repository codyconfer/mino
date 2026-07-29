package views

import (
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/codyconfer/viewkit/forms"
	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/theme"

	"gopkg.in/yaml.v3"

	vkdeck "github.com/codyconfer/viewkit/deck"

	"github.com/codyconfer/munin/internal/app/suggest"
	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/errs"
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

func directiveType(k directiveKind) config.DirectiveType {
	if k == directiveRole {
		return config.TypeRole
	}
	return config.TypeAuto
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

func directiveNoFileNote(name string) string {
	return "no file on disk for " + name + ".\n\n" +
		"It may exist only in DuckDB (the source of truth); use\n" +
		"`munin export directives` to write files first."
}

func (kit *Kit) directiveMultiDocNote(rel string) string {
	n := kit.d.App.Directives.DocCount(rel)
	if n <= 1 {
		return ""
	}
	return rel + " holds " + strconv.Itoa(n) + " directives; edit it by hand to keep the others intact."
}

func (kit *Kit) saveDirective(kind config.DirectiveType, rel, name string, doc any) (string, bool, error) {
	if note := kit.directiveMultiDocNote(rel); note != "" {
		return "", false, errs.New(errs.KindUsage, note)
	}
	path, stored, err := config.SaveDirective(kit.d.App.Mgr, kit.d.App.Cfg.Home, rel, kind, name, doc)
	if err != nil {
		return "", false, err
	}
	if !stored {
		return "wrote " + path + "\n\n" +
			"the config store is unavailable, so this file takes effect after\n" +
			"reconcile: run `munin import directives` or restart munin.", false, nil
	}
	return "wrote " + path + "\nimported the directives collection into DuckDB.", true, nil
}

func (kit *Kit) removeDirective(kind config.DirectiveType, name string) ([]string, string) {
	rel := kit.d.App.Directives.Source(kind, name)
	if rel == "" {
		return nil, directiveNoFileNote(name)
	}
	if note := kit.directiveMultiDocNote(rel); note != "" {
		return nil, "did not remove " + name + ": " + note
	}
	removed, err := config.RemoveDirective(kit.d.App.Cfg.Home, rel)
	if err != nil {
		return nil, err.Error()
	}
	if len(removed) == 0 {
		return nil, directiveNoFileNote(name)
	}
	return removed, ""
}

func (kit *Kit) deleteDirective(kind config.DirectiveType, name string) string {
	removed, note := kit.removeDirective(kind, name)
	if note != "" {
		return note
	}
	summary := "removed:\n  " + strings.Join(removed, "\n  ")
	stored, err := config.SyncDirectives(kit.d.App.Mgr, kit.d.App.Cfg.Home)
	switch {
	case err != nil:
		return summary + "\n\nthe store still holds it: " + err.Error()
	case !stored:
		return summary + "\n\nthe config store is unavailable, so this takes effect after\n" +
			"reconcile: run `munin import directives` or restart munin."
	}
	if err := kit.d.App.RefreshDirectives(config.ReconcileIgnore); err != nil {
		return summary + "\nremoved from DuckDB.\n\nreload failed: " + err.Error()
	}
	return summary + "\nremoved from DuckDB; the change is live in this session."
}

func (kit *Kit) directiveDeleteFile(k directiveKind, name string) string {
	removed, note := kit.removeDirective(directiveType(k), name)
	if note != "" {
		return note
	}
	return "removed:\n  " + strings.Join(removed, "\n  ") + "\n\n" +
		"DuckDB remains the source of truth; this takes effect after\n" +
		"reconcile: run `munin import directives` or restart munin."
}

func (kit *Kit) newDirectiveForm(k directiveKind, orig string) vkdeck.View {
	ctx := kit.directiveItemCtx(k, orig)
	title := "new " + directiveSingular(k)
	if orig != "" {
		title = "edit " + directiveSingular(k)
	}
	return vkdeck.NewFormView(vkdeck.FormSpec{
		Fields:  kit.directiveFormFields(k, orig),
		Keys:    vkdeck.FormKeys{Map: keymap.Form(), Save: keymap.Save},
		Title:   title,
		Context: ctx,
		OnSubmit: func(a *vkdeck.Model, vals map[string]any) tea.Cmd {
			return kit.submitDirectiveForm(a, k, ctx, vals)
		},
	})
}

func (kit *Kit) directiveFormFields(k directiveKind, name string) []forms.Field {
	st := kit.d.App.Directives
	switch k {
	case directiveRole:
		rd := st.Roles[name]
		return []forms.Field{
			{Key: "name", Label: "name", Kind: forms.FieldText, Text: rd.Name},
			{
				Key:     "flights",
				Label:   "flights (comma-sep)",
				Kind:    forms.FieldText,
				Text:    strings.Join(rd.Flights, ", "),
				Suggest: suggest.Flights(kit.d.App),
				Delim:   ",",
			},
			{
				Key:     "queries",
				Label:   "queries (comma-sep)",
				Kind:    forms.FieldText,
				Text:    strings.Join(rd.Queries, ", "),
				Suggest: suggest.Queries(kit.d.App),
				Delim:   ",",
			},
		}
	}
	return nil
}

func (kit *Kit) submitDirectiveForm(a *vkdeck.Model, k directiveKind, ctx [][2]string, vals map[string]any) tea.Cmd {
	name := forms.Str(vals, "name")
	if name == "" {
		return a.Push(vkdeck.NewMessage("save failed", "name is required", ctx))
	}

	var item any
	switch k {
	case directiveRole:
		item = config.RoleDef{
			Name:    name,
			Flights: directiveSplit(forms.Str(vals, "flights")),
			Queries: directiveSplit(forms.Str(vals, "queries")),
		}
	}

	summary, err := kit.directiveWriteFile(k, name, item)
	if err != nil {
		return a.Push(vkdeck.NewMessage("save failed", err.Error(), ctx))
	}
	return a.Push(vkdeck.NewMessage("saved", summary, ctx))
}

func (kit *Kit) directiveWriteFile(k directiveKind, name string, item any) (string, error) {
	kind := directiveType(k)
	summary, stored, err := kit.saveDirective(kind, kit.d.App.Directives.Source(kind, name), name, item)
	if err != nil {
		return "", err
	}
	if !stored {
		return summary, nil
	}
	if err := kit.d.App.RefreshDirectives(config.ReconcileIgnore); err != nil {
		return summary + "\n\nreload failed, so it is not live yet: " + err.Error(), nil
	}
	return summary + "\nit is live in this session.", nil
}
