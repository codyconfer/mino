package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

var openConfigEditor = OpenPathInEditor

var errNoEditor = errors.New("EDITOR not set")

func editorEnv() string {
	if ed := strings.TrimSpace(os.Getenv("VISUAL")); ed != "" {
		return ed
	}
	return strings.TrimSpace(os.Getenv("EDITOR"))
}

func editorArgv(ed, path string) (string, []string) {
	fields := splitEditorCommand(ed)
	if len(fields) == 0 {
		return "", nil
	}
	return fields[0], append(fields[1:], path)
}

func splitEditorCommand(s string) []string {
	var out []string
	var cur strings.Builder
	quoted, started := false, false
	for _, r := range s {
		switch {
		case r == '"':
			quoted = !quoted
			started = true
		case (r == ' ' || r == '\t') && !quoted:
			if started {
				out = append(out, cur.String())
				cur.Reset()
				started = false
			}
		default:
			cur.WriteRune(r)
			started = true
		}
	}
	if started {
		out = append(out, cur.String())
	}
	return out
}

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
