package build_test

import (
	"context"
	"testing"
	"time"

	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/signals"
	"github.com/codyconfer/mino/internal/signals/active"
	"github.com/codyconfer/mino/internal/signals/build"
	"github.com/codyconfer/mino/plugin"
)

type scopeProbeJob struct{}

func (scopeProbeJob) Name() string { return "hostscopeprobe" }

func (scopeProbeJob) Next(context.Context, time.Time) (time.Time, error) {
	return time.Time{}, nil
}

func (scopeProbeJob) Fetch(context.Context) ([]signals.Section, error) { return nil, nil }

func TestHostBuildContextScopesSettingsAndTokens(t *testing.T) {
	const signal = "hostscopeprobe"
	var captured plugin.BuildContext
	plugin.RegisterSignal(plugin.Descriptor{
		ID:           "test.hostscope.probe",
		Kind:         plugin.KindSignal,
		Signal:       signal,
		Capabilities: []plugin.Capability{plugin.CapScheduled},
	}, plugin.Builders{
		Scheduled: func(bc plugin.BuildContext) (plugin.Scheduled, error) {
			captured = bc
			return scopeProbeJob{}, nil
		},
	})

	cfg := config.Defaults()
	cfg.Plugins = map[string]map[string]any{
		signal:   {"max": "7"},
		"google": {"oauth_client_id": "abc"},
	}
	if _, err := build.ScheduledJob(signal, nil, cfg, nil, active.NewState(nil)); err != nil {
		t.Fatalf("build.ScheduledJob: %v", err)
	}
	if captured == nil {
		t.Fatal("scheduled builder never ran")
	}

	if got := plugin.Setting(plugin.SettingsOf(captured, signal), "max", ""); got != "7" {
		t.Errorf("own settings namespace = %q, want 7", got)
	}
	if got := plugin.SettingsOf(captured, "google"); got != nil {
		t.Errorf("foreign settings namespace leaked: %v", got)
	}

	src, ok := captured.(plugin.TokenSource)
	if !ok {
		t.Fatal("host build context should expose GetToken")
	}
	if _, _, _, err := src.GetToken(context.Background(), "github"); err == nil {
		t.Error("GetToken for an undeclared service must error even without a token store")
	}
	if _, _, found, err := src.GetToken(context.Background(), signal); err != nil || found {
		t.Errorf("GetToken(own service) = found %v err %v, want a clean miss with no store", found, err)
	}
}
