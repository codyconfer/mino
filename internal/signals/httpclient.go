package signals

import (
	"net/http"
	"time"
)

const (
	HTTPTimeout        = 30 * time.Second
	httpMaxIdleConns   = 32
	httpMaxIdlePerHost = 8
)

var sharedHTTPClient = &http.Client{Transport: newHTTPTransport(), Timeout: HTTPTimeout}

func HTTPClient() *http.Client { return sharedHTTPClient }

func newHTTPTransport() http.RoundTripper {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return http.DefaultTransport
	}
	t := base.Clone()
	t.MaxIdleConns = httpMaxIdleConns
	t.MaxIdleConnsPerHost = httpMaxIdlePerHost
	return t
}
