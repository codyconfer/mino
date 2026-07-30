//go:build windows

package config

import "os/exec"

func EditorCmd(path string) (*exec.Cmd, string, error) {
	ed := editorEnv()
	if ed == "" {
		return nil, "", errNoEditor
	}
	name, args := editorArgv(ed, path)
	if name == "" {
		return nil, "", errNoEditor
	}
	return exec.Command(name, args...), ed, nil
}
