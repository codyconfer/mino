package views

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/codyconfer/mino/internal/app"
	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/plugin"
	pub "github.com/codyconfer/mino/plugin"
)

func fakeToolOnPath(t *testing.T, name string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, name)
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	return bin
}

func kitWith(cfg *config.Config) *Kit {
	return New(Deps{App: &app.App{Cfg: cfg}})
}

func TestToolArgvResolvesAnInstalledTool(t *testing.T) {
	fakeToolOnPath(t, "k9s")
	k := kitWith(&config.Config{
		Tools: map[string]config.Tool{
			"k9s": {Argv: []string{"k9s", "--readonly"}, Title: "k9s"},
		},
	})
	argv, title, ok := k.toolArgv("k9s")
	if !ok {
		t.Fatal("an installed, configured tool must resolve")
	}
	if !slices.Equal(argv, []string{"k9s", "--readonly"}) {
		t.Errorf("argv = %v", argv)
	}
	if title != "k9s" {
		t.Errorf("title = %q", title)
	}
}

func TestToolArgvTitleDefaultsToTheToolName(t *testing.T) {
	fakeToolOnPath(t, "k9s")
	k := kitWith(&config.Config{
		Tools: map[string]config.Tool{"k9s": {Argv: []string{"k9s"}}},
	})
	if _, title, ok := k.toolArgv("k9s"); !ok || title != "k9s" {
		t.Fatalf("title = %q ok=%v", title, ok)
	}
}

func TestToolArgvIsInertWhenTheToolIsNotInstalled(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	k := kitWith(&config.Config{
		Tools: map[string]config.Tool{
			"k9s": {Argv: []string{"mino-not-installed-xyz"}},
		},
	})
	if _, _, ok := k.toolArgv("k9s"); ok {
		t.Fatal("a tool that is not installed must not resolve")
	}
}

func TestToolArgvUnknownToolAndNilConfig(t *testing.T) {
	fakeToolOnPath(t, "k9s")
	k := kitWith(&config.Config{Tools: map[string]config.Tool{}})
	if _, _, ok := k.toolArgv("k9s"); ok {
		t.Error("an unconfigured tool must not resolve")
	}
	if _, _, ok := New(Deps{}).toolArgv("k9s"); ok {
		t.Error("a nil app must not resolve a tool")
	}
	if _, _, ok := kitWith(nil).toolArgv("k9s"); ok {
		t.Error("a nil config must not resolve a tool")
	}
}

func TestToolArgvDropsAnUnresolvedContextFlag(t *testing.T) {
	fakeToolOnPath(t, "k9s")
	k := kitWith(&config.Config{
		Tools: map[string]config.Tool{
			"k9s": {Argv: []string{"k9s", "--context={{context.kubectl}}"}},
		},
	})
	argv, _, ok := k.toolArgv("k9s")
	if !ok {
		t.Fatal("the tool must still resolve")
	}
	if !slices.Equal(argv, []string{"k9s"}) {
		t.Fatalf("argv = %v, want the unresolved --context dropped", argv)
	}
}

type fakeContextProvider struct {
	tool string
	name string
}

func (p *fakeContextProvider) Tool() string { return p.tool }

func (p *fakeContextProvider) Switch(_ context.Context, name string) error {
	p.name = name
	return nil
}

func (p *fakeContextProvider) Current(context.Context) (string, bool, error) {
	if p.name == "" {
		return "", false, nil
	}
	return p.name, true, nil
}

func TestToolArgvSubstitutesTheSelectedContext(t *testing.T) {
	pub.ResetContextProvidersForTest()
	t.Cleanup(pub.ResetContextProvidersForTest)
	plugin.RegisterContextProvider(&fakeContextProvider{tool: "kubectl"})

	if err := plugin.SwitchContext(context.Background(), "kubectl", "k3d-grafana-dev"); err != nil {
		t.Fatal(err)
	}

	fakeToolOnPath(t, "k9s")
	k := kitWith(&config.Config{
		Tools: map[string]config.Tool{
			"k9s": {Argv: []string{"k9s", "--context={{context.kubectl}}"}},
		},
	})
	argv, _, ok := k.toolArgv("k9s")
	if !ok {
		t.Fatal("the tool must resolve")
	}
	want := []string{"k9s", "--context=k3d-grafana-dev"}
	if !slices.Equal(argv, want) {
		t.Fatalf("argv = %v, want %v — the deck and the tool must agree on the cluster", argv, want)
	}
}

func TestToolHintsListOnlyBoundAndInstalledTools(t *testing.T) {
	fakeToolOnPath(t, "k9s")
	cfg := &config.Config{
		Tools: map[string]config.Tool{
			"k9s":     {Argv: []string{"k9s"}, Title: "k9s"},
			"lazygit": {Argv: []string{"mino-not-installed-xyz"}},
		},
		Keybinds: map[string]string{
			"alt+k": "tool:k9s",
			"alt+g": "tool:lazygit",
			"alt+z": "tool:never-configured",
			"alt+n": "ntr.note.new",
		},
	}
	hints := kitWith(cfg).toolHints(cfg.Keybinds)
	if len(hints) != 1 {
		t.Fatalf("hints = %+v, want only the installed k9s binding", hints)
	}
	if hints[0].Label != "k9s" {
		t.Errorf("label = %q", hints[0].Label)
	}
}

func TestToolHintsAreOrderedDeterministically(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"aaa", "bbb", "ccc"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
	cfg := &config.Config{
		Tools: map[string]config.Tool{
			"aaa": {Argv: []string{"aaa"}},
			"bbb": {Argv: []string{"bbb"}},
			"ccc": {Argv: []string{"ccc"}},
		},
		Keybinds: map[string]string{
			"alt+3": "tool:ccc",
			"alt+1": "tool:aaa",
			"alt+2": "tool:bbb",
		},
	}
	k := kitWith(cfg)
	want := []string{"aaa", "bbb", "ccc"}
	for range 8 {
		var got []string
		for _, h := range k.toolHints(cfg.Keybinds) {
			got = append(got, h.Label)
		}
		if !slices.Equal(got, want) {
			t.Fatalf("hints = %v, want %v — map iteration must not leak into the footer", got, want)
		}
	}
}
