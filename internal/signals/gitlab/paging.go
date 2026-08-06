package gitlab

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/codyconfer/mino/internal/log"
	"github.com/codyconfer/mino/internal/signals"
)

const maxPages = 10

type pageMeta struct {
	shown     int
	total     int
	hasTotal  bool
	truncated bool
}

func (m pageMeta) sectionMeta() map[string]string {
	meta := map[string]string{"shown": strconv.Itoa(m.shown)}
	if m.hasTotal {
		meta["total"] = strconv.Itoa(m.total)
		if more := m.total - m.shown; more > 0 {
			meta[signals.MetaMore] = strconv.Itoa(more)
		}
	}
	if m.truncated {
		meta[signals.MetaTruncated] = "true"
		meta["truncated_reason"] = "stopped after " + strconv.Itoa(maxPages) +
			" pages; narrow the selector or lower the limit"
	}
	return meta
}

func nextPage(hdr http.Header) int {
	v := strings.TrimSpace(hdr.Get("X-Next-Page"))
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

func totalCount(hdr http.Header) (int, bool) {
	return headerInt(hdr, "X-Total")
}

func totalPages(hdr http.Header) (int, bool) {
	return headerInt(hdr, "X-Total-Pages")
}

func headerInt(hdr http.Header, key string) (int, bool) {
	v := strings.TrimSpace(hdr.Get(key))
	if v == "" {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, false
	}
	return n, true
}

func collect(ctx context.Context, b Backend, path string, base url.Values, perPage, max int,
	decode func([]byte, int) (int, error)) (pageMeta, error) {
	var m pageMeta
	page := 1
	for pages := 0; pages < maxPages; pages++ {
		q := cloneValues(base)
		q.Set("per_page", strconv.Itoa(perPage))
		if page > 1 {
			q.Set("page", strconv.Itoa(page))
		}

		p, err := b.Get(ctx, path, q)
		if err != nil {
			return m, err
		}
		room := max - m.shown
		n, err := decode(p.Body, room)
		if err != nil {
			return m, err
		}
		m.shown += n
		if !m.hasTotal {
			if total, ok := p.Total, p.HasTotal; ok {
				m.total, m.hasTotal = total, true
			}
		}
		if m.shown >= max {
			if p.NextPage > 0 || !p.Short {
				m.truncated = m.hasTotal && m.total > m.shown
			}
			return m, nil
		}
		if p.NextPage > 0 {
			page = p.NextPage
			continue
		}
		if p.Short || n == 0 {
			return m, nil
		}
		page++
	}
	m.truncated = true
	log.Debugf("gitlab: stopped paging %s after %d pages with %d items", path, maxPages, m.shown)
	return m, nil
}

func cloneValues(v url.Values) url.Values {
	out := make(url.Values, len(v)+2)
	for k, vals := range v {
		out[k] = append([]string(nil), vals...)
	}
	return out
}
