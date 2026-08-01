package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/codyconfer/mino/internal/auth"
	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/signals"
)

type Backend interface {
	SearchIssues(ctx context.Context, query string, perPage int) ([]byte, error)
	GraphQL(ctx context.Context, query string, vars map[string]any) ([]byte, error)
}

type ActionsBackend interface {
	WorkflowRuns(ctx context.Context, owner, repo string, perPage int) ([]byte, error)
	WorkflowJobs(ctx context.Context, owner, repo string, runID int64) ([]byte, error)
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

type CLIBackend struct {
	Hostname string
}

func (b CLIBackend) apiArgs(rest ...string) []string {
	args := []string{"api"}
	if b.Hostname != "" {
		args = append(args, "--hostname", b.Hostname)
	}
	return append(args, rest...)
}

func (b CLIBackend) SearchIssues(ctx context.Context, query string, perPage int) ([]byte, error) {
	return auth.GH(ctx, b.apiArgs("-X", "GET", "search/issues",
		"-f", "q="+query, "-f", fmt.Sprintf("per_page=%d", perPage))...)
}

func (b CLIBackend) WorkflowRuns(ctx context.Context, owner, repo string, perPage int) ([]byte, error) {
	path := fmt.Sprintf("repos/%s/%s/actions/runs", owner, repo)
	return auth.GH(ctx, b.apiArgs("-X", "GET", path, "-f", fmt.Sprintf("per_page=%d", perPage))...)
}

func (b CLIBackend) WorkflowJobs(ctx context.Context, owner, repo string, runID int64) ([]byte, error) {
	path := fmt.Sprintf("repos/%s/%s/actions/runs/%d/jobs", owner, repo, runID)
	return auth.GH(ctx, b.apiArgs("-X", "GET", path, "-f", "per_page=100")...)
}

func (b CLIBackend) GraphQL(ctx context.Context, query string, vars map[string]any) ([]byte, error) {
	args := b.apiArgs("graphql", "-f", "query="+query)
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		switch v := vars[k].(type) {
		case nil:
		case string:
			if v != "" {
				args = append(args, "-f", k+"="+v)
			}
		default:
			args = append(args, "-F", fmt.Sprintf("%s=%v", k, v))
		}
	}
	out, err := auth.GH(ctx, args...)
	if err != nil {
		return nil, graphQLCLIError(err)
	}
	return out, nil
}

func graphQLCLIError(err error) error {
	if strings.Contains(err.Error(), "INSUFFICIENT_SCOPES") || strings.Contains(err.Error(), "read:project") {
		return errs.Wrap(errs.KindAuth, err, "github: graphql").WithHint("%s", projectScopeHint)
	}
	return err
}

type APIBackend struct {
	Token   string
	HTTP    *http.Client
	BaseURL string
}

func (b APIBackend) client() *http.Client {
	if b.HTTP == nil {
		return signals.HTTPClient()
	}
	return b.HTTP
}

func (b APIBackend) SearchIssues(ctx context.Context, query string, perPage int) ([]byte, error) {
	path := fmt.Sprintf("/search/issues?q=%s&per_page=%d", url.QueryEscape(query), perPage)
	req, err := newGitHubRequest(ctx, b.BaseURL, path, b.Token)
	if err != nil {
		return nil, errs.Wrap(errs.KindSignal, err, "github: building search request")
	}

	resp, err := b.client().Do(req)
	if err != nil {
		return nil, errs.Wrap(errs.KindSignal, err, "github: search request failed")
	}
	defer resp.Body.Close()
	body, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if err := checkGitHubStatus(resp, body, "scopes"); err != nil {
		return nil, err
	}
	return body, nil
}

func (b APIBackend) WorkflowRuns(ctx context.Context, owner, repo string, perPage int) ([]byte, error) {
	path := fmt.Sprintf("/repos/%s/%s/actions/runs?per_page=%d",
		url.PathEscape(owner), url.PathEscape(repo), perPage)
	return b.get(ctx, path, "workflow runs", "the Actions read permission")
}

func (b APIBackend) WorkflowJobs(ctx context.Context, owner, repo string, runID int64) ([]byte, error) {
	path := fmt.Sprintf("/repos/%s/%s/actions/runs/%d/jobs?per_page=100",
		url.PathEscape(owner), url.PathEscape(repo), runID)
	return b.get(ctx, path, "workflow jobs", "the Actions read permission")
}

func (b APIBackend) get(ctx context.Context, path, label, permission string) ([]byte, error) {
	req, err := newGitHubRequest(ctx, b.BaseURL, path, b.Token)
	if err != nil {
		return nil, errs.Wrapf(errs.KindSignal, err, "github: building %s request", label)
	}
	resp, err := b.client().Do(req)
	if err != nil {
		return nil, errs.Wrapf(errs.KindSignal, err, "github: %s request failed", label)
	}
	defer resp.Body.Close()
	body, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if err := checkGitHubStatus(resp, body, permission); err != nil {
		return nil, err
	}
	return body, nil
}

func (b APIBackend) GraphQL(ctx context.Context, query string, vars map[string]any) ([]byte, error) {
	payload, err := json.Marshal(map[string]any{"query": query, "variables": vars})
	if err != nil {
		return nil, errs.Wrap(errs.KindSignal, err, "github: encoding graphql request")
	}
	req, err := newGitHubPost(ctx, graphQLURL(b.BaseURL), b.Token, payload)
	if err != nil {
		return nil, errs.Wrap(errs.KindSignal, err, "github: building graphql request")
	}

	resp, err := b.client().Do(req)
	if err != nil {
		return nil, errs.Wrap(errs.KindSignal, err, "github: graphql request failed")
	}
	defer resp.Body.Close()
	body, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if err := checkGitHubStatus(resp, body, "the read:project scope"); err != nil {
		return nil, err
	}
	return body, nil
}

func graphQLURL(raw string) string {
	base := strings.TrimRight(raw, "/")
	switch {
	case base == "" || base == "https://api.github.com":
		return "https://api.github.com/graphql"
	case strings.HasSuffix(base, "/api/v3"):
		return strings.TrimSuffix(base, "/v3") + "/graphql"
	default:
		return base + "/graphql"
	}
}
