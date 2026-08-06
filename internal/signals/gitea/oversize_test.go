package gitea

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/codyconfer/mino/internal/auth"
	"github.com/codyconfer/mino/internal/errs"
)

func TestOversizeBodyKeepsTheStatusClassification(t *testing.T) {
	resp := &http.Response{
		StatusCode:    http.StatusUnauthorized,
		Status:        "401 Unauthorized",
		Header:        http.Header{},
		ContentLength: maxResponseBytes + 1,
	}
	err := oversizeBody(resp, resp.ContentLength)
	if errs.KindOf(err) != errs.KindAuth {
		t.Fatalf("kind = %q, want auth so the caller keeps the 401", errs.KindOf(err))
	}
	if !strings.Contains(errs.Hint(err), "mino login gitea") {
		t.Fatalf("hint = %q, want the login remedy", errs.Hint(err))
	}
	if !strings.Contains(err.Error(), "401 Unauthorized") || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("message = %q, want the status and the oversize detail preserved", err.Error())
	}
}

func TestOversizeBodyOnASuccessStatusPointsAtTheInstanceURL(t *testing.T) {
	resp := &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: http.Header{}, ContentLength: maxResponseBytes + 1}
	err := oversizeBody(resp, resp.ContentLength)
	if errs.KindOf(err) != errs.KindSignal {
		t.Fatalf("kind = %q, want signal", errs.KindOf(err))
	}
	if !strings.Contains(errs.Hint(err), "gitea.url") {
		t.Fatalf("hint = %q, want the instance URL remedy", errs.Hint(err))
	}
}

func TestSearchIssuesKeepsA401BehindAnOversizeBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		chunk := []byte(strings.Repeat("a", 64<<10))
		for i := 0; i <= (maxResponseBytes >> 16); i++ {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	b := APIBackend{Auth: auth.StaticGiteaToken("t"), BaseURL: srv.URL + "/api/v1", HTTP: srv.Client()}
	_, err := b.SearchIssues(context.Background(), mustParse(t, ""), 5)
	if err == nil {
		t.Fatal("want an error")
	}
	if errs.KindOf(err) != errs.KindAuth {
		t.Fatalf("kind = %q, want auth: a 9 MiB 401 must still read as an auth failure (err %v)", errs.KindOf(err), err)
	}
}

func TestCheckGiteaStatusExcerptsAHostileBody(t *testing.T) {
	body := []byte("\x1b[2J\x1b[Herror: " + strings.Repeat("x", maxResponseBytes) + "\n\ttrailing")
	resp := &http.Response{StatusCode: http.StatusUnauthorized, Status: "401 Unauthorized", Header: http.Header{}}

	err := checkGiteaStatus(resp, body, scopeIssues)
	if err == nil {
		t.Fatal("a 401 was treated as success")
	}
	msg := err.Error()
	if len(msg) > 2048 {
		t.Errorf("message is %d bytes; a signal tree cannot render a whole API body", len(msg))
	}
	if hasControlBytes(msg) {
		t.Error("control bytes survived into the message, so a hostile body can rewrite the terminal")
	}
}

func hasControlBytes(s string) bool {
	for _, r := range s {
		if r < 0x20 && r != '\n' {
			return true
		}
	}
	return false
}
