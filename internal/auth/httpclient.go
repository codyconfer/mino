package auth

import (
	"io"
	"net/http"
	"time"

	"github.com/codyconfer/munin/internal/errs"
)

const (
	HTTPTimeout       = 30 * time.Second
	DeviceFlowTimeout = 60 * time.Second

	httpMaxIdleConns   = 32
	httpMaxIdlePerHost = 8

	maxAPIResponseBytes   = 8 << 20
	maxTokenResponseBytes = 1 << 20
	maxErrorBodyBytes     = 2 << 10
)

var (
	sharedTransport      = newHTTPTransport()
	sharedHTTPClient     = &http.Client{Transport: sharedTransport, Timeout: HTTPTimeout}
	deviceFlowHTTPClient = &http.Client{Transport: sharedTransport, Timeout: DeviceFlowTimeout}
)

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

func readBounded(resp *http.Response, what string, limit int64) ([]byte, error) {
	if resp.ContentLength > limit {
		return nil, oversizeBody(what, limit, resp.ContentLength)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, errs.Wrapf(errs.KindSignal, err, "%s: reading response body", what)
	}
	if int64(len(body)) > limit {
		return nil, oversizeBody(what, limit, int64(len(body)))
	}
	return body, nil
}

func oversizeBody(what string, limit, n int64) error {
	return errs.Newf(errs.KindSignal, "%s: response body exceeds the %d MiB limit (at least %d bytes)",
		what, limit>>20, n)
}

func errorExcerpt(r io.Reader) string {
	body, _ := io.ReadAll(io.LimitReader(r, maxErrorBodyBytes))
	return errs.ExcerptBytes(body)
}
