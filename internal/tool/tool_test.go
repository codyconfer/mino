package tool

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

func lookup(m map[string]string) func(string) string {
	return func(name string) string { return m[name] }
}

func TestExpandSubstitutesAContext(t *testing.T) {
	got := Expand(
		[]string{"k9s", "--context={{context.kubectl}}"},
		lookup(map[string]string{"kubectl": "prod-us-east"}),
	)
	want := []string{"k9s", "--context=prod-us-east"}
	if !slices.Equal(got, want) {
		t.Fatalf("Expand = %v, want %v", got, want)
	}
}

func TestExpandDropsTheWholeTokenWhenAContextIsUnset(t *testing.T) {
	cases := []struct {
		name string
		ctx  map[string]string
	}{
		{"no selection", map[string]string{"kubectl": ""}},
		{"unknown tool", map[string]string{}},
		{"whitespace only", map[string]string{"kubectl": "   "}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Expand([]string{"k9s", "--context={{context.kubectl}}"}, lookup(c.ctx))
			want := []string{"k9s"}
			if !slices.Equal(got, want) {
				t.Fatalf("Expand = %v, want %v — a bare --context= would break the tool", got, want)
			}
		})
	}
}

func TestExpandLeavesLiteralsAlone(t *testing.T) {
	argv := []string{"k9s", "--readonly", "-n", "payments"}
	if got := Expand(argv, lookup(nil)); !slices.Equal(got, argv) {
		t.Fatalf("Expand = %v, want %v", got, argv)
	}
}

func TestExpandToleratesSpacingAndMultiplePlaceholders(t *testing.T) {
	got := Expand(
		[]string{"x", "{{ context.kubectl }}", "{{context.a}}-{{context.b}}"},
		lookup(map[string]string{"kubectl": "c1", "a": "one", "b": "two"}),
	)
	want := []string{"x", "c1", "one-two"}
	if !slices.Equal(got, want) {
		t.Fatalf("Expand = %v, want %v", got, want)
	}
}

func TestExpandDropsATokenWhenAnyPlaceholderInItIsUnset(t *testing.T) {
	got := Expand(
		[]string{"x", "{{context.a}}-{{context.b}}"},
		lookup(map[string]string{"a": "one"}),
	)
	if !slices.Equal(got, []string{"x"}) {
		t.Fatalf("Expand = %v, want the partially-resolved token dropped", got)
	}
}

func TestExpandIgnoresUnknownPlaceholderNamespaces(t *testing.T) {
	got := Expand([]string{"x", "--home={{home}}"}, lookup(map[string]string{"home": "/tmp"}))
	if !slices.Equal(got, []string{"x"}) {
		t.Fatalf("Expand = %v; only the context. namespace is substitutable", got)
	}
}

func TestExpandNilLookupDropsPlaceholders(t *testing.T) {
	if got := Expand([]string{"x", "{{context.kubectl}}"}, nil); !slices.Equal(got, []string{"x"}) {
		t.Fatalf("Expand = %v", got)
	}
}

func TestAvailable(t *testing.T) {
	if Available(nil) || Available([]string{}) || Available([]string{"  "}) {
		t.Error("an empty argv is never available")
	}
	if Available([]string{"mino-definitely-not-installed-xyz"}) {
		t.Error("a missing binary must report unavailable, not error")
	}

	dir := t.TempDir()
	name := "mino-fake-tool"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	bin := filepath.Join(dir, name)
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	if !Available([]string{name}) {
		t.Errorf("%s on PATH should be available", name)
	}
	if !Available([]string{bin}) {
		t.Error("an absolute path should be available")
	}
}
