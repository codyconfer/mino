package views

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	vkdeck "github.com/codyconfer/viewkit/deck"

	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/deck"
	"github.com/codyconfer/mino/internal/filter"
	"github.com/codyconfer/mino/internal/signals"
)

func builderFor(t *testing.T, kit *Kit) *builderView {
	t.Helper()
	v, ok := kit.QueryBuilder().(*builderView)
	if !ok {
		t.Fatal("QueryBuilder did not return a builderView")
	}
	if len(v.signals) == 0 {
		t.Skip("no signal builders enabled in this environment")
	}
	return v
}

func (v *builderView) press(msgs ...tea.KeyMsg) {
	for _, msg := range msgs {
		v.Update(nil, msg)
	}
}

func runeKey(r rune) tea.KeyMsg {
	if r == ' ' {
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

func (v *builderView) focusedKey() string {
	if fd := v.Form().Focused(); fd != nil {
		return fd.Key
	}
	return ""
}

func (v *builderView) focusedIndex() int {
	fm := v.Form()
	fd := fm.Focused()
	if fd == nil {
		return -1
	}
	for i := range fm.Fields {
		if &fm.Fields[i] == fd {
			return i
		}
	}
	return -1
}

func (v *builderView) fieldIndex(t *testing.T, key string) int {
	t.Helper()
	for i := range v.Form().Fields {
		if v.Form().Fields[i].Key == key {
			return i
		}
	}
	t.Fatalf("builder has no field %q (fields: %v)", key, v.fieldKeys())
	return -1
}

func (v *builderView) set(t *testing.T, key, val string) {
	t.Helper()
	v.focus(t, key)
	for range len([]rune(textOf(t, v, key))) {
		v.press(tea.KeyMsg{Type: tea.KeyBackspace})
	}
	if got := textOf(t, v, key); got != "" {
		t.Fatalf("backspace did not clear field %q, still %q", key, got)
	}
	v.typeIn(t, val)
	if got := textOf(t, v, key); got != val {
		t.Fatalf("typing into %q produced %q, want %q", key, got, val)
	}
}

func (v *builderView) fieldKeys() []string {
	out := make([]string, 0, len(v.Form().Fields))
	for i := range v.Form().Fields {
		out = append(out, v.Form().Fields[i].Key)
	}
	return out
}

func (v *builderView) selectOption(t *testing.T, key string, idx int) {
	t.Helper()
	budget := 2 * (len(builderTypes) + len(v.signals) + 1)
	for range budget {
		cur := v.SelectedOf(key)
		switch {
		case cur < 0:
			t.Fatalf("builder has no %q selector (fields: %v)", key, v.fieldKeys())
		case cur == idx:
			return
		}
		v.focus(t, key)
		if cur < idx {
			v.press(tea.KeyMsg{Type: tea.KeyRight})
		} else {
			v.press(tea.KeyMsg{Type: tea.KeyLeft})
		}
		if got := v.SelectedOf(key); got == cur {
			t.Fatalf("←/→ on %q did not move off option %d of %d", key, cur, idx)
		}
	}
	t.Fatalf("←/→ never reached option %d of %q (stuck at %d)", idx, key, v.SelectedOf(key))
}

func (v *builderView) selectSignal(t *testing.T, name string) {
	t.Helper()
	for i, s := range v.signals {
		if s == name {
			v.selectOption(t, "signal", i)
			return
		}
	}
	t.Skipf("signal %q not available (have %v)", name, v.signals)
}

func (v *builderView) selectType(t *testing.T, want string) {
	t.Helper()
	for i, opt := range builderTypes {
		if opt == want {
			v.selectOption(t, "type", i)
			return
		}
	}
	t.Fatalf("no type option %q in %v", want, builderTypes)
}

func TestBuilderParamFieldsFollowTheSignal(t *testing.T) {
	v := builderFor(t, testKit(t))

	v.selectSignal(t, "github")
	keys := strings.Join(v.fieldKeys(), " ")
	for _, want := range []string{"param.query", "param.project", "param.field"} {
		if !strings.Contains(keys, want) {
			t.Fatalf("github builder missing %q: %v", want, v.fieldKeys())
		}
	}

	v.set(t, "param.query", "is:open is:pr")
	v.set(t, "name", "keep-me")

	v.selectSignal(t, "slack")
	keys = strings.Join(v.fieldKeys(), " ")
	if strings.Contains(keys, "param.project") {
		t.Fatalf("switching to slack kept github params: %v", v.fieldKeys())
	}
	if !strings.Contains(keys, "param.channel") {
		t.Fatalf("slack builder missing param.channel: %v", v.fieldKeys())
	}

	vals := v.Form().Values()
	if got, _ := vals["name"].(string); got != "keep-me" {
		t.Errorf("shared field lost on signal switch: name = %q", got)
	}
}

func TestBuilderQueryCollectsParamsFiltersAndRules(t *testing.T) {
	kit := testKit(t)
	v := builderFor(t, kit)
	v.selectSignal(t, "github")

	v.set(t, "param.query", "is:open is:pr")
	v.set(t, "extra", "sort=updated, per_page=5")
	v.set(t, "filters", "f1")
	v.set(t, "exclude", "(?i)bot$")
	v.set(t, "title", "My PRs")

	q, err := v.query()
	if err != nil {
		t.Fatal(err)
	}
	if q.Signal != "github" {
		t.Errorf("signal = %q", q.Signal)
	}
	want := map[string]string{"query": "is:open is:pr", "sort": "updated", "per_page": "5"}
	for k, wv := range want {
		if q.Params[k] != wv {
			t.Errorf("param %s = %q, want %q (all %v)", k, q.Params[k], wv, q.Params)
		}
	}
	if len(q.Filters) != 1 || q.Filters[0].Ref != "f1" {
		t.Errorf("filters = %#v", q.Filters)
	}
	if len(q.Rules) != 1 || q.Rules[0].Exclude != "(?i)bot$" {
		t.Errorf("rules = %#v", q.Rules)
	}
	if q.Title != "My PRs" {
		t.Errorf("title = %q", q.Title)
	}
	if q.Name != "" {
		t.Errorf("an unnamed build should stay unnamed, got %q", q.Name)
	}
}

func TestBuilderRejectsBadInput(t *testing.T) {
	kit := testKit(t)

	v := builderFor(t, kit)
	v.selectSignal(t, "github")
	v.set(t, "extra", "notakeyvalue")
	if _, err := v.query(); err == nil {
		t.Error("extra params that are not key=value should fail")
	}

	v = builderFor(t, kit)
	v.selectSignal(t, "github")
	v.set(t, "filters", "does-not-exist")
	if _, err := v.query(); err == nil {
		t.Error("an unknown filter reference should fail")
	}

	v = builderFor(t, kit)
	v.selectSignal(t, "github")
	v.set(t, "exclude", "(")
	if _, err := v.query(); err == nil {
		t.Error("an uncompilable regex should fail")
	}
}

func TestBuilderRunUsesFetchAdhoc(t *testing.T) {
	kit := testKit(t)
	var got config.Query
	kit.d.FetchAdhoc = func(q config.Query) []signals.Section {
		got = q
		return []signals.Section{{Signal: q.Signal, Title: "ad-hoc", Items: []signals.Item{{Title: "one hit"}}}}
	}

	v := builderFor(t, kit)
	v.selectSignal(t, "github")
	v.set(t, "param.query", "is:open")

	app := deck.New(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	app, cmd := update(app, tea.KeyMsg{Type: tea.KeyCtrlR})
	for _, c := range flattenCmds(cmd) {
		if c != nil {
			app = step(app, c())
		}
	}
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})

	if got.Signal != "github" || got.Params["query"] != "is:open" {
		t.Fatalf("FetchAdhoc got %#v", got)
	}
	if body := app.View(); !strings.Contains(body, "one hit") {
		t.Fatalf("results view missing the ad-hoc hit: %q", body)
	}
}

func TestBuilderSaveWritesQueryFile(t *testing.T) {
	kit := testKit(t)
	v := builderFor(t, kit)
	v.selectSignal(t, "github")
	v.set(t, "param.query", "is:open is:pr")
	v.set(t, "name", "built-prs")

	app := deck.New(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	step(app, tea.KeyMsg{Type: tea.KeyCtrlS})

	path := filepath.Join(kit.d.App.Cfg.Home, config.DirQueries, "built-prs.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("save did not write the query file: %v (status %q)", err, v.Status())
	}
	body := string(data)
	for _, want := range []string{"name: built-prs", "signal: github", "is:open is:pr"} {
		if !strings.Contains(body, want) {
			t.Errorf("saved query missing %q:\n%s", want, body)
		}
	}

	blob, err := json.Marshal(map[string]string{"built-prs.yaml": body})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := config.ParseDirectives(blob)
	if err != nil {
		t.Fatalf("saved query does not parse: %v", err)
	}
	queries := parsed.Queries
	if q := queries["built-prs"]; !q.Runnable() {
		t.Errorf("saved query is not runnable: %#v", q)
	}
}

func TestBuilderSaveRequiresAName(t *testing.T) {
	kit := testKit(t)
	v := builderFor(t, kit)
	v.selectSignal(t, "github")
	v.set(t, "param.query", "is:open")

	app := deck.New(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	step(app, tea.KeyMsg{Type: tea.KeyCtrlS})

	if !strings.Contains(v.Status(), "name is required") {
		t.Fatalf("status = %q, want a name-required message", v.Status())
	}
	entries, err := os.ReadDir(filepath.Join(kit.d.App.Cfg.Home, config.DirQueries))
	if err == nil && len(entries) > 0 {
		t.Fatalf("nameless save wrote files: %v", entries)
	}
}

func TestQueriesMenuListsSavedDocsAndNewEntry(t *testing.T) {
	kit := testKit(t)
	menu := kit.Queries()

	app := deck.New(menu)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	body := app.View()
	for _, want := range []string{"q1", "f1", "New"} {
		if !strings.Contains(body, want) {
			t.Fatalf("queries menu missing %q: %q", want, body)
		}
	}
	if !strings.Contains(body, "signal=github") {
		t.Errorf("queries menu missing the runnable summary: %q", body)
	}
	if !strings.Contains(body, "filter-only") {
		t.Errorf("queries menu missing the filter-only summary: %q", body)
	}
}

func TestRolesMenuPutsNewFirstAndOpensTheEditor(t *testing.T) {
	kit := testKit(t)
	app := deck.New(kit.Roles())
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	body := app.View()
	newAt, roleAt := strings.Index(body, "New"), strings.Index(body, "triage")
	if newAt < 0 || roleAt < 0 {
		t.Fatalf("roles menu missing entries: %q", body)
	}
	if newAt > roleAt {
		t.Errorf("New should come before saved roles:\n%s", body)
	}

	app, cmd := update(app, tea.KeyMsg{Type: tea.KeyDown})
	for _, c := range flattenCmds(cmd) {
		if c != nil {
			app = step(app, c())
		}
	}
	app, cmd = update(app, tea.KeyMsg{Type: tea.KeyEnter})
	for _, c := range flattenCmds(cmd) {
		if c != nil {
			app = step(app, c())
		}
	}
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	if got := app.View(); !strings.Contains(got, "edit triage") {
		t.Fatalf("selecting a role did not open its editor: %q", got)
	}
}

func TestQueryEditorPrefillsFromSavedDoc(t *testing.T) {
	kit := testKit(t)
	kit.d.App.Directives.Queries["prefill"] = config.Query{
		Name:   "prefill",
		Title:  "Prefilled",
		Signal: "github",
		Params: map[string]string{"query": "is:open", "unlisted": "keep"},
		Rules:  []filter.Rule{{Field: "meta.author", Exclude: "bot$"}},
	}

	v, ok := kit.QueryEditor("prefill").(*builderView)
	if !ok {
		t.Fatal("QueryEditor did not return a builderView")
	}
	if v.signal() != "github" {
		t.Fatalf("signal = %q, want github", v.signal())
	}
	vals := v.Form().Values()
	checks := map[string]string{
		"param.query": "is:open",
		"extra":       "unlisted=keep",
		"field":       "meta.author",
		"exclude":     "bot$",
		"name":        "prefill",
		"title":       "Prefilled",
	}
	for key, want := range checks {
		if got, _ := vals[key].(string); got != want {
			t.Errorf("field %s = %q, want %q", key, got, want)
		}
	}
}

func TestQueryEditorKeepsFieldsTheFormCannotShow(t *testing.T) {
	kit := testKit(t)
	kit.d.App.Directives.Queries["rich"] = config.Query{
		Name:     "rich",
		Signal:   "github",
		Aliases:  map[string]string{"REPOS": "repo:a repo:b"},
		Keywords: map[string]string{"hot": "urgent"},
		Rules: []filter.Rule{
			{Exclude: "bot$"},
			{Field: "title", Include: "deploy"},
		},
		Filters: []config.QueryFilter{
			{Ref: "f1"},
			{Inline: &filter.Rule{Include: "keep-me"}},
		},
	}

	v, _ := kit.QueryEditor("rich").(*builderView)
	v.set(t, "exclude", "robot$")

	q, err := v.query()
	if err != nil {
		t.Fatal(err)
	}
	if q.Aliases["REPOS"] != "repo:a repo:b" {
		t.Errorf("aliases lost on edit: %#v", q.Aliases)
	}
	if q.Keywords["hot"] != "urgent" {
		t.Errorf("keywords lost on edit: %#v", q.Keywords)
	}
	if len(q.Rules) != 2 {
		t.Fatalf("rules = %#v, want the edited rule plus the untouched second", q.Rules)
	}
	if q.Rules[0].Exclude != "robot$" {
		t.Errorf("first rule not edited: %#v", q.Rules[0])
	}
	if q.Rules[1].Include != "deploy" {
		t.Errorf("second rule lost: %#v", q.Rules[1])
	}
	var inline, refs int
	for _, qf := range q.Filters {
		if qf.Inline != nil {
			inline++
		}
		if qf.Ref != "" {
			refs++
		}
	}
	if inline != 1 || refs != 1 {
		t.Errorf("filters = %#v, want one inline and one ref preserved", q.Filters)
	}
}

func TestBuilderCanSaveAFilterWithNoSignal(t *testing.T) {
	kit := testKit(t)
	v := builderFor(t, kit)
	if v.signals[0] != builderNoSignal {
		t.Fatalf("signal options[0] = %q, want the filter-only option", v.signals[0])
	}
	v.set(t, "exclude", "(?i)bot$")
	v.set(t, "name", "built-filter")

	q, err := v.query()
	if err != nil {
		t.Fatal(err)
	}
	if q.Signal != "" {
		t.Errorf("signal = %q, want empty", q.Signal)
	}
	if q.Runnable() {
		t.Error("a signal-less document should not be runnable")
	}
	if !q.HasFilter() {
		t.Error("a signal-less document with rules should be a filter")
	}
}

func TestBuilderRejectsEmptyDocument(t *testing.T) {
	v := builderFor(t, testKit(t))
	v.set(t, "name", "nothing")
	if _, err := v.query(); err == nil {
		t.Error("a document with no signal and no rules should be rejected")
	}
}

func TestBuilderRunRefusesFilterOnlyDoc(t *testing.T) {
	kit := testKit(t)
	called := false
	kit.d.FetchAdhoc = func(config.Query) []signals.Section {
		called = true
		return nil
	}
	v := builderFor(t, kit)
	v.set(t, "exclude", "bot$")

	app := deck.New(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	step(app, tea.KeyMsg{Type: tea.KeyCtrlR})

	if called {
		t.Error("run should not fetch a document with no signal")
	}
	if !strings.Contains(v.Status(), "nothing to run") {
		t.Errorf("status = %q, want a nothing-to-run message", v.Status())
	}
}

func TestBuilderSaveRejectsNameCollision(t *testing.T) {
	kit := testKit(t)
	v := builderFor(t, kit)
	v.selectSignal(t, "github")
	v.set(t, "param.query", "is:open")
	v.set(t, "name", "q1")

	app := deck.New(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	step(app, tea.KeyMsg{Type: tea.KeyCtrlS})

	if !strings.Contains(v.Status(), "already exists") {
		t.Fatalf("status = %q, want an already-exists message", v.Status())
	}
	if _, err := os.Stat(filepath.Join(kit.d.App.Cfg.Home, config.DirQueries, "q1.yaml")); err == nil {
		t.Error("colliding save wrote a file")
	}
}

func TestQueryDeleteRemovesTheFile(t *testing.T) {
	kit := testKit(t)
	q := config.Query{Name: "doomed", Signal: "github"}
	if _, _, err := config.SaveDirective(nil, kit.d.App.Cfg.Home, "", config.TypeQuery, q.Name, q); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(kit.d.App.Cfg.Home, config.DirQueries, "doomed.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	loadKitDirectives(t, kit)

	summary := kit.deleteQuery("doomed")
	if !strings.Contains(summary, "removed") {
		t.Fatalf("delete summary = %q", summary)
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("delete left the file in place")
	}
}

func TestQueriesListPutsNewFirstAndOpensTheEditor(t *testing.T) {
	kit := testKit(t)
	kit.d.App.Directives.Queries = map[string]config.Query{
		"only": {Name: "only", Signal: "github", Params: map[string]string{"query": "is:open"}},
	}

	listed := deck.New(kit.Queries())
	listed = step(listed, tea.WindowSizeMsg{Width: 100, Height: 40})
	rendered := listed.View()
	newAt, queryAt := strings.Index(rendered, "New"), strings.Index(rendered, "only")
	if newAt < 0 || queryAt < 0 {
		t.Fatalf("queries list missing entries: %q", rendered)
	}
	if newAt > queryAt {
		t.Errorf("New should come before saved queries:\n%s", rendered)
	}

	app := deck.New(kit.Queries())
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	app = step(app, tea.KeyMsg{Type: tea.KeyDown})
	app, cmd := update(app, tea.KeyMsg{Type: tea.KeyEnter})
	for _, c := range flattenCmds(cmd) {
		if c != nil {
			app = step(app, c())
		}
	}
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})

	body := app.View()
	if !strings.Contains(body, "edit only") {
		t.Fatalf("selecting a saved query did not open the editor: %q", body)
	}
	if !strings.Contains(body, "is:open") {
		t.Errorf("editor did not prefill the saved param: %q", body)
	}
	for _, want := range []string{"ctrl+t validate", "ctrl+y yaml", "ctrl+x delete"} {
		if !strings.Contains(body, want) {
			t.Errorf("editor hints missing %q: %q", want, body)
		}
	}
}

func TestBuilderHintsOmitDeleteForUnsavedAndDoNotDuplicateBack(t *testing.T) {
	kit := testKit(t)

	fresh := builderFor(t, kit)
	for _, h := range fresh.Hints() {
		if h[1] == "delete" {
			t.Error("an unsaved builder should not advertise delete")
		}
		if h[0] == "esc" {
			t.Error("esc/back is added by the deck chrome; the view should not repeat it")
		}
	}

	saved, _ := kit.QueryEditor("q1").(*builderView)
	found := false
	for _, h := range saved.Hints() {
		if h[1] == "delete" {
			found = true
		}
	}
	if !found {
		t.Error("an editor on a saved query should advertise delete")
	}
}

func TestBuilderYAMLPreviewToggles(t *testing.T) {
	kit := testKit(t)
	v := builderFor(t, kit)
	v.selectSignal(t, "github")
	v.set(t, "param.query", "is:open is:pr")

	app := deck.New(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	if strings.Contains(app.View(), "params:") {
		t.Fatal("yaml preview showing before it was toggled on")
	}

	app = step(app, tea.KeyMsg{Type: tea.KeyCtrlY})
	body := app.View()
	for _, want := range []string{"params:", "query: is:open is:pr"} {
		if !strings.Contains(body, want) {
			t.Fatalf("yaml preview missing %q: %q", want, body)
		}
	}

	app = step(app, tea.KeyMsg{Type: tea.KeyCtrlY})
	if strings.Contains(app.View(), "params:") {
		t.Error("yaml preview did not toggle back off")
	}
}

func TestBuilderValidateReportsInline(t *testing.T) {
	kit := testKit(t)
	v := builderFor(t, kit)
	v.selectSignal(t, "github")
	v.set(t, "param.query", "is:open")

	app := deck.New(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	app = step(app, tea.KeyMsg{Type: tea.KeyCtrlT})

	if len(v.Notice()) == 0 {
		t.Fatal("validate produced no findings panel")
	}
	if body := app.View(); !strings.Contains(body, "validation") {
		t.Fatalf("validation panel not rendered: %q", body)
	}
}

func TestBuilderValidateCatchesProblemsInUnsavedEdits(t *testing.T) {
	kit := testKit(t)
	v := builderFor(t, kit)
	v.selectSignal(t, "github")
	v.set(t, "param.query", "is:open")
	v.set(t, "exclude", "(")

	app := deck.New(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	step(app, tea.KeyMsg{Type: tea.KeyCtrlT})

	if v.Status() == "" {
		t.Fatal("validate did not report the bad regex from the unsaved form")
	}
}

func TestBuilderDeleteAsksThenRemoves(t *testing.T) {
	kit := testKit(t)
	q := config.Query{Name: "doomed", Signal: "github"}
	if _, _, err := config.SaveDirective(nil, kit.d.App.Cfg.Home, "", config.TypeQuery, q.Name, q); err != nil {
		t.Fatal(err)
	}
	loadKitDirectives(t, kit)
	path := filepath.Join(kit.d.App.Cfg.Home, config.DirQueries, "doomed.yaml")

	v, _ := kit.QueryEditor("doomed").(*builderView)
	app := deck.New(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})

	app = step(app, tea.KeyMsg{Type: tea.KeyCtrlX})
	if !v.Confirming() {
		t.Fatal("delete did not raise a confirmation")
	}
	if body := app.View(); !strings.Contains(body, "delete doomed?") {
		t.Fatalf("confirmation not rendered: %q", body)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("file removed before confirming")
	}

	app = step(app, tea.KeyMsg{Type: tea.KeyEsc})
	if v.Confirming() {
		t.Error("esc did not dismiss the confirmation")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("cancelling the confirmation still deleted the file")
	}

	app = step(app, tea.KeyMsg{Type: tea.KeyCtrlX})
	app = step(app, tea.KeyMsg{Type: tea.KeyLeft})
	app, cmd := update(app, tea.KeyMsg{Type: tea.KeyEnter})
	for _, c := range flattenCmds(cmd) {
		if c != nil {
			app = step(app, c())
		}
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("confirming delete left the file in place")
	}
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	if body := app.View(); !strings.Contains(body, "removed") {
		t.Errorf("delete outcome not shown: %q", body)
	}
}

func TestBuilderDeleteRefusesUnsavedQuery(t *testing.T) {
	v := builderFor(t, testKit(t))
	app := deck.New(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	step(app, tea.KeyMsg{Type: tea.KeyCtrlX})

	if v.Confirming() {
		t.Error("an unsaved query should not raise a delete confirmation")
	}
	if !strings.Contains(v.Status(), "not been saved") {
		t.Errorf("status = %q, want a not-saved explanation", v.Status())
	}
}

func TestBuilderTypeIsTheFirstField(t *testing.T) {
	v := builderFor(t, testKit(t))
	if keys := v.fieldKeys(); len(keys) == 0 || keys[0] != "type" {
		t.Fatalf("first field = %v, want type first", keys)
	}
	if got := v.focusedKey(); got != "type" {
		t.Fatalf("a fresh builder focuses %q, want type: a new form always starts on field 0", got)
	}
}

const guardsC93 = "C-93 (FIXED, GUARDED HERE): viewkit/deck/editor.go syncFields used to replace " +
	"the whole form with forms.NewForm on every select change, and a new form starts focused on " +
	"field 0 — which in the query builder is `type`. Picking a signal therefore threw focus off " +
	"`signal` onto `type`, and the next two → keys, which read as \"cycle signals\", rewrote the " +
	"document type to `filter`; builder.go fields() then dropped `signal` and every param.* field, " +
	"wrecking the draft in three keystrokes. syncFields now captures forms.Form.FocusedKey before " +
	"the rebuild and restores it with forms.Form.FocusKey, so focus stays on `signal` and → keeps " +
	"walking signals. This test was the pin asserting the buggy behaviour; it is now inverted and " +
	"guards the fix. If it fails on focus, the focus restore in syncFields regressed."

func TestBuilderKeepsFocusAndDraftWhenCyclingSignals(t *testing.T) {
	t.Log(guardsC93)

	v := builderFor(t, testKit(t))
	v.selectSignal(t, "github")
	v.set(t, "param.query", "is:open is:pr")

	app := deck.New(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})

	typeBefore := v.Value("type")

	v.focus(t, "signal")
	if got := v.focusedKey(); got != "signal" {
		t.Fatalf("setup: focus = %q, want signal", got)
	}

	app = step(app, tea.KeyMsg{Type: tea.KeyRight})
	if got := v.signal(); got == "github" {
		t.Fatalf("→ did not move the signal select off github")
	}
	if got := v.focusedKey(); got != "signal" {
		t.Fatalf("focus after picking a signal = %q, want signal to be preserved.\n%s", got, guardsC93)
	}

	for i := 2; i <= 3; i++ {
		app = step(app, tea.KeyMsg{Type: tea.KeyRight})
		if got := v.focusedKey(); got != "signal" {
			t.Fatalf("focus after → number %d = %q, want signal.\n%s", i, got, guardsC93)
		}
		if got := v.Value("type"); got != typeBefore {
			t.Fatalf("→ number %d rewrote type to %q, want it left at %q; the keys must walk signals, not type", i, got, typeBefore)
		}
	}

	if got := strings.Join(v.fieldKeys(), " "); !strings.Contains(got, "signal") {
		t.Errorf("the signal field was dropped: %v", v.fieldKeys())
	}
	if got := v.signal(); got == "" {
		t.Error("signal is empty; three → keys should have left a signal selected")
	}
	if _, err := v.query(); err != nil {
		t.Errorf("the draft no longer builds a document: %v", err)
	}
}

func TestBuilderRestoresParamsWhenSignalCyclesBack(t *testing.T) {
	t.Log(guardsC93)

	v := builderFor(t, testKit(t))
	v.selectSignal(t, "github")
	v.set(t, "param.query", "is:open is:pr")

	app := deck.New(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})

	v.focus(t, "signal")
	app = step(app, tea.KeyMsg{Type: tea.KeyRight})
	if got := v.signal(); got == "github" {
		t.Fatalf("→ did not move the signal select off github")
	}
	if got := v.focusedKey(); got != "signal" {
		t.Fatalf("focus after → = %q, want signal.\n%s", got, guardsC93)
	}

	app = step(app, tea.KeyMsg{Type: tea.KeyLeft})
	if got := v.signal(); got != "github" {
		t.Fatalf("← back = %q, want github", got)
	}
	if got := v.Value(builderParamPrefix + "query"); got != "is:open is:pr" {
		t.Errorf("param.query = %q, want the typed value restored from sticky on the round trip", got)
	}
	if body := app.View(); !strings.Contains(body, "is:open is:pr") {
		t.Errorf("the typed param is not back on screen after cycling back to github:\n%s", body)
	}
}

func TestBuilderFilterTypeHidesQueryOnlyFields(t *testing.T) {
	v := builderFor(t, testKit(t))
	v.selectSignal(t, "github")
	if keys := strings.Join(v.fieldKeys(), " "); !strings.Contains(keys, "signal") {
		t.Fatalf("signal missing before switching type: %v", v.fieldKeys())
	}

	v.selectType(t, string(config.TypeFilter))
	keys := v.fieldKeys()
	joined := strings.Join(keys, " ")
	for _, gone := range []string{"signal", "param.", "extra", "filters"} {
		if strings.Contains(joined, gone) {
			t.Errorf("type: filter still shows %q: %v", gone, keys)
		}
	}
	for _, kept := range []string{"type", "field", "include", "exclude", "name", "title"} {
		if !strings.Contains(joined, kept) {
			t.Errorf("type: filter dropped %q: %v", kept, keys)
		}
	}
}

func TestBuilderQueryTypeShowsSignalAndParams(t *testing.T) {
	v := builderFor(t, testKit(t))
	v.selectType(t, string(config.TypeQuery))
	v.selectSignal(t, "slack")
	joined := strings.Join(v.fieldKeys(), " ")
	for _, want := range []string{"signal", "param.channel", "extra", "filters"} {
		if !strings.Contains(joined, want) {
			t.Errorf("type: query missing %q: %v", want, v.fieldKeys())
		}
	}
}

func TestBuilderSwitchingToFilterDropsTheHiddenSignal(t *testing.T) {
	v := builderFor(t, testKit(t))
	v.selectSignal(t, "github")
	v.set(t, "param.query", "is:open")
	v.selectType(t, string(config.TypeFilter))
	v.set(t, "exclude", "bot$")
	v.set(t, "name", "now-a-filter")

	q, err := v.query()
	if err != nil {
		t.Fatal(err)
	}
	if q.Signal != "" {
		t.Errorf("signal = %q, want empty: an invisible signal would fail `type: filter` validation", q.Signal)
	}
	if len(q.Params) != 0 {
		t.Errorf("params = %#v, want none on a filter", q.Params)
	}
	if q.Runnable() {
		t.Error("a filter document should not be runnable")
	}
	if !q.HasFilter() {
		t.Error("the document should be usable as a filter")
	}
}

func TestBuilderRestoresHiddenValuesWhenTypeSwitchesBack(t *testing.T) {
	v := builderFor(t, testKit(t))
	v.selectSignal(t, "github")
	v.set(t, "param.query", "is:open is:pr")
	v.set(t, "extra", "sort=updated")

	v.selectType(t, string(config.TypeFilter))
	v.selectType(t, string(config.TypeQuery))

	vals := v.Form().Values()
	if got, _ := vals["param.query"].(string); got != "is:open is:pr" {
		t.Errorf("param.query = %q, want it restored after a type round trip", got)
	}
	if got, _ := vals["extra"].(string); got != "sort=updated" {
		t.Errorf("extra = %q, want it restored after a type round trip", got)
	}
	if v.signal() != "github" {
		t.Errorf("signal = %q, want github remembered", v.signal())
	}
}

func TestBuilderTypeSpecificValidation(t *testing.T) {
	kit := testKit(t)

	v := builderFor(t, kit)
	v.selectType(t, string(config.TypeQuery))
	v.set(t, "name", "no-signal")
	if _, err := v.query(); err == nil {
		t.Error("type: query with no signal should fail")
	}

	v = builderFor(t, kit)
	v.selectType(t, string(config.TypeFilter))
	v.set(t, "name", "no-rules")
	if _, err := v.query(); err == nil {
		t.Error("type: filter with no rules should fail")
	}
}

func TestBuilderFilterSaveWarnsBeforeDroppingRememberedSignalAndParams(t *testing.T) {
	kit := testKit(t)
	v := builderFor(t, kit)
	v.selectSignal(t, "github")
	v.set(t, "param.query", "is:open")
	v.selectType(t, string(config.TypeFilter))
	v.set(t, "exclude", "(?i)bot$")
	v.set(t, "name", "warned-filter")

	app := deck.New(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	path := filepath.Join(kit.d.App.Cfg.Home, config.DirQueries, "warned-filter.yaml")

	step(app, tea.KeyMsg{Type: tea.KeyCtrlS})
	if _, err := os.Stat(path); err == nil {
		t.Fatal("the first ctrl+s wrote the filter, silently discarding the remembered signal and params")
	}
	status := v.Status()
	for _, want := range []string{"github", "query", "ctrl+s"} {
		if !strings.Contains(status, want) {
			t.Errorf("save refusal %q does not mention %q", status, want)
		}
	}

	step(app, tea.KeyMsg{Type: tea.KeyCtrlS})
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("a second ctrl+s did not save: %v (status %q)", err, v.Status())
	}
	if body := string(data); strings.Contains(body, "signal:") {
		t.Errorf("acknowledged filter save still carries a signal:\n%s", body)
	}
}

func TestBuilderFilterSaveWithoutARememberedSignalNeedsNoAck(t *testing.T) {
	kit := testKit(t)
	v := builderFor(t, kit)
	v.selectType(t, string(config.TypeFilter))
	v.set(t, "exclude", "(?i)bot$")
	v.set(t, "name", "clean-filter")

	app := deck.New(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	step(app, tea.KeyMsg{Type: tea.KeyCtrlS})

	path := filepath.Join(kit.d.App.Cfg.Home, config.DirQueries, "clean-filter.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("a filter with nothing to discard should save on the first ctrl+s: %v (status %q)", err, v.Status())
	}
}

func TestBuilderSavedFilterMatchesWhatTheFormShows(t *testing.T) {
	kit := testKit(t)
	v := builderFor(t, kit)
	v.selectSignal(t, "github")
	v.set(t, "param.query", "is:open")
	v.selectType(t, string(config.TypeFilter))
	v.set(t, "exclude", "(?i)bot$")
	v.set(t, "name", "saved-filter")

	app := deck.New(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	step(app, tea.KeyMsg{Type: tea.KeyCtrlS})
	step(app, tea.KeyMsg{Type: tea.KeyCtrlS})

	data, err := os.ReadFile(filepath.Join(kit.d.App.Cfg.Home, config.DirQueries, "saved-filter.yaml"))
	if err != nil {
		t.Fatalf("save failed: %v (status %q)", err, v.Status())
	}
	body := string(data)
	for _, gone := range []string{"signal:", "params:", "is:open"} {
		if strings.Contains(body, gone) {
			t.Errorf("saved filter still carries %q:\n%s", gone, body)
		}
	}
	if !strings.Contains(body, "type: filter") {
		t.Errorf("saved filter missing type: filter:\n%s", body)
	}

	blob, err := json.Marshal(map[string]string{"saved-filter.yaml": body})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := config.ParseDirectives(blob); err != nil {
		t.Fatalf("saved filter does not pass config validation: %v\n%s", err, body)
	}
}

func builderWithResults(t *testing.T, kit *Kit, items ...string) (*builderView, *vkdeck.Model) {
	t.Helper()
	secs := []signals.Section{{Signal: "github", Title: "Demo Items"}}
	for _, it := range items {
		secs[0].Items = append(secs[0].Items, signals.Item{Title: it, URL: "https://example.com/" + it})
	}
	kit.d.FetchAdhoc = func(config.Query) []signals.Section { return secs }

	v := builderFor(t, kit)
	v.selectSignal(t, "github")
	v.set(t, "param.query", "is:open")

	app := deck.New(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 44})
	app, cmd := update(app, tea.KeyMsg{Type: tea.KeyCtrlR})
	for _, c := range flattenCmds(cmd) {
		if c != nil {
			app = step(app, c())
		}
	}
	return v, step(app, tea.WindowSizeMsg{Width: 100, Height: 44})
}

func TestBuilderRunKeepsTheFormAndAddsAResultsPanel(t *testing.T) {
	_, app := builderWithResults(t, testKit(t), "one hit")
	body := app.View()

	if !strings.Contains(body, "one hit") {
		t.Fatalf("results not rendered: %q", body)
	}
	if !strings.Contains(body, "results: ad-hoc") {
		t.Errorf("results panel title missing: %q", body)
	}
	if !strings.Contains(body, "type") {
		t.Errorf("form gone after run; results should sit under the form, not replace it:\n%s", body)
	}
	if strings.Contains(body, "tab to edit  ·") {
		t.Errorf("form collapsed to a summary when it should still be editable:\n%s", body)
	}
	if strings.Index(body, "type") > strings.Index(body, "one hit") {
		t.Error("results panel should render below the form")
	}
}

func TestBuilderRunFetchesOffTheUpdateLoop(t *testing.T) {
	kit := testKit(t)
	fetched := false
	kit.d.FetchAdhoc = func(config.Query) []signals.Section {
		fetched = true
		return []signals.Section{{Signal: "github", Title: "t", Items: []signals.Item{{Title: "late"}}}}
	}

	v := builderFor(t, kit)
	v.selectSignal(t, "github")
	v.set(t, "param.query", "is:open")

	app := deck.New(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 44})
	app, cmd := update(app, tea.KeyMsg{Type: tea.KeyCtrlR})

	if fetched {
		t.Error("fetch ran inside Update; a slow signal would freeze the whole TUI")
	}
	if !v.Running() {
		t.Error("view should be in the running state before the fetch completes")
	}
	if body := app.View(); !strings.Contains(body, "running…") {
		t.Errorf("no running indicator while the fetch is in flight: %q", body)
	}

	for _, c := range flattenCmds(cmd) {
		if c != nil {
			app = step(app, c())
		}
	}
	if !fetched {
		t.Fatal("the returned command did not perform the fetch")
	}
	if v.Running() {
		t.Error("still running after the result arrived")
	}
}

func TestBuilderTabMovesFocusBetweenFormAndResults(t *testing.T) {
	v, app := builderWithResults(t, testKit(t), "alpha", "beta")

	if !v.OnResults() {
		t.Fatal("focus should land on the results after a run")
	}
	hints := hintLabels(v.Hints())
	if !strings.Contains(hints, "open") || !strings.Contains(hints, "page") {
		t.Errorf("result-focused hints = %q, want scroll/open hints", hints)
	}

	app = step(app, tea.KeyMsg{Type: tea.KeyTab})
	if v.OnResults() {
		t.Fatal("tab did not return focus to the form")
	}
	if hints := hintLabels(v.Hints()); !strings.Contains(hints, "field") {
		t.Errorf("form-focused hints = %q, want field navigation", hints)
	}

	app = step(app, tea.KeyMsg{Type: tea.KeyTab})
	if !v.OnResults() {
		t.Fatal("tab did not move focus back to the results")
	}
	_ = app
}

func TestBuilderResultFocusDoesNotTypeIntoTheForm(t *testing.T) {
	v, app := builderWithResults(t, testKit(t), "alpha")
	before, _ := v.Form().Values()["name"].(string)

	app = step(app, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("zzz")})
	after, _ := v.Form().Values()["name"].(string)
	if after != before {
		t.Errorf("typing while the results have focus edited the form: %q -> %q", before, after)
	}
	_ = app
}

func TestBuilderResultsScrollWithoutMovingFormFields(t *testing.T) {
	v, app := builderWithResults(t, testKit(t), "alpha", "beta", "gamma")
	focusedBefore := v.Form().Focused().Key

	app = step(app, tea.KeyMsg{Type: tea.KeyDown})
	app = step(app, tea.KeyMsg{Type: tea.KeyDown})

	if got := v.Form().Focused().Key; got != focusedBefore {
		t.Errorf("arrow keys moved the form cursor while results had focus: %q -> %q", focusedBefore, got)
	}
	sel, ok := v.Selected()
	if !ok {
		t.Fatal("no result selected after scrolling")
	}
	if sel.Key == "" {
		t.Error("selected result has no URL key, so enter cannot open it")
	}
	_ = app
}

func TestBuilderEmptyResultsSaySo(t *testing.T) {
	_, app := builderWithResults(t, testKit(t))
	if body := app.View(); !strings.Contains(body, "no items") {
		t.Errorf("empty run did not report no items: %q", body)
	}
}

func TestBuilderErrorRunShowsTheErrorNotNoItems(t *testing.T) {
	kit := testKit(t)
	kit.d.FetchAdhoc = func(config.Query) []signals.Section {
		return []signals.Section{{Signal: "github", Title: "Demo Items", Err: errors.New("token expired")}}
	}

	v := builderFor(t, kit)
	v.selectSignal(t, "github")
	v.set(t, "param.query", "is:open")

	app := deck.New(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 44})
	app, cmd := update(app, tea.KeyMsg{Type: tea.KeyCtrlR})
	for _, c := range flattenCmds(cmd) {
		if c != nil {
			app = step(app, c())
		}
	}
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 44})

	body := app.View()
	if strings.Contains(body, "no items") {
		t.Errorf("failed run reported no items, hiding the error:\n%s", body)
	}
	if !strings.Contains(body, "token expired") {
		t.Errorf("failed run did not surface the error text:\n%s", body)
	}
}

func hintLabels(hints [][2]string) string {
	var parts []string
	for _, h := range hints {
		parts = append(parts, h[1])
	}
	return strings.Join(parts, " ")
}

func TestBuilderResultSelectionSurvivesRepaint(t *testing.T) {
	v, app := builderWithResults(t, testKit(t), "alpha", "beta", "gamma")

	app = step(app, tea.KeyMsg{Type: tea.KeyDown})
	app = step(app, tea.KeyMsg{Type: tea.KeyDown})
	moved, ok := v.Selected()
	if !ok {
		t.Fatal("no selection after scrolling")
	}

	_ = app.View()
	_ = app.View()

	after, ok := v.Selected()
	if !ok {
		t.Fatal("selection lost after repaint")
	}
	if after.Key != moved.Key {
		t.Errorf("repaint reset the result selection: %q -> %q", moved.Key, after.Key)
	}
}

func TestBuilderKeepsFullFormWhenThereIsRoom(t *testing.T) {
	kit := testKit(t)
	kit.d.FetchAdhoc = func(config.Query) []signals.Section {
		return []signals.Section{{Signal: "github", Title: "t", Items: []signals.Item{{Title: "hit"}}}}
	}
	v := builderFor(t, kit)
	v.selectSignal(t, "github")
	v.set(t, "param.query", "is:open")

	app := deck.New(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 60})
	app, cmd := update(app, tea.KeyMsg{Type: tea.KeyCtrlR})
	for _, c := range flattenCmds(cmd) {
		if c != nil {
			app = step(app, c())
		}
	}
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 60})

	body := app.View()
	if !strings.Contains(body, "rule exclude regex") {
		t.Errorf("tall terminal should keep the whole form visible:\n%s", body)
	}
	if strings.Contains(body, "tab to edit  ·") {
		t.Error("form collapsed even though there was room for both panels")
	}
	if !strings.Contains(body, "hit") {
		t.Error("results missing")
	}
}

func TestBuilderCollapsesFormWhenResultsNeedRoom(t *testing.T) {
	kit := testKit(t)
	kit.d.FetchAdhoc = func(config.Query) []signals.Section {
		secs := []signals.Section{{Signal: "github", Title: "t"}}
		for i := range 12 {
			secs[0].Items = append(secs[0].Items, signals.Item{Title: "hit-" + string(rune('a'+i))})
		}
		return secs
	}
	v := builderFor(t, kit)
	v.selectSignal(t, "github")
	v.set(t, "param.query", "is:open")
	v.set(t, "name", "shortform")

	app := deck.New(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 26})
	app, cmd := update(app, tea.KeyMsg{Type: tea.KeyCtrlR})
	for _, c := range flattenCmds(cmd) {
		if c != nil {
			app = step(app, c())
		}
	}
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 26})

	focused := app.View()
	if strings.Contains(focused, "rule exclude regex") {
		t.Errorf("short terminal with results focused should collapse the form:\n%s", focused)
	}
	for _, want := range []string{"signal=github", "name=shortform", "tab to edit"} {
		if !strings.Contains(focused, want) {
			t.Errorf("collapsed summary missing %q:\n%s", want, focused)
		}
	}
	if !strings.Contains(focused, "hit-a") {
		t.Errorf("results still not visible after collapsing the form:\n%s", focused)
	}

	app = step(app, tea.KeyMsg{Type: tea.KeyTab})
	restored := app.View()
	if strings.Contains(restored, "tab to edit  ·") {
		t.Errorf("tabbing back to the form should expand it again:\n%s", restored)
	}
	if got := v.focusedKey(); got != "name" {
		t.Fatalf("focus = %q, want the last field typed into (name)", got)
	}
	for _, want := range []string{"name (required to save)", "shortform"} {
		if !strings.Contains(restored, want) {
			t.Errorf("expanded form does not show the focused field %q:\n%s", want, restored)
		}
	}
	if strings.Contains(restored, "name=shortform") {
		t.Errorf("expanded form still renders the collapsed summary:\n%s", restored)
	}
}

const builderMinTerminal = 20

func TestBuilderNeverOverflowsTheTerminal(t *testing.T) {
	kit := testKit(t)
	kit.d.FetchAdhoc = func(config.Query) []signals.Section {
		secs := []signals.Section{{Signal: "github", Title: "Open PRs"}}
		for i := range 20 {
			secs[0].Items = append(secs[0].Items, signals.Item{
				Title: "a reasonably long pull request title " + string(rune('a'+i)),
				URL:   "https://example.com/" + string(rune('a'+i)),
			})
		}
		return secs
	}
	v := builderFor(t, kit)
	v.selectSignal(t, "github")
	v.set(t, "param.query", "is:open")
	v.set(t, "name", "overflow-probe")

	app := deck.New(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	app, cmd := update(app, tea.KeyMsg{Type: tea.KeyCtrlR})
	for _, c := range flattenCmds(cmd) {
		if c != nil {
			app = step(app, c())
		}
	}

	for h := builderMinTerminal; h <= 60; h++ {
		for _, focus := range []string{"results", "form"} {
			app = step(app, tea.WindowSizeMsg{Width: 100, Height: h})
			if got := len(strings.Split(app.View(), "\n")); got > h {
				t.Fatalf("%s focus at height %d rendered %d lines", focus, h, got)
			}
			app = step(app, tea.KeyMsg{Type: tea.KeyTab})
		}
	}
}

func TestBuilderWindowsTheFormAroundTheFocusedField(t *testing.T) {
	kit := testKit(t)
	v := builderFor(t, kit)
	v.selectSignal(t, "github")

	app := deck.New(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 26})

	clipped := app.View()
	if !strings.Contains(clipped, "⋯") {
		t.Fatalf("tall form in a short terminal should show a clipped marker:\n%s", clipped)
	}
	if strings.Contains(clipped, "display title (optional)") {
		t.Errorf("last field should be scrolled out of the window:\n%s", clipped)
	}

	for range len(v.Form().Fields) {
		app = step(app, tea.KeyMsg{Type: tea.KeyDown})
	}
	scrolled := app.View()
	if !strings.Contains(scrolled, "display title (optional)") {
		t.Errorf("moving to the last field should scroll it into view:\n%s", scrolled)
	}
	if strings.Contains(scrolled, "▸ type") {
		t.Errorf("first field should have scrolled out once focus reached the last:\n%s", scrolled)
	}
}
