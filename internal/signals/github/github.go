package github

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/log"
	"github.com/codyconfer/munin/internal/signals"
)

const defaultPerPage = 30

const maxResponseBytes = 8 << 20

const githubAuthHintPrefix = "your GitHub token may be missing or lack "
const githubAuthHintSuffix = "; run `munin login github` or set $GITHUB_TOKEN"

func githubAuthHint(scope string) string {
	return githubAuthHintPrefix + scope + githubAuthHintSuffix
}

func githubBaseURL(raw string) string {
	base := strings.TrimRight(raw, "/")
	if base == "" {
		base = "https://api.github.com"
	}
	return base
}

func newGitHubRequest(ctx context.Context, base, path, token string) (*http.Request, error) {
	return newGitHubURLRequest(ctx, githubBaseURL(base)+path, token)
}

func newGitHubURLRequest(ctx context.Context, rawURL, token string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	return req, nil
}

func newGitHubPost(ctx context.Context, url, token string, body []byte) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	return req, nil
}

func readBody(resp *http.Response) ([]byte, error) {
	if resp.ContentLength > maxResponseBytes {
		return nil, oversizeBody(resp.ContentLength)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, errs.Wrap(errs.KindSignal, err, "github: reading response body")
	}
	if len(body) > maxResponseBytes {
		return nil, oversizeBody(int64(len(body)))
	}
	return body, nil
}

func oversizeBody(n int64) error {
	return errs.Newf(errs.KindSignal, "github: response body exceeds the %d MiB limit (%d bytes)", maxResponseBytes>>20, n).
		WithHint("check that github.api_url points at a GitHub API endpoint")
}

func checkGitHubStatus(resp *http.Response, body []byte, missingScope string) error {
	msg := strings.TrimSpace(string(body))
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return errs.Newf(errs.KindAuth, "github api %s: %s", resp.Status, msg).
			WithHint("%s", githubAuthHint(missingScope))
	case rateLimited(resp, body):
		err := errs.Newf(errs.KindSignal, "github api %s: %s", resp.Status, msg)
		if d, ok := retryAfter(resp.Header, time.Now()); ok {
			return err.WithHint("github rate limit reached; retry after %s", d.Round(time.Second))
		}
		return err.WithHint("github rate limit reached; retry in a few minutes")
	case resp.StatusCode == http.StatusForbidden:
		if hint := restrictionHint(body); hint != "" {
			return errs.Newf(errs.KindAuth, "github api %s: %s", resp.Status, msg).WithHint("%s", hint)
		}
		return errs.Newf(errs.KindAuth, "github api %s: %s", resp.Status, msg).
			WithHint("%s", githubAuthHint(missingScope))
	case resp.StatusCode >= 400:
		return errs.Newf(errs.KindSignal, "github api %s: %s", resp.Status, msg)
	}
	return nil
}

type query struct {
	q     string
	title string
}

type Signal struct {
	queries []query
	backend Backend
	max     int
	detail  Cache
	policy  CachePolicy
}

func New(queries []string, backend Backend, max int, opts ...Option) signals.Signal {
	qs := make([]query, 0, len(queries))
	if len(queries) == 0 {
		qs = []query{
			{q: "is:open is:pr author:@me", title: "Open Pull Requests"},
			{q: "is:open is:pr review-requested:@me", title: "Review Requests"},
		}
	} else {
		for _, q := range queries {
			qs = append(qs, query{q: q, title: q})
		}
	}
	if max <= 0 {
		max = defaultPerPage
	}
	o := applyOptions(opts)
	return &Signal{queries: qs, backend: backend, max: max, detail: o.detail, policy: o.policy}
}

func (s *Signal) Name() string { return "github" }

func (s *Signal) Fetch(ctx context.Context) ([]signals.Section, error) {
	sections := make([]signals.Section, 0, len(s.queries))
	for _, q := range s.queries {
		raw, err := s.backend.SearchIssues(ctx, q.q, s.max)
		if err != nil {
			return nil, wrapQuery(q.q, err)
		}
		sec, err := mapSearchResponse(raw, q.title)
		if err != nil {
			return nil, wrapQuery(q.q, err)
		}
		sections = append(sections, sec)
	}
	return sections, nil
}

func wrapQuery(q string, err error) error {
	werr := errs.Wrapf(errs.KindOf(err), err, "github: search %q", q)
	if h := errs.Hint(err); h != "" {
		werr = werr.WithHint("%s", h)
	}
	return werr
}

type searchResponse struct {
	TotalCount        int  `json:"total_count"`
	IncompleteResults bool `json:"incomplete_results"`
	Items             []struct {
		Title         string `json:"title"`
		HTMLURL       string `json:"html_url"`
		Body          string `json:"body"`
		UpdatedAt     string `json:"updated_at"`
		RepositoryURL string `json:"repository_url"`
		User          struct {
			Login string `json:"login"`
		} `json:"user"`
	} `json:"items"`
}

func mapSearchResponse(raw []byte, title string) (signals.Section, error) {
	var resp searchResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return signals.Section{}, errs.Wrap(errs.KindSignal, err, "github: decoding search response")
	}
	sec := signals.Section{
		Signal: "github",
		Title:  title,
		Items:  make([]signals.Item, 0, len(resp.Items)),
	}
	for _, it := range resp.Items {
		var ts time.Time
		if it.UpdatedAt != "" {
			ts, _ = time.Parse(time.RFC3339, it.UpdatedAt)
		}
		sec.Items = append(sec.Items, signals.Item{
			Kind:      "pr",
			Title:     it.Title,
			Subtitle:  repoSlug(it.RepositoryURL),
			Body:      it.Body,
			URL:       it.HTMLURL,
			Timestamp: ts,
			Meta:      map[string]string{"author": it.User.Login},
		})
	}
	sec.Meta = searchMeta(resp, title)
	return sec, nil
}

func searchMeta(resp searchResponse, title string) map[string]string {
	meta := map[string]string{"shown": strconv.Itoa(len(resp.Items))}
	if resp.TotalCount > 0 {
		meta["total"] = strconv.Itoa(resp.TotalCount)
	}
	if resp.TotalCount > len(resp.Items) {
		meta["more"] = strconv.Itoa(resp.TotalCount - len(resp.Items))
	}
	if resp.IncompleteResults {
		meta["truncated"] = "true"
		meta["truncated_reason"] = "github's search backend timed out; these results are incomplete"
		log.Debugf("github: search %q returned incomplete results (%d of %d)", title, len(resp.Items), resp.TotalCount)
	}
	return meta
}

func repoSlug(repoURL string) string {
	const marker = "/repos/"
	if i := strings.Index(repoURL, marker); i >= 0 {
		return repoURL[i+len(marker):]
	}
	return repoURL
}
