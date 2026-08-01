package log

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileSinkPlain(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "mino.log")
	c, err := SetFileSink(p)
	if err != nil {
		t.Fatalf("SetFileSink: %v", err)
	}
	defer c.Close()
	defer func() { file = nil }()

	ClearConsole()
	defer SetOutput(os.Stderr)

	SetLevel(LevelInfo)
	Infof("hello %d", 7)

	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, "hello 7") || !strings.Contains(s, "info") {
		t.Errorf("log file missing entry, got %q", s)
	}
	if strings.Contains(s, "\x1b[") {
		t.Errorf("log file should be plain (no ANSI), got %q", s)
	}
}
