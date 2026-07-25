package plugin_test

import (
	"context"
	"testing"

	"github.com/codyconfer/viewkit/deck"

	"github.com/codyconfer/munin/plugin"
)

func TestUIVisibleServiceOnly(t *testing.T) {
	plugin.SetServiceAttachedFunc(func() bool { return false })
	t.Cleanup(func() { plugin.SetServiceAttachedFunc(nil) })

	plain := plugin.Descriptor{ID: "x", ServiceOnly: false}
	gated := plugin.Descriptor{ID: "y", ServiceOnly: true}
	if !plugin.UIVisible(plain) {
		t.Fatal("non-service-only should be visible when detached")
	}
	if plugin.UIVisible(gated) {
		t.Fatal("service-only should be hidden when detached")
	}

	plugin.SetServiceAttachedFunc(func() bool { return true })
	if !plugin.UIVisible(gated) {
		t.Fatal("service-only should be visible when attached")
	}
}

func TestRegisterViewWithServiceOnly(t *testing.T) {
	id := "test.serviceonly.view"
	viewID := "test.serviceonly.view.home"
	if _, ok := plugin.Lookup(id); !ok {
		plugin.Register(plugin.Descriptor{
			ID: id, Kind: plugin.KindSignal, Signal: "testserviceonlyview",
			Capabilities: []plugin.Capability{plugin.CapQuery},
		})
	}
	plugin.RegisterView(id, viewID, func() deck.View { return stubDeckView{} }, plugin.WithServiceOnly())

	d, ok := plugin.ByKind(plugin.KindView, viewID)
	if !ok {
		t.Fatal("expected KindView descriptor")
	}
	if !d.ServiceOnly {
		t.Fatalf("ServiceOnly = false, want true: %+v", d)
	}

	plugin.SetServiceAttachedFunc(func() bool { return false })
	t.Cleanup(func() { plugin.SetServiceAttachedFunc(nil) })
	if plugin.ViewUIVisible(viewID) {
		t.Fatal("service-only view should be hidden when detached")
	}
	plugin.SetServiceAttachedFunc(func() bool { return true })
	if !plugin.ViewUIVisible(viewID) {
		t.Fatal("service-only view should be visible when attached")
	}
}

func TestRegisterActionWithServiceOnly(t *testing.T) {
	id := "test.serviceonly.act"
	sig := "testserviceonlyact"
	if _, ok := plugin.Lookup(id); !ok {
		plugin.Register(plugin.Descriptor{
			ID: id, Kind: plugin.KindSignal, Signal: sig,
			Capabilities: []plugin.Capability{plugin.CapQuery, plugin.CapAction},
		})
	}
	plugin.RegisterAction(sig, "ping", func(context.Context, map[string]string) error { return nil }, plugin.WithServiceOnly())

	d, ok := plugin.ByKind(plugin.KindAction, sig+"/ping")
	if !ok {
		t.Fatal("expected KindAction companion")
	}
	if !d.ServiceOnly {
		t.Fatalf("ServiceOnly = false, want true: %+v", d)
	}

	plugin.SetServiceAttachedFunc(func() bool { return false })
	t.Cleanup(func() { plugin.SetServiceAttachedFunc(nil) })
	if plugin.ActionUIVisible(sig, "ping") {
		t.Fatal("service-only action should be hidden when detached")
	}
}
