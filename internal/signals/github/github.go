package github

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/signals"
)

const defaultPerPage = 30

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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubBaseURL(base)+path, nil)
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

func checkGitHubStatus(resp *http.Response, body []byte, missingScope string) error {
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return errs.Newf(errs.KindAuth, "github api %s: %s", resp.Status, strings.TrimSpace(string(body))).
			WithHint("%s", githubAuthHint(missingScope))
	}
	if resp.StatusCode >= 400 {
		return errs.Newf(errs.KindSignal, "github api %s: %s", resp.Status, strings.TrimSpace(string(body)))
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
}

func New(queries []string, backend Backend, max int) signals.Signal {
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
	return &Signal{queries: qs, backend: backend, max: max}
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
	Items []struct {
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
	return sec, nil
}

func repoSlug(repoURL string) string {
	const marker = "/repos/"
	if i := strings.Index(repoURL, marker); i >= 0 {
		return repoURL[i+len(marker):]
	}
	return repoURL
}
