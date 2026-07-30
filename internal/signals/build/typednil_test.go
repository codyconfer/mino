package build_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/signals"
	"github.com/codyconfer/munin/internal/signals/active"
	"github.com/codyconfer/munin/internal/signals/build"
	"github.com/codyconfer/munin/plugin"
)

type hostTypedNilQuery struct{ name string }

func (q *hostTypedNilQuery) Name() string { return q.name }

func (q *hostTypedNilQuery) Fetch(context.Context) ([]signals.Section, error) {
	return []signals.Section{{Signal: q.name}}, nil
}

type hostTypedNilStream struct{ name string }

func (s *hostTypedNilStream) Name() string { return s.name }

func (s *hostTypedNilStream) Stream(context.Context) (<-chan plugin.Event, error) {
	ch := make(chan plugin.Event)
	close(ch)
	return ch, nil
}

func (s *hostTypedNilStream) LatencyFloor() time.Duration { return time.Second }

type hostTypedNilJob struct{ name string }

func (j *hostTypedNilJob) Name() string { return j.name }

func (j *hostTypedNilJob) Next(context.Context, time.Time) (time.Time, bool, error) {
	return time.Time{}, true, nil
}

func (j *hostTypedNilJob) Fetch(context.Context) ([]signals.Section, error) {
	return []signals.Section{{Signal: j.name}}, nil
}

func TestSignalRejectsTypedNilQuery(t *testing.T) {
	const signal = "hosttypednilquery"
	plugin.RegisterSignal(plugin.Descriptor{
		ID:           "test.hosttypednil.query",
		Kind:         plugin.KindSignal,
		Signal:       signal,
		Capabilities: []plugin.Capability{plugin.CapQuery},
	}, plugin.Builders{
		Query: func(plugin.BuildContext) (plugin.Query, error) {
			var q *hostTypedNilQuery
			return q, nil
		},
	})

	src, err := build.Signal(signal, nil, config.Defaults(), nil, nil)
	if err == nil {
		t.Fatalf("build.Signal accepted a nil *hostTypedNilQuery inside a non-nil interface: %v", src)
	}
	if src != nil {
		t.Fatalf("build.Signal returned %v alongside an error", src)
	}
	if !strings.Contains(err.Error(), signal) {
		t.Errorf("error %q should name the signal %q", err, signal)
	}
}

func TestActiveSignalRejectsTypedNilStream(t *testing.T) {
	const signal = "hosttypednilstream"
	plugin.RegisterSignal(plugin.Descriptor{
		ID:           "test.hosttypednil.stream",
		Kind:         plugin.KindSignal,
		Signal:       signal,
		Capabilities: []plugin.Capability{plugin.CapStream},
	}, plugin.Builders{
		Stream: func(plugin.BuildContext) (plugin.Stream, error) {
			var s *hostTypedNilStream
			return s, nil
		},
	})

	src, err := build.ActiveSignal(signal, nil, config.Defaults(), nil, active.NewState(nil))
	if err == nil {
		t.Fatalf("build.ActiveSignal accepted a nil *hostTypedNilStream inside a non-nil interface: %v", src)
	}
	if src != nil {
		t.Fatalf("build.ActiveSignal returned %v alongside an error", src)
	}
}

func TestScheduledJobRejectsTypedNilJob(t *testing.T) {
	const signal = "hosttypednilsched"
	plugin.RegisterSignal(plugin.Descriptor{
		ID:           "test.hosttypednil.sched",
		Kind:         plugin.KindSignal,
		Signal:       signal,
		Capabilities: []plugin.Capability{plugin.CapScheduled},
	}, plugin.Builders{
		Scheduled: func(plugin.BuildContext) (plugin.Scheduled, error) {
			var j *hostTypedNilJob
			return j, nil
		},
	})

	job, err := build.ScheduledJob(signal, nil, config.Defaults(), nil, active.NewState(nil))
	if err == nil {
		t.Fatalf("build.ScheduledJob accepted a nil *hostTypedNilJob inside a non-nil interface: %v", job)
	}
	if job != nil {
		t.Fatalf("build.ScheduledJob returned %v alongside an error", job)
	}
}
