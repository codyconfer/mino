package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
)

var jsonContentType = []string{"application/json"}

type minoJSON struct{ data any }

func (r minoJSON) Render(w http.ResponseWriter) error {
	r.WriteContentType(w)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(r.data)
}

func (r minoJSON) WriteContentType(w http.ResponseWriter) {
	if h := w.Header()["Content-Type"]; len(h) == 0 {
		w.Header()["Content-Type"] = jsonContentType
	}
}

func renderJSON(c *gin.Context, status int, v any) { c.Render(status, minoJSON{data: v}) }

func unwrapWriter(w http.ResponseWriter) http.ResponseWriter {
	for {
		u, ok := w.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			return w
		}
		w = u.Unwrap()
	}
}
