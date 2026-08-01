package console

import (
	"os"
	"strings"
	"testing"
)

func TestTitleCleansAndJoinsMetadata(t *testing.T) {
	got := Title("deck", " morning\nforged ", "", "role: triage")
	if got != "mino · deck · morningforged · role: triage" {
		t.Errorf("Title = %q", got)
	}
}

func TestSetWritesTitleAndWorkingDirectory(t *testing.T) {
	var out strings.Builder
	if err := Set(&out, "mino · deck\a"); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "\x1b]0;mino · deck\x07") {
		t.Errorf("missing title sequence: %q", got)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "\x1b]7;file://") || !strings.Contains(got, cwd) {
		t.Errorf("missing working-directory metadata: %q", got)
	}
}

func TestSetWithNilWriterIsInert(t *testing.T) {
	if err := Set(nil, "mino"); err != nil {
		t.Fatal(err)
	}
}

func TestLoadingTitleAnimatesAndRestores(t *testing.T) {
	var out strings.Builder
	if err := Set(&out, "mino · fly · morning"); err != nil {
		t.Fatal(err)
	}
	stop := startLoading(&out)
	if !strings.Contains(out.String(), "\x1b]0;⠋ mino · fly · morning\x07") {
		t.Fatalf("missing loading frame: %q", out.String())
	}
	stop()
	if !strings.HasSuffix(out.String(), "\x1b]0;mino · fly · morning\x07") {
		t.Fatalf("stable title was not restored: %q", out.String())
	}
}
