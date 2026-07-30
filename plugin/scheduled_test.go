package plugin_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/codyconfer/munin/plugin"
)

type testScheduled struct{ name string }

func (s testScheduled) Name() string { return s.name }
func (s testScheduled) Next(context.Context, time.Time) (time.Time, bool, error) {
	return time.Time{}, true, nil
}
func (s testScheduled) Fetch(context.Context) ([]plugin.Section, error) {
	return []plugin.Section{{Signal: s.name, Title: "fired"}}, nil
}

func TestScheduledBuilderIsRegisterable(t *testing.T) {
	const id = "test.sched.ok"
	const signal = "schedok"

	plugin.RegisterSignal(plugin.Descriptor{
		ID:           id,
		Kind:         plugin.KindSignal,
		Signal:       signal,
		Capabilities: []plugin.Capability{plugin.CapScheduled},
	}, plugin.Builders{
		Scheduled: func(bc plugin.BuildContext) (plugin.Scheduled, error) {
			return testScheduled{name: signal}, nil
		},
	})

	if !plugin.HasScheduledBuilder(signal) {
		t.Fatal("HasScheduledBuilder = false after registering Builders.Scheduled")
	}
	if !slices.Contains(plugin.ScheduledSignals(), signal) {
		t.Fatalf("ScheduledSignals = %v, want it to contain %q", plugin.ScheduledSignals(), signal)
	}
	if !slices.IsSorted(plugin.ScheduledSignals()) {
		t.Errorf("ScheduledSignals = %v, want sorted", plugin.ScheduledSignals())
	}
	job, err := plugin.BuildScheduled(signal, testBuildCtx{})
	if err != nil || job == nil || job.Name() != signal {
		t.Fatalf("BuildScheduled = %v, %v", job, err)
	}
	if len(plugin.DiagnosticsFor(id)) != 0 {
		t.Errorf("unexpected diagnostics: %v", plugin.DiagnosticsFor(id))
	}
}

func TestCapScheduledWithoutBuilderIsLoud(t *testing.T) {
	const id = "test.sched.silent"
	const signal = "schedsilent"

	plugin.RegisterSignal(plugin.Descriptor{
		ID:           id,
		Kind:         plugin.KindSignal,
		Signal:       signal,
		Capabilities: []plugin.Capability{plugin.CapQuery, plugin.CapScheduled},
	}, plugin.Builders{
		Query: func(plugin.BuildContext) (plugin.Query, error) { return testQuery{name: signal}, nil },
	})

	if plugin.HasScheduledBuilder(signal) {
		t.Fatal("HasScheduledBuilder = true without Builders.Scheduled")
	}
	findDiagnostic(t, id, "CapScheduled", "never run")
}

func TestBuildScheduledRejectsNilNil(t *testing.T) {
	const signal = "schednilnil"
	plugin.RegisterSignal(plugin.Descriptor{
		ID:           "test.sched.nilnil",
		Kind:         plugin.KindSignal,
		Signal:       signal,
		Capabilities: []plugin.Capability{plugin.CapScheduled},
	}, plugin.Builders{
		Scheduled: func(plugin.BuildContext) (plugin.Scheduled, error) { return nil, nil },
	})

	job, err := plugin.BuildScheduled(signal, testBuildCtx{})
	if err == nil {
		t.Fatalf("BuildScheduled returned (%v, nil); want an error", job)
	}
	if job != nil {
		t.Fatalf("BuildScheduled returned a job alongside an error: %v", job)
	}
}

func TestBuildScheduledUnknownSignal(t *testing.T) {
	if _, err := plugin.BuildScheduled("schednosuch", testBuildCtx{}); err == nil {
		t.Fatal("want an error for a signal with no scheduled builder")
	}
}
