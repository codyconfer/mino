package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

func TestSetValuesKeepsCommentsAndKeyOrder(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envHome, dir)

	src := "# munin config, hand written\n" +
		"output: terminal\n" +
		"\n" +
		"# github access\n" +
		"github:\n" +
		"  max: 30 # capped on purpose\n" +
		"  queries: [\"is:open is:pr\"]\n" +
		"\n" +
		"daemon:\n" +
		"  bell: true\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := SetValues("", map[string]any{"daemon.bell": false}); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	for _, want := range []string{
		"# munin config, hand written",
		"# github access",
		"# capped on purpose",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("comment %q was dropped by SetValues; the user's config file was reformatted:\n%s", want, got)
		}
	}
	if !strings.Contains(got, "bell: false") {
		t.Errorf("daemon.bell was not edited:\n%s", got)
	}
	if idx, jdx := strings.Index(got, "output:"), strings.Index(got, "github:"); idx > jdx {
		t.Errorf("key order was not preserved (output must stay before github):\n%s", got)
	}
	if strings.Index(got, "github:") > strings.Index(got, "daemon:") {
		t.Errorf("key order was not preserved (github must stay before daemon):\n%s", got)
	}
}

func TestSetValuesWritesBackToConfigYml(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envHome, dir)

	ymlPath := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(ymlPath, []byte("output: terminal\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	path, err := SetValues("", map[string]any{"output": "json"})
	if err != nil {
		t.Fatal(err)
	}
	if path != ymlPath {
		t.Errorf("SetValues wrote %q, want the discovered file %q", path, ymlPath)
	}
	if _, err := os.Stat(filepath.Join(dir, "config.yaml")); err == nil {
		t.Error("SetValues created a second config.yaml, leaving the live config.yml stale")
	}
	raw, err := os.ReadFile(ymlPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "output: json") {
		t.Errorf("config.yml was not updated: %q", raw)
	}
}

func TestSetValuesWritesJSONConfigInPlace(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envHome, dir)

	jsonPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(jsonPath, []byte("{\"output\":\"terminal\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path, err := SetValues("", map[string]any{"daemon.bell": false})
	if err != nil {
		t.Fatal(err)
	}
	if path != jsonPath {
		t.Fatalf("SetValues wrote %q, want %q", path, jsonPath)
	}
	raw, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("config.json is no longer valid JSON: %v (%q)", err, raw)
	}
	if got["output"] != "terminal" {
		t.Errorf("output not preserved: %#v", got)
	}
}

func TestSetValuesCreatesConfigWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envHome, dir)

	path, err := SetValues("", map[string]any{"google.oauth_client_id": "abc"})
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(dir, "config.yaml") {
		t.Fatalf("SetValues wrote %q, want a fresh config.yaml", path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "oauth_client_id: abc") {
		t.Errorf("value not written: %q", raw)
	}
}

func TestSetValuesRejectsScalarParent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envHome, dir)
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("output: terminal\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := SetValues("", map[string]any{"output.mode": "json"}); err == nil {
		t.Fatal("SetValues silently replaced a scalar setting with a mapping")
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

func TestSetValuesHandlesACommentOnlyConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envHome, dir)
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("# nothing set yet\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := SetValues("", map[string]any{"output": "json"}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "output: json") {
		t.Fatalf("value not written: %q", raw)
	}
}
