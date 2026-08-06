package gitea

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/codyconfer/mino/internal/auth"
	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/signals"
)

const apiSuffix = "/api/v1"

type Result struct {
	Body  []byte
	Total int
}

type Backend interface {
	SearchIssues(ctx context.Context, q Query, limit int) (Result, error)
	Issue(ctx context.Context, ref Ref) ([]byte, error)
	IssueComments(ctx context.Context, ref Ref, page, limit int) ([]byte, error)
	PullRequest(ctx context.Context, ref Ref) ([]byte, error)
	PullReviews(ctx context.Context, ref Ref, limit int) ([]byte, error)
	Whoami(ctx context.Context) ([]byte, error)
}

func NormalizeAPIURL(raw string) (string, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")
	if trimmed == "" {
		return "", nil
	}
	if strings.HasSuffix(trimmed, apiSuffix) {
		return auth.NormalizeGiteaAPIURL(trimmed)
	}
	root, err := auth.NormalizeGiteaURL(trimmed)
	if err != nil {
		return "", err
	}
	return root + apiSuffix, nil
}

type APIBackend struct {
	Auth    auth.TokenSource
	HTTP    *http.Client
	BaseURL string
}

func (b APIBackend) token(ctx context.Context) (string, error) {
	if b.Auth == nil {
		return "", errs.New(errs.KindAuth, "gitea: no authentication configured for this backend")
	}
	return b.Auth.Token(ctx)
}

func (b APIBackend) client() *http.Client {
	if b.HTTP == nil {
		return signals.HTTPClient()
	}
	return b.HTTP
}

func (b APIBackend) SearchIssues(ctx context.Context, q Query, limit int) (Result, error) {
	return b.fetch(ctx, q.Path()+"?"+q.Values(limit, 1).Encode(), "search", scopeIssues)
}

func (b APIBackend) Issue(ctx context.Context, ref Ref) ([]byte, error) {
	res, err := b.fetch(ctx, ref.path(""), "issue", scopeIssues)
	return res.Body, err
}

func (b APIBackend) IssueComments(ctx context.Context, ref Ref, page, limit int) ([]byte, error) {
	res, err := b.fetch(ctx, ref.path(fmt.Sprintf("/comments?page=%d&limit=%d", max(page, 1), limit)), "comments", scopeIssues)
	return res.Body, err
}

func (b APIBackend) PullRequest(ctx context.Context, ref Ref) ([]byte, error) {
	res, err := b.fetch(ctx, ref.pullPath(""), "pull request", scopeRepo)
	return res.Body, err
}

func (b APIBackend) PullReviews(ctx context.Context, ref Ref, limit int) ([]byte, error) {
	res, err := b.fetch(ctx, ref.pullPath(fmt.Sprintf("/reviews?limit=%d", limit)), "reviews", scopeRepo)
	return res.Body, err
}

func (b APIBackend) Whoami(ctx context.Context) ([]byte, error) {
	res, err := b.fetch(ctx, "/user", "the authenticated user", scopeUser)
	return res.Body, err
}

func (b APIBackend) fetch(ctx context.Context, path, label, scope string) (Result, error) {
	tok, err := b.token(ctx)
	if err != nil {
		return Result{}, err
	}
	req, err := newGiteaRequest(ctx, b.BaseURL, path, tok)
	if err != nil {
		return Result{}, errs.Wrapf(errs.KindSignal, err, "gitea: building the %s request", label)
	}
	resp, err := b.client().Do(req)
	if err != nil {
		return Result{}, errs.Wrapf(errs.KindSignal, err, "gitea: the %s request failed", label)
	}
	defer resp.Body.Close()
	body, err := readBody(resp)
	if err != nil {
		return Result{}, err
	}
	if err := checkGiteaStatus(resp, body, scope); err != nil {
		return Result{}, err
	}
	return Result{Body: body, Total: totalCount(resp.Header)}, nil
}

func totalCount(hdr http.Header) int {
	n, err := strconv.Atoi(strings.TrimSpace(hdr.Get("X-Total-Count")))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func giteaBaseURL(raw string) string {
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}

func newGiteaRequest(ctx context.Context, base, path, token string) (*http.Request, error) {
	return newGiteaURLRequest(ctx, giteaBaseURL(base)+path, token)
}

func newGiteaURLRequest(ctx context.Context, rawURL, token string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/json")
	return req, nil
}

func escapePath(v string) string { return url.PathEscape(v) }
