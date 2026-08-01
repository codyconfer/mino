package format

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/codyconfer/mino/internal/errs"
)

func TestDeliverStdoutOnly(t *testing.T) {
	var out, status strings.Builder
	if err := Deliver(&out, &status, "report body", nil, ""); err != nil {
		t.Fatalf("Deliver err = %v", err)
	}
	if out.String() != "report body\n" {
		t.Errorf("out = %q", out.String())
	}
	if status.String() != "" {
		t.Errorf("status = %q, want empty", status.String())
	}
}

func TestDeliverNewlineNormalisation(t *testing.T) {
	cases := map[string]string{
		"a":         "a\n",
		"a\n":       "a\n",
		"a\n\n\n":   "a\n",
		"a\nb\n\n":  "a\nb\n",
		"":          "\n",
		"a\n\nb\n":  "a\n\nb\n",
		"a  \n\n\n": "a  \n",
	}
	for in, want := range cases {
		var out strings.Builder
		if err := Deliver(&out, nil, in, nil, ""); err != nil {
			t.Fatalf("Deliver(%q) err = %v", in, err)
		}
		if out.String() != want {
			t.Errorf("Deliver(%q) = %q, want %q", in, out.String(), want)
		}
	}
}

func TestDeliverOutPathSuppressesStdout(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.md")
	var out, status strings.Builder
	if err := Deliver(&out, &status, "body\n\n", nil, path); err != nil {
		t.Fatalf("Deliver err = %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(b) != "body\n" {
		t.Errorf("file = %q, want %q", b, "body\n")
	}
	if out.String() != "" {
		t.Errorf("out = %q, want empty when --out is set", out.String())
	}
	if !strings.Contains(status.String(), "wrote "+path) {
		t.Errorf("status = %q", status.String())
	}
}

func TestDeliverOutPathMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.md")
	if err := Deliver(nil, nil, "body", nil, path); err != nil {
		t.Fatalf("Deliver err = %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not modelled on Windows")
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", fi.Mode().Perm())
	}
}

func TestDeliverMissingParentDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nope", "report.md")
	var out, status strings.Builder
	err := Deliver(&out, &status, "body", nil, path)
	if err == nil {
		t.Fatal("want an error for a missing parent directory")
	}
	if errs.KindOf(err) != errs.KindUsage {
		t.Errorf("kind = %q, want usage", errs.KindOf(err))
	}
	if h := errs.Hint(err); h == "" || !strings.Contains(h, filepath.Join(dir, "nope")) {
		t.Errorf("hint = %q, want it to mention the directory", h)
	}
	if _, statErr := os.Stat(path); statErr == nil {
		t.Error("Deliver created the file anyway")
	}
	if out.String() != "" {
		t.Errorf("out = %q, want empty on error", out.String())
	}
}

func TestDeliverCopySuppressesStdout(t *testing.T) {
	var out, status strings.Builder
	var copied string
	copyFn := func(s string) error {
		copied = s
		return nil
	}
	if err := Deliver(&out, &status, "body", copyFn, ""); err != nil {
		t.Fatalf("Deliver err = %v", err)
	}
	if copied != "body\n" {
		t.Errorf("copyFn received %q, want %q", copied, "body\n")
	}
	if out.String() != "" {
		t.Errorf("out = %q, want empty when copying", out.String())
	}
	if !strings.Contains(status.String(), "copied 5 bytes") {
		t.Errorf("status = %q", status.String())
	}
}

func TestDeliverCopyError(t *testing.T) {
	boom := errors.New("no clipboard")
	var out strings.Builder
	err := Deliver(&out, nil, "body", func(string) error { return boom }, "")
	if err == nil {
		t.Fatal("want an error")
	}
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want it to wrap %v", err, boom)
	}
	if errs.KindOf(err) != errs.KindUsage {
		t.Errorf("kind = %q, want usage", errs.KindOf(err))
	}
}

func TestDeliverBothSinks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.md")
	var out, status strings.Builder
	var copied string
	err := Deliver(&out, &status, "body", func(s string) error { copied = s; return nil }, path)
	if err != nil {
		t.Fatalf("Deliver err = %v", err)
	}
	if copied != "body\n" {
		t.Errorf("copied = %q", copied)
	}
	b, _ := os.ReadFile(path)
	if string(b) != "body\n" {
		t.Errorf("file = %q", b)
	}
	if out.String() != "" {
		t.Errorf("out = %q, want empty", out.String())
	}
	s := status.String()
	if !strings.Contains(s, "wrote ") || !strings.Contains(s, "copied ") {
		t.Errorf("status = %q, want both confirmations", s)
	}
}

func TestDeliverNilStatus(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.md")
	if err := Deliver(nil, nil, "body", nil, path); err != nil {
		t.Fatalf("Deliver err = %v", err)
	}
}

func TestDeliverNilOutNoSinks(t *testing.T) {
	if err := Deliver(nil, nil, "body", nil, ""); err != nil {
		t.Fatalf("Deliver err = %v", err)
	}
}
