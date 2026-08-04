package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/signals"
)

type runRequest struct {
	Params map[string]string `json:"params,omitempty"`
}

func decodeBody(c *gin.Context) (runRequest, error) {
	var req runRequest
	if c.Request.Body == nil {
		return req, nil
	}
	if ct := c.GetHeader("Content-Type"); ct != "" {
		if mt := strings.TrimSpace(strings.Split(ct, ";")[0]); mt != "application/json" {
			return req, errs.Newf(errs.KindUsage, "unsupported content type %q", mt).
				WithHint("send application/json, or no body at all")
		}
	}
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, maxBodyBytes))
	if err != nil {
		return req, errs.Wrap(errs.KindUsage, err, "reading the request body")
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return req, nil
	}
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		return req, errs.Wrap(errs.KindUsage, err, "parsing the request body")
	}
	return req, nil
}

func (a *API) acquire() bool {
	select {
	case a.runs <- struct{}{}:
		return true
	default:
		return false
	}
}

func (a *API) release() { <-a.runs }

func (a *API) busy(c *gin.Context) {
	c.Header("Retry-After", "1")
	abortErrStatus(c, http.StatusTooManyRequests, errs.KindUsage,
		fmt.Sprintf("%d runs already in flight", cap(a.runs)),
		"retry shortly, or raise daemon.http.max_concurrent")
}

func (a *API) runFlight(c *gin.Context) {
	name := c.Param("name")
	if !a.deps.FlightExists(name) {
		abortErr(c, withStatus(http.StatusNotFound, errs.Newf(errs.KindUsage, "no flight named %q", name).
			WithHint("GET /api/v1/list?kind=flights to see what is available")))
		return
	}
	if !a.deps.FlightVisible(name) {
		abortErr(c, withStatus(http.StatusForbidden, errs.Newf(errs.KindUsage, "flight %q is not visible in this role", name).
			WithHint("serve is running as a role that does not include it")))
		return
	}
	a.runSections(c, name, a.deps.RunFlight)
}

func (a *API) runQuery(c *gin.Context) {
	name := c.Param("name")
	if !a.deps.QueryExists(name) {
		abortErr(c, withStatus(http.StatusNotFound, errs.Newf(errs.KindUsage, "no saved query named %q", name).
			WithHint("GET /api/v1/list?kind=queries to see what is available")))
		return
	}
	if !a.deps.QueryVisible(name) {
		abortErr(c, withStatus(http.StatusForbidden, errs.Newf(errs.KindUsage, "query %q is not visible in this role", name).
			WithHint("serve is running as a role that does not include it")))
		return
	}
	a.runSections(c, name, a.deps.RunQuery)
}

func (a *API) runSections(c *gin.Context, name string,
	run func(ctx context.Context, name string) ([]signals.Section, error),
) {
	if _, err := decodeBody(c); err != nil {
		abortErr(c, err)
		return
	}
	if !a.acquire() {
		a.busy(c)
		return
	}
	defer a.release()

	sections, runErr := run(c.Request.Context(), name)

	var buf bytes.Buffer
	if err := a.deps.EmitJSON(&buf, name, sections); err != nil {
		abortErr(c, err)
		return
	}

	failed, total := a.deps.Tally(sections)
	if failed > 0 {
		c.Header("X-Mino-Sections-Failed", fmt.Sprintf("%d/%d", failed, total))
	}
	status := http.StatusOK
	if runErr != nil {
		c.Header("X-Mino-Outcome", "failed")
		status = statusFor(runErr)
	}
	c.Data(status, "application/json", buf.Bytes())
}

func (a *API) runAction(c *gin.Context) {
	signal, name := c.Param("signal"), c.Param("name")
	req, err := decodeBody(c)
	if err != nil {
		abortErr(c, err)
		return
	}
	for _, k := range []string{"home", "role"} {
		if _, ok := req.Params[k]; ok {
			abortErr(c, errs.Newf(errs.KindUsage, "param %q cannot be set over http", k).
				WithHint("it comes from the serve session"))
			return
		}
	}
	if !a.deps.ActionExists(signal, name) {
		abortErr(c, withStatus(http.StatusNotFound, errs.Newf(errs.KindUsage, "no action %s/%s", signal, name).
			WithHint("GET /api/v1/actions to see what is registered")))
		return
	}
	if !a.deps.SignalEnabled(signal) {
		abortErr(c, withStatus(http.StatusConflict, errs.Newf(errs.KindUsage, "signal %q is disabled", signal).
			WithHint("enable it with `mino plugins enable %s`", signal)))
		return
	}
	if !a.acquire() {
		a.busy(c)
		return
	}
	defer a.release()

	ctx, cancel := context.WithTimeout(c.Request.Context(), a.deps.Timeout())
	defer cancel()

	started := time.Now()
	if err := a.deps.RunAction(ctx, signal, name, req.Params); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			abortErrStatus(c, http.StatusGatewayTimeout, errs.KindSignal,
				fmt.Sprintf("action %s/%s did not finish within %s", signal, name, a.deps.Timeout()), "")
			return
		}
		abortErr(c, err)
		return
	}
	renderJSON(c, http.StatusOK, map[string]any{
		"ok":      true,
		"signal":  signal,
		"name":    name,
		"took_ms": time.Since(started).Milliseconds(),
	})
}
