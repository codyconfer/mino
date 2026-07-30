package cmd

import (
	"strings"
	"testing"
)

func TestNoCacheAndRefreshAreMutuallyExclusive(t *testing.T) {
	t.Cleanup(func() { flagNoCache, flagRefresh = false, false })

	fly := findCmd(Root(), "fly")
	if fly == nil {
		t.Fatal("missing top-level `fly` command")
	}
	if err := fly.ParseFlags([]string{"--no-cache", "--refresh"}); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	err := fly.ValidateFlagGroups()
	if err == nil {
		t.Fatal("--no-cache --refresh was accepted; the two have contradictory semantics")
	}
	for _, want := range []string{"no-cache", "refresh"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %v, want it to name %q", err, want)
		}
	}
}

func TestNoCacheAndRefreshAreFineAlone(t *testing.T) {
	t.Cleanup(func() { flagNoCache, flagRefresh = false, false })

	for _, flag := range []string{"--no-cache", "--refresh"} {
		fly := findCmd(Root(), "fly")
		if err := fly.ParseFlags([]string{flag}); err != nil {
			t.Fatalf("ParseFlags(%s): %v", flag, err)
		}
		if err := fly.ValidateFlagGroups(); err != nil {
			t.Errorf("%s alone = %v, want it accepted", flag, err)
		}
		flagNoCache, flagRefresh = false, false
	}
}
