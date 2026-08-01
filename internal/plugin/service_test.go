package plugin

import (
	"testing"

	"github.com/codyconfer/sisyphus/mode"

	pub "github.com/codyconfer/mino/plugin"
)

func TestServiceAttachedAtEmpty(t *testing.T) {
	if ServiceAttachedAt("") {
		t.Fatal("empty home should be detached")
	}
	if ServiceAttachedAt(t.TempDir()) {
		t.Fatal("temp home without socket should be detached")
	}
}

func TestServiceAttachedRespectsDaemonSupported(t *testing.T) {
	if mode.DaemonSupported {
		t.Skip("covered by empty-home cases in default builds")
	}
	if ServiceAttachedAt("/anywhere") {
		t.Error("nodaemon builds have no serve socket to attach to")
	}
	if ServiceAttached() {
		t.Error("nodaemon builds have no serve socket to attach to")
	}
	if UIVisible(Descriptor{ServiceOnly: true}) {
		t.Error("service-only contributions must stay hidden in nodaemon builds")
	}
}

func TestUIVisibleReExport(t *testing.T) {
	pub.SetServiceAttachedFunc(func() bool { return false })
	t.Cleanup(func() { pub.SetServiceAttachedFunc(ServiceAttached) })

	if !UIVisible(Descriptor{ServiceOnly: false}) {
		t.Fatal("plain should be visible")
	}
	if UIVisible(Descriptor{ServiceOnly: true}) {
		t.Fatal("service-only should be hidden when detached")
	}
}
