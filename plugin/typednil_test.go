package plugin_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/codyconfer/munin/plugin"
)

// typedNilQuery models the most common way Go produces a non-nil interface
// wrapping a nil pointer: `var q *typedNilQuery; return q, nil`. Every method
// dereferences the receiver, so the guard must reject it before any call.
type typedNilQuery struct{ name string }

func (q *typedNilQuery) Name() string { return q.name }

func (q *typedNilQuery) Fetch(context.Context) ([]plugin.Section, error) {
	return []plugin.Section{{Signal: q.name}}, nil
}

type typedNilStream struct{ name string }

func (s *typedNilStream) Name() string { return s.name }

func (s *typedNilStream) Stream(context.Context) (<-chan plugin.Event, error) {
	ch := make(chan plugin.Event)
	close(ch)
	return ch, nil
}

func (s *typedNilStream) LatencyFloor() time.Duration { return time.Second }

type typedNilScheduled struct{ name string }

func (s *typedNilScheduled) Name() string { return s.name }

func (s *typedNilScheduled) Next(context.Context, time.Time) (time.Time, bool, error) {
	return time.Time{}, true, nil
}

func (s *typedNilScheduled) Fetch(context.Context) ([]plugin.Section, error) {
	return []plugin.Section{{Signal: s.name}}, nil
}

func TestBuildQueryRejectsTypedNil(t *testing.T) {
	const signal = "typednilquery"
	plugin.RegisterSignal(plugin.Descriptor{
		ID:           "test.typednil.query",
		Kind:         plugin.KindSignal,
		Signal:       signal,
		Capabilities: []plugin.Capability{plugin.CapQuery},
	}, plugin.Builders{
		Query: func(plugin.BuildContext) (plugin.Query, error) {
			var q *typedNilQuery
			return q, nil
		},
	})

	q, err := plugin.BuildQuery(signal, testBuildCtx{})
	if err == nil {
		t.Fatalf("BuildQuery accepted a nil *typedNilQuery inside a non-nil interface; the caller panics on the first method call (q=%v)", q)
	}
	if q != nil {
		t.Fatalf("BuildQuery returned %v alongside an error", q)
	}
	if !strings.Contains(err.Error(), signal) {
		t.Errorf("error %q should name the signal %q", err, signal)
	}
}

func TestBuildStreamRejectsTypedNil(t *testing.T) {
	const signal = "typednilstream"
	plugin.RegisterSignal(plugin.Descriptor{
		ID:           "test.typednil.stream",
		Kind:         plugin.KindSignal,
		Signal:       signal,
		Capabilities: []plugin.Capability{plugin.CapStream},
	}, plugin.Builders{
		Stream: func(plugin.BuildContext) (plugin.Stream, error) {
			var s *typedNilStream
			return s, nil
		},
	})

	s, err := plugin.BuildStream(signal, testBuildCtx{})
	if err == nil {
		t.Fatalf("BuildStream accepted a nil *typedNilStream inside a non-nil interface (s=%v)", s)
	}
	if s != nil {
		t.Fatalf("BuildStream returned %v alongside an error", s)
	}
}

func TestBuildScheduledRejectsTypedNil(t *testing.T) {
	const signal = "typednilsched"
	plugin.RegisterSignal(plugin.Descriptor{
		ID:           "test.typednil.sched",
		Kind:         plugin.KindSignal,
		Signal:       signal,
		Capabilities: []plugin.Capability{plugin.CapScheduled},
	}, plugin.Builders{
		Scheduled: func(plugin.BuildContext) (plugin.Scheduled, error) {
			var j *typedNilScheduled
			return j, nil
		},
	})

	j, err := plugin.BuildScheduled(signal, testBuildCtx{})
	if err == nil {
		t.Fatalf("BuildScheduled accepted a nil *typedNilScheduled inside a non-nil interface (j=%v)", j)
	}
	if j != nil {
		t.Fatalf("BuildScheduled returned %v alongside an error", j)
	}
}

func TestBuildersStillAcceptRealPointerImplementations(t *testing.T) {
	const signal = "typednilhappy"
	plugin.RegisterSignal(plugin.Descriptor{
		ID:           "test.typednil.happy",
		Kind:         plugin.KindSignal,
		Signal:       signal,
		Capabilities: []plugin.Capability{plugin.CapQuery, plugin.CapStream, plugin.CapScheduled},
	}, plugin.Builders{
		Query: func(plugin.BuildContext) (plugin.Query, error) {
			return &typedNilQuery{name: signal}, nil
		},
		Stream: func(plugin.BuildContext) (plugin.Stream, error) {
			return &typedNilStream{name: signal}, nil
		},
		Scheduled: func(plugin.BuildContext) (plugin.Scheduled, error) {
			return &typedNilScheduled{name: signal}, nil
		},
	})

	q, err := plugin.BuildQuery(signal, testBuildCtx{})
	if err != nil || q == nil || q.Name() != signal {
		t.Fatalf("BuildQuery = %v, %v", q, err)
	}
	s, err := plugin.BuildStream(signal, testBuildCtx{})
	if err != nil || s == nil || s.Name() != signal {
		t.Fatalf("BuildStream = %v, %v", s, err)
	}
	j, err := plugin.BuildScheduled(signal, testBuildCtx{})
	if err != nil || j == nil || j.Name() != signal {
		t.Fatalf("BuildScheduled = %v, %v", j, err)
	}
}
