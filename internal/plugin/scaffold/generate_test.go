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
		`"github.com/codyconfer/mino/plugin"`,
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
	if !strings.Contains(string(qb), "anywhere under the mino home") {
		t.Fatalf("scaffolded query comment still points at a fixed directory: %s", qb)
	}

	if _, err := Generate(GenerateOptions{ID: "team.example", Dir: out}); err == nil {
		t.Fatal("expected error without --force")
	}
	if _, err := Generate(GenerateOptions{ID: "team.example", Dir: out, Force: true}); err != nil {
		t.Fatal(err)
	}
}

func filesUnder(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		out = append(out, rel)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestGenerateRejectsUnsafeSignal(t *testing.T) {
	bad := []string{
		"../../pwned",
		`..\..\pwned`,
		"/etc/pwned",
		"/tmp/pwned",
		"..",
		".",
		"queries/../../pwned",
		"sub/dir",
		"Pwned",
		"has space",
		"trailing/",
	}
	for _, sig := range bad {
		t.Run(sig, func(t *testing.T) {
			base := t.TempDir()
			target := filepath.Join(base, "target")
			if _, err := Generate(GenerateOptions{ID: "team.example", Dir: target, Signal: sig}); err == nil {
				t.Errorf("Generate accepted --signal %q", sig)
			}
			for _, f := range filesUnder(t, base) {
				t.Errorf("--signal %q wrote %s (expected nothing)", sig, f)
			}
		})
	}
}

func TestGenerateForceCannotClobberOutsideDir(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "target")
	victim := filepath.Join(base, "pwned-ping.yaml")
	const userData = "important: user data\n"
	if err := os.WriteFile(victim, []byte(userData), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(GenerateOptions{
		ID:     "team.example",
		Dir:    target,
		Signal: "../../pwned",
		Force:  true,
	}); err == nil {
		t.Error("Generate with --force accepted a traversing --signal")
	}
	got, err := os.ReadFile(victim)
	if err != nil {
		t.Fatalf("file outside --dir removed/renamed: %v", err)
	}
	if string(got) != userData {
		t.Fatalf("clobbered a file outside --dir: %q", got)
	}
}

func TestGenerateExplicitPackageStillContained(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "target")
	if _, err := Generate(GenerateOptions{
		ID:      "team.example",
		Dir:     target,
		Signal:  "../../pwned",
		Package: "example",
	}); err == nil {
		t.Error("an explicit --package must not smuggle a traversing --signal past validation")
	}
	for _, f := range filesUnder(t, base) {
		t.Errorf("wrote %s (expected nothing)", f)
	}
}

func TestGenerateAcceptsDashedSignal(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "example")
	res, err := Generate(GenerateOptions{ID: "team.example", Dir: out, Signal: "my-signal"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Written) != 3 {
		t.Fatalf("written = %v", res.Written)
	}
	if _, err := os.Stat(filepath.Join(out, "queries", "my-signal-ping.yaml")); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(out, "plugin.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "package my_signal") {
		t.Fatalf("plugin.go = %s", body)
	}
}

func TestResolveWithin(t *testing.T) {
	dir := filepath.Join(string(filepath.Separator), "tmp", "sc", "target")
	for _, rel := range []string{
		"",
		"..",
		filepath.Join("..", "pwned.yaml"),
		filepath.Join("queries", "..", "..", "pwned.yaml"),
		filepath.Join(string(filepath.Separator), "etc", "pwned.yaml"),
	} {
		if got, err := resolveWithin(dir, rel); err == nil {
			t.Errorf("resolveWithin(%q) = %q, want error", rel, got)
		}
	}
	want := filepath.Join(dir, "queries", "x-ping.yaml")
	got, err := resolveWithin(dir, filepath.Join("queries", "x-ping.yaml"))
	if err != nil || got != want {
		t.Fatalf("resolveWithin = %q, %v", got, err)
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

func TestGenerateRejectsOverlongSignalWithoutLeavingFiles(t *testing.T) {
	out := filepath.Join(t.TempDir(), "toolong")
	long := strings.Repeat("s", 400)

	if _, err := Generate(GenerateOptions{ID: "team.toolong", Dir: out, Signal: long}); err == nil {
		t.Fatal("Generate accepted a 400-character signal name")
	}

	entries, err := os.ReadDir(out)
	if err != nil {
		return
	}
	var left []string
	for _, e := range entries {
		left = append(left, e.Name())
	}
	if len(left) > 0 {
		t.Fatalf("a failed scaffold left files behind with no cleanup: %v", left)
	}
}

func TestGenerateRejectsOverlongPluginID(t *testing.T) {
	out := filepath.Join(t.TempDir(), "longid")
	if _, err := Generate(GenerateOptions{ID: strings.Repeat("a", 400), Dir: out}); err == nil {
		t.Fatal("Generate accepted a 400-character plugin id")
	}
}
