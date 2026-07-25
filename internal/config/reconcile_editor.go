package config

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

var openConfigEditor = OpenPathInEditor

// EditorCmd returns a command that opens path with $VISUAL or $EDITOR.
// VISUAL wins when set; otherwise EDITOR. Returns an error if neither is set.
func EditorCmd(path string) (*exec.Cmd, string, error) {
	ed := strings.TrimSpace(os.Getenv("VISUAL"))
	if ed == "" {
		ed = strings.TrimSpace(os.Getenv("EDITOR"))
	}
	if ed == "" {
		return nil, "", fmt.Errorf("EDITOR not set")
	}
	return exec.Command("sh", "-c", ed+` "$1"`, "munin-editor", path), ed, nil
}

// OpenPathInEditor opens path with $VISUAL or $EDITOR (same resolution as EditorCmd).
func OpenPathInEditor(path string) error {
	cmd, ed, err := EditorCmd(path)
	if err != nil {
		return err
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("open %s with %s: %w", path, ed, err)
	}
	return nil
}
