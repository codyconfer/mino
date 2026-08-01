package httpx

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	sauth "github.com/codyconfer/sisyphus/auth"
	"github.com/codyconfer/viewkit/browser"

	"github.com/codyconfer/mino/external/plugins/internal/errx"
)

const (
	Timeout = 30 * time.Second

	maxIdleConns       = 32
	maxIdleConnsPerHos = 8

	MaxTokenResponseBytes = 1 << 20
	maxErrorBodyBytes     = 2 << 10
)

var (
	sharedTransport = newTransport()
	sharedClient    = &http.Client{Transport: sharedTransport, Timeout: Timeout}
)

var OpenBrowser = browser.Open

func Client() *http.Client { return sharedClient }

func newTransport() http.RoundTripper {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return http.DefaultTransport
	}
	t := base.Clone()
	t.MaxIdleConns = maxIdleConns
	t.MaxIdleConnsPerHost = maxIdleConnsPerHos
	return t
}

func ReadBounded(resp *http.Response, what string, limit int64) ([]byte, error) {
	if resp.ContentLength > limit {
		return nil, oversize(what, limit, resp.ContentLength)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, errx.Wrapf(err, "%s: reading response body", what)
	}
	if int64(len(body)) > limit {
		return nil, oversize(what, limit, int64(len(body)))
	}
	return body, nil
}

func oversize(what string, limit, n int64) error {
	return errx.Newf("%s: response body exceeds the %d MiB limit (at least %d bytes)", what, limit>>20, n)
}

func ErrorExcerpt(r io.Reader) string {
	body, _ := io.ReadAll(io.LimitReader(r, maxErrorBodyBytes))
	return errx.ExcerptBytes(body)
}

func PostForm(ctx context.Context, client *http.Client, endpoint string, form url.Values, out any) error {
	if client == nil {
		client = Client()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("%s: %s", resp.Status, ErrorExcerpt(resp.Body))
	}
	body, err := ReadBounded(resp, "oauth", MaxTokenResponseBytes)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}

func LoopbackAuthCode(ctx context.Context, w io.Writer, service string, buildURL func(redirect, state string) string) (code, redirect string, err error) {
	code, redirect, err = sauth.LoopbackAuthCode(ctx, w, service, sauth.LoopbackOptions{
		Product: "mino",
		Open:    OpenBrowser,
	}, buildURL)
	if err != nil {
		return "", "", errx.Wrap(err, "oauth loopback")
	}
	return code, redirect, nil
}
