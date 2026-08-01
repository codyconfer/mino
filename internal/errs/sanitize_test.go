package errs_test

import (
	"strings"
	"testing"

	"github.com/codyconfer/mino/internal/errs"
)

func TestExcerptIsBoundedSingleLineAndClean(t *testing.T) {
	huge := strings.Repeat("A\x1b[2J\n", 500000)
	got := errs.Excerpt(huge)
	if hasTerminalEscapes(got) {
		t.Fatalf("Excerpt leaked control sequences: %q", got)
	}
	if strings.ContainsAny(got, "\n\t") {
		t.Fatalf("Excerpt is not a single line: %q", got)
	}
	if len(got) > 2048 {
		t.Fatalf("Excerpt length = %d, want a short excerpt", len(got))
	}
	if got == "" {
		t.Fatal("Excerpt dropped everything")
	}
}

func TestExcerptLeavesShortTextAlone(t *testing.T) {
	if got := errs.Excerpt("Bad credentials"); got != "Bad credentials" {
		t.Fatalf("Excerpt = %q", got)
	}
}

func TestCleanKeepsNewlinesAndTabs(t *testing.T) {
	if got := errs.Clean("a\nb\tc"); got != "a\nb\tc" {
		t.Fatalf("Clean = %q", got)
	}
	if got := errs.Clean("a\x1b[31mb\x07c\x7fd\re"); got != "a[31mbcde" {
		t.Fatalf("Clean = %q", got)
	}
}
