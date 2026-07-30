package build

import "testing"

func TestParamsHandsPluginsACopy(t *testing.T) {
	directive := map[string]string{}
	bc := hostBuildCtx{params: directive}

	p := bc.Params()
	p["cursor"] = "page-2"

	if _, ok := directive["cursor"]; ok {
		t.Fatal("a plugin writing to Params() mutated the directive's own params map")
	}
	if len(bc.Params()) != 0 {
		t.Fatalf("Params() still carries the injected key: %+v", bc.Params())
	}
}

func TestParamsCopyKeepsValues(t *testing.T) {
	directive := map[string]string{"q": "is:open", "repo": "o/r"}
	bc := hostBuildCtx{params: directive}

	got := bc.Params()
	if got["q"] != "is:open" || got["repo"] != "o/r" {
		t.Fatalf("Params() = %+v", got)
	}
	got["q"] = "clobbered"
	if directive["q"] != "is:open" {
		t.Fatal("Params() aliased the directive's params for non-empty maps")
	}
}

func TestParamsNilIsUsableAndIsolated(t *testing.T) {
	bc := hostBuildCtx{}
	p := bc.Params()
	if p == nil {
		t.Fatal("Params() returned a nil map: a plugin writing to it panics")
	}
	p["cursor"] = "x"
	if len(bc.Params()) != 0 {
		t.Fatalf("Params() = %+v, want empty", bc.Params())
	}
}
