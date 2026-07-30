package filter

import "testing"

func TestExpandParamsAlwaysCopies(t *testing.T) {
	empty := map[string]string{}
	out, err := ExpandParams(empty, nil)
	if err != nil {
		t.Fatalf("ExpandParams: %v", err)
	}
	out["cursor"] = "injected"
	if _, ok := empty["cursor"]; ok {
		t.Fatal("ExpandParams handed back the caller's map: a plugin writing to it mutates the loaded directive for the whole process")
	}

	src := map[string]string{"q": "is:open"}
	out, err = ExpandParams(src, nil)
	if err != nil {
		t.Fatalf("ExpandParams: %v", err)
	}
	out["cursor"] = "injected"
	if _, ok := src["cursor"]; ok {
		t.Fatal("ExpandParams leaked the caller's map for non-empty params")
	}
	if out["q"] != "is:open" {
		t.Fatalf("expanded params lost their values: %+v", out)
	}
}
