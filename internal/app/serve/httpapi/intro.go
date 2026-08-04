package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/codyconfer/mino/internal/errs"
)

func (a *API) healthz(c *gin.Context) {
	st := a.deps.Status()
	renderJSON(c, http.StatusOK, map[string]any{
		"status":   "ok",
		"flight":   st.Flight,
		"uptime_s": int64(a.Uptime().Seconds()),
	})
}

func (a *API) status(c *gin.Context) {
	st := a.deps.Status()
	st.SSEClients = a.SSEClients()
	st.RunsInFlight = a.RunsInFlight()
	st.UptimeSec = int64(a.Uptime().Seconds())
	if st.Sources == nil {
		st.Sources = []SourceInfo{}
	}
	renderJSON(c, http.StatusOK, st)
}

func (a *API) config(c *gin.Context) {
	renderJSON(c, http.StatusOK, a.deps.Config())
}

func (a *API) list(c *gin.Context) {
	all := c.Query("all") == "1"
	switch kind := c.Query("kind"); kind {
	case "flights":
		renderJSON(c, http.StatusOK, map[string]any{"kind": kind, "flights": a.deps.Flights(all)})
	case "queries":
		renderJSON(c, http.StatusOK, map[string]any{"kind": kind, "queries": a.deps.Queries(all)})
	case "":
		abortErr(c, errs.New(errs.KindUsage, "kind is required").
			WithHint("use ?kind=flights or ?kind=queries"))
	default:
		abortErr(c, withStatus(http.StatusNotFound, errs.Newf(errs.KindUsage, "unknown kind %q", kind).
			WithHint("use flights or queries")))
	}
}

func (a *API) listActions(c *gin.Context) {
	acts := a.deps.Actions(c.Param("signal"))
	if acts == nil {
		acts = []ActionInfo{}
	}
	renderJSON(c, http.StatusOK, map[string]any{"actions": acts})
}
