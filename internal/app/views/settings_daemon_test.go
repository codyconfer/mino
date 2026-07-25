//go:build !nodaemon

package views

import (
	"strings"
	"testing"
)

func TestSettingsEditConfigHasDaemonFields(t *testing.T) {
	kit := testKit(t)
	vals := kit.setvEditConfigView().(*setvEditForm).form.Values()
	for _, key := range []string{"daemon.interval", "daemon.bell", "daemon.desktop", "daemon.tray", "daemon.theme"} {
		if _, ok := vals[key]; !ok {
			t.Errorf("edit config missing field %q: %v", key, vals)
		}
	}
	if body := setvRender(kit.setvEditConfigView()); !strings.Contains(body, "daemon.interval") {
		t.Fatalf("edit config body missing daemon rows: %q", body)
	}
}

func TestSettingsStatusBarHasDaemonChip(t *testing.T) {
	kit := testKit(t)
	if _, ok := kit.setvStatusBarView().(*setvStatusBarForm).form.Values()["daemon"]; !ok {
		t.Error("status bar form missing daemon toggle")
	}
	if body := setvRender(kit.setvStatusBarView()); !strings.Contains(body, "daemon") {
		t.Fatalf("status bar body missing daemon toggle: %q", body)
	}
}
