package views

import (
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"

	vkdeck "github.com/codyconfer/viewkit/deck"
	"github.com/codyconfer/viewkit/forms"
	"github.com/codyconfer/viewkit/keys"

	"github.com/codyconfer/mino/internal/app/verify"
	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/render/glyph"
	"github.com/codyconfer/mino/internal/signals"
)

const reportNoFlight = "(none — save only)"

func (kit *Kit) reportsCtx() []keys.Hint {
	return append(kit.menuCtx(), keys.Hint{Key: "directive", Label: "Reports"})
}

func (kit *Kit) Reports() vkdeck.View {
	items := []vkdeck.MenuItem{{
		Label:    "New",
		Desc:     "compose, render, and save a new report",
		Icon:     glyph.Builder(),
		OnSelect: func(a *vkdeck.Model) tea.Cmd { return a.Push(kit.ReportBuilder()) },
	}}
	for _, n := range kit.d.App.VisibleFormatters() {
		items = append(items, vkdeck.MenuItem{
			Label:    n,
			Desc:     formatterSummary(kit.d.App.Dirs().Formatters[n]),
			OnSelect: func(a *vkdeck.Model) tea.Cmd { return a.Push(kit.ReportEditor(n)) },
		})
	}
	return vkdeck.NewMenu("reports", kit.reportsCtx(), items...)
}

func formatterSummary(fd config.FormatterDef) string {
	if fd.Title != "" {
		return fd.Title
	}
	body := strings.TrimRight(fd.Template, "\n")
	if body == "" {
		return "no template"
	}
	n := len(strings.Split(body, "\n"))
	if n == 1 {
		return "1 line"
	}
	return strconv.Itoa(n) + " lines"
}

type reportView struct {
	*editorShell

	kit     *Kit
	flights []string
	held    textHolder

	orig string
	base config.FormatterDef
}

func (kit *Kit) ReportBuilder() vkdeck.View {
	return kit.newReportView("", config.FormatterDef{})
}

func (kit *Kit) ReportEditor(name string) vkdeck.View {
	return kit.newReportView(name, kit.d.App.Dirs().Formatters[name])
}

func (kit *Kit) newReportView(orig string, base config.FormatterDef) *reportView {
	v := &reportView{
		kit:     kit,
		flights: append([]string{reportNoFlight}, kit.d.App.VisibleFlights()...),
		orig:    orig,
		base:    base,
	}
	v.editorShell = newEditorShell(v, map[string]any{
		"name":     base.Name,
		"title":    base.Title,
		"template": base.Template,
	}, kit.scope().Keys)
	return v
}

func (v *reportView) editorKind() string { return "report" }

func (v *reportView) editorSync() bool { return false }

func (v *reportView) editorTitle() string {
	if v.orig != "" {
		return "edit " + v.orig
	}
	return "build report"
}

func (v *reportView) editorCtx() []keys.Hint {
	ctx := v.kit.reportsCtx()
	if v.orig != "" {
		ctx = append(ctx, keys.Hint{Key: "item", Label: v.orig})
	}
	if fl := v.flight(); fl != "" {
		ctx = append(ctx, keys.Hint{Key: "on", Label: fl})
	}
	return ctx
}

func (v *reportView) editorSavedName() string { return v.orig }

func (v *reportView) editorFields(prev map[string]any) []forms.Field {
	return []forms.Field{
		{Key: "flight", Label: "render on", Kind: forms.FieldSelect, Options: v.flights},
		{Key: "template", Label: "template", Kind: forms.FieldMultiline, Text: forms.Raw(prev, "template")},
		{Key: "title", Label: "title", Kind: forms.FieldText, Text: forms.Raw(prev, "title")},
		{Key: "name", Label: "name (required to save)", Kind: forms.FieldText, Text: forms.Raw(prev, "name")},
	}
}

func (v *reportView) flight() string {
	idx := v.SelectedOf("flight")
	if idx <= 0 || idx >= len(v.flights) {
		return ""
	}
	return v.flights[idx]
}

func (v *reportView) editorSummary() string {
	fd := v.formatter()
	var parts []string
	if fd.Name != "" {
		parts = append(parts, "name="+fd.Name)
	}
	if fl := v.flight(); fl != "" {
		parts = append(parts, "on="+fl)
	}
	if fd.Template != "" {
		parts = append(parts, formatterSummary(fd))
	}
	if len(parts) == 0 {
		return "unsaved draft"
	}
	return strings.Join(parts, "  ")
}

func (v *reportView) formatter() config.FormatterDef {
	fd := v.base
	fd.Name = strings.TrimSpace(v.Value("name"))
	fd.Title = strings.TrimSpace(v.Value("title"))
	fd.Template = v.Value("template")
	return fd
}

func (v *reportView) editorValue() (any, error) {
	fd := v.formatter()
	if fd.Template == "" {
		return config.FormatterDef{}, errs.New(errs.KindConfig, "a report needs a template").
			WithHint("use text/template over the results, e.g. {{ range .Sections }}")
	}
	return fd, nil
}

type reportRender struct {
	label  string
	flight string
	run    func() (string, error)
}

func (v *reportView) renderer() (reportRender, error) {
	val, err := v.editorValue()
	if err != nil {
		return reportRender{}, err
	}
	fd, _ := val.(config.FormatterDef)
	flight := v.flight()
	if flight == "" {
		return reportRender{}, errs.New(errs.KindUsage, "pick a flight to render on").
			WithHint("choose one with ←/→ on the first field")
	}
	fetch := v.kit.d.FetchFlightQueries
	if fetch == nil {
		return reportRender{}, errs.New(errs.KindInternal, "flight runs are unavailable in this session")
	}
	render := v.kit.d.RenderReport
	if render == nil {
		return reportRender{}, errs.New(errs.KindInternal, "rendering is unavailable in this session")
	}
	label := fd.Name
	if label == "" {
		label = editorAdhocLabel
	}
	queries := v.kit.d.App.Dirs().Flights[flight].Queries
	return reportRender{
		label:  label,
		flight: flight,
		run: func() (string, error) {
			return render(fd, flight, fetch(flight, queries))
		},
	}, nil
}

func (v *reportView) editorRun() (string, func() []signals.Section, error) {
	render, err := v.renderer()
	if err != nil {
		return "", nil, err
	}
	held := &v.held
	return render.label, func() []signals.Section {
		text, err := render.run()
		if err != nil {
			held.clear()
			return []signals.Section{{Signal: "report", Title: "render: " + render.flight, Err: err}}
		}
		held.set(text)
		return []signals.Section{reportSection(render.flight, text)}
	}, nil
}

func reportSection(flight, text string) signals.Section {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	items := make([]signals.Item, 0, len(lines))
	for _, ln := range lines {
		items = append(items, signals.Item{Kind: "line", Title: ln})
	}
	return signals.Section{Signal: "report", Title: "report on " + flight, Items: items}
}

func (v *reportView) editorVerify(val any) Finding {
	fd, _ := val.(config.FormatterDef)
	name := fd.Name
	if name == "" {
		name = editorAdhocLabel
	}
	return verify.Formatter(name, fd)
}

func (v *reportView) editorPersist(val any) (string, error) {
	fd, _ := val.(config.FormatterDef)
	if fd.Name == "" {
		return "", errs.New(errs.KindUsage, "name is required to save")
	}
	d := v.kit.d.App.Dirs()
	if fd.Name != v.orig {
		if _, exists := d.Formatters[fd.Name]; exists {
			return "", errs.Newf(errs.KindUsage, "a report named %s already exists", fd.Name)
		}
	}
	rel := d.Source(config.TypeFormatter, v.orig)
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

func (v *reportView) editorRemove() (string, error) {
	return v.kit.deleteDirective(config.TypeFormatter, v.orig)
}

func (v *reportView) CopyOutput() (string, error) {
	text, err := v.rendered()
	if err != nil {
		return "", err
	}
	if v.kit.d.CopyText == nil {
		return "", errs.New(errs.KindInternal, "no clipboard available in this build")
	}
	if err := v.kit.d.CopyText(text); err != nil {
		return "", errs.Wrap(errs.KindInternal, err, "copying the report")
	}
	return "copied " + strconv.Itoa(len(text)) + " bytes to the clipboard.", nil
}

func (v *reportView) WriteOutput() (string, error) {
	text, err := v.rendered()
	if err != nil {
		return "", err
	}
	if v.kit.d.SaveReport == nil {
		return "", errs.New(errs.KindInternal, "saving is unavailable in this build")
	}
	name := v.formatter().Name
	if name == "" {
		name = editorAdhocLabel
	}
	path, err := v.kit.d.SaveReport(name, text)
	if err != nil {
		return "", err
	}
	return "wrote " + filepath.Clean(path), nil
}

func (v *reportView) rendered() (string, error) {
	text, ok := v.held.get()
	if !ok {
		return "", errs.New(errs.KindUsage, "nothing rendered yet").
			WithHint("render the report with ctrl+r first")
	}
	return text, nil
}

type textHolder struct {
	mu     sync.Mutex
	text   string
	loaded bool
}

func (h *textHolder) set(text string) {
	h.mu.Lock()
	h.text, h.loaded = text, true
	h.mu.Unlock()
}

func (h *textHolder) clear() {
	h.mu.Lock()
	h.text, h.loaded = "", false
	h.mu.Unlock()
}

func (h *textHolder) get() (string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.text, h.loaded
}
