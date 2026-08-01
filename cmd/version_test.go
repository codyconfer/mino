package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/codyconfer/mino/internal/render/glyph"
)

func TestVersionLine(t *testing.T) {
	prev := Version
	t.Cleanup(func() { Version = prev })
	Version = "v1.2.3-test"
	got := versionLine()
	wantPrefix := glyph.Brand() + " MINO "
	if !strings.HasPrefix(got, wantPrefix) {
		t.Fatalf("versionLine = %q, want prefix %q", got, wantPrefix)
	}
	if !strings.HasSuffix(got, "v1.2.3-test") {
		t.Fatalf("versionLine = %q, want suffix v1.2.3-test", got)
	}
}

func TestVersionCmd(t *testing.T) {
	prev := Version
	t.Cleanup(func() { Version = prev })
	Version = "v9.9.9"

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"version"})
	err := root.Execute()
	Shutdown()
	if err != nil {
		t.Fatalf("version: %v\n%s", err, out.String())
	}
	got := strings.TrimSpace(out.String())
	want := glyph.Brand() + " MINO v9.9.9"
	if got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}
