package build_test

import (
	"context"
	"testing"

	"github.com/codyconfer/mino/internal/signals/build"
	"github.com/codyconfer/mino/plugin"
)

func TestPublicRegisterSignalWiresHostBuilders(t *testing.T) {
	const id = "test.overlay.wire"
	const signal = "overlaywire"
	if _, ok := plugin.Lookup(id); ok {
		t.Skip("already registered")
	}
	plugin.RegisterSignal(plugin.Descriptor{
		ID:           id,
		Kind:         plugin.KindSignal,
		Signal:       signal,
		Capabilities: []plugin.Capability{plugin.CapQuery},
	}, plugin.Builders{
		Query: func(plugin.BuildContext) (plugin.Query, error) {
			return overlayQuery{name: signal}, nil
		},
	})
	if !build.HasBuilder(signal) {
		t.Fatal("expected host HasBuilder after public RegisterSignal")
	}
	if !build.BuilderSignals()[signal] {
		t.Fatal("expected BuilderSignals entry")
	}
	if !plugin.KnownSignals()[signal] {
		t.Fatal("expected KnownSignals entry")
	}
}

type overlayQuery struct{ name string }

func (q overlayQuery) Name() string { return q.name }
func (q overlayQuery) Fetch(context.Context) ([]plugin.Section, error) {
	return []plugin.Section{{Signal: q.name, Title: q.name}}, nil
}
