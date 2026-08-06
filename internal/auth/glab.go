package auth

import (
	"context"
	"net/http"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/codyconfer/mino/internal/errs"
)

const (
	defaultGitLabAPI  = "https://gitlab.com/api/v4"
	defaultGitLabHost = "gitlab.com"
)

const maxGitLabRetryAfter = time.Hour

const gitlabScopeHint = "your GitLab credential may lack the required scopes; run `glab auth login`, set $GITLAB_TOKEN, or run `mino login gitlab`"

func GLabAvailable() bool {
	_, err := exec.LookPath("glab")
	return err == nil
}

func GLab(ctx context.Context, args ...string) ([]byte, error) {
	return runTool(ctx, []string{"glab"}, "glab", errs.KindAuth,
		"the GitLab CLI `glab` is not installed or not on PATH",
		"install glab and run `glab auth login`, or run `mino login gitlab`",
		"run `glab auth login` or `mino login gitlab` to (re)authenticate",
		args...)
}

func GLabHostname(apiURL string) string {
	u, err := url.Parse(strings.TrimSpace(apiURL))
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Host)
}

func GLabHostFlag(apiURL string) []string {
	host := GLabHostname(apiURL)
	if host == "" {
		return nil
	}
	return []string{"--hostname", host}
}

func GitLabAPIBase(raw string) string {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" {
		return defaultGitLabAPI
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if p := strings.Trim(u.Path, "/"); p == "" {
		return raw + "/api/v4"
	}
	return raw
}

func GitLabInstanceURL(apiURL string) string {
	u, err := url.Parse(strings.TrimSpace(apiURL))
	if err != nil || u.Host == "" {
		return "https://" + defaultGitLabHost
	}
	scheme := u.Scheme
	if scheme == "" {
		scheme = "https"
	}
	return scheme + "://" + u.Host
}

func GLAPIGet(ctx context.Context, sel GitLabSelection, path string) ([]byte, error) {
	body, _, err := glAPIGet(ctx, sel, path)
	return body, err
}

func glAPIGet(ctx context.Context, sel GitLabSelection, path string) ([]byte, http.Header, error) {
	tok, err := sel.Token(ctx)
	if err != nil {
		return nil, nil, err
	}
	u := GitLabAPIBase(sel.APIURL) + "/" + strings.TrimLeft(path, "/")
	if listEndpoint(path) {
		u = withPerPage(u, 100)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, nil, errs.Wrap(errs.KindSignal, err, "gitlab: building request")
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")

	resp, err := HTTPClient().Do(req)
	if err != nil {
		return nil, nil, errs.Wrap(errs.KindSignal, err, "gitlab: request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, resp.Header, classifyGitLabStatus(resp, errorExcerpt(resp.Body))
	}
	body, err := readBounded(resp, "gitlab", maxAPIResponseBytes)
	return body, resp.Header, err
}

func listEndpoint(path string) bool {
	switch strings.Trim(path, "/") {
	case "user/keys", "user/gpg_keys", "user/emails":
		return true
	}
	return false
}

func withPerPage(rawURL string, n int) string {
	sep := "?"
	if strings.Contains(rawURL, "?") {
		sep = "&"
	}
	return rawURL + sep + "per_page=" + strconv.Itoa(n)
}

func classifyGitLabStatus(resp *http.Response, msg string) error {
	statusErr := func(kind errs.Kind) *errs.Error {
		return errs.Newf(kind, "gitlab api %s: %s", resp.Status, msg)
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return statusErr(errs.KindAuth).WithHint("%s", gitlabScopeHint)
	case gitlabRateLimited(resp, msg):
		e := statusErr(errs.KindSignal)
		if d, ok := gitlabRetryAfter(resp.Header, time.Now()); ok {
			return e.WithHint("gitlab rate limit reached; retry after %s", d.Round(time.Second))
		}
		return e.WithHint("gitlab rate limit reached; retry in a few minutes")
	case resp.StatusCode == http.StatusForbidden:
		if scope := insufficientScope(msg); scope != "" {
			return statusErr(errs.KindAuth).
				WithHint("your GitLab token needs the %s scope; run `glab auth login` or create a "+
					"personal access token with it", scope)
		}
		return statusErr(errs.KindAuth).WithHint("%s", gitlabScopeHint)
	default:
		return statusErr(errs.KindSignal)
	}
}

func gitlabRateLimited(resp *http.Response, msg string) bool {
	if resp.StatusCode == http.StatusTooManyRequests {
		return true
	}
	if resp.StatusCode != http.StatusForbidden {
		return false
	}
	if strings.TrimSpace(resp.Header.Get("RateLimit-Remaining")) == "0" {
		return true
	}
	if strings.TrimSpace(resp.Header.Get("Retry-After")) != "" {
		return true
	}
	return strings.Contains(strings.ToLower(msg), "rate limit")
}

func insufficientScope(msg string) string {
	low := strings.ToLower(msg)
	if !strings.Contains(low, "insufficient_scope") {
		return ""
	}
	for _, scope := range []string{"read_api", "read_user", "api"} {
		if strings.Contains(low, scope) {
			return scope
		}
	}
	return "read_api"
}

func gitlabRetryAfter(hdr http.Header, now time.Time) (time.Duration, bool) {
	if v := strings.TrimSpace(hdr.Get("Retry-After")); v != "" {
		if secs, err := strconv.Atoi(v); err == nil {
			return boundGitLabRetry(time.Duration(secs) * time.Second)
		}
		if t, err := http.ParseTime(v); err == nil {
			return boundGitLabRetry(t.Sub(now))
		}
	}
	if strings.TrimSpace(hdr.Get("RateLimit-Remaining")) == "0" {
		if epoch, err := strconv.ParseInt(strings.TrimSpace(hdr.Get("RateLimit-Reset")), 10, 64); err == nil {
			return boundGitLabRetry(time.Unix(epoch, 0).Sub(now))
		}
		if t, err := http.ParseTime(strings.TrimSpace(hdr.Get("RateLimit-ResetTime"))); err == nil {
			return boundGitLabRetry(t.Sub(now))
		}
	}
	return 0, false
}

func boundGitLabRetry(d time.Duration) (time.Duration, bool) {
	if d <= 0 {
		return 0, false
	}
	return min(d, maxGitLabRetryAfter), true
}
