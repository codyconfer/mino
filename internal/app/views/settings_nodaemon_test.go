//go:build nodaemon

package views

import (
	"strings"
	"testing"
)

func TestSettingsEditConfigOmitsDaemonFields(t *testing.T) {
	kit := testKit(t)
	vals := setvFormValues(kit.setvEditConfigView())
	for key := range vals {
		if strings.HasPrefix(key, "daemon.") {
			t.Errorf("nodaemon build still offers config field %q", key)
		}
	}
	for _, want := range []string{"output", "timeout", "audit.enabled", "backup.destination", "backup.keep"} {
		if _, ok := vals[want]; !ok {
			t.Errorf("edit config lost field %q: %v", want, vals)
		}
	}
	if body := setvRender(kit.setvEditConfigView()); strings.Contains(body, "daemon") {
		t.Fatalf("nodaemon edit config body mentions daemon: %q", body)
	}
}

func TestSettingsStatusBarOmitsDaemonChip(t *testing.T) {
	kit := testKit(t)
	if _, ok := setvFormValues(kit.setvStatusBarView())["daemon"]; ok {
		t.Error("nodaemon build still offers a daemon status chip toggle")
	}
	body := setvRender(kit.setvStatusBarView())
	if strings.Contains(body, "daemon") {
		t.Fatalf("nodaemon status bar body mentions daemon: %q", body)
	}
	for _, want := range []string{"github", "slack", "google"} {
		if !strings.Contains(body, want) {
			t.Fatalf("status bar lost builtin %q: %q", want, body)
		}
	}
}
