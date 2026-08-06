package gitlab

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/codyconfer/mino/internal/auth"
	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/signals"
)

type Page struct {
	Body       []byte
	NextPage   int
	Total      int
	HasTotal   bool
	TotalPages int
	Short      bool
}

type Backend interface {
	Get(ctx context.Context, path string, query url.Values) (Page, error)
}

func NormalizeAPIURL(raw string) (string, error) {
	base, err := auth.NormalizeGitLabAPIURL(raw)
	if err != nil {
		return "", err
	}
	if base == "" {
		return "", nil
	}
	return auth.GitLabAPIBase(base), nil
}

func encodeProject(path string) string {
	path = strings.Trim(strings.TrimSpace(path), "/")
	if path == "" {
		return ""
	}
	if _, err := strconv.Atoi(path); err == nil {
		return path
	}
	return url.PathEscape(path)
}

func projectPathFromWebURL(rawURL string) string {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	p := strings.Trim(u.Path, "/")
	if i := strings.Index(p, "/-/"); i >= 0 {
		return p[:i]
	}
	return p
}

type CLIBackend struct {
	Hostname string
}

func (b CLIBackend) args(path string, q url.Values) []string {
	a := []string{"api"}
	if b.Hostname != "" {
		a = append(a, "--hostname", b.Hostname)
	}
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	return append(a, "-X", "GET", strings.TrimLeft(path, "/"))
}

func (b CLIBackend) Get(ctx context.Context, path string, q url.Values) (Page, error) {
	out, err := auth.GLab(ctx, b.args(path, q)...)
	if err != nil {
		return Page{}, cliError(err)
	}
	return Page{Body: out, Short: shortPage(out, q)}, nil
}

func shortPage(body []byte, q url.Values) bool {
	perPage, err := strconv.Atoi(q.Get("per_page"))
	if err != nil || perPage <= 0 {
		return true
	}
	return countTopLevel(body) < perPage
}

type APIBackend struct {
	Auth    auth.GitLabSource
	HTTP    *http.Client
	BaseURL string
	Rate    *RateHint
}

func (b APIBackend) token(ctx context.Context) (string, error) {
	if b.Auth == nil {
		return "", errs.New(errs.KindAuth, "gitlab: no authentication configured for this backend")
	}
	return b.Auth.Token(ctx)
}

func (b APIBackend) client() *http.Client {
	if b.HTTP == nil {
		return signals.HTTPClient()
	}
	return b.HTTP
}

func (b APIBackend) Get(ctx context.Context, path string, q url.Values) (Page, error) {
	tok, err := b.token(ctx)
	if err != nil {
		return Page{}, err
	}
	req, err := newGitLabRequest(ctx, b.BaseURL, path, q, tok)
	if err != nil {
		return Page{}, err
	}

	resp, err := b.client().Do(req)
	if err != nil {
		return Page{}, errs.Wrapf(errs.KindSignal, err, "gitlab: %s request failed", path)
	}
	defer resp.Body.Close()

	b.Rate.observe(resp.Header, timeNow())
	body, err := readBody(resp)
	if err != nil {
		return Page{}, err
	}
	if err := checkGitLabStatus(resp, body, "read_api", scopedPath(path)); err != nil {
		return Page{}, err
	}

	p := Page{Body: body, NextPage: nextPage(resp.Header)}
	p.Total, p.HasTotal = totalCount(resp.Header)
	p.TotalPages, _ = totalPages(resp.Header)
	p.Short = shortPage(body, q)
	return p, nil
}

func newGitLabRequest(ctx context.Context, base, path string, q url.Values, tok string) (*http.Request, error) {
	u := strings.TrimRight(base, "/") + "/" + strings.TrimLeft(path, "/")
	if enc := q.Encode(); enc != "" {
		u += "?" + enc
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, errs.Wrapf(errs.KindSignal, err, "gitlab: building %s request", path)
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")
	return req, nil
}

func scopedPath(path string) bool {
	return strings.HasPrefix(strings.TrimLeft(path, "/"), "projects/") ||
		strings.HasPrefix(strings.TrimLeft(path, "/"), "groups/")
}
