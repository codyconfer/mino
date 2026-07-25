package testenv

import (
	"os"
	"path/filepath"
	"testing"
)

const sandboxEnv = "MUNIN_TEST_SANDBOX"

type Env struct {
	Home      string
	ConfigDir string
}

func Isolate(t *testing.T) Env {
	t.Helper()
	if home := os.Getenv(sandboxEnv); home != "" {
		return resolve(t, home)
	}
	home := t.TempDir()
	t.Setenv(sandboxEnv, home)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("AppData", filepath.Join(home, "AppData", "Roaming"))
	t.Setenv("MUNIN_HOME", "")
	return resolve(t, home)
}

func resolve(t *testing.T, home string) Env {
	t.Helper()
	dir, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("testenv: resolve user config dir: %v", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("testenv: create %s: %v", dir, err)
	}
	return Env{Home: home, ConfigDir: dir}
}
