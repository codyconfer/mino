package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateWritesPublicSDKPackage(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "example")
	res, err := Generate(GenerateOptions{
		ID:  "team.example",
		Dir: out,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Dir != out {
		t.Fatalf("Dir = %q", res.Dir)
	}
	pluginGo := filepath.Join(out, "plugin.go")
	body, err := os.ReadFile(pluginGo)
	if err != nil {
		t.Fatal(err)
	}
	src := string(body)
	for _, want := range []string{
		"package example",
		`PluginID    = "team.example"`,
		`SignalName  = "example"`,
		`"github.com/codyconfer/munin/plugin"`,
		"plugin.RegisterSignal",
		"plugin.RegisterFilterEngine",
		"plugin.RegisterContext",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("plugin.go missing %q", want)
		}
	}
	if _, err := os.Stat(filepath.Join(out, "plugin_test.go")); err != nil {
		t.Fatal(err)
	}
	q := filepath.Join(out, "queries", "example-ping.yaml")
	qb, err := os.ReadFile(q)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(qb), "filters: [example-clean]") {
		t.Fatalf("query yaml = %s", qb)
	}
	if !strings.Contains(string(qb), "type: query") {
		t.Fatalf("scaffolded query must declare its type: %s", qb)
	}
	if !strings.Contains(string(qb), "anywhere under the munin home") {
		t.Fatalf("scaffolded query comment still points at a fixed directory: %s", qb)
	}

	if _, err := Generate(GenerateOptions{ID: "team.example", Dir: out}); err == nil {
		t.Fatal("expected error without --force")
	}
	if _, err := Generate(GenerateOptions{ID: "team.example", Dir: out, Force: true}); err != nil {
		t.Fatal(err)
	}
}

func TestSanitizeIdent(t *testing.T) {
	if got := sanitizeIdent("My-Signal"); got != "my_signal" {
		t.Fatalf("got %q", got)
	}
	if got := defaultSignal("acme.widgets"); got != "widgets" {
		t.Fatalf("defaultSignal = %q", got)
	}
}
