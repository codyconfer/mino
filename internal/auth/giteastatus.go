package auth

import (
	"net/http"
	"time"

	"github.com/codyconfer/mino/internal/errs"
)

const giteaScopeHint = "the token may lack a scope; regenerate it under Settings -> Applications with " +
	"read:user (plus read:repository and read:notification for repository data), then run `mino login gitea`"

const giteaNotFoundHint = "either the token lacks read:user, or this Gitea version does not expose that " +
	"endpoint; compare against <instance>/api/swagger"

func classifyGiteaStatus(forge string, resp *http.Response, msg string) error {
	statusErr := func(kind errs.Kind) *errs.Error {
		return errs.Newf(kind, "%s api %s: %s", forge, resp.Status, msg)
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return statusErr(errs.KindAuth).WithHint("%s", giteaScopeHint)
	case giteaRateLimited(resp):
		e := statusErr(errs.KindSignal)
		if d, ok := retryAfterHeader(resp.Header, time.Now()); ok {
			return e.WithHint("%s is rate limiting requests; retry after %s", forge, d.Round(time.Second))
		}
		return e.WithHint("%s is rate limiting requests; retry in a few minutes", forge)
	case resp.StatusCode == http.StatusForbidden:
		return statusErr(errs.KindAuth).WithHint("%s", giteaScopeHint)
	case resp.StatusCode == http.StatusNotFound:
		return statusErr(errs.KindSignal).WithHint("%s", giteaNotFoundHint)
	default:
		return statusErr(errs.KindSignal)
	}
}

func giteaRateLimited(resp *http.Response) bool {
	if resp.StatusCode == http.StatusTooManyRequests {
		return true
	}
	return resp.StatusCode == http.StatusForbidden && resp.Header.Get("Retry-After") != ""
}
