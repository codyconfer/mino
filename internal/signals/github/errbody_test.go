package github

import (
	"net/http"
	"strings"
	"testing"

	"github.com/codyconfer/munin/internal/errs"
)

func hasControlBytes(s string) bool {
	for _, r := range s {
		if r == 0x1b || r == 0x9b || r == 0x07 || r == 0x7f || r < 0x20 {
			return true
		}
	}
	return false
}

func TestCheckGitHubStatusBoundsAndSanitisesTheRemoteBody(t *testing.T) {
	body := []byte("\x1b]0;pwned\a\x1b[2J\x1b[32mall checks passed\x1b[0m\x7f rate limit is not the phrase\n" +
		strings.Repeat("PADDING ", 1<<20))
	resp := &http.Response{StatusCode: http.StatusUnauthorized, Status: "401 Unauthorized", Header: http.Header{}}

	err := checkGitHubStatus(resp, body, "read:org")
	if err == nil {
		t.Fatal("want an error for a 401")
	}
	msg := err.Error()
	if hasControlBytes(msg) {
		t.Fatalf("error message carries terminal control bytes: %q", msg)
	}
	if len(msg) > 2048 {
		t.Fatalf("error message is %d bytes; a short excerpt is enough to diagnose", len(msg))
	}
	if !strings.Contains(msg, "all checks passed") {
		t.Fatalf("error message dropped the readable excerpt: %q", msg)
	}
	if errs.KindOf(err) != errs.KindAuth {
		t.Fatalf("kind = %q, want auth", errs.KindOf(err))
	}
}

func TestCheckGitHubStatusExcerptStaysSingleLine(t *testing.T) {
	body := []byte("line one\nline two\nline three")
	resp := &http.Response{StatusCode: http.StatusNotFound, Status: "404 Not Found", Header: http.Header{}}
	err := checkGitHubStatus(resp, body, "repo")
	if err == nil {
		t.Fatal("want an error for a 404")
	}
	if strings.ContainsAny(err.Error(), "\n\t") {
		t.Fatalf("error message spans lines: %q", err.Error())
	}
}
