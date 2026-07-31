package demo

import (
	"testing"

	"github.com/codyconfer/munin/plugin"
)

func TestNoLoremEngineDropsPlaceholderNoise(t *testing.T) {
	got := noLoremEngine([]plugin.Item{
		{Title: "real", Body: "ship it"},
		{Title: "Lorem noise", Body: "ok"},
		{Title: "body", Body: "ipsum dolor"},
		{Title: "", Body: "untitled"},
	})
	if len(got) != 1 || got[0].Title != "real" {
		t.Fatalf("demo-no-lorem = %+v", got)
	}
}

func TestRegisterInstallsTheFilterEngine(t *testing.T) {
	Register()
	if !plugin.HasFilterEngine("demo-no-lorem") {
		t.Fatal("demo-no-lorem should be an engine after Register")
	}
}
