//go:build !nodaemon

package daemon

import (
	"slices"
	"testing"
	"time"
)

func TestRunArgs(t *testing.T) {
	got := RunArgs(options{Flight: "work", Interval: 30 * time.Second, Bell: true, Theme: "dark"})
	want := []string{"daemon", "run", "work", "--interval", "30s"}
	if len(got) != len(want) {
		t.Fatalf("args = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("args = %v, want %v", got, want)
		}
	}

	got = RunArgs(options{Flight: "work", Interval: time.Minute, Desktop: true, Theme: "light"})
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

func TestRunArgsCarriesTheHTTPAPI(t *testing.T) {
	// The unit file is written once at install time, so the resolved API
	// settings must be baked in; an installed service that silently lost half
	// its contract because config changed later is the failure to prevent.
	got := RunArgs(options{
		Flight: "work", Interval: time.Minute, Bell: true,
		HTTP: true, HTTPHost: "0.0.0.0", HTTPPort: 9001,
	})
	for _, need := range []string{"--http", "--http-port", "9001", "--http-host", "0.0.0.0"} {
		if !containsArg(got, need) {
			t.Fatalf("missing %q in %v", need, got)
		}
	}
}

func TestRunArgsOmitsTheHTTPAPIWhenOff(t *testing.T) {
	got := RunArgs(options{Flight: "work", Interval: time.Minute, Bell: true, HTTPHost: "0.0.0.0", HTTPPort: 9001})
	for _, unwanted := range []string{"--http", "--http-port", "9001", "--http-host", "0.0.0.0"} {
		if containsArg(got, unwanted) {
			t.Fatalf("%q leaked into %v with HTTP off; the service would expose a port nobody asked for", unwanted, got)
		}
	}
}

func containsArg(args []string, want string) bool { return slices.Contains(args, want) }
