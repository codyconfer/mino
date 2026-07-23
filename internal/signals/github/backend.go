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
	base := strings.TrimRight(b.BaseURL, "/")
	if base == "" {
		base = "https://api.github.com"
	}
	u := fmt.Sprintf("%s/search/issues?q=%s&per_page=%d", base, url.QueryEscape(query), perPage)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, errs.Wrap(errs.KindSignal, err, "github: building search request")
	}
	req.Header.Set("Authorization", "Bearer "+b.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

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
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, errs.Newf(errs.KindAuth, "github api %s: %s", resp.Status, strings.TrimSpace(string(body))).
			WithHint("your GitHub token may be missing or lack scopes; run `munin login github` or set $GITHUB_TOKEN")
	}
	if resp.StatusCode >= 400 {
		return nil, errs.Newf(errs.KindSignal, "github api %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return body, nil
}
