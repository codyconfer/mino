package plugin_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/codyconfer/munin/plugin"
)

type testStream struct{ name string }

func (s testStream) Name() string { return s.name }
func (s testStream) Stream(context.Context) (<-chan plugin.Event, error) {
	ch := make(chan plugin.Event)
	close(ch)
	return ch, nil
}
func (s testStream) LatencyFloor() time.Duration { return time.Second }

func TestBuildQueryRejectsNilNil(t *testing.T) {
	const signal = "nilnilquery"
	plugin.RegisterSignal(plugin.Descriptor{
		ID:           "test.nilnil.query",
		Kind:         plugin.KindSignal,
		Signal:       signal,
		Capabilities: []plugin.Capability{plugin.CapQuery},
	}, plugin.Builders{
		Query: func(plugin.BuildContext) (plugin.Query, error) { return nil, nil },
	})

	q, err := plugin.BuildQuery(signal, testBuildCtx{})
	if err == nil {
		t.Fatalf("BuildQuery returned (%v, nil); want an error, not a nil query", q)
	}
	if q != nil {
		t.Fatalf("BuildQuery returned a non-nil query alongside an error: %v", q)
	}
	if !strings.Contains(err.Error(), signal) {
		t.Errorf("error %q should name the signal %q", err, signal)
	}
}

func TestBuildStreamRejectsNilNil(t *testing.T) {
	const signal = "nilnilstream"
	plugin.RegisterSignal(plugin.Descriptor{
		ID:           "test.nilnil.stream",
		Kind:         plugin.KindSignal,
		Signal:       signal,
		Capabilities: []plugin.Capability{plugin.CapStream},
	}, plugin.Builders{
		Stream: func(plugin.BuildContext) (plugin.Stream, error) { return nil, nil },
	})

	s, err := plugin.BuildStream(signal, testBuildCtx{})
	if err == nil {
		t.Fatalf("BuildStream returned (%v, nil); want an error, not a nil stream", s)
	}
	if s != nil {
		t.Fatalf("BuildStream returned a non-nil stream alongside an error: %v", s)
	}
	if !strings.Contains(err.Error(), signal) {
		t.Errorf("error %q should name the signal %q", err, signal)
	}
}

func TestBuildQueryStillReturnsRealBuilders(t *testing.T) {
	const signal = "nilnilhappy"
	plugin.RegisterSignal(plugin.Descriptor{
		ID:           "test.nilnil.happy",
		Kind:         plugin.KindSignal,
		Signal:       signal,
		Capabilities: []plugin.Capability{plugin.CapQuery, plugin.CapStream},
	}, plugin.Builders{
		Query:  func(plugin.BuildContext) (plugin.Query, error) { return testQuery{name: signal}, nil },
		Stream: func(plugin.BuildContext) (plugin.Stream, error) { return testStream{name: signal}, nil },
	})

	q, err := plugin.BuildQuery(signal, testBuildCtx{})
	if err != nil || q == nil || q.Name() != signal {
		t.Fatalf("BuildQuery = %v, %v", q, err)
	}
	s, err := plugin.BuildStream(signal, testBuildCtx{})
	if err != nil || s == nil || s.Name() != signal {
		t.Fatalf("BuildStream = %v, %v", s, err)
	}
}

func TestBuilderErrorTakesPrecedence(t *testing.T) {
	const signal = "nilnilerr"
	wantErr := context.Canceled
	plugin.RegisterSignal(plugin.Descriptor{
		ID:           "test.nilnil.err",
		Kind:         plugin.KindSignal,
		Signal:       signal,
		Capabilities: []plugin.Capability{plugin.CapQuery},
	}, plugin.Builders{
		Query: func(plugin.BuildContext) (plugin.Query, error) { return nil, wantErr },
	})

	if _, err := plugin.BuildQuery(signal, testBuildCtx{}); err != wantErr {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}
