package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
	if !strings.Contains(errs.Hint(err), "mino login github") {
		t.Fatalf("hint = %q, want the login remedy", errs.Hint(err))
	}
	if !strings.Contains(err.Error(), "401 Unauthorized") {
		t.Fatalf("message = %q, want the status preserved", err.Error())
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("message = %q, want the oversize detail preserved", err.Error())
	}
}

func TestOversizeBodyOnASuccessStatusStillPointsAtAPIURL(t *testing.T) {
	resp := &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: http.Header{}, ContentLength: maxResponseBytes + 1}
	err := oversizeBody(resp, resp.ContentLength)
	if errs.KindOf(err) != errs.KindSignal {
		t.Fatalf("kind = %q, want signal", errs.KindOf(err))
	}
	if !strings.Contains(errs.Hint(err), "github.api_url") {
		t.Fatalf("hint = %q, want the api_url remedy", errs.Hint(err))
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

	b := APIBackend{Token: "t", BaseURL: srv.URL, HTTP: srv.Client()}
	_, err := b.SearchIssues(context.Background(), "is:open", 5)
	if err == nil {
		t.Fatal("want an error")
	}
	if errs.KindOf(err) != errs.KindAuth {
		t.Fatalf("kind = %q, want auth: a 9 MiB 401 must still read as an auth failure (err %v)", errs.KindOf(err), err)
	}
	if !strings.Contains(errs.Hint(err), "mino login github") {
		t.Fatalf("hint = %q, want the login remedy", errs.Hint(err))
	}
}
