//go:build !nodaemon

package daemon

import (
	"testing"
	"time"
)

func TestDaemonRunArgs(t *testing.T) {
	got := DaemonRunArgs("work", 30*time.Second, true, false, "dark")
	want := []string{"daemon", "run", "work", "--interval", "30s"}
	if len(got) != len(want) {
		t.Fatalf("args = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("args = %v, want %v", got, want)
		}
	}

	got = DaemonRunArgs("work", time.Minute, false, true, "light")
	if got[0] != "daemon" || got[1] != "run" {
		t.Fatalf("service must exec daemon run, got %v", got)
	}
	for _, need := range []string{"--bell=false", "--desktop", "--theme", "light"} {
		if !containsArg(got, need) {
			t.Fatalf("missing %q in %v", need, got)
		}
	}
	if containsArg(got, "serve") {
		t.Fatalf("service must not exec serve, got %v", got)
	}
}

func containsArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}
