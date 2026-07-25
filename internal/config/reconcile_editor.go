package config

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

var openConfigEditor = openPathInEditor

func openPathInEditor(path string) error {
	ed := strings.TrimSpace(os.Getenv("VISUAL"))
	if ed == "" {
		ed = strings.TrimSpace(os.Getenv("EDITOR"))
	}
	if ed == "" {
		return fmt.Errorf("EDITOR not set")
	}
	cmd := exec.Command("sh", "-c", ed+` "$1"`, "munin-editor", path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("open %s with %s: %w", path, ed, err)
	}
	return nil
}
