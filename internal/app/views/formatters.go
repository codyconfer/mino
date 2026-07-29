package views

import (
	"path/filepath"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	vkdeck "github.com/codyconfer/viewkit/deck"
	"github.com/codyconfer/viewkit/forms"
	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/munin/internal/app/verify"
	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/render/glyph"
	"github.com/codyconfer/munin/internal/signals"
)

func (kit *Kit) formattersCtx() [][2]string {
	return append(kit.menuCtx(), [2]string{"directive", "Formatters"})
}

func (kit *Kit) Formatters() vkdeck.View {
	var items []vkdeck.MenuItem
	for _, n := range kit.d.App.VisibleFormatters() {
		items = append(items, vkdeck.MenuItem{
			Label: n,
			Desc:  formatterSummary(kit.d.App.Directives.Formatters[n]),
			Icon:  glyph.Builder(),
			Do:    func(a *vkdeck.Model) tea.Cmd { return a.Push(kit.FormatterRunMenu(n)) },
		})
	}
	if len(items) == 0 {
		items = append(items, vkdeck.MenuItem{Label: "(none)", Desc: "no formatters visible in this role"})
	}
	return vkdeck.NewMenu("formatters", kit.formattersCtx(), items...)
}

func (kit *Kit) FormatterDirectives() vkdeck.View {
	items := []vkdeck.MenuItem{{
		Label: "New",
		Desc:  "compose, render, and save a new formatter",
		Icon:  glyph.Builder(),
		Do:    func(a *vkdeck.Model) tea.Cmd { return a.Push(kit.FormatterBuilder()) },
	}}
	for _, n := range kit.d.App.VisibleFormatters() {
		items = append(items, vkdeck.MenuItem{
			Label: n,
			Desc:  formatterSummary(kit.d.App.Directives.Formatters[n]),
			Do:    func(a *vkdeck.Model) tea.Cmd { return a.Push(kit.FormatterEditor(n)) },
		})
	}
	return vkdeck.NewMenu("formatters", kit.formattersCtx(), items...)
}

func formatterSummary(fd config.FormatterDef) string {
	if fd.Title != "" {
		return fd.Title
	}
	n := len(strings.Split(strings.TrimRight(fd.Template, "\n"), "\n"))
	if n == 1 {
		return "1 line"
	}
	return strconv.Itoa(n) + " lines"
}

func (kit *Kit) FormatterRunMenu(formatter string) vkdeck.View {
	ctx := append(kit.formattersCtx(), [2]string{"formatter", formatter})
	var items []vkdeck.MenuItem
	for _, fl := range kit.d.App.VisibleFlights() {
		items = append(items, vkdeck.MenuItem{
			Label: fl,
			Desc:  "run this flight and render the report",
			Do: func(a *vkdeck.Model) tea.Cmd {
				return a.Push(kit.FormatterReport(formatter, fl, func() (string, error) {
					return kit.d.FormatFlight(formatter, fl)
				}))
			},
		})
	}
	if len(items) == 0 {
		items = append(items, vkdeck.MenuItem{Label: "(none)", Desc: "no flights visible in this role"})
	}
	return vkdeck.NewMenu("run: "+formatter, ctx, items...)
}

func (kit *Kit) FormatterReport(formatter, label string, load func() (string, error)) vkdeck.View {
	ctx := append(kit.formattersCtx(), [2]string{"formatter", formatter}, [2]string{"on", label})
	var held reportHolder
	body := func() string {
		text, err := held.load(load)
		if err != nil {
			return theme.Cur().Cant.Render("error: " + err.Error())
		}
		return text
	}
	return vkdeck.NewMenu("report: "+formatter, ctx,
		vkdeck.MenuItem{Label: "View", Desc: "render and read the report", Do: func(a *vkdeck.Model) tea.Cmd {
			return a.Push(vkdeck.NewScroll("report: "+formatter, ctx, nil, func() string {
				text, err := held.load(load)
				if err != nil {
					return theme.Cur().Cant.Render("error: " + err.Error())
				}
				return layout.NewFrame(theme.BodyWidth).Panel(formatter, strings.Split(strings.TrimRight(text, "\n"), "\n")...)
			}))
		}},
		vkdeck.MenuItem{Label: "Copy", Desc: "copy the report to the clipboard", Do: func(a *vkdeck.Model) tea.Cmd {
			return a.Push(vkdeck.NewMessage("copy", kit.copyReport(load), ctx))
		}},
		vkdeck.MenuItem{Label: "Save", Desc: "write the report under <home>/reports", Do: func(a *vkdeck.Model) tea.Cmd {
			return a.Push(vkdeck.NewMessage("save", kit.saveReport(formatter, load), ctx))
		}},
		vkdeck.MenuItem{Label: "Preview", Desc: "first lines of the report", Do: func(a *vkdeck.Model) tea.Cmd {
			return a.Push(vkdeck.NewMessage("preview", firstLines(body(), 12), ctx))
		}},
	)
}

type formatterView struct {
	*editorShell

	kit  *Kit
	orig string
	base config.FormatterDef
}

func (kit *Kit) FormatterBuilder() vkdeck.View {
	return kit.newFormatterView("", config.FormatterDef{})
}

func (kit *Kit) FormatterEditor(name string) vkdeck.View {
	return kit.newFormatterView(name, kit.d.App.Directives.Formatters[name])
}

func (kit *Kit) newFormatterView(orig string, base config.FormatterDef) *formatterView {
	v := &formatterView{kit: kit, orig: orig, base: base}
	v.editorShell = newEditorShell(v, map[string]any{
		"name":     base.Name,
		"title":    base.Title,
		"template": base.Template,
	})
	return v
}

func (v *formatterView) editorKind() string { return "formatter" }

func (v *formatterView) editorSync() bool { return false }

func (v *formatterView) editorTitle() string {
	if v.orig != "" {
		return "edit " + v.orig
	}
	return "build formatter"
}

func (v *formatterView) editorCtx() [][2]string {
	ctx := v.kit.formattersCtx()
	if v.orig != "" {
		ctx = append(ctx, [2]string{"item", v.orig})
	}
	return ctx
}

func (v *formatterView) editorSavedName() string { return v.orig }

func (v *formatterView) editorFields(prev map[string]any) []forms.Field {
	return []forms.Field{
		{Key: "template", Label: "template", Kind: forms.FieldMultiline, Text: forms.Raw(prev, "template")},
		{Key: "title", Label: "title", Kind: forms.FieldText, Text: forms.Raw(prev, "title")},
		{Key: "name", Label: "name (required to save)", Kind: forms.FieldText, Text: forms.Raw(prev, "name")},
	}
}

func (v *formatterView) editorSummary() string {
	fd := v.formatter()
	var parts []string
	if fd.Name != "" {
		parts = append(parts, "name="+fd.Name)
	}
	if fd.Template != "" {
		parts = append(parts, formatterSummary(fd))
	}
	if len(parts) == 0 {
		return "unsaved draft"
	}
	return strings.Join(parts, "  ")
}

func (v *formatterView) formatter() config.FormatterDef {
	fd := v.base
	fd.Name = strings.TrimSpace(v.Value("name"))
	fd.Title = strings.TrimSpace(v.Value("title"))
	fd.Template = v.Value("template")
	return fd
}

func (v *formatterView) editorValue() (any, error) {
	fd := v.formatter()
	if fd.Template == "" {
		return config.FormatterDef{}, errs.New(errs.KindConfig, "a formatter needs a template").
			WithHint("use text/template over the report, e.g. {{ range .Sections }}")
	}
	return fd, nil
}

func (v *formatterView) editorRun() (string, func() []signals.Section, error) {
	val, err := v.editorValue()
	if err != nil {
		return "", nil, err
	}
	fd, _ := val.(config.FormatterDef)
	render := v.kit.d.FormatSections
	if render == nil {
		return "", nil, errs.New(errs.KindInternal, "rendering is unavailable in this session")
	}
	fetch := v.kit.d.FetchFlightQueries
	if fetch == nil {
		return "", nil, errs.New(errs.KindInternal, "flight runs are unavailable in this session")
	}
	flights := v.kit.d.App.VisibleFlights()
	if len(flights) == 0 {
		return "", nil, errs.New(errs.KindUsage, "no flight is visible in this role to render against")
	}
	flight := flights[0]
	label := fd.Name
	if label == "" {
		label = editorAdhocLabel
	}
	return label, func() []signals.Section {
		sections := fetch(flight, v.kit.d.App.Directives.Flights[flight].Queries)
		text, err := render(fd.Name, flight, sections)
		if err != nil {
			return []signals.Section{{Signal: "formatter", Title: "render: " + flight, Err: err}}
		}
		return []signals.Section{formatterReportSection(flight, text)}
	}, nil
}

func formatterReportSection(flight, text string) signals.Section {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	items := make([]signals.Item, 0, len(lines))
	for _, ln := range lines {
		items = append(items, signals.Item{Kind: "line", Title: ln})
	}
	return signals.Section{Signal: "formatter", Title: "report on " + flight, Items: items}
}

func (v *formatterView) editorVerify(val any) Finding {
	fd, _ := val.(config.FormatterDef)
	name := fd.Name
	if name == "" {
		name = editorAdhocLabel
	}
	return verify.Formatter(name, fd)
}

func (v *formatterView) editorPersist(val any) (string, error) {
	fd, _ := val.(config.FormatterDef)
	if fd.Name == "" {
		return "", errs.New(errs.KindUsage, "name is required to save")
	}
	if fd.Name != v.orig {
		if _, exists := v.kit.d.App.Directives.Formatters[fd.Name]; exists {
			return "", errs.Newf(errs.KindUsage, "a formatter named %s already exists", fd.Name)
		}
	}
	rel := v.kit.d.App.Directives.Source(config.TypeFormatter, v.orig)
	summary, _, err := v.kit.saveDirective(config.TypeFormatter, rel, fd.Name, fd)
	if err != nil {
		return "", err
	}
	if fd.Name != v.orig && v.orig != "" {
		summary += editorRenameNote(v.orig, rel)
	}
	v.orig = fd.Name
	v.base = fd
	if err := v.kit.d.App.RefreshDirectives(config.ReconcileIgnore); err == nil {
		summary += "\nit is live in this session."
	}
	return summary, nil
}

func (v *formatterView) editorRemove() string {
	return v.kit.deleteDirective(config.TypeFormatter, v.orig)
}

type reportHolder struct {
	text   string
	err    error
	loaded bool
}

func (h *reportHolder) load(fn func() (string, error)) (string, error) {
	if !h.loaded {
		h.text, h.err = fn()
		h.loaded = true
	}
	return h.text, h.err
}

func firstLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[:n], "\n") + "\n…"
}

func (kit *Kit) copyReport(load func() (string, error)) string {
	text, err := load()
	if err != nil {
		return "render failed: " + err.Error()
	}
	if kit.d.CopyText == nil {
		return "no clipboard available in this build"
	}
	if err := kit.d.CopyText(text); err != nil {
		return "copy failed: " + err.Error()
	}
	return "copied " + strconv.Itoa(len(text)) + " bytes to the clipboard."
}

func (kit *Kit) saveReport(formatter string, load func() (string, error)) string {
	text, err := load()
	if err != nil {
		return "render failed: " + err.Error()
	}
	if kit.d.SaveReport == nil {
		return "saving is unavailable in this build"
	}
	path, err := kit.d.SaveReport(formatter, text)
	if err != nil {
		return "save failed: " + err.Error()
	}
	return "wrote " + filepath.Clean(path)
}
