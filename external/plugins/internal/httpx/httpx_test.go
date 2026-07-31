package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestPostFormBoundsAnEndlessBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		chunk := strings.Repeat("a", 64<<10)
		for {
			if _, err := w.Write([]byte(chunk)); err != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	t.Cleanup(srv.Close)

	done := make(chan error, 1)
	go func() {
		var out struct{}
		done <- PostForm(context.Background(), nil, srv.URL, url.Values{}, &out)
	}()

	var err error
	select {
	case err = <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("PostForm never stopped reading: the body read is unbounded")
	}
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("error = %v, want the size limit named", err)
	}
}

func TestClientHasABoundedTimeout(t *testing.T) {
	if Client().Timeout <= 0 {
		t.Fatal("shared client must carry a timeout")
	}
}
