//go:build !windows

package config

import "os/exec"

func EditorCmd(path string) (*exec.Cmd, string, error) {
	ed := editorEnv()
	if ed == "" {
		return nil, "", errNoEditor
	}
	return exec.Command("sh", "-c", ed+` "$1"`, "mino-editor", path), ed, nil
}
