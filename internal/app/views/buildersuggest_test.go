package views

import (
	"slices"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/codyconfer/viewkit/forms"

	vkdeck "github.com/codyconfer/viewkit/deck"

	"github.com/codyconfer/munin/internal/deck"
)

func (v *builderView) focus(t *testing.T, key string) {
	t.Helper()
	want := v.fieldIndex(t, key)
	for range len(v.Form().Fields) + 1 {
		at := v.focusedIndex()
		if at == want {
			if got := v.focusedKey(); got != key {
				t.Fatalf("field %d is %q, want %q (fields %v)", want, got, key, v.fieldKeys())
			}
			return
		}
		if at < want {
			v.press(tea.KeyMsg{Type: tea.KeyDown})
		} else {
			v.press(tea.KeyMsg{Type: tea.KeyUp})
		}
	}
	t.Fatalf("↑/↓ never reached field %q (focus stuck on %q, fields %v)", key, v.focusedKey(), v.fieldKeys())
}

func (v *builderView) typeIn(t *testing.T, s string) {
	t.Helper()
	before := v.focusedKey()
	for _, r := range s {
		v.press(runeKey(r))
	}
	if got := v.focusedKey(); got != before {
		t.Fatalf("typing %q moved focus off %q to %q", s, before, got)
	}
}

func textOf(t *testing.T, v *builderView, key string) string {
	t.Helper()
	for i := range v.Form().Fields {
		if v.Form().Fields[i].Key == key {
			return v.Form().Fields[i].Text
		}
	}
	t.Fatalf("builder has no field %q", key)
	return ""
}

func TestBuilderFiltersFieldSuggestsSavedFilters(t *testing.T) {
	v := builderFor(t, testKit(t))
	v.selectSignal(t, "github")

	v.focus(t, "filters")
	v.typeIn(t, "f")
	if got := v.Form().Suggestions(); !slices.Contains(got, "f1") {
		t.Fatalf("Suggestions() = %v, want the saved filter f1", got)
	}
	v.press(tea.KeyMsg{Type: tea.KeyTab})
	if got := textOf(t, v, "filters"); got != "f1" {
		t.Fatalf("tab did not accept the suggestion, filters = %q, want %q", got, "f1")
	}
	if got := v.focusedKey(); got != "filters" {
		t.Fatalf("accepting a suggestion moved focus to %q, want to stay on filters", got)
	}
}

func TestBuilderGithubQueryParamSuggestsSearchTerms(t *testing.T) {
	v := builderFor(t, testKit(t))
	v.selectSignal(t, "github")

	v.focus(t, "param.query")
	v.typeIn(t, "is:o")
	v.press(tea.KeyMsg{Type: tea.KeyTab})
	if got := textOf(t, v, "param.query"); got != "is:open" {
		t.Fatalf("param.query = %q, want %q", got, "is:open")
	}

	v.typeIn(t, " author")
	v.press(tea.KeyMsg{Type: tea.KeyTab})
	if got := textOf(t, v, "param.query"); got != "is:open author:@me" {
		t.Fatalf("param.query = %q, want both terms", got)
	}
	if got := v.focusedKey(); got != "param.query" {
		t.Fatalf("two accepted suggestions moved focus to %q, want to stay on param.query", got)
	}
}

func TestBuilderExtraFieldSuggestsParamKeys(t *testing.T) {
	v := builderFor(t, testKit(t))
	v.selectSignal(t, "github")

	v.focus(t, "extra")
	v.typeIn(t, "tea")
	if got := v.Form().Suggestions(); !slices.Contains(got, "team=") {
		t.Fatalf("Suggestions() = %v, want team=", got)
	}
}

func TestBuilderRegexFieldsOfferNothing(t *testing.T) {
	v := builderFor(t, testKit(t))
	v.selectSignal(t, "github")

	v.focus(t, "include")
	v.typeIn(t, "^wip")
	if got := v.Form().Suggestions(); len(got) != 0 {
		t.Fatalf("a regex field has no vocabulary, got %v", got)
	}
	if v.Form().AcceptSuggestion() {
		t.Fatal("AcceptSuggestion must report false so tab keeps its focus meaning")
	}
}

func TestTabAcceptsSuggestionThenSwitchesFocus(t *testing.T) {
	kit := testKit(t)
	v := builderFor(t, kit)
	v.selectSignal(t, "github")

	a := deck.New(v)
	a, _ = updateBuilder(a, tea.WindowSizeMsg{Width: 100, Height: 40})

	v.focus(t, "filters")
	v.typeIn(t, "f")
	a, _ = updateBuilder(a, tea.KeyMsg{Type: tea.KeyTab})
	if got := textOf(t, v, "filters"); got != "f1" {
		t.Fatalf("tab with a suggestion showing must accept it, filters = %q", got)
	}
	if v.OnResults() {
		t.Fatal("tab must not also switch panes in the same keystroke")
	}

	updateBuilder(a, tea.KeyMsg{Type: tea.KeyTab})
	if v.OnResults() {
		t.Fatal("with no results, focus stays on the form")
	}
	if got := textOf(t, v, "filters"); got != "f1" {
		t.Fatalf("a second tab must not re-edit the value, filters = %q", got)
	}
}

func TestFlightEditorSuggestsSavedQueries(t *testing.T) {
	kit := testKit(t)
	fv, ok := kit.FlightBuilder().(*flightView)
	if !ok {
		t.Fatal("FlightBuilder did not return a flightView")
	}

	fm := fv.Form()
	for i := range fm.Fields {
		if fm.Fields[i].Key != "queries" {
			continue
		}
		if fm.Fields[i].Suggest == nil {
			t.Fatal("the queries field has no suggester")
		}
		if got := fm.Fields[i].Delim; got != "," {
			t.Fatalf("Delim = %q, want a comma so each entry completes on its own", got)
		}
		if got := fm.Fields[i].Suggest("q"); !slices.Contains(got, "q1") {
			t.Fatalf("Suggest(q) = %v, want the saved query q1", got)
		}
		return
	}
	t.Fatalf("flight editor has no queries field")
}

func TestRoleFormSuggestsFlightsAndQueries(t *testing.T) {
	kit := testKit(t)
	fields := roleFor(t, kit, "triage").editorFields(nil)

	want := map[string]string{"home": "default", "flights": "default", "queries": "q1"}
	for _, f := range fields {
		w, ok := want[f.Key]
		if !ok {
			continue
		}
		if f.Suggest == nil {
			t.Fatalf("role field %q has no suggester", f.Key)
		}
		if got := f.Suggest(w[:1]); !slices.Contains(got, w) {
			t.Fatalf("role field %q Suggest = %v, want %q", f.Key, got, w)
		}
		delete(want, f.Key)
	}
	if len(want) != 0 {
		t.Fatalf("role form is missing fields: %v", want)
	}
}

func TestSuggesterVocabularyStaysLive(t *testing.T) {
	kit := testKit(t)
	v := builderFor(t, kit)
	v.selectSignal(t, "github")
	v.focus(t, "filters")

	sg := suggesterFor(t, v, "filters")
	before := sg("f")
	kit.d.App.Directives.Queries["f2"] = kit.d.App.Directives.Queries["f1"]
	after := sg("f")
	if len(after) <= len(before) {
		t.Fatalf("a filter added mid-session must show up: before %v, after %v", before, after)
	}
}

func suggesterFor(t *testing.T, v *builderView, key string) forms.Suggester {
	t.Helper()
	for i := range v.Form().Fields {
		if v.Form().Fields[i].Key == key {
			if v.Form().Fields[i].Suggest == nil {
				t.Fatalf("field %q has no suggester", key)
			}
			return v.Form().Fields[i].Suggest
		}
	}
	t.Fatalf("builder has no field %q", key)
	return nil
}

func updateBuilder(a *vkdeck.Model, msg tea.Msg) (*vkdeck.Model, tea.Cmd) {
	m, cmd := a.Update(msg)
	return m.(*vkdeck.Model), cmd
}
