package build_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/signals/build"
	"github.com/codyconfer/mino/plugin"
)

type hostScheduled struct{ name string }

func (s hostScheduled) Name() string { return s.name }
func (s hostScheduled) Next(_ context.Context, now time.Time) (time.Time, error) {
	return now, nil
}
func (s hostScheduled) Fetch(context.Context) ([]plugin.Section, error) {
	return []plugin.Section{{Signal: s.name, Title: "fired"}}, nil
}

func TestScheduledJobBuildsExternalPluginJobs(t *testing.T) {
	const signal = "hostschedok"
	plugin.RegisterSignal(plugin.Descriptor{
		ID:           "test.hostsched.ok",
		Kind:         plugin.KindSignal,
		Signal:       signal,
		Capabilities: []plugin.Capability{plugin.CapScheduled},
	}, plugin.Builders{
		Scheduled: func(bc plugin.BuildContext) (plugin.Scheduled, error) {
			return hostScheduled{name: signal}, nil
		},
	})

	if !build.HasScheduledBuilder(signal) {
		t.Fatal("build.HasScheduledBuilder = false")
	}
	job, err := build.ScheduledJob(signal, nil, "", config.Defaults(), nil, nil)
	if err != nil {
		t.Fatalf("build.ScheduledJob: %v", err)
	}
	if job == nil || job.Name() != signal {
		t.Fatalf("job = %v", job)
	}
	if !slices.Contains(build.ScheduledSignals(), signal) {
		t.Fatalf("build.ScheduledSignals = %v, want it to contain %q", build.ScheduledSignals(), signal)
	}
}

func TestScheduledJobRejectsNilNil(t *testing.T) {
	const signal = "hostschednil"
	plugin.RegisterSignal(plugin.Descriptor{
		ID:           "test.hostsched.nil",
		Kind:         plugin.KindSignal,
		Signal:       signal,
		Capabilities: []plugin.Capability{plugin.CapScheduled},
	}, plugin.Builders{
		Scheduled: func(plugin.BuildContext) (plugin.Scheduled, error) { return nil, nil },
	})

	job, err := build.ScheduledJob(signal, nil, "", config.Defaults(), nil, nil)
	if err == nil {
		t.Fatalf("build.ScheduledJob returned (%v, nil); want an error", job)
	}
	if job != nil {
		t.Fatalf("build.ScheduledJob returned a job alongside an error: %v", job)
	}
}

func TestScheduledJobRejectsSignalWithoutTheCapability(t *testing.T) {
	const signal = "hostschednocap"
	plugin.RegisterSignal(plugin.Descriptor{
		ID:           "test.hostsched.nocap",
		Kind:         plugin.KindSignal,
		Signal:       signal,
		Capabilities: []plugin.Capability{plugin.CapQuery},
	}, plugin.Builders{
		Scheduled: func(plugin.BuildContext) (plugin.Scheduled, error) {
			return hostScheduled{name: signal}, nil
		},
	})

	if _, err := build.ScheduledJob(signal, nil, "", config.Defaults(), nil, nil); err == nil {
		t.Fatal("want an error for a signal that does not advertise CapScheduled")
	}
}

func TestScheduledJobWithoutBuilder(t *testing.T) {
	const signal = "hostschednone"
	plugin.RegisterSignal(plugin.Descriptor{
		ID:           "test.hostsched.none",
		Kind:         plugin.KindSignal,
		Signal:       signal,
		Capabilities: []plugin.Capability{plugin.CapQuery},
	}, plugin.Builders{
		Query: func(plugin.BuildContext) (plugin.Query, error) { return overlayQuery{name: signal}, nil },
	})

	_, err := build.ScheduledJob(signal, nil, "", config.Defaults(), nil, nil)
	if !errors.Is(err, build.ErrNoScheduled) {
		t.Fatalf("err = %v, want ErrNoScheduled", err)
	}
}

func TestScheduledJobReportsCapWithoutBuilder(t *testing.T) {
	const signal = "hostschedliar"
	plugin.RegisterSignal(plugin.Descriptor{
		ID:           "test.hostsched.liar",
		Kind:         plugin.KindSignal,
		Signal:       signal,
		Capabilities: []plugin.Capability{plugin.CapQuery, plugin.CapScheduled},
	}, plugin.Builders{
		Query: func(plugin.BuildContext) (plugin.Query, error) { return overlayQuery{name: signal}, nil },
	})

	_, err := build.ScheduledJob(signal, nil, "", config.Defaults(), nil, nil)
	if err == nil {
		t.Fatal("want an error naming the missing scheduled builder")
	}
	if errors.Is(err, build.ErrNoScheduled) {
		t.Fatalf("err = %v, want a CapScheduled mismatch error", err)
	}
}
