package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestSetValuesMergesAndPreservesSections(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envHome, dir)

	src := "output: terminal\n" +
		"github:\n  queries: [\"is:open is:pr\"]\n  max: 30\n" +
		"gmail:\n  query: is:unread\n" +
		"daemon:\n  bell: true\n  theme: dark\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}

	path, err := SetValues("", map[string]any{
		"output":       "json",
		"daemon.bell":  false,
		"daemon.theme": "light",
		"backup.keep":  3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(dir, "config.yaml") {
		t.Fatalf("unexpected path %q", path)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]any{}
	if err := yaml.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}

	if gh, ok := got["github"].(map[string]any); !ok || gh["max"] != 30 {
		t.Fatalf("github section not preserved: %#v", got["github"])
	}
	if gm, ok := got["gmail"].(map[string]any); !ok || gm["query"] != "is:unread" {
		t.Fatalf("gmail section not preserved: %#v", got["gmail"])
	}
	if got["output"] != "json" {
		t.Errorf("output not edited: %v", got["output"])
	}
	dm := got["daemon"].(map[string]any)
	if dm["bell"] != false || dm["theme"] != "light" {
		t.Errorf("daemon not merged: %#v", dm)
	}
	if bk := got["backup"].(map[string]any); bk["keep"] != 3 {
		t.Errorf("backup.keep not created: %#v", bk)
	}
}

func TestSetValuesCreatesNestedParent(t *testing.T) {
	doc := map[string]any{}
	setDotted(doc, "google.oauth_client_id", "abc")
	g, ok := doc["google"].(map[string]any)
	if !ok || g["oauth_client_id"] != "abc" {
		t.Fatalf("nested parent not created: %#v", doc)
	}
}
