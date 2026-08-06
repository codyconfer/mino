package gitlab

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/log"
)

const maxResponseBytes = 8 << 20

const gitlabAuthHintSuffix = "; grant it with `mino login gitlab` or $GITLAB_TOKEN, or run `glab auth login`"

func gitlabAuthHint(scope string) string {
	return "your GitLab credential may lack " + scope + gitlabAuthHintSuffix
}

func readBody(resp *http.Response) ([]byte, error) {
	if resp.ContentLength > maxResponseBytes {
		return nil, oversizeBody(resp, resp.ContentLength)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, errs.Wrap(errs.KindSignal, err, "gitlab: reading response body")
	}
	if len(body) > maxResponseBytes {
		return nil, oversizeBody(resp, int64(len(body)))
	}
	return body, nil
}

func oversizeBody(resp *http.Response, n int64) error {
	detail := fmt.Sprintf("response body exceeds the %d MiB limit (%d bytes)", maxResponseBytes>>20, n)
	if resp.StatusCode >= 400 {
		return checkGitLabStatus(resp, []byte(detail), "the required scopes", false)
	}
	return errs.Newf(errs.KindSignal, "gitlab: %s", detail).
		WithHint("check that gitlab.api_url points at a GitLab instance, e.g. https://gitlab.example.com")
}

func checkGitLabStatus(resp *http.Response, body []byte, missingScope string, scoped bool) error {
	msg := gitlabMessage(body)
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return errs.Newf(errs.KindAuth, "gitlab api %s: %s", resp.Status, msg).
			WithHint("%s", gitlabAuthHint(missingScope))
	case rateLimited(resp, body):
		return rateLimitError(resp, msg)
	case resp.StatusCode == http.StatusForbidden:
		return forbiddenError(resp, msg, missingScope)
	case resp.StatusCode == http.StatusNotFound:
		return notFoundError(resp, msg, scoped)
	case resp.StatusCode >= 400:
		return errs.Newf(errs.KindSignal, "gitlab api %s: %s", resp.Status, msg)
	}
	return nil
}

func forbiddenError(resp *http.Response, msg, missingScope string) error {
	if scope := insufficientScope(msg); scope != "" {
		return errs.Newf(errs.KindAuth, "gitlab api %s: %s", resp.Status, msg).
			WithHint("your GitLab token needs the %s scope; create a personal access token with it "+
				"or run `glab auth login`", scope)
	}
	return errs.Newf(errs.KindAuth, "gitlab api %s: %s", resp.Status, msg).
		WithHint("%s", gitlabAuthHint(missingScope))
}

func notFoundError(resp *http.Response, msg string, scoped bool) error {
	if !scoped {
		return errs.Newf(errs.KindSignal, "gitlab api %s: %s", resp.Status, msg)
	}
	return errs.Newf(errs.KindUsage, "gitlab api %s: %s", resp.Status, msg).
		WithHint("gitlab returns 404 for projects you cannot see; check the path in project:/group: " +
			"and that the credential has read access to it")
}

func rateLimitError(resp *http.Response, msg string) error {
	err := errs.Newf(errs.KindSignal, "gitlab api %s: %s", resp.Status, msg)
	if d, ok := retryAfter(resp.Header, timeNow()); ok {
		return err.WithHint("gitlab rate limit reached; retry after %s", d.Round(time.Second))
	}
	return err.WithHint("gitlab rate limit reached; retry in a few minutes")
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

func gitlabMessage(body []byte) string {
	var envelope struct {
		Message          json.RawMessage `json:"message"`
		Error            string          `json:"error"`
		ErrorDescription string          `json:"error_description"`
		Scope            string          `json:"scope"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return errs.ExcerptBytes(body)
	}
	if s := flattenMessage(envelope.Message); s != "" {
		return s
	}
	switch {
	case envelope.Error != "" && envelope.Scope != "":
		return envelope.Error + " (scope " + envelope.Scope + ")"
	case envelope.Error != "" && envelope.ErrorDescription != "":
		return envelope.Error + ": " + envelope.ErrorDescription
	case envelope.Error != "":
		return envelope.Error
	}
	return errs.ExcerptBytes(body)
}

func flattenMessage(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(errs.Excerpt(s))
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		return strings.TrimSpace(errs.Excerpt(strings.Join(list, "; ")))
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return ""
	}
	parts := make([]string, 0, len(fields))
	for _, key := range sortedKeys(fields) {
		if v := flattenMessage(fields[key]); v != "" {
			parts = append(parts, key+" "+v)
		}
	}
	return strings.TrimSpace(errs.Excerpt(strings.Join(parts, "; ")))
}

func cliError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	low := strings.ToLower(msg)
	switch {
	case strings.Contains(low, "insufficient_scope"):
		log.Debugf("gitlab: glab scope failure: %v", err)
		return errs.New(errs.KindAuth, "gitlab: the glab credential lacks the required scope").
			WithHint("%s", gitlabAuthHint("read_api"))
	case strings.Contains(low, "401") || strings.Contains(low, "unauthorized"):
		log.Debugf("gitlab: glab auth failure: %v", err)
		return errs.New(errs.KindAuth, "gitlab: glab is not authenticated").
			WithHint("%s", gitlabAuthHint("a valid credential"))
	case strings.Contains(low, "404") || strings.Contains(low, "not found"):
		log.Debugf("gitlab: glab 404: %v", err)
		return errs.New(errs.KindUsage, "gitlab: glab reported 404").
			WithHint("gitlab returns 404 for projects you cannot see; check the path in project:/group: " +
				"and that the credential has read access to it")
	}
	return err
}
