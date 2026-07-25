package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEditorCmdResolvesVisualThenEditor(t *testing.T) {
	t.Setenv("VISUAL", "visual-ed")
	t.Setenv("EDITOR", "editor-ed")
	cmd, ed, err := EditorCmd("/tmp/cfg.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if ed != "visual-ed" {
		t.Fatalf("ed = %q, want visual-ed", ed)
	}
	if cmd == nil || len(cmd.Args) < 2 {
		t.Fatalf("unexpected cmd: %#v", cmd)
	}

	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "editor-ed")
	_, ed, err = EditorCmd("/tmp/cfg.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if ed != "editor-ed" {
		t.Fatalf("ed = %q, want editor-ed", ed)
	}

	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")
	if _, _, err := EditorCmd("/tmp/cfg.yaml"); err == nil || err.Error() != "EDITOR not set" {
		t.Fatalf("err = %v, want EDITOR not set", err)
	}
}

func TestConfigFilePathFindsOnDiskConfig(t *testing.T) {
	home := t.TempDir()
	if _, err := ConfigFilePath(home); err == nil {
		t.Fatal("expected error with no config file")
	}
	path := filepath.Join(home, "config.yml")
	if err := os.WriteFile(path, []byte("output: terminal\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ConfigFilePath(home)
	if err != nil {
		t.Fatal(err)
	}
	if got != path {
		t.Fatalf("ConfigFilePath = %q, want %q", got, path)
	}
}
