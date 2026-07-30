package cmd

import (
	"os"
	"strings"

	"github.com/charmbracelet/x/term"

	"github.com/codyconfer/viewkit/forms"

	vkdeck "github.com/codyconfer/viewkit/deck"

	"github.com/codyconfer/munin/internal/app/qform"
	"github.com/codyconfer/munin/internal/deck"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/signals/build"
)

func (f *queryBuildFlags) prompt() error {
	if !term.IsTerminal(os.Stdin.Fd()) {
		return errs.New(errs.KindUsage, "-i needs a terminal").
			WithHint("drop -i and pass --signal/--param instead")
	}
	_ = build.KnownSignals()
	signals := build.QueryableSignals()
	if len(signals) == 0 {
		return errs.New(errs.KindConfig, "no queryable signals are enabled")
	}

	sigIdx := forms.SelectIndex(signals, f.signal)
	seed := f.promptSeed()

	spec := vkdeck.PromptSpec{
		Title: "build query",
		Seed:  seed,
		Fields: func(prev map[string]any) []forms.Field {
			fields := []forms.Field{{
				Key:      "signal",
				Label:    "signal (required)",
				Kind:     forms.FieldSelect,
				Options:  signals,
				Selected: sigIdx,
			}}
			return append(fields, qform.Query(qform.Opts{
				App:       shared,
				Signal:    signals[sigIdx],
				Prev:      prev,
				RuleLabel: "inline rule field (blank = whole item)",
				NameLabel: "save as (blank = do not save)",
				NameKey:   "save",
			})...)
		},
		Sync: func(vals map[string]any) bool {
			idx := forms.SelectIndex(signals, forms.Str(vals, "signal"))
			if idx == sigIdx {
				return false
			}
			sigIdx = idx
			return true
		},
	}

	vals, ok, err := deck.Prompt(spec)
	if err != nil {
		return err
	}
	if !ok {
		return errs.New(errs.KindUsage, "cancelled")
	}
	f.apply(signals[sigIdx], vals)
	return nil
}

func (f *queryBuildFlags) promptSeed() map[string]any {
	seed := map[string]any{
		"filters": strings.Join(f.filters, ", "),
		"field":   f.field,
		"include": f.include,
		"exclude": f.exclude,
		"title":   f.title,
		"save":    f.save,
	}
	known := map[string]bool{}
	for _, k := range build.ParamKeys(f.signal) {
		known[k] = true
	}
	var extra []string
	for _, pair := range f.params {
		k, _, _ := strings.Cut(pair, "=")
		if known[strings.TrimSpace(k)] {
			seed[qform.ParamPrefix+strings.TrimSpace(k)] = pairValue(pair)
			continue
		}
		extra = append(extra, pair)
	}
	seed["extra"] = strings.Join(extra, ", ")
	return seed
}

func (f *queryBuildFlags) apply(signal string, vals map[string]any) {
	f.signal = signal
	f.params = nil
	for _, k := range build.ParamKeys(signal) {
		if v := forms.Str(vals, qform.ParamPrefix+k); v != "" {
			f.params = append(f.params, k+"="+v)
		}
	}
	f.params = append(f.params, splitList(forms.Str(vals, "extra"))...)
	f.filters = splitList(forms.Str(vals, "filters"))
	f.field = forms.Str(vals, "field")
	f.include = forms.Str(vals, "include")
	f.exclude = forms.Str(vals, "exclude")
	f.title = forms.Str(vals, "title")
	f.save = forms.Str(vals, "save")
}

func pairValue(pair string) string {
	_, v, _ := strings.Cut(pair, "=")
	return v
}

func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
