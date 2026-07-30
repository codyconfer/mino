package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/codyconfer/munin/internal/errs"
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
	src := "{\n  \"output\": \"terminal\",\n  \"github\": {\n    \"max\": 30,\n    \"api_url\": \"https://x\"\n  },\n  \"gmail\": {\n    \"query\": \"is:unread\"\n  }\n}\n"
	if err := os.WriteFile(jsonPath, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	path, err := SetValues("", map[string]any{"daemon.bell": false, "github.max": 99})
	if err != nil {
		t.Fatal(err)
	}
	if path != jsonPath {
		t.Fatalf("SetValues wrote %q, want %q", path, jsonPath)
	}
	if _, err := os.Stat(filepath.Join(dir, "config.yaml")); err == nil {
		t.Error("SetValues created a config.yaml beside the live config.json")
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
	gh, ok := got["github"].(map[string]any)
	if !ok || gh["max"] != float64(99) || gh["api_url"] != "https://x" {
		t.Errorf("github section not merged in place: %#v", got["github"])
	}
	if gm, ok := got["gmail"].(map[string]any); !ok || gm["query"] != "is:unread" {
		t.Errorf("gmail section not preserved: %#v", got["gmail"])
	}
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("the rewritten config.json no longer loads: %v\n%s", err, raw)
	}
	if cfg.Output != "terminal" || cfg.GitHub.Max != 99 || cfg.Daemon.Bell {
		t.Errorf("rewritten config.json loaded wrong: output=%q max=%d bell=%v", cfg.Output, cfg.GitHub.Max, cfg.Daemon.Bell)
	}
	body := string(raw)
	order := []string{"\"output\"", "\"github\"", "\"gmail\"", "\"daemon\""}
	at := -1
	for _, key := range order {
		i := strings.Index(body, key)
		if i < 0 {
			t.Fatalf("key %s missing from the rewritten config.json:\n%s", key, body)
		}
		if i < at {
			t.Errorf("SetValues reordered the JSON config: %s moved ahead of an earlier key:\n%s", key, body)
		}
		at = i
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
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if len(names) != 1 || names[0] != "config.yaml" {
		t.Fatalf("SetValues left %v in the home, want only config.yaml", names)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := yaml.Unmarshal(raw, &got); err != nil {
		t.Fatalf("the created config is not valid YAML: %v (%q)", err, raw)
	}
	g, ok := got["google"].(map[string]any)
	if !ok || g["oauth_client_id"] != "abc" {
		t.Fatalf("value not nested under google: %#v", got)
	}
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("the config SetValues created does not load back: %v\n%s", err, raw)
	}
	if cfg.Google.OAuthClientID != "abc" {
		t.Errorf("Load() = %q, want the value SetValues wrote", cfg.Google.OAuthClientID)
	}
}

func TestSetValuesRejectsScalarParent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envHome, dir)
	const src = "output: terminal\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := SetValues("", map[string]any{"output.mode": "json"}); err == nil {
		t.Fatal("SetValues silently replaced a scalar setting with a mapping")
	}
	raw, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != src {
		t.Errorf("the refused write still rewrote the config, so the user's value is gone either way:\n%s", raw)
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
	src := "# nothing set yet\n# but these notes matter\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := SetValues("", map[string]any{"output": "json"}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if !strings.Contains(body, "output: json") {
		t.Fatalf("value not written: %q", raw)
	}
	if strings.Contains(body, "null") {
		t.Errorf("a comment-only config was replaced by a null document:\n%s", body)
	}
	for _, want := range []string{"# nothing set yet", "# but these notes matter"} {
		if !strings.Contains(body, want) {
			t.Errorf("comment %q was dropped from a comment-only config:\n%s", want, body)
		}
	}
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("the rewritten comment-only config does not load: %v\n%s", err, body)
	}
	if cfg.Output != "json" {
		t.Errorf("Output = %q, want json", cfg.Output)
	}
}

func TestSetValuesResolvesAYAMLAlias(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envHome, dir)
	src := "base: &base\n  max: 10\ngithub: *base\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := SetValues("", map[string]any{"github.max": 99}); err != nil {
		t.Fatalf("SetValues refused a config that uses a YAML alias, so `munin login` cannot write its client id: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := yaml.Unmarshal(raw, &got); err != nil {
		t.Fatalf("the rewritten config is not valid YAML: %v (%q)", err, raw)
	}
	gh, ok := got["github"].(map[string]any)
	if !ok || gh["max"] != 99 {
		t.Fatalf("github.max was not set through the alias: %#v", got["github"])
	}
	base, ok := got["base"].(map[string]any)
	if !ok || base["max"] != 10 {
		t.Errorf("editing through the alias changed the anchor it pointed at: %#v", got["base"])
	}
}

func TestSetValuesRejectsAnAliasOfAScalar(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envHome, dir)
	src := "base: &base 10\ngithub: *base\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := SetValues("", map[string]any{"github.max": 99})
	if err == nil {
		t.Fatal("SetValues accepted a nested write under an alias of a scalar")
	}
	full := err.Error() + " " + errs.Hint(err)
	if !strings.Contains(full, "base") {
		t.Errorf("the error must name the anchor so the user can find it, got %q", full)
	}
	if strings.Contains(full, "github: base") {
		t.Errorf("the hint quotes the anchor name as if it were the text in the file, got %q", full)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != src {
		t.Errorf("a refused edit still rewrote the config:\n%s", raw)
	}
}

func TestSetValuesRefusesAMultiDocumentConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envHome, dir)
	src := "output: terminal\n---\ndaemon:\n  bell: true\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := SetValues("", map[string]any{"output": "json"}); err == nil {
		t.Error("SetValues accepted a multi-document config; documents 2..N are dropped silently")
	}
	raw, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != src {
		t.Errorf("documents after the first were lost:\n%s", raw)
	}
}

func TestSetValuesKeepsAMergeKeyAsWritten(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envHome, dir)
	src := "defaults: &d\n  max: 1\ngithub:\n  <<: *d\n  api_url: x\ndaemon:\n  bell: true\n"
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
	body := string(raw)
	if strings.Contains(body, "!!merge") {
		t.Errorf("SetValues injected a !!merge tag into the user's file:\n%s", body)
	}
	if !strings.Contains(body, "<<: *d") {
		t.Errorf("the merge key was rewritten:\n%s", body)
	}
}

func TestSetValuesLeavesFlowScalarsUnquoted(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envHome, dir)
	src := "github: {max: 30, api_url: https://x}\ndaemon:\n  bell: true\n"
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
	body := string(raw)
	if strings.Contains(body, "'https://x'") || strings.Contains(body, "\"https://x\"") {
		t.Errorf("SetValues quoted a value it did not touch:\n%s", body)
	}
	if !strings.Contains(body, "https://x") {
		t.Errorf("the untouched value was lost:\n%s", body)
	}
}

func TestSetValuesRefusesToReplaceASectionWithAValue(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envHome, dir)
	src := "# how many\ngithub:\n  # capped on purpose\n  max: 30\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := SetValues("", map[string]any{"github": "nope"}); err == nil {
		t.Error("SetValues replaced a whole section with a scalar, producing a config that cannot load")
	}
	raw, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != src {
		t.Errorf("the section and its comments were destroyed:\n%s", raw)
	}
}

func TestSetValuesKeepsHarmlessFlowCollectionsInFlow(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envHome, dir)
	src := "github: {max: 30, queries: [prs, issues]}\ndaemon:\n  bell: true\n"
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
	if !strings.Contains(string(raw), "{max: 30, queries: [prs, issues]}") {
		t.Errorf("a flow mapping that needs no quoting was reformatted:\n%s", raw)
	}
}
