package build_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/codyconfer/sisyphus/daemon"
	"github.com/codyconfer/sisyphus/kv"

	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/signals"
	"github.com/codyconfer/mino/internal/signals/active"
	"github.com/codyconfer/mino/internal/signals/build"
	"github.com/codyconfer/mino/plugin"
)

type kvProbeJob struct{}

func (kvProbeJob) Name() string { return "kvscopeprobe" }

func (kvProbeJob) Next(context.Context, time.Time) (time.Time, bool, error) {
	return time.Time{}, false, nil
}

func (kvProbeJob) Fetch(context.Context) ([]signals.Section, error) { return nil, nil }

// TestPluginKVCannotReachAnotherPluginsNamespace pins the blast radius of the
// KV handle handed to external plugins through the BuildContext: it must not be
// the raw, unnamespaced daemon.KV over every other signal's persisted state.
func TestPluginKVCannotReachAnotherPluginsNamespace(t *testing.T) {
	ctx := context.Background()
	store, err := kv.Open(ctx, filepath.Join(t.TempDir(), "serve.duckdb"))
	if err != nil {
		t.Fatalf("kv.Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	const victimNS = "ntr"
	const victimKey = "reminders:default"
	if err := store.Put(ctx, victimNS, victimKey, "host-watermark", time.Time{}); err != nil {
		t.Fatalf("seed victim namespace: %v", err)
	}

	const signal = "kvscopeprobe"
	var captured daemon.KV
	plugin.RegisterSignal(plugin.Descriptor{
		ID:           "test.kvscope.probe",
		Kind:         plugin.KindSignal,
		Signal:       signal,
		Capabilities: []plugin.Capability{plugin.CapScheduled},
	}, plugin.Builders{
		Scheduled: func(bc plugin.BuildContext) (plugin.Scheduled, error) {
			if src, ok := bc.(interface{ KV() daemon.KV }); ok {
				captured = src.KV()
			}
			return kvProbeJob{}, nil
		},
	})

	if _, err := build.ScheduledJob(signal, nil, config.Defaults(), nil, active.NewState(store)); err != nil {
		t.Fatalf("build.ScheduledJob: %v", err)
	}
	if captured == nil {
		t.Skip("no KV is exposed to plugins at all; nothing to scope")
	}

	if _, found, err := captured.Get(ctx, victimNS, victimKey); err == nil && found {
		t.Errorf("a plugin read another signal's persisted cursor through BuildContext.KV() (namespace %q)", victimNS)
	}
	if entries, err := captured.List(ctx, victimNS); err == nil && len(entries) > 0 {
		t.Errorf("a plugin listed another signal's namespace %q: %v", victimNS, entries)
	}
	_ = captured.Delete(ctx, victimNS, victimKey)
	_ = captured.Put(ctx, victimNS, victimKey, "clobbered", time.Time{})

	e, found, err := store.Get(ctx, victimNS, victimKey)
	if err != nil {
		t.Fatalf("re-read victim: %v", err)
	}
	if !found {
		t.Fatalf("a plugin deleted another signal's persisted cursor (namespace %q, key %q)", victimNS, victimKey)
	}
	if e.Value != "host-watermark" {
		t.Fatalf("a plugin overwrote another signal's persisted cursor: %q", e.Value)
	}

	if err := captured.Put(ctx, "cursor", "page", "2", time.Time{}); err != nil {
		t.Fatalf("plugin cannot persist its own cursor: %v", err)
	}
	got, found, err := captured.Get(ctx, "cursor", "page")
	if err != nil || !found || got.Value != "2" {
		t.Fatalf("plugin cannot read back its own cursor: %v %v %v", got, found, err)
	}
}
