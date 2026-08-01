package qform

import (
	"github.com/codyconfer/viewkit/forms"

	"github.com/codyconfer/mino/internal/app"
	"github.com/codyconfer/mino/internal/app/suggest"
	"github.com/codyconfer/mino/internal/signals/build"
)

const ParamPrefix = "param."

type Opts struct {
	App    *app.App
	Signal string
	Prev   map[string]any

	RuleLabel string
	NameLabel string
	NameKey   string
}

func Params(signal string, prev map[string]any) []forms.Field {
	specs := build.QueryParams(signal)
	out := make([]forms.Field, 0, len(specs))
	for _, p := range specs {
		out = append(out, forms.Field{
			Key:     ParamPrefix + p.Key,
			Label:   ParamLabel(p),
			Kind:    forms.FieldText,
			Text:    forms.Raw(prev, ParamPrefix+p.Key),
			Suggest: suggest.ParamValues(p),
			Delim:   p.Delim,
		})
	}
	return out
}

func ParamLabel(p build.ParamSpec) string {
	if p.Example != "" {
		return p.Key + " (e.g. " + p.Example + ")"
	}
	return p.Key
}

func Extra(signal string, prev map[string]any) forms.Field {
	return forms.Field{
		Key:     "extra",
		Label:   "extra params (k=v, comma-sep)",
		Kind:    forms.FieldText,
		Text:    forms.Raw(prev, "extra"),
		Suggest: suggest.ParamKeys(signal),
		Delim:   ",",
	}
}

func Filters(a *app.App, prev map[string]any) forms.Field {
	return forms.Field{
		Key:     "filters",
		Label:   "filters (comma-sep saved filters)",
		Kind:    forms.FieldText,
		Text:    forms.Raw(prev, "filters"),
		Suggest: suggest.Filters(a),
		Delim:   ",",
	}
}

func Rules(label string, prev map[string]any) []forms.Field {
	return []forms.Field{
		{Key: "field", Label: label, Kind: forms.FieldText, Text: forms.Raw(prev, "field"), Suggest: suggest.Fields()},
		{Key: "include", Label: "rule include regex", Kind: forms.FieldText, Text: forms.Raw(prev, "include")},
		{Key: "exclude", Label: "rule exclude regex", Kind: forms.FieldText, Text: forms.Raw(prev, "exclude")},
	}
}

func Query(o Opts) []forms.Field {
	out := Params(o.Signal, o.Prev)
	out = append(out, Extra(o.Signal, o.Prev), Filters(o.App, o.Prev))
	out = append(out, Rules(o.RuleLabel, o.Prev)...)
	if o.NameLabel != "" {
		key := o.NameKey
		if key == "" {
			key = "name"
		}
		out = append(out, forms.Field{
			Key: key, Label: o.NameLabel, Kind: forms.FieldText, Text: forms.Raw(o.Prev, key),
		})
	}
	return append(out, forms.Field{
		Key: "title", Label: "display title (optional)", Kind: forms.FieldText, Text: forms.Raw(o.Prev, "title"),
	})
}
