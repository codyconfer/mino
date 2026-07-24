package kubectl

import (
	"context"
	"testing"

	"github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/munin/internal/plugin"
)

func TestStubRegistration(t *testing.T) {
	d, ok := plugin.Lookup(PluginID)
	if !ok || d.Signal != SignalName {
		t.Fatalf("lookup = %+v ok=%v", d, ok)
	}
	if _, ok := glyph.Lookup(GlyphID); !ok {
		t.Fatal("glyph missing")
	}
	secs, err := (Signal{}).Fetch(context.Background())
	if err != nil || len(secs) != 1 {
		t.Fatalf("Fetch = %v err=%v", secs, err)
	}
	_ = StatusContribution()
}

// TestSharedProviderUsedByFetch ensures SwitchContext and Fetch share one
// provider instance (the package-level shared value registered at init).
func TestSharedProviderUsedByFetch(t *testing.T) {
	prev := shared.last
	shared.last = ""
	defer func() { shared.last = prev }()

	want := "munin-shared-provider-ctx"
	if err := plugin.SwitchContext(context.Background(), ContextTool, want); err != nil {
		t.Fatal(err)
	}
	if shared.last != want {
		t.Fatalf("SwitchContext did not update shared provider instance: last=%q want=%q", shared.last, want)
	}

	secs, err := (Signal{}).Fetch(context.Background())
	if err != nil || len(secs) != 1 || len(secs[0].Items) == 0 {
		t.Fatalf("Fetch = %v err=%v", secs, err)
	}
	if body := secs[0].Items[0].Body; body != want {
		t.Fatalf("Fetch body = %q, want in-process %q", body, want)
	}
}

func TestSwitchDoesNotRequireKubectlBinary(t *testing.T) {
	prev := shared.last
	shared.last = ""
	defer func() { shared.last = prev }()

	// Switch must succeed without mutating kubeconfig even for a fake name.
	if err := shared.Switch(context.Background(), "definitely-not-a-real-context-xyz"); err != nil {
		t.Fatalf("Switch = %v", err)
	}
	if shared.last != "definitely-not-a-real-context-xyz" {
		t.Fatalf("last = %q", shared.last)
	}
}
