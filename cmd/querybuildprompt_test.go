package cmd

import (
	"slices"
	"testing"

	"github.com/codyconfer/munin/internal/app/qform"
	"github.com/codyconfer/munin/internal/signals/build"
)

func TestPromptSeedSplitsKnownParamsFromExtras(t *testing.T) {
	_ = build.KnownSignals()
	f := queryBuildFlags{
		signal:  "github",
		params:  []string{"query=is:unread", "project=owner/12", "custom=on"},
		filters: []string{"no-bots", "mine"},
		field:   "title",
		title:   "Inbox",
		save:    "unread",
	}

	seed := f.promptSeed()

	if got := seed[qform.ParamPrefix+"query"]; got != "is:unread" {
		t.Errorf("param.query = %v, want is:unread", got)
	}
	if got := seed[qform.ParamPrefix+"project"]; got != "owner/12" {
		t.Errorf("param.project = %v, want owner/12", got)
	}
	if got := seed["extra"]; got != "custom=on" {
		t.Errorf("extra = %v, want custom=on", got)
	}
	if got := seed["filters"]; got != "no-bots, mine" {
		t.Errorf("filters = %v, want the comma-joined list", got)
	}
	if got := seed["save"]; got != "unread" {
		t.Errorf("save = %v, want unread", got)
	}
}

func TestPromptApplyRoundTripsThroughFlags(t *testing.T) {
	_ = build.KnownSignals()
	var f queryBuildFlags

	f.apply("github", map[string]any{
		qform.ParamPrefix + "query":   "is:open is:pr author:@me",
		qform.ParamPrefix + "project": "owner/12",
		"extra":                       "custom=on, other=off",
		"filters":                     "no-bots, mine",
		"field":                       "title",
		"include":                     "^wip",
		"exclude":                     "",
		"title":                       "Inbox",
		"save":                        "unread",
	})

	if f.signal != "github" {
		t.Errorf("signal = %q", f.signal)
	}
	for _, want := range []string{"query=is:open is:pr author:@me", "project=owner/12", "custom=on", "other=off"} {
		if !slices.Contains(f.params, want) {
			t.Errorf("params = %v, want it to contain %q", f.params, want)
		}
	}
	if !slices.Equal(f.filters, []string{"no-bots", "mine"}) {
		t.Errorf("filters = %v", f.filters)
	}
	if f.field != "title" || f.include != "^wip" || f.exclude != "" {
		t.Errorf("rule = %q/%q/%q", f.field, f.include, f.exclude)
	}
	if f.save != "unread" || f.title != "Inbox" {
		t.Errorf("save/title = %q/%q", f.save, f.title)
	}
}

func TestPromptApplyDropsBlankParams(t *testing.T) {
	_ = build.KnownSignals()
	var f queryBuildFlags

	f.apply("github", map[string]any{
		qform.ParamPrefix + "query":   "is:open",
		qform.ParamPrefix + "project": "",
		"extra":                       "",
		"filters":                     "",
	})

	if len(f.params) != 1 || f.params[0] != "query=is:open" {
		t.Fatalf("params = %v, want only the filled one", f.params)
	}
	if len(f.filters) != 0 {
		t.Fatalf("filters = %v, want none", f.filters)
	}
}

func TestPromptFieldsCoverEveryQueryBuildFlag(t *testing.T) {
	_ = build.KnownSignals()
	fields := qform.Query(qform.Opts{
		Signal:    "github",
		Prev:      map[string]any{},
		RuleLabel: "rule field",
		NameLabel: "save as",
		NameKey:   "save",
	})

	got := make([]string, 0, len(fields))
	for _, f := range fields {
		got = append(got, f.Key)
	}
	for _, want := range []string{
		qform.ParamPrefix + "query", qform.ParamPrefix + "project",
		"extra", "filters", "field", "include", "exclude", "save", "title",
	} {
		if !slices.Contains(got, want) {
			t.Errorf("prompt fields %v are missing %q", got, want)
		}
	}
}
