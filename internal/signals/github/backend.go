package github

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/codyconfer/munin/internal/auth"
	"github.com/codyconfer/munin/internal/errs"
)

type Backend interface {
	SearchIssues(ctx context.Context, query string, perPage int) ([]byte, error)
}

func NormalizeAPIURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", errs.Wrapf(errs.KindConfig, err, "github: invalid api_url %q", raw)
	}
	if u.Scheme != "https" {
		return "", errs.Newf(errs.KindConfig, "github: api_url must use https (refusing to send token over %q)", raw).
			WithHint("set github.api_url to an https:// endpoint, e.g. https://api.github.com or your enterprise host")
	}
	if u.Host == "" {
		return "", errs.Newf(errs.KindConfig, "github: api_url has no host: %q", raw)
	}
	return strings.TrimRight(raw, "/"), nil
}

type CLIBackend struct{}

func (CLIBackend) SearchIssues(ctx context.Context, query string, perPage int) ([]byte, error) {
	return auth.GH(ctx, "api", "-X", "GET", "search/issues",
		"-f", "q="+query, "-f", fmt.Sprintf("per_page=%d", perPage))
}

type APIBackend struct {
	Token   string
	HTTP    *http.Client
	BaseURL string
}

func (b APIBackend) SearchIssues(ctx context.Context, query string, perPage int) ([]byte, error) {
	path := fmt.Sprintf("/search/issues?q=%s&per_page=%d", url.QueryEscape(query), perPage)
	req, err := newGitHubRequest(ctx, b.BaseURL, path, b.Token)
	if err != nil {
		return nil, errs.Wrap(errs.KindSignal, err, "github: building search request")
	}

	hc := b.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, errs.Wrap(errs.KindSignal, err, "github: search request failed")
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if err := checkGitHubStatus(resp, body, "scopes"); err != nil {
		return nil, err
	}
	return body, nil
}
