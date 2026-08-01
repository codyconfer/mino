package config

import (
	"path/filepath"
	"testing"

	"github.com/codyconfer/mino/internal/testenv"
)

func TestLogDirDefault(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv(envLogDir, "")
	got := LogDir("/home/x/.mino")
	want := filepath.Join("/home/x/.mino", DirLogs)
	if got != want {
		t.Errorf("LogDir default = %q, want %q", got, want)
	}
}

func TestLogDirEnvOverride(t *testing.T) {
	t.Setenv(envLogDir, "/tmp/mino-logs")
	if got := LogDir("/home/x/.mino"); got != "/tmp/mino-logs" {
		t.Errorf("LogDir env = %q, want /tmp/mino-logs", got)
	}
}
