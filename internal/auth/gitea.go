package auth

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/codyconfer/mino/internal/errs"
)

const giteaTokenScope = "read:user read:repository read:issue read:notification"

func GiteaHostname(webURL string) string {
	u, err := url.Parse(strings.TrimSpace(webURL))
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Host)
}

func GiteaWebURL(webURL, path string) string {
	base := strings.TrimRight(strings.TrimSpace(webURL), "/")
	if base == "" {
		return ""
	}
	return base + "/" + strings.TrimLeft(path, "/")
}

func GiteaAPIGet(ctx context.Context, sel GiteaSelection, path string) ([]byte, error) {
	forge := sel.Forge()
	base := strings.TrimRight(sel.APIURL, "/")
	if base == "" {
		return nil, errs.Newf(errs.KindConfig, "%s: no instance URL configured", forge).
			WithHint("%s", giteaURLHint)
	}
	tok, err := sel.Token(ctx)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/"+strings.TrimLeft(path, "/"), nil)
	if err != nil {
		return nil, errs.Wrapf(errs.KindSignal, err, "%s: building request", forge)
	}
	req.Header.Set("Authorization", "token "+tok)
	req.Header.Set("Accept", "application/json")

	resp, err := HTTPClient().Do(req)
	if err != nil {
		return nil, errs.Wrapf(errs.KindSignal, err, "%s: request failed", forge)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, classifyGiteaStatus(forge, resp, errorExcerpt(resp.Body))
	}
	return readBounded(resp, forge, maxAPIResponseBytes)
}
