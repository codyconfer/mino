package httpapi

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/signals"
)

// statusError pins a response status onto an error without polluting its
// message. errs.KindUsage covers 400, 404, 403 and 409 alike, so a handler that
// knows which one it means says so here rather than encoding it in text.
type statusError struct {
	status int
	err    error
}

func (e *statusError) Error() string { return e.err.Error() }
func (e *statusError) Unwrap() error { return e.err }

// withStatus marks err as mapping to status.
func withStatus(status int, err error) error { return &statusError{status: status, err: err} }

type errBody struct {
	Error errPayload `json:"error"`
}

type errPayload struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

// statusFor maps an error to a response status. An explicit status wins over the
// kind, so a handler can say "this name does not exist" without a new errs.Kind.
func statusFor(err error) int {
	var se *statusError
	if errors.As(err, &se) {
		return se.status
	}
	switch errs.KindOf(err) {
	case errs.KindUsage, errs.KindConfig:
		return http.StatusBadRequest
	case errs.KindAuth:
		// Upstream credentials, not ours. 401 here would be indistinguishable
		// from a bad bearer token, which is the worst confusion this API could
		// hand a caller.
		return http.StatusBadGateway
	case errs.KindSignal:
		return http.StatusBadGateway
	case errs.KindStore:
		return http.StatusServiceUnavailable
	case errs.KindOnboarding:
		return http.StatusPreconditionFailed
	case errs.KindBackup, errs.KindInternal:
		return http.StatusInternalServerError
	}
	return http.StatusInternalServerError
}

func abortErr(c *gin.Context, err error) {
	renderJSON(c, statusFor(err), errBody{Error: errPayload{
		Kind:    string(errs.KindOf(err)),
		Message: signals.CleanLine(err.Error()),
		Hint:    signals.CleanLine(errs.Hint(err)),
	}})
	c.Abort()
}

func abortErrStatus(c *gin.Context, status int, kind errs.Kind, msg, hint string) {
	renderJSON(c, status, errBody{Error: errPayload{
		Kind:    string(kind),
		Message: signals.CleanLine(msg),
		Hint:    signals.CleanLine(hint),
	}})
	c.Abort()
}
