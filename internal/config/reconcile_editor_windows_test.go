//go:build windows

package config

import "testing"

func TestEditorCmdOnWindowsRunsTheEditorDirectly(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", `"C:\Program Files\Vim\vim.exe" --nofork`)
	cmd, _, err := EditorCmd(`C:\Users\x\.munin\config.yaml`)
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Path == "sh" || cmd.Args[0] == "sh" {
		t.Fatalf("windows build shells out to sh: %v", cmd.Args)
	}
	want := []string{`C:\Program Files\Vim\vim.exe`, "--nofork", `C:\Users\x\.munin\config.yaml`}
	if len(cmd.Args) != len(want) {
		t.Fatalf("cmd.Args = %v, want %v", cmd.Args, want)
	}
	for i := range want {
		if cmd.Args[i] != want[i] {
			t.Fatalf("cmd.Args = %v, want %v", cmd.Args, want)
		}
	}
}
