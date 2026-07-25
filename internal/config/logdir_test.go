package config

import (
	"path/filepath"
	"testing"

	"github.com/codyconfer/munin/internal/testenv"
)

func TestLogDirDefault(t *testing.T) {
	testenv.Isolate(t)
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
