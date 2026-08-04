package build_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/signals/build"
	"github.com/codyconfer/mino/plugin"
)

type nilStream struct{}

func (nilStream) Name() string { return "nilstream" }
func (nilStream) Stream(context.Context) (<-chan plugin.Event, error) {
	ch := make(chan plugin.Event)
	close(ch)
	return ch, nil
}
func (nilStream) LatencyFloor() time.Duration { return time.Second }

func TestSignalRejectsNilQueryFromBuilder(t *testing.T) {
	const signal = "hostnilquery"
	plugin.RegisterSignal(plugin.Descriptor{
		ID:           "test.hostnil.query",
		Kind:         plugin.KindSignal,
		Signal:       signal,
		Capabilities: []plugin.Capability{plugin.CapQuery},
	}, plugin.Builders{
		Query: func(plugin.BuildContext) (plugin.Query, error) { return nil, nil },
	})

	src, err := build.Signal(signal, nil, "", config.Defaults(), nil, nil)
	if err == nil {
		t.Fatalf("build.Signal returned (%v, nil); a (nil, nil) builder must be an error", src)
	}
	if src != nil {
		t.Fatalf("build.Signal returned a non-nil signal with an error: %v", src)
	}
	if !strings.Contains(err.Error(), signal) {
		t.Errorf("error %q should name the signal %q", err, signal)
	}
}

func TestActiveSignalRejectsNilStreamFromBuilder(t *testing.T) {
	const signal = "hostnilstream"
	plugin.RegisterSignal(plugin.Descriptor{
		ID:           "test.hostnil.stream",
		Kind:         plugin.KindSignal,
		Signal:       signal,
		Capabilities: []plugin.Capability{plugin.CapStream},
	}, plugin.Builders{
		Stream: func(plugin.BuildContext) (plugin.Stream, error) { return nil, nil },
	})

	src, err := build.ActiveSignal(signal, nil, "", config.Defaults(), nil, nil)
	if err == nil {
		t.Fatalf("build.ActiveSignal returned (%v, nil); a (nil, nil) builder must be an error", src)
	}
	if src != nil {
		t.Fatalf("build.ActiveSignal returned a non-nil stream with an error: %v", src)
	}
	if !strings.Contains(err.Error(), signal) {
		t.Errorf("error %q should name the signal %q", err, signal)
	}
}

func TestActiveSignalStillBuildsRealStreams(t *testing.T) {
	const signal = "hostokstream"
	plugin.RegisterSignal(plugin.Descriptor{
		ID:           "test.hostok.stream",
		Kind:         plugin.KindSignal,
		Signal:       signal,
		Capabilities: []plugin.Capability{plugin.CapStream},
	}, plugin.Builders{
		Stream: func(plugin.BuildContext) (plugin.Stream, error) { return nilStream{}, nil },
	})

	src, err := build.ActiveSignal(signal, nil, "", config.Defaults(), nil, nil)
	if err != nil || src == nil {
		t.Fatalf("build.ActiveSignal = %v, %v", src, err)
	}
}
