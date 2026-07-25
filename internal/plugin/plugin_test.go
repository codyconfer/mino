package plugin

import (
	"context"
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
