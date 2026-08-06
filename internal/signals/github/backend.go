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
	"github.com/codyconfer/mino/internal/log"
	"github.com/codyconfer/mino/internal/signals"
)

type Backend interface {
	SearchIssues(ctx context.Context, query string, perPage int) ([]byte, error)
	GraphQL(ctx context.Context, query string, vars map[string]any) ([]byte, error)
}

type ActionsBackend interface {
	WorkflowRuns(ctx context.Context, owner, repo string, perPage int) ([]byte, error)
	WorkflowRun(ctx context.Context, owner, repo string, runID int64) ([]byte, error)
	WorkflowJobs(ctx context.Context, owner, repo string, runID int64) ([]byte, error)
}

func NormalizeAPIURL(raw string) (string, error) {
	return auth.NormalizeGitHubAPIURL(raw)
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

func (b CLIBackend) WorkflowRun(ctx context.Context, owner, repo string, runID int64) ([]byte, error) {
	path := fmt.Sprintf("repos/%s/%s/actions/runs/%d", owner, repo, runID)
	return auth.GH(ctx, b.apiArgs("-X", "GET", path)...)
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
		return nil, graphQLCLIError(b.Hostname, err)
	}
	return out, nil
}

// graphQLCLIError turns a `gh api graphql` scope failure into a one-line error
// plus the command that fixes it. gh echoes the whole query and repeats its
// complaint once per offending field, which is unreadable in a signal tree, so
// the raw output only goes to the debug log.
func graphQLCLIError(hostname string, err error) error {
	msg := err.Error()
	scopes := missingScopes(msg)
	if len(scopes) == 0 && !strings.Contains(msg, "INSUFFICIENT_SCOPES") && !strings.Contains(msg, "not been granted the required scopes") {
		return err
	}
	log.Debugf("github: graphql scope failure: %v", err)
	return errs.Newf(errs.KindAuth, "github: graphql: %s", scopeSummary(scopes)).
		WithHint("%s", scopeHint(hostname, scopes, projectScopeHint))
}

type APIBackend struct {
	Auth    auth.GitHubSource
	HTTP    *http.Client
	BaseURL string
}

func (b APIBackend) token(ctx context.Context) (string, error) {
	if b.Auth == nil {
		return "", errs.New(errs.KindAuth, "github: no authentication configured for this backend")
	}
	return b.Auth.Token(ctx)
}

func (b APIBackend) client() *http.Client {
	if b.HTTP == nil {
		return signals.HTTPClient()
	}
	return b.HTTP
}

func (b APIBackend) SearchIssues(ctx context.Context, query string, perPage int) ([]byte, error) {
	path := fmt.Sprintf("/search/issues?q=%s&per_page=%d", url.QueryEscape(query), perPage)
	tok, err := b.token(ctx)
	if err != nil {
		return nil, err
	}
	req, err := newGitHubRequest(ctx, b.BaseURL, path, tok)
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

func (b APIBackend) WorkflowRun(ctx context.Context, owner, repo string, runID int64) ([]byte, error) {
	path := fmt.Sprintf("/repos/%s/%s/actions/runs/%d",
		url.PathEscape(owner), url.PathEscape(repo), runID)
	return b.get(ctx, path, "workflow run", "the Actions read permission")
}

func (b APIBackend) WorkflowJobs(ctx context.Context, owner, repo string, runID int64) ([]byte, error) {
	path := fmt.Sprintf("/repos/%s/%s/actions/runs/%d/jobs?per_page=100",
		url.PathEscape(owner), url.PathEscape(repo), runID)
	return b.get(ctx, path, "workflow jobs", "the Actions read permission")
}

func (b APIBackend) get(ctx context.Context, path, label, permission string) ([]byte, error) {
	tok, err := b.token(ctx)
	if err != nil {
		return nil, err
	}
	req, err := newGitHubRequest(ctx, b.BaseURL, path, tok)
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
	tok, err := b.token(ctx)
	if err != nil {
		return nil, err
	}
	req, err := newGitHubPost(ctx, graphQLURL(b.BaseURL), tok, payload)
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
