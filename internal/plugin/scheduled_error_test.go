package plugin_test

import (
	"context"
	"testing"

	"github.com/codyconfer/munin/internal/plugin"
)

func TestRunScheduledRejectsNilJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := plugin.RunScheduled(ctx, []plugin.Scheduled{nil}, nil)
	if err == nil {
		t.Fatal("RunScheduled returned nil for a nil job: the caller's `if err != nil` branch is unreachable and the scheduler nil-derefs")
	}
}
