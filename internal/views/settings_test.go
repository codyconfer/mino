package views

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestConfigEditMergePreservesOtherSections(t *testing.T) {
	src := "output: terminal\n" +
		"github:\n  queries: [\"is:open is:pr\"]\n  max: 30\n" +
		"gmail:\n  query: is:unread\n" +
		"daemon:\n  bell: true\n  theme: dark\n"

	doc := map[string]any{}
	if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
		t.Fatal(err)
	}

	doc["output"] = "json"
	setvSetChild(doc, "daemon", "bell", false)
	setvSetChild(doc, "daemon", "theme", "light")
	setvSetChild(doc, "backup", "keep", 3)

	out, err := yaml.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]any{}
	if err := yaml.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}

	gh, ok := got["github"].(map[string]any)
	if !ok || gh["max"] != 30 {
		t.Fatalf("github section not preserved: %#v", got["github"])
	}
	if gm, ok := got["gmail"].(map[string]any); !ok || gm["query"] != "is:unread" {
		t.Fatalf("gmail section not preserved: %#v", got["gmail"])
	}
	if got["output"] != "json" {
		t.Errorf("output not edited: %v", got["output"])
	}
	svc := got["daemon"].(map[string]any)
	if svc["bell"] != false || svc["theme"] != "light" {
		t.Errorf("daemon not merged: %#v", svc)
	}
	if bk := got["backup"].(map[string]any); bk["keep"] != 3 {
		t.Errorf("backup.keep not created: %#v", bk)
	}
}

func TestSetvSetChildCreatesParent(t *testing.T) {
	doc := map[string]any{}
	setvSetChild(doc, "daemon", "tray", true)
	svc, ok := doc["daemon"].(map[string]any)
	if !ok || svc["tray"] != true {
		t.Fatalf("parent not created: %#v", doc)
	}
}
