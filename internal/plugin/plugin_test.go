package plugin

import (
	"context"
	"strings"
	"testing"
)

func TestRegisterAndCapabilities(t *testing.T) {
	id := "test.cap.signal"
	if _, ok := Lookup(id); !ok {
		Register(Descriptor{
			ID:           id,
			Kind:         KindSignal,
			Signal:       "testcap",
			Capabilities: []Capability{CapQuery, CapScheduled},
		})
	}
	if !HasCapability("testcap", CapScheduled) {
		t.Fatal("expected CapScheduled")
	}
	if !KnownSignals()["testcap"] {
		t.Fatal("expected known signal")
	}
}

type stubProvider struct{ tool, cur string }

func (s *stubProvider) Tool() string { return s.tool }
func (s *stubProvider) Switch(_ context.Context, name string) error {
	s.cur = name
	return nil
}
func (s *stubProvider) Current(context.Context) (string, bool, error) {
	if s.cur == "" {
		return "", false, nil
	}
	return s.cur, true, nil
}

func TestContextSwitch(t *testing.T) {
	id := "test.cap.context"
	if _, ok := Lookup(id); !ok {
		Register(Descriptor{ID: id, Kind: KindSignal, Signal: "testcapctx", Capabilities: []Capability{CapQuery}})
	}
	p := &stubProvider{tool: "testtool"}
	RegisterContext(id, p)
	if err := SwitchContext(context.Background(), "testtool", "prod"); err != nil {
		t.Fatal(err)
	}
	c, ok := CurrentContext(context.Background(), "testtool")
	if !ok || c.Name != "prod" {
		t.Fatalf("current = %+v ok=%v", c, ok)
	}
	if _, ok := ByKind(KindContext, "testtool"); !ok {
		t.Fatal("expected KindContext companion")
	}
}

func TestDiagnosticsAreReachableForListings(t *testing.T) {
	Register(Descriptor{ID: "test.diagexport.a", Kind: KindSignal, Signal: "testdiagexport"})
	Register(Descriptor{ID: "test.diagexport.b", Kind: KindSignal, Signal: "testdiagexport"})

	if len(DiagnosticsFor("test.diagexport.b")) == 0 {
		t.Fatal("the skipped contribution has no diagnostic reachable from internal/plugin, so plugins list cannot surface it")
	}
	if !HasDiagnostics() {
		t.Fatal("HasDiagnostics = false")
	}
	found := false
	for _, d := range Diagnostics() {
		if d.PluginID == "test.diagexport.b" {
			found = true
		}
	}
	if !found {
		t.Fatal("Diagnostics() does not include the skipped contribution")
	}
}

func TestDiagnosticLinesRenderForListings(t *testing.T) {
	Register(Descriptor{ID: "test.diaglines.a", Kind: KindSignal, Signal: "testdiaglines"})
	Register(Descriptor{ID: "test.diaglines.b", Kind: KindSignal, Signal: "testdiaglines"})

	for _, line := range DiagnosticLines() {
		if strings.Contains(line, "test.diaglines.b") {
			return
		}
	}
	t.Fatalf("DiagnosticLines has no line for the skipped contribution: %v", DiagnosticLines())
}
