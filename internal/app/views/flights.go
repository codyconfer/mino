package views

import (
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/codyconfer/viewkit/forms"

	vkdeck "github.com/codyconfer/viewkit/deck"
	"github.com/codyconfer/viewkit/keys"

	"github.com/codyconfer/mino/internal/app/suggest"
	"github.com/codyconfer/mino/internal/app/verify"
	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/render/glyph"
	"github.com/codyconfer/mino/internal/signals"
)

func (kit *Kit) flightsCtx() []keys.Hint {
	return append(kit.menuCtx(), keys.Hint{Key: "directive", Label: "Flights"})
}

func (kit *Kit) Flights() vkdeck.View {
	items := []vkdeck.MenuItem{{
		Label:    "New",
		Desc:     "compose, run, and save a new flight",
		Icon:     glyph.Builder(),
		OnSelect: func(a *vkdeck.Model) tea.Cmd { return a.Push(kit.FlightBuilder()) },
	}}
	for _, n := range kit.d.App.VisibleFlights() {
		n := n
		items = append(items, vkdeck.MenuItem{
			Label:    n,
			Desc:     flightSummary(kit.d.App.Dirs().Flights[n]),
			OnSelect: func(a *vkdeck.Model) tea.Cmd { return a.Push(kit.FlightEditor(n)) },
		})
	}
	return vkdeck.NewMenu("flights", kit.flightsCtx(), items...)
}

func flightSummary(fl config.Flight) string {
	if len(fl.Queries) == 0 {
		return "no queries"
	}
	return strings.Join(fl.Queries, ", ")
}

func (kit *Kit) deleteFlight(name string) (string, error) {
	return kit.deleteDirective(config.TypeFlight, name)
}

type flightView struct {
	*editorShell

	kit  *Kit
	orig string
	base config.Flight
}

func (kit *Kit) FlightBuilder() vkdeck.View {
	return kit.newFlightView("", config.Flight{})
}

func (kit *Kit) FlightEditor(name string) vkdeck.View {
	return kit.newFlightView(name, kit.d.App.Dirs().Flights[name])
}

func (kit *Kit) newFlightView(orig string, base config.Flight) *flightView {
	v := &flightView{kit: kit, orig: orig, base: base}
	v.editorShell = newEditorShell(v, map[string]any{
		"name":    base.Name,
		"queries": strings.Join(base.Queries, ", "),
	})
	return v
}

func (v *flightView) editorKind() string { return "flight" }

func (v *flightView) editorTitle() string {
	if v.orig != "" {
		return "edit " + v.orig
	}
	return "build flight"
}

func (v *flightView) editorCtx() []keys.Hint {
	ctx := v.kit.flightsCtx()
	if v.orig != "" {
		ctx = append(ctx, keys.Hint{Key: "item", Label: v.orig})
	}
	return ctx
}

func (v *flightView) editorSavedName() string { return v.orig }

func (v *flightView) editorFields(prev map[string]any) []forms.Field {
	return []forms.Field{
		{
			Key:     "queries",
			Label:   "queries (comma-sep, in order)",
			Kind:    forms.FieldText,
			Text:    forms.Raw(prev, "queries"),
			Suggest: suggest.Queries(v.kit.d.App),
			Delim:   ",",
		},
		{Key: "name", Label: "name (required to save)", Kind: forms.FieldText, Text: forms.Raw(prev, "name")},
	}
}

func (v *flightView) editorSync() bool { return false }

func (v *flightView) editorSummary() string {
	var parts []string
	if name := v.Value("name"); name != "" {
		parts = append(parts, "name="+name)
	}
	if n := len(v.queryNames()); n > 0 {
		parts = append(parts, "queries="+strconv.Itoa(n))
	}
	if len(parts) == 0 {
		return "unsaved draft"
	}
	return strings.Join(parts, "  ")
}

func (v *flightView) queryNames() []string {
	return directiveSplit(v.Value("queries"))
}

func (v *flightView) flight() (config.Flight, error) {
	names := v.queryNames()
	if len(names) == 0 {
		return config.Flight{}, errs.New(errs.KindConfig, "a flight needs at least one query").
			WithHint("list saved query names, comma-separated")
	}
	var unknown []string
	for _, n := range names {
		if _, ok := v.kit.d.App.Dirs().Queries[n]; !ok {
			unknown = append(unknown, n)
		}
	}
	if len(unknown) > 0 {
		return config.Flight{}, errs.Newf(errs.KindConfig, "unknown queries: %s", strings.Join(unknown, ", ")).
			WithHint("pick from: %s", strings.Join(v.kit.d.App.VisibleQueries(), ", "))
	}
	fl := v.base
	fl.Name = v.Value("name")
	fl.Queries = names
	return fl, nil
}

func (v *flightView) editorValue() (any, error) { return v.flight() }

func (v *flightView) editorRun() (string, func() []signals.Section, error) {
	fl, err := v.flight()
	if err != nil {
		return "", nil, err
	}
	fetch := v.kit.d.FetchFlightQueries
	if fetch == nil {
		return "", nil, errs.New(errs.KindInternal, "flight runs are unavailable in this session")
	}
	label := fl.Name
	if label == "" {
		label = editorAdhocLabel
	}
	names := fl.Queries
	return label, func() []signals.Section { return fetch(label, names) }, nil
}

func (v *flightView) editorVerify(val any) Finding {
	fl, _ := val.(config.Flight)
	name := fl.Name
	if name == "" {
		name = editorAdhocLabel
	}
	return verify.Flight(v.kit.d.App.Dirs(), name, fl)
}

func (v *flightView) editorPersist(val any) (string, error) {
	fl, _ := val.(config.Flight)
	if fl.Name == "" {
		return "", errs.New(errs.KindUsage, "name is required to save")
	}
	d := v.kit.d.App.Dirs()
	if fl.Name != v.orig {
		if _, exists := d.Flights[fl.Name]; exists {
			return "", errs.Newf(errs.KindUsage, "a flight named %s already exists", fl.Name)
		}
	}
	rel := d.Source(config.TypeFlight, v.orig)
	summary, _, err := v.kit.saveDirective(config.TypeFlight, rel, fl.Name, fl)
	if err != nil {
		return "", err
	}
	if fl.Name != v.orig && v.orig != "" {
		summary += editorRenameNote(v.orig, rel)
	}
	v.orig = fl.Name
	v.base = fl
	if err := v.kit.d.App.RefreshDirectives(config.ReconcileIgnore); err == nil {
		summary += "\nit is live in this session."
	}
	return summary, nil
}

func (v *flightView) editorRemove() (string, error) { return v.kit.deleteFlight(v.orig) }
