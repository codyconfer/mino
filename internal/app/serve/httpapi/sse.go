package httpapi

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/log"
)

func (a *API) events(c *gin.Context) {
	select {
	case a.sseSlot <- struct{}{}:
		defer func() { <-a.sseSlot }()
	default:
		c.Header("Retry-After", "1")
		abortErrStatus(c, http.StatusTooManyRequests, errs.KindUsage,
			fmt.Sprintf("%d event streams already open", maxSSEConns), "close one and retry")
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-store")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	c.Writer.WriteHeaderNow()

	sub, unsubscribe := a.deps.Subscribe(eventBuffer)
	defer unsubscribe()

	w := c.Writer
	rc := http.NewResponseController(unwrapWriter(w))
	write := func(b []byte) bool {
		if err := rc.SetWriteDeadline(time.Now().Add(sseWriteGrace)); err != nil {
			log.Debugf("serve: http api: sse write deadline unsupported: %v", err)
		}
		if _, err := w.Write(b); err != nil {
			return false
		}
		return rc.Flush() == nil
	}

	if !write([]byte("retry: 5000\n\n: connected\n\n")) {
		return
	}

	ticker := time.NewTicker(sseHeartbeat)
	defer ticker.Stop()

	var id uint64
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case ev, ok := <-sub:
			if !ok {
				return
			}
			b, err := a.deps.Encode(ev)
			if err != nil {
				log.Debugf("serve: http api: dropping an unencodable event: %v", err)
				continue
			}
			id++
			if !write(fmt.Appendf(nil, "id: %d\nevent: signal\ndata: %s\n\n", id, b)) {
				return
			}
		case <-ticker.C:
			if !write([]byte(": ping\n\n")) {
				return
			}
		}
	}
}
