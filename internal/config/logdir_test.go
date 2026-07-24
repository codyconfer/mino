package config

import (
	"path/filepath"
	"testing"
)

func TestLogDirDefault(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv(envLogDir, "")
	got := LogDir("/home/x/.munin")
	want := filepath.Join("/home/x/.munin", DirLogs)
	if got != want {
		t.Errorf("LogDir default = %q, want %q", got, want)
	}
}

func TestLogDirEnvOverride(t *testing.T) {
	t.Setenv(envLogDir, "/tmp/munin-logs")
	if got := LogDir("/home/x/.munin"); got != "/tmp/munin-logs" {
		t.Errorf("LogDir env = %q, want /tmp/munin-logs", got)
	}
}
