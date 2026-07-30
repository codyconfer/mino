package signals

import (
	"net/http"
	"testing"
)

func TestHTTPClientIsShared(t *testing.T) {
	if HTTPClient() != HTTPClient() {
		t.Error("HTTPClient must return one shared client so connections are reused")
	}
}

func TestHTTPClientHasTimeout(t *testing.T) {
	if HTTPClient().Timeout <= 0 {
		t.Fatal("HTTPClient has no Timeout: a server that accepts and never responds would block forever")
	}
	if HTTPClient().Timeout != HTTPTimeout {
		t.Errorf("Timeout = %s, want %s", HTTPClient().Timeout, HTTPTimeout)
	}
}

func TestHTTPClientIdleConnsCoverConcurrentBursts(t *testing.T) {
	tr, ok := HTTPClient().Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport is %T, want *http.Transport", HTTPClient().Transport)
	}
	if tr.MaxIdleConnsPerHost < 8 {
		t.Errorf("MaxIdleConnsPerHost = %d, want at least 8 (the search page burst is 6)", tr.MaxIdleConnsPerHost)
	}
	if tr.MaxIdleConns < 32 {
		t.Errorf("MaxIdleConns = %d, want at least 32", tr.MaxIdleConns)
	}
	if def, ok := http.DefaultTransport.(*http.Transport); ok && tr == def {
		t.Error("the shared transport must not be http.DefaultTransport")
	}
}
