package views

import (
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/codyconfer/viewkit/forms"

	vkdeck "github.com/codyconfer/viewkit/deck"

	"github.com/codyconfer/munin/internal/app/verify"
	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/filter"
	"github.com/codyconfer/munin/internal/plugin"
	"github.com/codyconfer/munin/internal/render/glyph"
	"github.com/codyconfer/munin/internal/signals"
	"github.com/codyconfer/munin/internal/signals/build"
)

const (
	builderParamPrefix = "param."
	builderNoSignal    = "(none — filter only)"
)

var builderTypes = []string{"(infer)", string(config.TypeQuery), string(config.TypeFilter)}

func (kit *Kit) queriesCtx() [][2]string {
	return append(kit.menuCtx(), [2]string{"directive", "Queries"})
}

func (kit *Kit) Queries() vkdeck.View {
	items := []vkdeck.MenuItem{{
		Label: "New",
		Desc:  "build, run, and save a new query or filter",
		Icon:  glyph.Builder(),
		Do:    func(a *vkdeck.Model) tea.Cmd { return a.Push(kit.QueryBuilder()) },
	}}
	for _, n := range kit.d.App.VisibleQueries() {
		n := n
		items = append(items, vkdeck.MenuItem{
			Label: n,
			Desc:  querySummary(kit.d.App.Directives.Queries[n]),
			Do:    func(a *vkdeck.Model) tea.Cmd { return a.Push(kit.QueryEditor(n)) },
		})
	}
	return vkdeck.NewMenu("queries", kit.queriesCtx(), items...)
}

func querySummary(q config.Query) string {
	var parts []string
	if q.Runnable() {
		parts = append(parts, "signal="+q.Signal)
	} else {
		parts = append(parts, "filter-only")
	}
	if q.HasRules() {
		parts = append(parts, "+rules")
	}
	if len(q.Filters) > 0 {
		parts = append(parts, "+filters")
	}
	if q.Title != "" {
		parts = append(parts, q.Title)
	}
	return strings.Join(parts, "  ")
}

func (kit *Kit) deleteQuery(name string) string {
	home := kit.d.App.Cfg.Home
	removed, err := config.RemoveCollectionItem(home, config.DirQueries, name)
	if err != nil {
		return err.Error()
	}
	if len(removed) == 0 {
		return "no file found for " + name + " in " + config.CollectionDir(home, config.DirQueries) + ".\n\n" +
			"It may exist only in DuckDB; use `munin export queries` to write files first."
	}
	summary := "removed:\n  " + strings.Join(removed, "\n  ")
	stored, err := config.SyncCollection(kit.d.App.Mgr, home, config.DirQueries)
	switch {
	case err != nil:
		return summary + "\n\nthe store still holds it: " + err.Error()
	case !stored:
		return summary + "\n\nthe config store is unavailable, so this takes effect after\n" +
			"reconcile: run `munin import queries` or restart munin."
	}
	if err := kit.d.App.RefreshDirectives(config.ReconcileIgnore); err != nil {
		return summary + "\nremoved from DuckDB.\n\nreload failed: " + err.Error()
	}
	return summary + "\nremoved from DuckDB; the change is live in this session."
}

func (kit *Kit) directiveWriteQuery(q config.Query) (string, error) {
	path, stored, err := config.SaveCollectionItem(kit.d.App.Mgr, kit.d.App.Cfg.Home, config.DirQueries, q.Name, q)
	if err != nil {
		return "", err
	}
	if !stored {
		return "wrote " + path + "\n\n" +
			"the config store is unavailable, so this file takes effect after\n" +
			"reconcile: run `munin import queries` or restart munin.", nil
	}
	return "wrote " + path + "\nimported the queries collection into DuckDB.", nil
}

type builderView struct {
	*editorShell

	kit     *Kit
	signals []string
	sigIdx  int
	typeIdx int

	orig string
	base config.Query
}

func (kit *Kit) QueryBuilder() vkdeck.View {
	return kit.newBuilder("", config.Query{})
}

func (kit *Kit) QueryEditor(name string) vkdeck.View {
	return kit.newBuilder(name, kit.d.App.Directives.Queries[name])
}

func (kit *Kit) newBuilder(orig string, base config.Query) *builderView {
	v := &builderView{kit: kit, signals: kit.builderSignals(), orig: orig, base: base}
	v.sigIdx = forms.SelectIndex(v.signals, base.Signal)
	v.typeIdx = forms.SelectIndex(builderTypes, string(base.Type))
	v.editorShell = newEditorShell(v, v.seed())
	return v
}

func (kit *Kit) builderSignals() []string {
	var named []string
	for name := range build.BuilderSignals() {
		if plugin.SignalEnabled(name) {
			named = append(named, name)
		}
	}
	sort.Strings(named)
	return append([]string{builderNoSignal}, named...)
}

func (v *builderView) docType() config.DirectiveType {
	if v.typeIdx <= 0 || v.typeIdx >= len(builderTypes) {
		return config.TypeAuto
	}
	return config.DirectiveType(builderTypes[v.typeIdx])
}

func (v *builderView) isFilter() bool { return v.docType() == config.TypeFilter }

func (v *builderView) signal() string {
	if v.isFilter() || v.sigIdx <= 0 || v.sigIdx >= len(v.signals) {
		return ""
	}
	return v.signals[v.sigIdx]
}

func (v *builderView) seed() map[string]any {
	out := map[string]any{
		"name":    v.base.Name,
		"title":   v.base.Title,
		"filters": strings.Join(builderFilterRefs(v.base.Filters), ", "),
	}
	if len(v.base.Rules) > 0 {
		out["field"] = v.base.Rules[0].Field
		out["include"] = v.base.Rules[0].Include
		out["exclude"] = v.base.Rules[0].Exclude
	}
	known := map[string]bool{}
	for _, p := range build.QueryParams(v.base.Signal) {
		known[p.Key] = true
		out[builderParamPrefix+p.Key] = v.base.Params[p.Key]
	}
	var extra []string
	for _, k := range sortedParamKeys(v.base.Params) {
		if !known[k] {
			extra = append(extra, k+"="+v.base.Params[k])
		}
	}
	out["extra"] = strings.Join(extra, ", ")
	return out
}

func sortedParamKeys(params map[string]string) []string {
	out := make([]string, 0, len(params))
	for k := range params {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func builderFilterRefs(refs []config.QueryFilter) []string {
	var out []string
	for _, qf := range refs {
		if qf.Ref != "" {
			out = append(out, qf.Ref)
		}
	}
	return out
}

func (v *builderView) fields(prev map[string]any) []forms.Field {
	str := func(key string) string {
		s, _ := prev[key].(string)
		return s
	}
	text := func(key, label string) forms.Field {
		return forms.Field{Key: key, Label: label, Kind: forms.FieldText, Text: str(key)}
	}

	out := []forms.Field{{
		Key:      "type",
		Label:    "type",
		Kind:     forms.FieldSelect,
		Options:  builderTypes,
		Selected: v.typeIdx,
	}}
	if !v.isFilter() {
		out = append(out, forms.Field{
			Key:      "signal",
			Label:    builderSignalLabel(v.docType()),
			Kind:     forms.FieldSelect,
			Options:  v.signals,
			Selected: v.sigIdx,
		})
		for _, p := range build.QueryParams(v.signal()) {
			out = append(out, text(builderParamPrefix+p.Key, builderParamLabel(p)))
		}
		out = append(out,
			text("extra", "extra params (k=v, comma-sep)"),
			text("filters", "filters (comma-sep saved filters)"),
		)
	}
	return append(out,
		text("field", builderRuleLabel(v.isFilter())),
		text("include", "rule include regex"),
		text("exclude", "rule exclude regex"),
		text("name", "name (required to save)"),
		text("title", "display title (optional)"),
	)
}

func builderSignalLabel(t config.DirectiveType) string {
	if t == config.TypeQuery {
		return "signal (required)"
	}
	return "signal (blank = filter only)"
}

func builderRuleLabel(isFilter bool) string {
	if isFilter {
		return "rule field (blank = whole item)"
	}
	return "inline rule field (blank = whole item)"
}

func builderParamLabel(p build.ParamSpec) string {
	if p.Example != "" {
		return p.Key + " (e.g. " + p.Example + ")"
	}
	return p.Key
}

func (v *builderView) editorKind() string { return "query" }

func (v *builderView) editorTitle() string {
	verb := "build query"
	if v.orig != "" {
		verb = "edit " + v.orig
	}
	if sig := v.signal(); sig != "" {
		return verb + " · " + sig
	}
	return verb
}

func (v *builderView) editorCtx() [][2]string {
	ctx := v.kit.queriesCtx()
	if v.orig != "" {
		ctx = append(ctx, [2]string{"item", v.orig})
	}
	if sig := v.signal(); sig != "" {
		ctx = append(ctx, [2]string{"signal", sig})
	}
	return ctx
}

func (v *builderView) editorSavedName() string { return v.orig }

func (v *builderView) editorFields(prev map[string]any) []forms.Field { return v.fields(prev) }

func (v *builderView) editorSync() bool {
	changed := false
	if idx := v.SelectedOf("type"); idx >= 0 && idx != v.typeIdx {
		v.typeIdx = idx
		changed = true
	}
	if idx := v.SelectedOf("signal"); idx >= 0 && idx != v.sigIdx {
		v.sigIdx = idx
		changed = true
	}
	return changed
}

func (v *builderView) editorSummary() string {
	var parts []string
	if t := v.docType(); t != config.TypeAuto {
		parts = append(parts, "type="+string(t))
	}
	if sig := v.signal(); sig != "" {
		parts = append(parts, "signal="+sig)
	}
	if name := v.Value("name"); name != "" {
		parts = append(parts, "name="+name)
	}
	if len(parts) == 0 {
		return "unsaved draft"
	}
	return strings.Join(parts, "  ")
}

func (v *builderView) editorValue() (any, error) {
	return v.query()
}

func (v *builderView) editorRun() (string, func() []signals.Section, error) {
	q, err := v.query()
	if err != nil {
		return "", nil, err
	}
	if !q.Runnable() {
		return "", nil, errs.New(errs.KindUsage, "nothing to run: this document defines no signal")
	}
	fetch := v.kit.d.FetchAdhoc
	if fetch == nil {
		return "", nil, errs.New(errs.KindInternal, "ad-hoc runs are unavailable in this session")
	}
	label := q.Name
	if label == "" {
		label = editorAdhocLabel
	}
	return label, func() []signals.Section { return fetch(q) }, nil
}

func (v *builderView) editorVerify(val any) Finding {
	q, _ := val.(config.Query)
	name := q.Name
	if name == "" {
		name = editorAdhocLabel
	}
	return verify.Query(v.kit.d.App.Directives, name, q)
}

func (v *builderView) editorPersist(val any) (string, error) {
	q, _ := val.(config.Query)
	if q.Name == "" {
		return "", errs.New(errs.KindUsage, "name is required to save")
	}
	if q.Name != v.orig {
		if _, exists := v.kit.d.App.Directives.Queries[q.Name]; exists {
			return "", errs.Newf(errs.KindUsage, "a query named %s already exists", q.Name)
		}
	}
	summary, err := v.kit.directiveWriteQuery(q)
	if err != nil {
		return "", err
	}
	if q.Name != v.orig && v.orig != "" {
		home := v.kit.d.App.Cfg.Home
		if removed, err := config.RemoveCollectionItem(home, config.DirQueries, v.orig); err == nil && len(removed) > 0 {
			_, storeErr := config.SyncCollection(v.kit.d.App.Mgr, home, config.DirQueries)
			summary += editorRenameNote(v.editorKind(), v.orig, removed, storeErr)
		}
	}
	v.orig = q.Name
	v.base = q
	if err := v.kit.d.App.RefreshDirectives(config.ReconcileIgnore); err == nil {
		summary += "\nit is live in this session."
	}
	return summary, nil
}

func (v *builderView) editorRemove() string { return v.kit.deleteQuery(v.orig) }

func (v *builderView) query() (config.Query, error) {
	vals := v.Form().Values()

	q := v.base
	q.Type = v.docType()
	q.Name = forms.Str(vals, "name")
	q.Title = forms.Str(vals, "title")
	q.Signal = v.signal()
	q.Params = nil
	q.Filters = builderInlineFilters(v.base.Filters)

	if !v.isFilter() {
		params := map[string]string{}
		for _, p := range build.QueryParams(v.signal()) {
			if val := forms.Str(vals, builderParamPrefix+p.Key); val != "" {
				params[p.Key] = val
			}
		}
		extra, err := builderParamPairs(forms.Str(vals, "extra"))
		if err != nil {
			return config.Query{}, err
		}
		for k, val := range extra {
			params[k] = val
		}
		if len(params) > 0 {
			q.Params = params
		}
		for _, ref := range directiveSplit(forms.Str(vals, "filters")) {
			if _, ok := v.kit.d.App.Directives.LookupFilter(ref); !ok {
				return config.Query{}, errs.Newf(errs.KindConfig, "unknown filter %q", ref).
					WithHint("pick one of: %s", strings.Join(v.kit.d.App.VisibleFilters(), ", "))
			}
			q.Filters = append(q.Filters, config.QueryFilter{Ref: ref})
		}
	}

	q.Rules = builderRules(v.base.Rules, filter.Rule{
		Field:   forms.Str(vals, "field"),
		Include: forms.Str(vals, "include"),
		Exclude: forms.Str(vals, "exclude"),
	})
	if q.HasRules() {
		if _, err := filter.Compile(q.AsFilter()); err != nil {
			return config.Query{}, err
		}
	}
	switch {
	case q.Type == config.TypeQuery && q.Signal == "":
		return config.Query{}, errs.New(errs.KindConfig, "`type: query` needs a signal").
			WithHint("pick one, or switch type to filter")
	case q.Type == config.TypeFilter && !q.HasRules():
		return config.Query{}, errs.New(errs.KindConfig, "`type: filter` needs at least one rule").
			WithHint("fill in an include or exclude regex")
	case q.Signal == "" && !q.HasRules():
		return config.Query{}, errs.New(errs.KindConfig, "pick a signal, or add a rule to make this a filter").
			WithHint("a document with neither a signal nor filter content does nothing")
	}
	return q, nil
}

func builderInlineFilters(base []config.QueryFilter) []config.QueryFilter {
	var out []config.QueryFilter
	for _, qf := range base {
		if qf.Ref == "" && qf.Inline != nil {
			out = append(out, qf)
		}
	}
	return out
}

func builderRules(base []filter.Rule, edited filter.Rule) []filter.Rule {
	var rest []filter.Rule
	if len(base) > 1 {
		rest = base[1:]
	}
	if edited == (filter.Rule{}) {
		if len(rest) == 0 {
			return nil
		}
		return append([]filter.Rule(nil), rest...)
	}
	return append([]filter.Rule{edited}, rest...)
}

func builderParamPairs(s string) (map[string]string, error) {
	out := map[string]string{}
	for _, pair := range directiveSplit(s) {
		k, val, found := strings.Cut(pair, "=")
		k, val = strings.TrimSpace(k), strings.TrimSpace(val)
		if !found || k == "" {
			return nil, errs.Newf(errs.KindUsage, "extra param %q is not key=value", pair)
		}
		out[k] = val
	}
	return out, nil
}
