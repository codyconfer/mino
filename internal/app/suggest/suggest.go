package suggest

import (
	"sort"

	"github.com/codyconfer/viewkit/forms"

	"github.com/codyconfer/munin/internal/app"
	"github.com/codyconfer/munin/internal/app/loginflow"
	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/filter"
	"github.com/codyconfer/munin/internal/plugin"
	"github.com/codyconfer/munin/internal/signals/build"
)

func QueryNames(a *app.App) []string {
	if a == nil {
		return nil
	}
	return a.VisibleQueries()
}

func FilterNames(a *app.App) []string {
	if a == nil {
		return nil
	}
	return a.VisibleFilters()
}

func FlightNames(a *app.App) []string {
	if a == nil {
		return nil
	}
	return a.VisibleFlights()
}

func FormatterNames(a *app.App) []string {
	if a == nil {
		return nil
	}
	return a.VisibleFormatters()
}

func RoleNames(a *app.App) []string {
	if a == nil || a.Directives == nil {
		return nil
	}
	return a.Directives.RoleNames()
}

func Signals() []string {
	_ = build.KnownSignals()
	return build.QueryableSignals()
}

func DetailSignals() []string {
	_ = build.KnownSignals()
	return build.DetailSignals()
}

func ActionNames(signal string) []string {
	_ = build.KnownSignals()
	var out []string
	for _, a := range build.Actions(signal) {
		out = append(out, a.Name)
	}
	sort.Strings(out)
	return out
}

func ParamAssignments(signal string) []string {
	keys := build.ParamKeys(signal)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"=")
	}
	return out
}

func PluginIDs() []string {
	var out []string
	for _, d := range plugin.All() {
		out = append(out, d.ID)
	}
	sort.Strings(out)
	return out
}

func InstalledPluginIDs() []string {
	var out []string
	for _, row := range plugin.ListEnabled() {
		out = append(out, row.ID)
	}
	sort.Strings(out)
	return out
}

func Directives() []string { return config.ValidDirectives() }

func ReconcilePolicies() []string { return config.ReconcilePolicyNames() }

func LoginTargets() []string { return loginflow.Names() }

func RuleFields() []string { return filter.FieldNames() }

func OutputFormats() []string { return []string{"terminal", "json"} }

func Themes() []string { return []string{"dark", "light"} }

func Durations() []string {
	return []string{"30s", "1m", "5m", "15m", "30m", "1h"}
}

func Queries(a *app.App) forms.Suggester {
	return forms.From(func() []string { return QueryNames(a) })
}

func Filters(a *app.App) forms.Suggester {
	return forms.From(func() []string { return FilterNames(a) })
}

func Flights(a *app.App) forms.Suggester {
	return forms.From(func() []string { return FlightNames(a) })
}

func Formatters(a *app.App) forms.Suggester {
	return forms.From(func() []string { return FormatterNames(a) })
}

func Roles(a *app.App) forms.Suggester {
	return forms.From(func() []string { return RoleNames(a) })
}

func ParamKeys(signal string) forms.Suggester {
	return forms.Static(ParamAssignments(signal)...)
}

func ParamValues(p build.ParamSpec) forms.Suggester {
	if len(p.Values) == 0 {
		return nil
	}
	return forms.Static(p.Values...)
}

func Fields() forms.Suggester { return forms.Static(RuleFields()...) }

func DurationValues() forms.Suggester { return forms.Static(Durations()...) }
