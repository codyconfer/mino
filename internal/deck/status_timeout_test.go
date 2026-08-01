package deck

import (
	"testing"
	"time"

	vkglyph "github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/mino/internal/plugin"
)

func TestPluginServicesSurvivesBlockingContribution(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })

	blockID := "test.deck.status.block"
	if _, ok := plugin.Lookup(blockID); !ok {
		plugin.Register(plugin.Descriptor{
			ID:           blockID,
			Kind:         plugin.KindSignal,
			Signal:       "testdeckstatusblock",
			Capabilities: []plugin.Capability{plugin.CapQuery},
		})
	}
	plugin.RegisterStatusContribution(blockID, func(_, _ string) vkglyph.StatusContribution {
		return vkglyph.StatusContribution{
			Info: func() string { return "blocker" },
			Status: func() (string, vkglyph.Severity) {
				<-release
				return "BLOCKED", vkglyph.SeverityNegative
			},
		}
	})

	fastID := "test.deck.status.fast"
	if _, ok := plugin.Lookup(fastID); !ok {
		plugin.Register(plugin.Descriptor{
			ID:           fastID,
			Kind:         plugin.KindSignal,
			Signal:       "testdeckstatusfast",
			Capabilities: []plugin.Capability{plugin.CapQuery},
		})
	}
	plugin.RegisterStatusContribution(fastID, func(_, _ string) vkglyph.StatusContribution {
		return vkglyph.StatusContribution{
			Info:   func() string { return "fastplug" },
			Status: func() (string, vkglyph.Severity) { return "FAST-MARK", vkglyph.SeverityPositive },
		}
	})

	out := make(chan []ServiceStatus, 1)
	start := time.Now()
	go func() { out <- PluginServices("", "") }()

	select {
	case svcs := <-out:
		if d := time.Since(start); d > 8*time.Second {
			t.Fatalf("PluginServices took %v: a blocking contribution must not hold the status strip", d)
		}
		fast := false
		for _, s := range svcs {
			if s.Glyph == "BLOCKED" {
				t.Fatalf("blocked contribution reported a status: %+v", s)
			}
			if s.Name == "fastplug" && s.Glyph == "FAST-MARK" {
				fast = true
			}
		}
		if !fast {
			t.Fatalf("fast contribution lost while a sibling blocked: %+v", svcs)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("PluginServices never returned: one blocking plugin Status() freezes the status strip for the whole session")
	}
}
