package cmd

import (
	"slices"
	"testing"

	"github.com/spf13/cobra"

	"github.com/codyconfer/munin/internal/app"
	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/signals/build"
)

func withDirectives(t *testing.T) {
	t.Helper()
	shared = &app.App{
		Cfg: &config.Config{Home: t.TempDir()},
		Directives: &config.Directives{
			Queries: map[string]config.Query{
				"stale-prs": {Name: "stale-prs", Signal: "github"},
				"standup":   {Name: "standup", Signal: "github"},
			},
			Flights: map[string]config.Flight{
				"morning": {Name: "morning", Queries: []string{"standup"}},
			},
			Roles: map[string]config.RoleDef{
				"focus": {Name: "focus"},
			},
		},
	}
	t.Cleanup(func() { shared = nil })
}

func names(t *testing.T, fn completer, args ...string) []string {
	t.Helper()
	out, _ := fn(&cobra.Command{}, args, "")
	return out
}

func TestCompletersReturnTheirVocabulary(t *testing.T) {
	withDirectives(t)

	cases := []struct {
		name string
		got  []string
		want string
	}{
		{"queries", names(t, completeQueryNames), "stale-prs"},
		{"flights", names(t, completeFlightNames), "morning"},
		{"roles", names(t, completeRoleNames), "focus"},
		{"directives", names(t, completeDirectives), "config"},
		{"list kinds", names(t, completeListKinds), "flights"},
		{"reconcile policies", names(t, completeFlagValues(config.ReconcilePolicyNames)), "ignore"},
		{"output formats", names(t, completeFlagValues(func() []string { return []string{"terminal", "json"} })), "json"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !slices.Contains(c.got, c.want) {
				t.Fatalf("got %v, want it to contain %q", c.got, c.want)
			}
		})
	}
}

func TestPositionalCompletersStopAfterTheirArg(t *testing.T) {
	withDirectives(t)

	if got := names(t, completeQueryNames, "stale-prs"); len(got) != 0 {
		t.Fatalf("a single-arg command must offer nothing for a second arg, got %v", got)
	}
	if got := names(t, completeFlightNames, "morning"); len(got) != 0 {
		t.Fatalf("got %v, want none", got)
	}
}

func TestCompleteActionRunIsPositionAware(t *testing.T) {
	withDirectives(t)
	_ = build.KnownSignals()

	first := names(t, completeActionRun)
	if !slices.Contains(first, "github") {
		t.Fatalf("first arg should offer signals, got %v", first)
	}
	// The second arg is scoped to the signal already on the line, so an
	// unknown signal must not fall back to offering every action.
	if got := names(t, completeActionRun, "not-a-signal"); len(got) != 0 {
		t.Fatalf("got %v, want none", got)
	}
	if got := names(t, completeActionRun, "github", "some-action"); len(got) != 0 {
		t.Fatalf("a third arg has nothing to offer, got %v", got)
	}
}

func TestCompleteParamAssignmentsFollowsTheSignalFlag(t *testing.T) {
	withDirectives(t)
	_ = build.KnownSignals()

	c := newQueryBuildCmd()
	if got, _ := completeParamAssignments(c, nil, ""); len(got) != 0 {
		t.Fatalf("without --signal there is nothing to offer, got %v", got)
	}
	if err := c.Flags().Set("signal", "gmail"); err != nil {
		t.Fatal(err)
	}
	got, directive := completeParamAssignments(c, nil, "")
	if !slices.Contains(got, "query=") || !slices.Contains(got, "max=") {
		t.Fatalf("got %v, want gmail's params as key= pairs", got)
	}
	if directive&cobra.ShellCompDirectiveNoSpace == 0 {
		t.Error("a key= pair must not get a trailing space; the value follows it")
	}
}

func TestRootBindsPersistentFlagCompletions(t *testing.T) {
	root := Root()
	for _, flag := range []string{"output", "reconcile", "role", "home", "dir"} {
		if _, ok := root.GetFlagCompletionFunc(flag); !ok {
			t.Errorf("--%s has no completion function", flag)
		}
	}
}

func TestCompletionSkipsOnboardingAndHeavyLoad(t *testing.T) {
	for _, name := range []string{"__complete", "__completeNoDesc", "completion"} {
		c := &cobra.Command{Use: name, Run: func(*cobra.Command, []string) {}}
		if !isCompletion(c) {
			t.Errorf("%s must be treated as a completion run", name)
		}
		if !skipsOnboarding(c) {
			t.Errorf("%s must skip the onboarding gate", name)
		}
	}
	c := &cobra.Command{Use: "fly", Run: func(*cobra.Command, []string) {}}
	if isCompletion(c) {
		t.Error("an ordinary command must not be treated as a completion run")
	}
}
